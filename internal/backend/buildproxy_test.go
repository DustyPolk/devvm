package backend

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/msg"
)

// A build's proxy starts with its own password, its policy and the
// company proxy's file, all the user's alone, and none of them in its
// arguments; it's found by the port its status file names. Stop ends it
// and deletes the password and the company proxy's file, leaving the
// logs.
func TestProxyStarter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "build")
	var args []string
	var ended <-chan struct{}
	s := ProxyStarter{Self: "/devvm", Tick: time.Millisecond, Spawn: func(c daemon.Cmd) (*daemon.Handle, error) {
		args = append([]string{c.Path}, c.Args...)
		h, err := sleeper(t, c, "127.0.0.1:40123")
		if err == nil {
			ended = h.Done()
		}
		return h, err
	}}
	up := []byte("HTTPS_PROXY=" + corpProxy + "\nHTTP_PROXY=\nNO_PROXY=\n")
	p, err := s.Start("devvm-img-x", dir, "/run/ha.pid", up, "-listen-unix", "/s.sock")
	if err != nil {
		t.Fatal(err)
	}
	if p.Port != 40123 || len(p.Password) != 64 {
		t.Errorf("port %d, password %q", p.Port, p.Password)
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
	case <-ended:
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

// sleeper starts a sleep in c's place that says it's ready with say in
// c's status file ("" for never), stopped when the test ends.
func sleeper(t *testing.T, c daemon.Cmd, say string) (*daemon.Handle, error) {
	t.Helper()
	h, err := daemon.Spawn(daemon.Cmd{Path: "sleep", Args: []string{"60"}, Out: c.Out})
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { h.Stop(time.Second) })
	if say != "" {
		err = daemon.WriteStatus(c.Status, h.PID(), say)
	}
	return h, err
}

// A proxy that never says it listens, or says so of an address that
// isn't loopback or has no port, is stopped, naming its log; so is one
// that said it listens but has already ended (another program took the
// port from it).
func TestProxyStarterFails(t *testing.T) {
	for _, say := range []string{"", "192.0.2.1:3128", "127.0.0.1:0", "127.0.0.1:x", "ended"} {
		dir := t.TempDir()
		var h *daemon.Handle
		s := ProxyStarter{Self: "/devvm", Tick: time.Millisecond, Spawn: func(c daemon.Cmd) (*daemon.Handle, error) {
			if say != "ended" {
				var err error
				h, err = sleeper(t, c, say)
				return h, err
			}
			var err error
			if h, err = daemon.Spawn(daemon.Cmd{Path: "true", Out: c.Out}); err != nil {
				return nil, err
			}
			<-h.Done()
			return h, daemon.WriteStatus(c.Status, h.PID(), "127.0.0.1:40123")
		}}
		_, err := s.Start("b", dir, "/l", nil)
		if msg.NamesOf(err) != "proxy_start_failed log="+filepath.Join(dir, "proxy.log")+" vm=b" {
			t.Errorf("%q: Start = %v", say, err)
		}
		select {
		case <-h.Done():
		case <-time.After(5 * time.Second):
			t.Errorf("%q: left running", say)
		}
	}
}

// A build's proxy is stopped through the process devvm started: one still
// running ends; one that has ended is left alone (no signal reaches a
// process given its pid since); none is nothing.
func TestBuildProxyStop(t *testing.T) {
	h, err := daemon.Spawn(daemon.Cmd{Path: "sleep", Args: []string{"60"}})
	if err != nil {
		t.Fatal(err)
	}
	bp := &BuildProxy{proc: h, dir: t.TempDir()}
	bp.Stop()
	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("not stopped")
	}
	bp.Stop()
	var none *BuildProxy
	none.Stop()
}
