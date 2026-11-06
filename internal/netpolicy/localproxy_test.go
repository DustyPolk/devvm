package netpolicy

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devvmhq/devvm/assets"
	"github.com/devvmhq/devvm/internal/state"
)

// localProxyScript is the guest setup's devvm-local-proxy as sync.sh writes it, sending to
// upstream with the password key.
func localProxyScript(t *testing.T, upstream, key string) string {
	t.Helper()
	b, err := fs.ReadFile(assets.FS, "guest/sync.sh")
	if err != nil {
		t.Fatal(err)
	}
	const open = "cat <<'SH'\n# The VM's local proxy"
	i := bytes.Index(b, []byte(open))
	if i < 0 {
		t.Fatal("sync.sh has no devvm-local-proxy")
	}
	body := b[i+len("cat <<'SH'\n"):]
	body = body[:bytes.Index(body, []byte("\nSH\n"))+1]
	host, port, _ := net.SplitHostPort(upstream)
	auth := base64.StdEncoding.EncodeToString([]byte("devvm:" + key))
	p := filepath.Join(t.TempDir(), "devvm-local-proxy")
	script := fmt.Sprintf("#!/bin/bash\nup_host=%s up_port=%s auth=%s\nlisten=127.0.77.1 port=3128\n%s", host, port, auth, body)
	if err := state.WriteFileMode(p, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeProxy is devvm's proxy as the local proxy sees it: the password in Proxy-Authorization, or
// 407; blocked.test's 403; an echo for a CONNECT. It keeps the last request's proxy headers.
type fakeProxy struct {
	key  string
	mu   sync.Mutex
	seen []http.Header
	shut []bool
}

func (f *fakeProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.seen, f.shut = append(f.seen, r.Header.Clone()), append(f.shut, r.Close)
	f.mu.Unlock()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("devvm:"+f.key))
	if v := r.Header.Values("Proxy-Authorization"); len(v) != 1 || v[0] != want {
		w.Header().Set("Proxy-Authenticate", `Basic realm="devvm"`)
		http.Error(w, "devvm proxy: this proxy belongs to another VM", http.StatusProxyAuthRequired)
		return
	}
	switch {
	case strings.HasPrefix(r.Host, "blocked.test"):
		http.Error(w, "devvm: blocked.test:80 is blocked", http.StatusForbidden)
	case r.Method == http.MethodConnect:
		c, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_, _ = io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
		line, _ := rw.ReadString('\n')
		_, _ = io.WriteString(c, "echo:"+line)
	case r.URL.String() == "http://ok.test/x":
		_, _ = io.WriteString(w, "hello ok.test\n")
	default:
		http.NotFound(w, r)
	}
}

// through sends req through the local proxy at script, on one connection of its own, as socat's
// nofork hands it over, and returns all that comes back until the connection ends. then, when
// given, runs on the connection once the reply's head has come.
func through(t *testing.T, script, req string, then func(net.Conn, *bufio.Reader) string) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx // a test's moment
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	client, err := net.Dial("tcp", l.Addr().String()) //nolint:noctx // a test's moment
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	server, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	f, err := server.(*net.TCPConn).File()
	_ = server.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), script)     // its own #!/bin/bash: bash 3.2 on macOS
	cmd.Stdin, cmd.Stdout, cmd.Stderr = f, f, os.Stderr //nolint:forbidigo // the script's errors, in the test's output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = client.Close(); _ = cmd.Wait() }()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.WriteString(client, req); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(client)
	if then != nil {
		return then(client, r)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("%q: %v, after %q", req, err, b)
	}
	return string(b)
}

// devvm-local-proxy sends a proxy's requests to devvm's proxy with the VM's password in place of
// the client's, each plain request's connection closed after its reply; a CONNECT tunnels; the
// proxy's 403 reaches the client; what isn't a proxy's request gets 400 and goes nowhere.
func TestLocalProxy(t *testing.T) {
	for _, c := range []string{"bash", "socat"} {
		if _, err := exec.LookPath(c); err != nil {
			t.Skipf("no %s: %v", c, err)
		}
	}
	up := &fakeProxy{key: "k3y"}
	l, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx // a test's moment
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: up, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	script := localProxyScript(t, l.Addr().String(), up.key)

	got := through(t, script, "GET http://ok.test/x HTTP/1.1\r\nHost: ok.test\r\nproxy-authorization: Basic d3Jvbmc=\r\n"+
		"Proxy-Connection: keep-alive\r\nConnection: keep-alive\r\nX-Kept: 1\r\n\r\n", nil)
	if !strings.HasPrefix(got, "HTTP/1.1 200 ") || !strings.HasSuffix(got, "hello ok.test\n") {
		t.Errorf("GET = %q", got)
	}
	up.mu.Lock()
	h, shut := up.seen[0], up.shut[0]
	up.mu.Unlock()
	if !shut || h.Get("Proxy-Connection") != "" || h.Get("X-Kept") != "1" {
		t.Errorf("the proxy got %v, closing %v", h, shut)
	}

	if got := through(t, script, "GET http://blocked.test/ HTTP/1.1\r\nHost: blocked.test\r\n\r\n", nil); !strings.HasPrefix(got, "HTTP/1.1 403 ") ||
		!strings.Contains(got, "devvm: blocked.test:80 is blocked") {
		t.Errorf("a blocked host = %q", got)
	}

	got = through(t, script, "CONNECT ok.test:443 HTTP/1.1\r\nHost: ok.test:443\r\n\r\n", func(c net.Conn, r *bufio.Reader) string {
		for {
			line, err := r.ReadString('\n')
			if err != nil || line == "\r\n" {
				break
			}
		}
		_, _ = io.WriteString(c, "ping\n")
		b, _ := io.ReadAll(r)
		return string(b)
	})
	if got != "echo:ping\n" {
		t.Errorf("CONNECT's tunnel = %q", got)
	}

	up.mu.Lock()
	n := len(up.seen)
	up.mu.Unlock()
	for _, req := range []string{
		"GET / HTTP/1.1\r\nHost: 127.0.77.1:3128\r\n\r\n",                        // a web page's, to it as a server
		"POST /x HTTP/1.1\r\nHost: 127.0.77.1:3128\r\nContent-Length: 0\r\n\r\n", // the same
		"CONNECT ok.test:443\r\n\r\n",                                            // not HTTP
		"GET ftp://ok.test/ HTTP/1.1\r\n\r\n",
		"CONNECT a b:1 HTTP/1.1\r\n\r\n",
		"hello\r\n",
	} {
		got := through(t, script, req, func(_ net.Conn, r *bufio.Reader) string {
			res, err := http.ReadResponse(r, nil)
			if err != nil {
				return err.Error()
			}
			b, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			return fmt.Sprint(res.StatusCode, " ", string(b))
		})
		if !strings.HasPrefix(got, "400 devvm-local-proxy: ") {
			t.Errorf("%q = %q", req, got)
		}
	}
	up.mu.Lock()
	defer up.mu.Unlock()
	if len(up.seen) != n {
		t.Errorf("refused requests reached the proxy: %v", up.seen[n:])
	}
}
