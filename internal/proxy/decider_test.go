package proxy

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// preludeFor is what smolvm writes to the decider for a connection to dest that starts with first.
func preludeFor(token []byte, dest netip.AddrPort, first []byte) []byte {
	var b bytes.Buffer
	b.WriteString(deciderMagic)
	b.Write(token)
	b.Write(bytes.Repeat([]byte{7}, 16)) // machine ID
	b.Write(make([]byte, 16))            // no parent
	a := dest.Addr()
	if a.Is4() {
		b.WriteByte(4)
	} else {
		b.WriteByte(6)
	}
	_ = binary.Write(&b, binary.BigEndian, dest.Port())
	b.Write(a.AsSlice())
	_ = binary.Write(&b, binary.BigEndian, uint16(min(len(first), 0xffff))) //nolint:gosec // capped
	b.Write(first)
	return b.Bytes()
}

var testToken = bytes.Repeat([]byte{0xab}, 32)

func TestReadPrelude(t *testing.T) {
	v4 := netip.MustParseAddrPort("100.96.0.1:3128")
	v6 := netip.MustParseAddrPort("[2001:db8::1]:443")
	good := preludeFor(testToken, v4, []byte("CONNECT x:443"))
	for _, c := range []struct {
		name  string
		in    []byte
		dest  netip.AddrPort
		first string
		err   bool
	}{
		{"IPv4", good, v4, "CONNECT x:443", false},
		{"IPv6", preludeFor(testToken, v6, nil), v6, "", false},
		{"another protocol", append([]byte("SMOLMEG1"), good[8:]...), v4, "", true},
		{"a family that isn't one", append(append(clone(good[:72]), 5), good[73:]...), v4, "", true},
		{"cut short", good[:len(good)-1], v4, "", true},
		{"nothing", nil, v4, "", true},
		{"8 KiB of first bytes", preludeFor(testToken, v4, bytes.Repeat([]byte{'x'}, maxFirst)), v4, strings.Repeat("x", maxFirst), false},
		{"more than smolvm sends", preludeFor(testToken, v4, bytes.Repeat([]byte{'x'}, maxFirst+1)), v4, "", true},
	} {
		p, err := readPrelude(bytes.NewReader(c.in), testToken)
		if (err != nil) != c.err {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !c.err && (p.dest != c.dest || string(p.first) != c.first || !bytes.Equal(p.token, testToken)) {
			t.Errorf("%s: %+v", c.name, p)
		}
	}
	// Without the token, nothing past it is read.
	r := bytes.NewReader(preludeFor(bytes.Repeat([]byte{1}, 32), v4, []byte("GET / HTTP/1.1")))
	if _, err := readPrelude(r, testToken); !errors.Is(err, errToken) || int(r.Size())-r.Len() != len(deciderMagic)+32 {
		t.Errorf("without the token: %v, read %d bytes", err, int(r.Size())-r.Len())
	}
}

func clone(b []byte) []byte { return append([]byte(nil), b...) }

// readDeciderToken takes 64 hex digits, not all zero.
func TestReadDeciderToken(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		text string
		ok   bool
	}{
		{hex.EncodeToString(testToken) + "\n", true},
		{strings.Repeat("0", 64), false},
		{strings.Repeat("a", 63), false},
		{strings.Repeat("g", 64), false},
		{"", false},
	} {
		f := filepath.Join(dir, "token")
		must(t, os.WriteFile(f, []byte(c.text), 0o600))
		if _, err := readDeciderToken(f); (err == nil) != c.ok {
			t.Errorf("%q: %v", c.text, err)
		}
	}
}

