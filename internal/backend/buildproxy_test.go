package backend

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A build's proxy starts with its own password, its policy and the
// company proxy's file, all the user's alone, and none of them in its
// arguments; it's found by the port its log names. Stop ends it and
// deletes the password and the company proxy's file, leaving the logs.
func TestProxyStarter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "build")
	var args []string
	var ended chan error
	s := ProxyStarter{Self: "/devvm", Tick: time.Millisecond, Spawn: func(path string, a []string, out *os.File) (*os.Process, error) {
		args = append([]string{path}, a...)
		p := exec.CommandContext(t.Context(), "sleep", "60")
		if err := p.Start(); err != nil {
			return nil, err
		}
		ended = make(chan error, 1)
		go func() { ended <- p.Wait() }()
		t.Cleanup(func() { _ = p.Process.Kill() })
		_, _ = out.WriteString("devvm proxy: listening on 127.0.0.1:40123 for x\n")
		return p.Process, nil
	}}
	up := []byte("HTTPS_PROXY=" + corpProxy + "\nHTTP_PROXY=\nNO_PROXY=\n")
	p, err := s.Start("devvm-img-x", dir, "/run/ha.pid", up, "-listen-unix", "/s.sock")
	if err != nil {
		t.Fatal(err)
	}
	if p.Port != 40123 || len(p.Password) != 64 {
		t.Errorf("port %d, password %q", p.Port, p.Password)
	}
	if got, want := p.URL("192.168.5.2"), "http://devvm:"+p.Password+"@192.168.5.2:40123"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
	line := strings.Join(args, " ")
	for _, want := range []string{
		"/devvm _proxy -listen 127.0.0.1:0 -vm devvm-img-x -policy " + filepath.Join(dir, "policy"),
		"-auth-file " + filepath.Join(dir, "auth"), "-lifeline /run/ha.pid", "-listen-unix /s.sock",
		"-upstream " + filepath.Join(dir, "upstream"),
	} {
		if !strings.Contains(line, want) {
			t.Errorf("arguments %q lack %q", line, want)
		}
	}
	if strings.Contains(line, p.Password) || strings.Contains(line, "u:p@") {
		t.Errorf("a password in the arguments: %s", line)
	}
	for f, want := range map[string]string{"auth": p.Password + "\n", "upstream": string(up), "policy": "**"} {
		b, err := os.ReadFile(filepath.Join(dir, f)) //nolint:gosec // the test's own folder
		fi, serr := os.Stat(filepath.Join(dir, f))
		if err != nil || serr != nil || !strings.Contains(string(b), want) || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: %q %v %v", f, b, err, serr)
		}
	}
	p.Stop()
	select {
	case err := <-ended:
		t.Logf("stopped: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("not stopped")
	}
	for _, f := range []string{"auth", "upstream"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s stayed: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "proxy.log")); err != nil {
		t.Errorf("the log went: %v", err)
	}

	// Without a company proxy, no file for one.
	if p, err = s.Start("devvm-img-x", dir, "/run/ha.pid", nil); err != nil || slices.Contains(args, "-upstream") {
		t.Errorf("no upstream: %v %v", err, args)
	}
	p.Stop()
}

// A proxy that never says it listens is stopped, naming its log.
func TestProxyStarterFails(t *testing.T) {
	dir := t.TempDir()
	s := ProxyStarter{Self: "/devvm", Tick: time.Millisecond, Spawn: func(string, []string, *os.File) (*os.Process, error) {
		p := exec.CommandContext(t.Context(), "sleep", "60")
		if err := p.Start(); err != nil {
			return nil, err
		}
		go func() { _ = p.Wait() }()
		return p.Process, nil
	}}
	_, err := s.Start("b", dir, "/l", nil)
	if names(t, err) != "proxy_start_failed log="+filepath.Join(dir, "proxy.log")+" vm=b" {
		t.Errorf("Start = %v", err)
	}
}

func TestListeningPort(t *testing.T) {
	for log, want := range map[string]int{
		"devvm proxy: listening on 127.0.0.1:3128 for b\n": 3128,
		"devvm proxy: listening on 127.0.0.1:3128\n":       3128,
		"devvm proxy: loaded\n":                            0,
		"devvm proxy: listening on 127.0.0.1:x for b\n":    0,
		"devvm proxy: listening on 127.0.0.1:99999 for b":  0,
	} {
		if got := listeningPort(log); got != want {
			t.Errorf("listeningPort(%q) = %d, want %d", log, got, want)
		}
	}
}

// A build's proxy is stopped through the process devvm started: one still
// running ends; one that has ended and been waited for is left alone (no
// signal reaches a process given its pid since); none is nothing.
func TestBuildProxyStop(t *testing.T) {
	p := exec.CommandContext(t.Context(), "sleep", "60")
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- p.Wait() }()
	bp := &BuildProxy{proc: p.Process, dir: t.TempDir()}
	bp.Stop()
	select {
	case err := <-exited:
		if ee, ok := errors.AsType[*exec.ExitError](err); !ok || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGTERM {
			t.Errorf("ended: %v, want by SIGTERM", err)
		}
	case <-time.After(2 * time.Second):
		_ = p.Process.Kill()
		t.Fatal("not stopped")
	}
	if err := p.Process.Signal(syscall.SIGTERM); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("a signal to the ended process: %v, want it refused as done", err)
	}
	bp.Stop()
	var none *BuildProxy
	none.Stop()
}
