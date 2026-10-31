package vm

import (
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/w3cj/devvm/internal/desktop"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

func desktopVM(t *testing.T) (paths.Dirs, *state.VM) {
	t.Helper()
	root := t.TempDir()
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config")}
	st, err := state.CreateVM(d, "devvm-p-00000000000000", state.NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	return d, st
}

// The password is made once and kept; one that isn't a password devvm
// makes is made again.
func TestDesktopPassword(t *testing.T) {
	d, st := desktopVM(t)
	p1, err := desktopPassword(st)
	if err != nil || !desktop.ValidPassword(p1) {
		t.Fatalf("desktopPassword = %q, %v", p1, err)
	}
	again, err := state.ReadVM(d, st.Name())
	if err != nil {
		t.Fatal(err)
	}
	if p2, _ := desktopPassword(again); p2 != p1 {
		t.Errorf("second desktopPassword = %q, want %q", p2, p1)
	}
	for _, bad := range []string{"short", "has spaces!", "ABCDEFGH"} {
		if err := again.Set(KeyDesktopPassword, bad); err != nil {
			t.Fatal(err)
		}
		if p, _ := desktopPassword(again); p == bad || !desktop.ValidPassword(p) {
			t.Errorf("with %q kept: desktopPassword = %q", bad, p)
		}
	}
}

// The listener is on 127.0.0.1 alone, on the port kept for the VM while
// it's free, else on a new one, which is kept.
func TestDesktopListener(t *testing.T) {
	d, st := desktopVM(t)
	e := Desktop{&Env{Dirs: d}}
	l, err := e.listener(t.Context(), st)
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	if !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) || st.Get(KeyDesktopPort) != strconv.Itoa(addr.Port) {
		t.Fatalf("listening on %v, kept %q", addr, st.Get(KeyDesktopPort))
	}
	// Taken (by the first): another one, kept in its place.
	l2, err := e.listener(t.Context(), st)
	if err != nil {
		t.Fatal(err)
	}
	p2 := l2.Addr().(*net.TCPAddr).Port
	if p2 == addr.Port || st.Get(KeyDesktopPort) != strconv.Itoa(p2) {
		t.Errorf("second on %d, kept %q", p2, st.Get(KeyDesktopPort))
	}
	_ = l.Close()
	_ = l2.Close()
	// Free again: the kept one.
	l3, err := e.listener(t.Context(), st)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l3.Close() }()
	if p := l3.Addr().(*net.TCPAddr).Port; p != p2 {
		t.Errorf("third on %d, want the kept %d", p, p2)
	}
}

// The ssh channel: devvm's ssh config, a channel to the desktop's socket,
// no session, agent or forwarding.
func TestDesktopSSHArgs(t *testing.T) {
	args := desktopSSHArgs("/cfg/ssh_config", "devvm-p-00000000000000")
	for _, want := range [][]string{
		{"-F", "/cfg/ssh_config"},
		{"-W", desktop.Socket, "devvm-p-00000000000000"},
		{"-o", "IdentityAgent=none"},
		{"-o", "ForwardAgent=no"},
		{"-o", "ForwardX11=no"},
		{"-o", "ClearAllForwardings=yes"},
		{"-o", "ControlMaster=no"},
		{"-T"},
	} {
		found := false
		for i := range args {
			if slices.Equal(args[i:min(len(args), i+len(want))], want) {
				found = true
			}
		}
		if !found {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	if args[len(args)-1] != "devvm-p-00000000000000" {
		t.Errorf("the host isn't last: %q", args)
	}
}