// A proxy with a decider: it takes over connections to its address in the VM that carry the token
// (still needing the VM's password), refuses and logs the rest, answers nothing without the token,
// and once it's gone the decider's port takes nothing.
func TestDecider(t *testing.T) {
	lifelineTick = 50 * time.Millisecond
	t.Cleanup(func() { lifelineTick = 3 * time.Second; proxyAuth = "" })
	dir := t.TempDir()
	key, tokFile, pidFile, logFile := filepath.Join(dir, "key"), filepath.Join(dir, "token"), filepath.Join(dir, "agent.pid"), filepath.Join(dir, "log.jsonl")
	must(t, os.WriteFile(key, []byte("k3y\n"), 0o600))
	must(t, os.WriteFile(tokFile, []byte(hex.EncodeToString(testToken)+"\n"), 0o600))
	sleep := exec.CommandContext(t.Context(), "sleep", "60")
	must(t, sleep.Start())
	t.Cleanup(func() { _ = sleep.Process.Kill() })
	must(t, os.WriteFile(pidFile, []byte(strconv.Itoa(sleep.Process.Pid)+"\n1792910265912021"), 0o600))
	addr, dec := freeAddr(t), freeAddr(t)
	to := netip.MustParseAddrPort("100.96.0.1:3128")
	errs := &syncBuffer{}
	done := make(chan error)
	go func() {
		done <- Main([]string{
			"-listen", addr, "-vm", "devvm-x", "-auth-file", key, "-lifeline", pidFile, "-log", logFile,
			"-policy", filepath.Join(dir, "missing"), "-decider", dec, "-decider-to", to.String(), "-decider-token-file", tokFile,
		}, IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: errs})
	}()
	for i := 0; !strings.Contains(errs.String(), "listening on "); i++ {
		if i > 200 {
			t.Fatalf("not listening: %q", errs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(errs.String(), "deciding on "+dec+" for devvm-x, taking over "+to.String()) {
		t.Errorf("stderr = %q", errs.String())
	}
	// ask sends a prelude, then rest; it returns the answer byte (-1 for none) and the connection.
	ask := func(token []byte, dest netip.AddrPort, first, rest string) (int, net.Conn) {
		t.Helper()
		c, err := new(net.Dialer).DialContext(t.Context(), "tcp", dec)
		must(t, err)
		t.Cleanup(func() { _ = c.Close() })
		_, err = c.Write(preludeFor(token, dest, []byte(first)))
		must(t, err)
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		b := make([]byte, 1)
		if _, err := io.ReadFull(c, b); err != nil {
			return -1, c
		}
		_, _ = c.Write([]byte(rest))
		return int(b[0]), c
	}
	auth := "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("devvm:k3y")) + "\r\n"
	req := "GET http://devvm.proxy/blocked HTTP/1.1\r\nHost: devvm.proxy\r\n"

	// Taken over: its first bytes, then the rest, reach the proxy.
	status := func(c net.Conn) int {
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		if err != nil {
			return 0
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if v, c := ask(testToken, to, req, auth+"\r\n"); v != deciderTakeOver {
		t.Errorf("to the proxy: %d", v)
	} else if s := status(c); s != http.StatusOK {
		t.Errorf("to the proxy, with the password: %d", s)
	}
	// Taken over, without the password.
	if v, c := ask(testToken, to, req+"\r\n", ""); v != deciderTakeOver {
		t.Errorf("to the proxy: %d", v)
	} else if s := status(c); s != http.StatusProxyAuthRequired {
		t.Errorf("to the proxy, without the password: %d", s)
	}
	// Anywhere else is refused: another port of the gateway, the internet, IPv6.
	for _, d := range []string{"100.96.0.1:22", "1.1.1.1:443", "[2606:4700::1111]:443", "100.96.0.2:3128"} {
		if v, _ := ask(testToken, netip.MustParseAddrPort(d), "", ""); v != deciderRefuse {
			t.Errorf("%s: %d, want refused", d, v)
		}
	}
	// Without the token: no answer at all.
	if v, _ := ask(bytes.Repeat([]byte{1}, 32), to, req, ""); v != -1 {
		t.Errorf("without the token: %d", v)
	}
	b, _ := os.ReadFile(logFile) //nolint:gosec // the test's own file
	if !strings.Contains(string(b), `"event":"blocked","host":"1.1.1.1","port":443,"method":"TCP"`) ||
		!strings.Contains(string(b), `"refused":true`) || strings.Contains(string(b), `"host":"100.96.0.1","port":3128`) {
		t.Errorf("event log = %s", b)
	}
	// The VM's process gone, the proxy goes, and its decider with it.
	time.Sleep(5 * lifelineTick)
	_ = sleep.Process.Kill()
	_ = sleep.Wait()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Main: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("still running after the VM's process was gone")
	}
	if c, err := new(net.Dialer).DialContext(t.Context(), "tcp", dec); err == nil {
		_ = c.Close()
		t.Error("the decider still listens")
	}
}

// Decider flags that don't fit are refused before anything listens.
func TestDeciderFlagsRefused(t *testing.T) {
	dir := t.TempDir()
	key, tok := filepath.Join(dir, "key"), filepath.Join(dir, "token")
	must(t, os.WriteFile(key, []byte("k3y\n"), 0o600))
	must(t, os.WriteFile(tok, []byte(hex.EncodeToString(testToken)), 0o600))
	std := IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard}
	base := []string{"-listen", "127.0.0.1:0", "-auth-file", key}
	for _, extra := range [][]string{
		{"-decider-to", "100.96.0.1:3128"},
		{"-decider-token-file", tok},
		{"-decider", "127.0.0.1:0", "-decider-token-file", tok},
		{"-decider", "127.0.0.1:0", "-decider-to", "100.96.0.1:3128"},
		{"-decider", "0.0.0.0:0", "-decider-to", "100.96.0.1:3128", "-decider-token-file", tok},
		{"-decider", "127.0.0.1:0", "-decider-to", "100.96.0.1", "-decider-token-file", tok},
		{"-decider", "127.0.0.1:0", "-decider-to", "100.96.0.1:3128", "-decider-token-file", key},
	} {
		args := append(append([]string{}, base...), extra...)
		if err := Main(args, std); err == nil {
			t.Errorf("%q: no error", extra)
		}
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	must(t, err)
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// Whatever the decider is sent, reading it doesn't panic, and what it reads back is what was sent.
func FuzzPrelude(f *testing.F) {
	f.Add(preludeFor(testToken, netip.MustParseAddrPort("100.96.0.1:3128"), []byte("CONNECT x:443 HTTP/1.1\r\n")))
	f.Add(preludeFor(testToken, netip.MustParseAddrPort("[::1]:22"), nil))
	f.Add([]byte("SMOLMEG2"))
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := readPrelude(bytes.NewReader(b), testToken)
		if err != nil {
			if !errors.Is(err, errPrelude) && !errors.Is(err, errToken) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("unexpected error %v", err)
			}
			return
		}
		if got := preludeFor(p.token, p.dest, p.first); !bytes.HasPrefix(b, got[:8+32]) || len(got) > len(b) {
			t.Fatalf("read back differently: %x from %x", got, b)
		}
	})
}
