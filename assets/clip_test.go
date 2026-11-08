package assets

import (
	"bufio"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// clipServer is devvm's end of clipboard images in guest/clip.sh's test:
// it answers each connection's one line as internal/clipboard does, from
// img ("" for a clipboard with no image), and records the lines.
type clipServer struct {
	mu   sync.Mutex
	img  string
	asks []string
}

func (s *clipServer) serve(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		line, _ := bufio.NewReader(c).ReadString('\n')
		s.mu.Lock()
		s.asks = append(s.asks, strings.TrimSuffix(line, "\n"))
		img := s.img
		s.mu.Unlock()
		switch {
		case img == "":
		case line == "types\n":
			_, _ = c.Write([]byte("image/png\n"))
		case line == "image/png\n":
			_, _ = c.Write([]byte(img))
		}
		_ = c.Close()
	}
}

// clip.sh answers what Claude Code and pi run when you paste (their own
// command lines, from their releases), from devvm's answer: the types and
// the image, or a failure when the clipboard has none or devvm doesn't
// answer. Anything else goes to the VM's own command, else fails, and asks
// devvm nothing.
func TestClipScript(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("no bash: %v", err)
	}
	script, err := fs.ReadFile(FS, "guest/clip.sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin, own, conf := filepath.Join(dir, "bin"), filepath.Join(dir, "own"), filepath.Join(dir, "clipboard")
	must(t, os.MkdirAll(bin, 0o700))
	must(t, os.MkdirAll(own, 0o700))
	s := string(script)
	for from, to := range map[string]string{"conf=/etc/devvm/clipboard": "conf=" + conf, "for d in /usr/bin /bin;": "for d in " + own + ";"} {
		if strings.Count(s, from) != 1 {
			t.Fatalf("clip.sh has %q %d times", from, strings.Count(s, from))
		}
		s = strings.Replace(s, from, to, 1)
	}
	must(t, os.WriteFile(filepath.Join(bin, "devvm-clip"), []byte(s), 0o700)) //nolint:gosec // the test's own script
	for _, n := range []string{"xclip", "wl-paste"} {
		must(t, os.Symlink("devvm-clip", filepath.Join(bin, n)))
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	srv := &clipServer{}
	go srv.serve(ln)
	host, port, err := net.SplitHostPort(ln.Addr().String())
	must(t, err)
	must(t, os.WriteFile(conf, []byte(host+" "+port+"\n"), 0o600))

	run := func(name string, args ...string) (string, bool) {
		cmd := exec.CommandContext(t.Context(), filepath.Join(bin, name), args...) //nolint:gosec // the test's own script
		out, err := cmd.Output()
		return string(out), err == nil
	}
	const png = "\x89PNG\r\n\x1a\nfake image data\x00\xff"
	for _, c := range []struct {
		name string
		img  string
		cmd  []string
		out  string
		ok   bool
		asks []string
	}{
		{"claude code's check", png, []string{"xclip", "-selection", "clipboard", "-t", "TARGETS", "-o"}, "image/png\n", true, []string{"types"}},
		{"claude code's save", png, []string{"xclip", "-selection", "clipboard", "-t", "image/png", "-o"}, png, true, []string{"image/png"}},
		{"claude code's wl-paste check", png, []string{"wl-paste", "-l"}, "image/png\n", true, []string{"types"}},
		{"claude code's wl-paste save", png, []string{"wl-paste", "--type", "image/png"}, png, true, []string{"image/png"}},
		{"pi's list", png, []string{"wl-paste", "--list-types"}, "image/png\n", true, []string{"types"}},
		{"pi's read", png, []string{"wl-paste", "--type", "image/png", "--no-newline"}, png, true, []string{"image/png"}},
		{"pi's xclip read", png, []string{"xclip", "-selection", "clipboard", "-t", "image/png", "-o"}, png, true, []string{"image/png"}},
		{"abbreviated", png, []string{"xclip", "-sel", "c", "-target", "image/png", "-out"}, png, true, []string{"image/png"}},
		{"no image: the check", "", []string{"xclip", "-selection", "clipboard", "-t", "TARGETS", "-o"}, "", false, []string{"types"}},
		{"no image: the save", "", []string{"wl-paste", "--type=image/png"}, "", false, []string{"image/png"}},
		{"text", png, []string{"xclip", "-selection", "clipboard", "-o"}, "", false, nil},
		{"another type", png, []string{"xclip", "-selection", "clipboard", "-t", "image/bmp", "-o"}, "", false, nil},
		{"the primary selection", png, []string{"wl-paste", "--primary", "--type", "image/png"}, "", false, nil},
		{"copying", png, []string{"xclip", "-selection", "clipboard", "-i"}, "", false, nil},
	} {
		srv.mu.Lock()
		srv.img, srv.asks = c.img, nil
		srv.mu.Unlock()
		out, ok := run(c.cmd[0], c.cmd[1:]...)
		srv.mu.Lock()
		asks := srv.asks
		srv.mu.Unlock()
		if out != c.out || ok != c.ok || !slices.Equal(asks, c.asks) {
			t.Errorf("%s: %q, ok %v, asked %q; want %q, ok %v, asked %q", c.name, out, ok, asks, c.out, c.ok, c.asks)
		}
	}

	// The VM's own command, when it has one, gets what devvm-clip doesn't
	// answer, as it was given.
	must(t, os.WriteFile(filepath.Join(own, "xclip"), []byte("#!/bin/sh\necho \"own xclip $*\"\n"), 0o700)) //nolint:gosec // the test's own script
	if out, ok := run("xclip", "-selection", "clipboard", "-o"); out != "own xclip -selection clipboard -o\n" || !ok {
		t.Errorf("text, with the VM's own xclip: %q, ok %v", out, ok)
	}

	// devvm gone, or the setting off (no address): nothing, and a failure.
	_ = ln.Close()
	if out, ok := run("xclip", "-selection", "clipboard", "-t", "image/png", "-o"); out != "" || ok {
		t.Errorf("devvm gone: %q, ok %v", out, ok)
	}
	must(t, os.Remove(conf))
	if out, ok := run("wl-paste", "--list-types"); out != "" || ok {
		t.Errorf("no address: %q, ok %v", out, ok)
	}
}
