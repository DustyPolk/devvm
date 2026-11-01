package vm

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// vmssh can't import vm, so it has its own check for the links'
// addresses: the two agree, on every address a link gets and around them.
func TestLinkAddrsAgree(t *testing.T) {
	addrs := []string{"127.0.0.1", "127.76.255.255", "127.78.0.0", "::1", "::ffff:127.77.0.1", "::ffff:127.0.0.1", "10.77.0.1", "0.0.0.0", "::"}
	for i := range 0x10000 {
		addrs = append(addrs, linkNetAddr(uint16(i)).String())
	}
	for _, s := range addrs {
		a := netip.MustParseAddr(s)
		if IsLinkAddr(a) != vmssh.IsLinkAddr(a) {
			t.Errorf("%s: vm says %v, vmssh %v", s, IsLinkAddr(a), vmssh.IsLinkAddr(a))
		}
	}
	if !vmssh.IsLinkAddr(linkNetAddr(1)) || !vmssh.IsLinkAddr(linkNetAddr(0xfffe)) {
		t.Error("a link's address isn't vmssh's")
	}
}

// linkSessionEnv is an Env with a fake backend whose VMs are app, a
// client linked to db's port 5432, and db, which publishes it, runs, and
// has its host key pinned.
func linkSessionEnv(t *testing.T) (Links, *backendtest.Fake, *state.VM, *state.VM, Link) {
	t.Helper()
	root := t.TempDir()
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config")}
	f := backendtest.New("fake", backend.Caps{backend.FeatureSSH: backend.Native, backend.FeatureLink: backend.Native})
	f.Target = backend.SSHTarget{
		Host: "127.0.0.1", Port: 62022, User: "u",
		Options: []backend.SSHOption{{Key: "IdentityFile", Value: `"/k"`}, {Key: "IdentityFile", Value: `"/other"`}},
	}
	reg, err := backend.NewRegistry(f)
	if err != nil {
		t.Fatal(err)
	}
	client, err := state.CreateVM(d, "devvm-app-0123456789abcd", state.NewVM{Image: "base", Backend: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := state.CreateVM(d, "devvm-db-0123456789abcd", state.NewVM{Image: "base", Backend: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Set(KeyPublish, "5432"); err != nil {
		t.Fatal(err)
	}
	l := Link{Name: "db", Port: 5432, Target: target.Name(), TargetID: target.Get(state.KeyID), Addr: netip.MustParseAddr("127.77.0.1")}
	if err := writeLinks(client, []Link{l}); err != nil {
		t.Fatal(err)
	}
	for _, vm := range []string{client.Name(), target.Name()} {
		f.Instances[vm] = backend.Instance{Name: vm, Status: backend.StatusRunning}
	}
	if err := os.MkdirAll(d.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	pins := client.Name() + " ssh-ed25519 AAAA\n" + target.Name() + " ssh-ed25519 BBBB\n"
	if err := os.WriteFile(d.KnownHosts(), []byte(pins), 0o600); err != nil {
		t.Fatal(err)
	}
	return Links{Env: &Env{Dirs: d, Registry: reg}}, f, client, target, l
}

// Each connection is checked against the states as they are then: a link
// that applies reaches its running target as devvm pins it; one the
// client no longer has, one whose target is gone, made again, no longer
// publishing the port or without the network policy the client has, one
// whose target's ssh isn't devvm's, and a target that doesn't run, are
// each refused with why, and the target is never started.
func TestLinkConn(t *testing.T) {
	e, f, client, target, l := linkSessionEnv(t)
	conn := vmssh.Link{Name: l.Name, Target: l.Target, Addr: l.Addr, Port: l.Port}
	check := func(want, why string) vmssh.LinkTarget {
		t.Helper()
		got, ev, w := e.linkConn(t.Context(), client.Name(), conn)
		if ev != want || !strings.HasPrefix(w, why) {
			t.Errorf("linkConn = %q, %q; want %q, %q", ev, w, want, why)
		}
		return got
	}
	got := check("", "")
	want := vmssh.SSH{VM: target.Name(), User: "u", Key: "/k", KnownHosts: e.Dirs.KnownHosts(), Port: 62022}
	if got.SSH != want || got.Start != "start-0" {
		t.Errorf("the target: %+v, want %+v", got, want)
	}

	// The target's ssh: through its backend's command, else on the
	// host's loopback alone, and with its host key pinned.
	f.Target.ProxyCommand = "'/devvm' '_docker-connect' 'devvm-db'"
	f.Target.Host, f.Target.Port = target.Name(), 0
	if got := check("", ""); got.Proxy != f.Target.ProxyCommand {
		t.Errorf("through a command: %+v", got)
	}
	f.Target.ProxyCommand = "x\nProxyCommand y"
	check(vmssh.EventLinkRefused, LinkNoSSH)
	f.Target.ProxyCommand, f.Target.Host, f.Target.Port = "", "10.0.0.2", 22
	check(vmssh.EventLinkRefused, LinkNoSSH)
	f.Target.Host = "127.0.0.1"
	if err := os.WriteFile(e.Dirs.KnownHosts(), []byte(client.Name()+" ssh-ed25519 AAAA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	check(vmssh.EventLinkRefused, LinkNoSSH)
	if err := os.WriteFile(e.Dirs.KnownHosts(), []byte(target.Name()+" ssh-ed25519 BBBB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	check("", "")

	// A target that doesn't run: refused, never started.
	f.Instances[target.Name()] = backend.Instance{Name: target.Name(), Status: backend.StatusStopped}
	check(vmssh.EventLinkStopped, "")
	for _, m := range f.Methods() {
		if m == "Start" {
			t.Error("a connection started the target")
		}
	}
	f.Instances[target.Name()] = backend.Instance{Name: target.Name(), Status: backend.StatusRunning}

	// The policy rule: a client with network policy, a target without.
	if err := client.Set(KeyNet, "on"); err != nil {
		t.Fatal(err)
	}
	check(vmssh.EventLinkRefused, LinkPolicy)
	if err := target.Set(netpolicy.KeyNet, "on"); err != nil {
		t.Fatal(err)
	}
	check("", "")

	if err := target.Set(KeyPublish, "80"); err != nil {
		t.Fatal(err)
	}
	check(vmssh.EventLinkRefused, LinkUnpublished)
	if err := target.Set(KeyPublish, "5432"); err != nil {
		t.Fatal(err)
	}

	// The link removed from the client, then another address's.
	if err := writeLinks(client, nil); err != nil {
		t.Fatal(err)
	}
	check(vmssh.EventLinkRefused, LinkRemoved)
	other := l
	other.Addr = netip.MustParseAddr("127.77.0.2")
	if err := writeLinks(client, []Link{other}); err != nil {
		t.Fatal(err)
	}
	check(vmssh.EventLinkRefused, LinkRemoved)
	if err := writeLinks(client, []Link{l}); err != nil {
		t.Fatal(err)
	}
	check("", "")

	// The target made again, then gone.
	if _, err := state.CreateVM(e.Dirs, target.Name(), state.NewVM{Image: "base", Backend: "fake"}); err != nil {
		t.Fatal(err)
	}
	check(vmssh.EventLinkRefused, LinkReplaced)
	if err := target.Remove(); err != nil {
		t.Fatal(err)
	}
	check(vmssh.EventLinkRefused, LinkGone)
}

// A session carries every link its client's state has, applying or not,
// and none once the client is gone; it refuses a client that isn't
// running, or whose ssh isn't devvm's.
func TestSessionLinks(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	e, _, client, _, l := linkSessionEnv(t)
	got, err := e.sessionLinks(client.Name())
	if err != nil || len(got) != 1 || got[0] != (vmssh.Link{Name: "db", Target: l.Target, Addr: l.Addr, Port: 5432}) {
		t.Errorf("sessionLinks = %+v, %v", got, err)
	}
	if err := client.Remove(); err != nil {
		t.Fatal(err)
	}
	if got, err := e.sessionLinks(client.Name()); err != nil || len(got) != 0 {
		t.Errorf("a client that's gone: %+v, %v", got, err)
	}
	if err := e.Session(t.Context(), client.Name(), nil); err == nil || !strings.Contains(err.Error(), "no such VM") {
		t.Errorf("Session of a client that's gone: %v", err)
	}
	e, f, client, _, _ := linkSessionEnv(t)
	f.Instances[client.Name()] = backend.Instance{Name: client.Name(), Status: backend.StatusStopped}
	if err := e.Session(t.Context(), client.Name(), nil); names(err) != "vm_not_running vm="+client.Name() {
		t.Errorf("Session of a stopped client: %v", err)
	}
	f.Instances[client.Name()] = backend.Instance{Name: client.Name(), Status: backend.StatusRunning}
	f.Capabilities = backend.Caps{backend.FeatureLink: backend.Native}
	if err := e.Session(t.Context(), client.Name(), nil); err == nil || !strings.Contains(err.Error(), "ssh isn't usable") {
		t.Errorf("Session without ssh: %v", err)
	}
	f.Capabilities = backend.Caps{backend.FeatureSSH: backend.Native}
	if err := e.Session(t.Context(), client.Name(), nil); names(err) != "backend_lacks backend=fake feature=link vm="+client.Name() {
		t.Errorf("Session on a backend without links: %v", err)
	}
}

// A link session's events go to its client's event log, as devvm network
// log reads it, whether or not the client has network policy.
func TestLinkEvents(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	le := linkEvents{path: netpolicy.EventLog(d, "devvm-app-0123456789abcd"), vm: "devvm-app-0123456789abcd", mu: new(sync.Mutex)}
	le.add(vmssh.EventLinkRefused, vmssh.Link{Name: "db", Target: "devvm-db-0123456789abcd", Port: 5432}, LinkUnpublished)
	events, err := netpolicy.NewLog(d, "devvm-app-0123456789abcd").Events(5)
	if err != nil || len(events) != 1 {
		t.Fatalf("Events = %+v, %v", events, err)
	}
	if ev := events[0]; ev.Event != "link_refused" || ev.Host != "db" || ev.Port != 5432 || ev.Target != "devvm-db-0123456789abcd" || ev.Reason != "unpublished" {
		t.Errorf("the event: %+v", ev)
	}
}
