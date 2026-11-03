package proxy

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// sockDir is a new folder for unix sockets, short enough for macOS's limit on their paths.
func sockDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "pr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// listenOn replaces a socket left behind, refuses anything else at the path, and leaves the
// socket the user's alone.
func TestListenOn(t *testing.T) {
	d := sockDir(t)
	p := filepath.Join(d, "s")
	old, err := net.Listen("unix", p) //nolint:noctx // a test's
	if err != nil {
		t.Fatal(err)
	}
	if ul, ok := old.(*net.UnixListener); ok {
		ul.SetUnlinkOnClose(false) // as a proxy that was killed leaves it
	}
	_ = old.Close()
	ln, err := listenOn(p)
	if err != nil {
		t.Fatalf("over a stale socket: %v", err)
	}
	defer func() { _ = ln.Close() }()
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("socket mode: %v %v", fi.Mode(), err)
	}
	f := filepath.Join(d, "file")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenOn(f); err == nil {
		t.Error("a file at the path: no error")
	}
	if b, _ := os.ReadFile(f); string(b) != "x" { //nolint:gosec // the test's own file
		t.Errorf("the file was changed: %q", b)
	}
}

// The relay passes each connection's bytes both ways, unread, and the end of what each side sent;
// a connection with no socket behind it is closed.
func TestRelay(t *testing.T) {
	d := sockDir(t)
	sock := filepath.Join(d, "p.sock")
	up, err := net.Listen("unix", sock) //nolint:noctx // a test's
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = up.Close() }()
	go func() {
		for {
			c, err := up.Accept()
			if err != nil {
				return
			}
			go func() {
				b, _ := io.ReadAll(c) // until the client's end
				_, _ = c.Write(append([]byte("got:"), b...))
				_ = c.Close()
			}()
		}
	}()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	go func() { _ = runRelay(addr, sock) }()
	var c net.Conn
	for range 100 {
		if c, err = (&net.Dialer{}).DialContext(t.Context(), "tcp", addr); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Write([]byte("CONNECT x:443 HTTP/1.1\r\n\r\n"))
	_ = c.(*net.TCPConn).CloseWrite()
	b, _ := io.ReadAll(c)
	_ = c.Close()
	if string(b) != "got:CONNECT x:443 HTTP/1.1\r\n\r\n" {
		t.Errorf("relayed = %q", b)
	}
	_ = up.Close()
	_ = os.Remove(sock)
	c, err = (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	if n, err := c.Read(make([]byte, 1)); n != 0 || err == nil {
		t.Errorf("no socket: read %d, %v", n, err)
	}
	_ = c.Close()
	if err := runRelay("127.0.0.1:0", ""); err == nil {
		t.Error("-relay without -relay-to: no error")
	}
}

// A lifeline written after the proxy started whose process is already gone (a VM stopped at once)
// ends it; one written before, a dead earlier start's, doesn't.
func TestLifelineGoneBeforeSeen(t *testing.T) {
	lifelineTick = 20 * time.Millisecond
	t.Cleanup(func() { lifelineTick = 3 * time.Second })
	dead := exec.CommandContext(t.Context(), "true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "ha.pid")
	pid := []byte(strconv.Itoa(dead.Process.Pid) + "\n")
	if err := os.WriteFile(pidFile, pid, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(pidFile, old, old); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { watchLifeline(pidFile); close(done) }()
	select {
	case <-done:
		t.Fatal("returned for an earlier start's pid file")
	case <-time.After(10 * lifelineTick):
	}
	// A new start's hostagent, already gone.
	again := exec.CommandContext(t.Context(), "true")
	if err := again.Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(again.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("still watching a pid file written since, whose process is gone")
	}
}

// With -listen-unix, the proxy serves on the socket as on its port, the password required.
func TestMainListensOnSocket(t *testing.T) {
	lifelineTick = 50 * time.Millisecond
	t.Cleanup(func() { lifelineTick = 3 * time.Second; proxyAuth = "" })
	dir, d := t.TempDir(), sockDir(t)
	key, sock, pidFile := filepath.Join(dir, "key"), filepath.Join(d, "p.sock"), filepath.Join(dir, "ha.pid")
	if err := os.WriteFile(key, []byte("k3y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sleep := exec.CommandContext(t.Context(), "sleep", "60")
	if err := sleep.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sleep.Process.Kill() })
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(sleep.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	errs := &syncBuffer{}
	done := make(chan error)
	go func() {
		done <- Main([]string{
			"-listen", "127.0.0.1:0", "-listen-unix", sock, "-vm", "devvm-x", "-auth-file", key,
			"-lifeline", pidFile, "-log", filepath.Join(dir, "log.jsonl"),
		}, IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: errs})
	}()
	defer func() {
		time.Sleep(5 * lifelineTick) // the proxy has seen the lifeline
		_ = sleep.Process.Kill()
		_ = sleep.Wait()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Main: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("still running after its lifeline was gone")
		}
	}()
	ask := func(auth string) (int, error) {
		c, err := (&net.Dialer{}).DialContext(t.Context(), "unix", sock)
		if err != nil {
			return 0, err
		}
		defer func() { _ = c.Close() }()
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://devvm.proxy/blocked", nil)
		if auth != "" {
			req.Header.Set("Proxy-Authorization", auth)
		}
		if err := req.WriteProxy(c); err != nil {
			return 0, err
		}
		resp, err := http.ReadResponse(bufio.NewReader(c), req)
		if err != nil {
			return 0, err
		}
		_ = resp.Body.Close()
		return resp.StatusCode, nil
	}
	var code int
	var err error
	for range 100 {
		if code, err = ask("Basic ZGV2dm06azN5"); err == nil { // devvm:k3y
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil || code != http.StatusOK {
		t.Fatalf("with the password: %d %v", code, err)
	}
	if code, err := ask(""); err != nil || code != http.StatusProxyAuthRequired {
		t.Errorf("without it: %d %v", code, err)
	}
	if s := errs.String(); !strings.Contains(s, "listening on 127.0.0.1:") {
		t.Errorf("stderr = %q", s)
	}
}
