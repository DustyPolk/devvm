package netpolicy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/state"
)

// sandbox returns devvm's folders in a temporary folder, and a VM's state
// there with that image.
// vmV is a VM's name, for tests of a VM the sandbox's isn't.
const vmV = "devvm-v-0123456789abc6"

func sandbox(t *testing.T, img string, tools ...string) (paths.Dirs, *state.VM) {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root) // the run log
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config"), Lima: filepath.Join(root, "lima")}
	st, err := state.CreateVM(d, "devvm-web-0123456789abcd", state.NewVM{Image: img, Tools: tools, Dir: "/w", Backend: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	return d, st
}

// A VM with network policy reaches only itself without its proxy: its
// links go through the proxy, so no link name is in its NO_PROXY.
func TestNoProxy(t *testing.T) {
	want := map[string]string{
		"NO_PROXY": "localhost,127.0.0.1,::1", "no_proxy": "localhost,127.0.0.1,::1", "NODE_USE_ENV_PROXY": "1",
	}
	if NoProxy != "localhost,127.0.0.1,::1" || !maps.Equal(Env, want) {
		t.Errorf("NoProxy = %q, Env = %q", NoProxy, Env)
	}
}

// A VM's policies: devvm's built-in ones by name, then the user's files.
func TestPolicies(t *testing.T) {
	d := paths.Dirs{Data: "/d", Config: "/c", Lima: "/l"}
	user := []string{"/c/policy", "/d/vms/0123456789abc1/policy", "/d/vms/0123456789abc1/run/secrets.policy"}
	for _, c := range []struct {
		img           string
		tools         []string
		builtin, file []string
	}{
		{"base", []string{"node", "claude"}, []string{"layers/base.policy", "shared-tools/node.policy", "shared-tools/claude.policy"}, user},
		// A custom layer's allowlist is the user's, next to its script.
		{"pg", []string{"node"}, []string{"layers/base.policy", "shared-tools/node.policy"}, slices.Concat([]string{"/c/layers/pg.policy"}, user)},
		// A built-in layer's is devvm's.
		{"docker", []string{"node"}, []string{"layers/base.policy", "shared-tools/node.policy", "layers/docker.policy"}, user},
		{"base", nil, []string{"layers/base.policy"}, user},
	} {
		builtin, files := Policies(d, "devvm-vm1-0123456789abc1", c.img, c.tools)
		if !slices.Equal(builtin, c.builtin) || !slices.Equal(files, c.file) {
			t.Errorf("Policies(%s, %q) = %q, %q; want %q, %q", c.img, c.tools, builtin, files, c.builtin, c.file)
		}
	}
	// A part that's no layer's name never names a file outside the
	// user's layers, and a name that's no shared tool's names none of
	// devvm's.
	for _, bad := range []string{"../x", "pg+../../etc/x", "a/b", "X", ".x"} {
		builtin, files := Policies(d, "devvm-vm1-0123456789abc1", bad, []string{"../x", "X"})
		if got := slices.Concat(builtin, files); slices.ContainsFunc(got, func(f string) bool {
			return strings.Contains(f, "x.policy") || strings.Contains(f, "b.policy") || strings.Contains(f, "X.policy")
		}) {
			t.Errorf("Policies(%q) = %q", bad, got)
		}
	}
}

// Every image part and shared tool devvm has comes with its allowlist, in
// devvm's files.
func TestEveryPartHasAPolicy(t *testing.T) {
	for _, p := range slices.Concat([]string{"base"}, image.Toolchains, image.Agents, image.Builtins) {
		d := paths.Dirs{Data: "/d", Config: "/c"}
		img, tools := p, []string(nil)
		if image.IsTool(p) {
			img, tools = "base", []string{p}
		}
		builtin, _ := Policies(d, "devvm-vm1-0123456789abc1", img, tools)
		if p != "base" {
			builtin = builtin[1:]
		}
		if len(builtin) != 1 {
			t.Fatalf("%s's built-in policies: %q", p, builtin)
		}
		if _, err := fs.Stat(assets.FS, builtin[0]); err != nil {
			t.Errorf("%s has no policy: %v", p, err)
		}
	}
}

// Each VM has its own random key (its proxy's password) and salt (the end
// of its placeholders), kept.
func TestKeyAndSalt(t *testing.T) {
	d, a := sandbox(t, "base")
	b, err := state.CreateVM(d, "devvm-api-0123456789abc7", state.NewVM{Image: "base", Dir: "/a", Backend: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	ka, _ := Key(a)
	kb, _ := Key(b)
	sa, _ := Salt(a)
	sb, _ := Salt(b)
	for v, re := range map[string]string{ka: `^[0-9a-f]{64}$`, kb: `^[0-9a-f]{64}$`, sa: `^[0-9a-f]{32}$`, sb: `^[0-9a-f]{32}$`} {
		if !regexp.MustCompile(re).MatchString(v) {
			t.Errorf("%q doesn't match %s", v, re)
		}
	}
	if ka == kb || sa == sb || sa == ka {
		t.Errorf("keys %s %s, salts %s %s: not each its own", ka, kb, sa, sb)
	}
	again, _ := state.ReadVM(d, a.Name())
	if k, _ := Key(again); k != ka {
		t.Errorf("key not kept: %s, then %s", ka, k)
	}
	claude, _ := secrets.Lookup("claude")
	github, _ := secrets.Lookup("github")
	if got := claude.Placeholder(sa); got != "sk-ant-oat01-devvm-proxy-managed-"+sa {
		t.Errorf("claude's placeholder = %q", got)
	}
	if got := github.Placeholder(sb); got != "devvm-proxy-managed-github-"+sb {
		t.Errorf("github's placeholder = %q", got)
	}
	if fi, err := os.Stat(d.VMs()); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("the state folder: %v, %v", fi.Mode(), err)
	}
}

// The flag wins; without one and without a terminal, on; in a terminal,
// the answer, asked yes by default ([Y/n]) after the tradeoff.
func TestChoose(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	tty, answer, asked := false, false, 0
	var said []string
	p := &Proxy{
		Note:     func(m msg.M) { said = append(said, m.Names()) },
		Confirm:  func(m msg.M) bool { asked++; said = append(said, m.Names()); return answer },
		Terminal: func() bool { return tty },
	}
	if !p.Choose(true, false, "vm") || p.Choose(false, true, "vm") || !p.Choose(false, false, "vm") || asked != 0 {
		t.Error("without a terminal: not the flags, or off")
	}
	tty = true
	if p.Choose(false, true, "vm") || !p.Choose(true, false, "vm") || asked != 0 {
		t.Error("in a terminal: a flag asked, or wasn't followed")
	}
	if p.Choose(false, false, "vm") || asked != 1 {
		t.Error("no answered yes")
	}
	answer = true
	if !p.Choose(false, false, "vm") || asked != 2 {
		t.Error("yes answered no")
	}
	if want := "net_about vm=vm,net_ask,net_about vm=vm,net_ask"; strings.Join(said, ",") != want {
		t.Errorf("said %v, want %s", said, want)
	}
}

// UsedPorts lists the ports VMs have; freePort avoids them.
func TestFreePort(t *testing.T) {
	d, st := sandbox(t, "base")
	if err := st.Set(KeySSHPort, "23456"); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(KeyProxyPort, "34567"); err != nil {
		t.Fatal(err)
	}
	used, err := UsedPorts(d, "")
	if err != nil || !used[23456] || !used[34567] || len(used) != 2 {
		t.Errorf("UsedPorts = %v, %v", used, err)
	}
	// And the ports VMs forward, entries that aren't ports skipped: all
	// and none name none.
	if err := st.Set(keyPorts, "5173,40000-40002, 41000, bad,9-1"); err != nil {
		t.Fatal(err)
	}
	used, err = UsedPorts(d, "")
	if err != nil || len(used) != 7 || !used[5173] || !used[40000] || !used[40002] || !used[41000] {
		t.Errorf("UsedPorts with forwards = %v, %v", used, err)
	}
	// The VM's own forwarder: its listed ports aren't reserved from it,
	// its ssh and proxy ports are.
	used, err = UsedPorts(d, st.Name())
	if err != nil || len(used) != 2 || !used[23456] || !used[34567] {
		t.Errorf("UsedPorts for its own forwarder = %v, %v", used, err)
	}
	if err := st.Set(keyPorts, "all"); err != nil {
		t.Fatal(err)
	}
	if got := forwarded(st); len(got) != 0 {
		t.Errorf("forwarded with all = %v", got)
	}
	p := &Proxy{Dirs: d, Listening: func(_ context.Context, port int) bool { return port%2 == 0 }}
	for range 200 {
		port, err := p.freePort(t.Context())
		if r := platform.ChoosablePorts(); err != nil || port < r.Lo || port > r.Hi || port%2 == 0 || used[port] {
			t.Fatalf("freePort = %d, %v", port, err)
		}
	}
}

// A VM's ssh port: chosen once and recorded, kept while it's free, or
// while the VM runs on it; a stopped VM gets a new one when another
// program has it, or it lists it to forward.
func TestSSHPort(t *testing.T) {
	d, st := sandbox(t, "base")
	taken := 0
	p := &Proxy{Dirs: d, Listening: func(_ context.Context, port int) bool { return port == taken || port%2 == 0 }}
	sp, err := p.SSHPort(t.Context(), st, false)
	if err != nil || sp == 0 || sp%2 == 0 || st.Get(KeySSHPort) != strconv.Itoa(sp) {
		t.Fatalf("new: %d, %v, state %q", sp, err, st.Get(KeySSHPort))
	}
	if got, err := p.SSHPort(t.Context(), st, false); err != nil || got != sp {
		t.Errorf("free: %d, %v, want %d", got, err, sp)
	}
	// Another program has it: kept while the VM runs (its backend has
	// it), a new one once it's stopped.
	taken = sp
	if got, err := p.SSHPort(t.Context(), st, true); err != nil || got != sp {
		t.Errorf("running: %d, %v, want %d", got, err, sp)
	}
	got, err := p.SSHPort(t.Context(), st, false)
	if err != nil || got == sp || st.Get(KeySSHPort) != strconv.Itoa(got) {
		t.Errorf("taken: %d (had %d), %v, state %q", got, sp, err, st.Get(KeySSHPort))
	}
	// One of its own forwards.
	sp = got
	if err := st.Set(keyPorts, strconv.Itoa(sp)); err != nil {
		t.Fatal(err)
	}
	if got, err := p.SSHPort(t.Context(), st, false); err != nil || got == sp || st.Get(KeySSHPort) != strconv.Itoa(got) {
		t.Errorf("forwarded: %d (had %d), %v", got, sp, err)
	}
}

// proxies is a fake start: each "proxy" is a sleep, which says it listens
// unless quiet.
type proxies struct {
	mu    sync.Mutex
	quiet bool
	// stubborn proxies ignore SIGTERM.
	stubborn bool
	args     map[int][]string
	n        int
}

func (f *proxies) start(t *testing.T) daemon.Starter {
	return func(c daemon.Cmd) (int, error) {
		path, args := c.Path, c.Args
		f.mu.Lock()
		defer f.mu.Unlock()
		cmd := exec.CommandContext(t.Context(), "sleep", "30")
		if f.stubborn {
			cmd = exec.CommandContext(t.Context(), "sh", "-c", `trap "" TERM; exec sleep 30`)
		}
		if f.quiet {
			cmd = exec.CommandContext(t.Context(), "true")
		}
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		pid := cmd.Process.Pid
		if f.quiet {
			_ = cmd.Wait()
			_, _ = fmt.Fprintf(c.Out, "devvm proxy: listen tcp %s: bind: address already in use\n", args[2])
		} else {
			// Reaped once it ends, as a detached process is.
			done := make(chan struct{})
			go func() { _ = cmd.Wait(); close(done) }()
			t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
			if err := daemon.WriteStatus(c.Status, pid, args[2]); err != nil {
				return 0, err
			}
		}
		f.args[pid] = append([]string{path}, args...)
		f.n++
		return pid, nil
	}
}

func (f *proxies) argsOf(pid int) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.args[pid]; ok {
		return a, nil
	}
	return nil, errors.New("not a proxy")
}

func (f *proxies) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

// testUI logs what the proxies and backends print, and refuses what they
// ask, without a terminal; it runs commands as they are, counts no steps
// and keeps no cleanups.
type testUI struct{ t *testing.T }

func (u testUI) Info(m msg.M)                                            { u.t.Log(m.Names()) }
func (u testUI) Note(m msg.M)                                            { u.t.Log(m.Names()) }
func (u testUI) Warn(m msg.M)                                            { u.t.Log(m.Names()) }
func (testUI) Ask(msg.M) bool                                            { return false }
func (testUI) Confirm(...msg.M) bool                                     { return false }
func (testUI) StdinIsTerminal() bool                                     { return false }
func (testUI) LogNote()                                                  {}
func (testUI) WithSteps(ctx context.Context, _ int) context.Context      { return ctx }
func (testUI) AddCleanup(func(context.Context)) func()                   { return func() {} }
func (testUI) Interrupted() bool                                         { return false }
func (testUI) LoggedCmd(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }

func testProxy(t *testing.T, d paths.Dirs) (*Proxy, *proxies) {
	t.Helper()
	home := d
	home.Home = t.TempDir()
	store := secrets.NewStore(home, testUI{t}.Warn)
	store.Keychain, store.Getenv = false, func(string) string { return "" }
	f := &proxies{args: map[int][]string{}}
	p := NewProxy(d, assets.FS, "/bin/sh", store, testUI{t})
	p.Start, p.Args, p.Tick = f.start(t), f.argsOf, time.Millisecond
	p.Getenv = func(string) string { return "" } // not the host's proxy settings
	p.Listening = func(context.Context, int) bool { return false }
	p.ExcludeFromBackup = func(context.Context, string) error { return nil }
	return p, f
}

// lifelined is a backend whose VMs' lifeline is /ha.pid.
func lifelined() *backendtest.Fake {
	f := backendtest.New("fake", nil)
	f.LifelineFile = "/ha.pid"
	return f
}

// A VM about to start: the CA, its ports, the confinement its backend
// gets, and its proxy, told the password in a file.
func TestPrepare(t *testing.T) {
	d, st := sandbox(t, "base", "node", "claude")
	p, f := testProxy(t, d)
	lima := backendtest.New("lima", nil)
	lima.LifelineFile = "/lima/vm/ha.pid"
	var excluded []string
	p.OS = "darwin"
	p.ExcludeFromBackup = func(_ context.Context, dir string) error { excluded = append(excluded, dir); return nil }
	// The run, the VM's run and logs, and CA folders are the user's alone
	// after, whatever mode they had.
	files := d.VM(st.Name())
	for _, dir := range []string{d.Run(), files.Run(), files.Logs(), d.CA()} {
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // the test's own folder
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // the test's own folder
			t.Fatal(err)
		}
	}
	conf, err := p.Prepare(t.Context(), lima, st, false)
	if err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	key := st.Get(KeyProxyKey)
	sp, _ := strconv.Atoi(st.Get(KeySSHPort))
	pp, _ := strconv.Atoi(st.Get(KeyProxyPort))
	if conf.Net != backend.NetProxy || conf.ProxyPassword != key || key == "" ||
		conf.ProxyPort != pp || pp == 0 || conf.SSHPort != sp || sp == 0 || sp == pp || conf.Env["NO_PROXY"] != "localhost,127.0.0.1,::1" {
		t.Errorf("confinement = %+v", conf)
	}
	for _, f := range []string{"ca.pem", "ca.key"} {
		if _, err := os.Stat(filepath.Join(d.CA(), f)); err != nil {
			t.Errorf("the CA: %v", err)
		}
	}
	if !slices.Equal(excluded, []string{d.CA()}) {
		t.Errorf("kept out of backups: %q", excluded)
	}
	pid, ok := p.PID(st.Name())
	if !ok {
		t.Fatal("no proxy runs")
	}
	args := strings.Join(f.args[pid], " ")
	// devvm's files stay in the binary: nothing is written out for the proxy.
	if _, err := os.Stat(filepath.Join(d.Data, "assets")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("devvm's files written out: %v", err)
	}
	for _, want := range []string{
		"/bin/sh _proxy -listen 127.0.0.1:" + strconv.Itoa(pp) + " -vm " + st.Name() +
			" -builtin-policy layers/base.policy,shared-tools/node.policy,shared-tools/claude.policy -policy " + d.Policy() + ",",
		" -log " + files.Logs() + "/network.jsonl -proxy-log " + files.Logs() + "/proxy.log -lifeline /lima/vm/ha.pid -ca " + d.CA() + "/ca.pem -cakey " + d.CA() + "/ca.key",
		" -secrets " + files.Run() + "/secrets.json -secret-cmd /bin/sh -auth-file " + files.Run() + "/proxy.auth",
		// Its links' list: in its run folder, devvm's alone.
		" -links " + files.Run() + "/links.json -link-sockets " + d.Run(),
	} {
		if !strings.Contains(args, want) {
			t.Errorf("the proxy's arguments %q lack %q", args, want)
		}
	}
	// The key and salt are in no process's arguments.
	if strings.Contains(args, key) || strings.Contains(args, st.Get(KeyProxySalt)) {
		t.Errorf("the proxy's arguments hold its key or salt: %q", args)
	}
	auth := files.Proxy(".auth")
	b, err := os.ReadFile(auth) //nolint:gosec // the test's own folder
	if err != nil || strings.TrimSpace(string(b)) != key {
		t.Errorf("auth file = %q, %v", b, err)
	}
	for path, mode := range map[string]os.FileMode{
		auth: 0o600, d.Run(): 0o700, files.Run(): 0o700, files.Logs(): 0o700, files.ProxyLog(): 0o600,
		files.Secrets(): 0o600, d.CA(): 0o700,
	} {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != mode {
			t.Errorf("%s: %v, want %v (%v)", path, fi.Mode().Perm(), mode, err)
		}
	}
	// Not on Linux.
	p.OS, excluded = "linux", nil
	p.Stop(st.Name())
	if _, err := p.Prepare(t.Context(), lima, st, false); err != nil || excluded != nil {
		t.Errorf("Linux: %v, excluded %q", err, excluded)
	}
	// Its ssh port is one of its own forwards now: a new one, as its
	// backend would refuse it.
	p.Stop(st.Name())
	if err := st.Set(keyPorts, st.Get(KeySSHPort)); err != nil {
		t.Fatal(err)
	}
	conf, err = p.Prepare(t.Context(), lima, st, false)
	if err != nil || conf.SSHPort == sp || st.Get(KeySSHPort) != strconv.Itoa(conf.SSHPort) {
		t.Errorf("forwarded ssh port: %d (had %d), %v", conf.SSHPort, sp, err)
	}
}

// The proxy has started only when it says it's listening: not when
// something else holds the port and it exits.
func TestPrepareWaitsForTheProxy(t *testing.T) {
	d, st := sandbox(t, "base")
	p, f := testProxy(t, d)
	f.quiet = true
	_, err := p.Prepare(t.Context(), lifelined(), st, false)
	if want := "proxy_start_failed log=" + d.VM(st.Name()).ProxyLog() + " vm=" + st.Name(); msg.NamesOf(err) != want {
		t.Errorf("Prepare = %v, want %s", msg.NamesOf(err), want)
	}
}

// A running VM keeps its proxy, unless the proxy is from another devvm
// binary, holds a CA since replaced, or its allowlist files changed. A
// stopped VM's always starts again.
func TestPrepareRestartsTheProxy(t *testing.T) {
	d, st := sandbox(t, "base", "node")
	p, f := testProxy(t, d)
	prepare := func(running bool) {
		t.Helper()
		if _, err := p.Prepare(t.Context(), lifelined(), st, running); err != nil {
			t.Fatal(msg.NamesOf(err))
		}
	}
	prepare(false)
	pp := st.Get(KeyProxyPort)
	prepare(true)
	if f.count() != 1 {
		t.Fatalf("a running VM's proxy started again: %d starts", f.count())
	}
	for i, change := range []func(){
		func() { _ = os.WriteFile(d.VM(st.Name()).Proxy(".bin"), []byte("old\n"), 0o600) },
		func() {
			_ = os.WriteFile(filepath.Join(d.CA(), "ca.pem"), []byte("rotated\n"), 0o600)
			p.InitCA = func(string) error { return nil }
		},
		// Its built-in policies change, then its files alone (a layer of
		// the user's).
		func() { _ = st.Set(state.KeyToolNames, "node claude") },
		func() { _ = st.Set(state.KeyImage, "pg") },
	} {
		change()
		prepare(true)
		if f.count() != i+2 {
			t.Errorf("change %d: %d starts", i, f.count())
		}
	}
	if st.Get(KeyProxyPort) != pp {
		t.Errorf("the port moved: %s, then %s", pp, st.Get(KeyProxyPort))
	}
	// What it started with: both lists, a line each.
	want := "layers/base.policy,shared-tools/node.policy,shared-tools/claude.policy\n" +
		filepath.Join(d.Layers(), "pg.policy") + "," + d.Policy() + "," + d.VM(st.Name()).Policy() + "," + d.VM(st.Name()).SecretsPolicy() + "\n"
	if b, _ := os.ReadFile(d.VM(st.Name()).Proxy(".policy")); string(b) != want {
		t.Errorf("proxy.policy = %q, want %q", b, want)
	}
	prepare(false)
	if f.count() != 6 {
		t.Errorf("a stopped VM's proxy didn't start again: %d starts", f.count())
	}
	// A running VM whose proxy is gone, with its port taken by another
	// program.
	p.Stop(st.Name())
	p.Listening = func(_ context.Context, port int) bool { return strconv.Itoa(port) == pp }
	if _, err := p.Prepare(t.Context(), lifelined(), st, true); msg.NamesOf(err) != "proxy_port_taken port="+pp+" vm="+st.Name() {
		t.Errorf("port taken: %v", msg.NamesOf(err))
	}
	// Stopped, it moves.
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); err != nil || st.Get(KeyProxyPort) == pp {
		t.Errorf("stopped with its port taken: %v, port %s", err, st.Get(KeyProxyPort))
	}
}

// The company proxy in devvm's environment goes to the VM's proxy in a
// file, the user's alone, never in its arguments; a change starts it
// again, and one it can't go through is refused.
func TestPrepareUpstream(t *testing.T) {
	d, st := sandbox(t, "base")
	p, f := testProxy(t, d)
	const corp = "http://corp:pa55@proxy.corp:8080" //nolint:gosec // a fake company proxy's fake password
	env := map[string]string{"https_proxy": corp, "NO_PROXY": ".corp.example"}
	p.Getenv = func(k string) string { return env[k] }
	prepare := func(running bool) error {
		t.Helper()
		_, err := p.Prepare(t.Context(), lifelined(), st, running)
		return err
	}
	if err := prepare(false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	file := d.VM(st.Name()).Proxy(".upstream")
	pid, _ := p.PID(st.Name())
	args := strings.Join(f.args[pid], " ")
	if !strings.Contains(args, " -upstream "+file) || strings.Contains(args, "pa55") {
		t.Errorf("the proxy's arguments %q: want -upstream %s, and no password", args, file)
	}
	b, err := os.ReadFile(file) //nolint:gosec // the test's own folder
	if want := "HTTPS_PROXY=" + corp + "\nHTTP_PROXY=\nNO_PROXY=.corp.example\n"; err != nil || string(b) != want {
		t.Errorf("upstream file = %q, %v; want %q", b, err, want)
	}
	if fi, err := os.Stat(file); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("upstream file mode: %v %v", fi, err)
	}
	// Running, the same settings keep it; others start it again.
	if err := prepare(true); err != nil || f.count() != 1 {
		t.Errorf("unchanged: %v, %d starts", msg.NamesOf(err), f.count())
	}
	env["HTTP_PROXY"] = "proxy.corp:3128"
	if err := prepare(true); err != nil || f.count() != 2 {
		t.Errorf("changed: %v, %d starts", msg.NamesOf(err), f.count())
	}
	// None: no file, no flag.
	env = map[string]string{}
	if err := prepare(true); err != nil || f.count() != 3 {
		t.Fatalf("none: %v, %d starts", msg.NamesOf(err), f.count())
	}
	pid, _ = p.PID(st.Name())
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) || strings.Contains(strings.Join(f.args[pid], " "), "-upstream ") {
		t.Errorf("without a company proxy: %v, %q", err, f.args[pid])
	}
	for _, c := range []struct{ env, value, want string }{
		{"HTTPS_PROXY", "https://corp:pa55@proxy.corp", "upstream_proxy_scheme scheme=https:// setting=HTTPS_PROXY"},
		{"http_proxy", "socks5://proxy.corp:1080", "upstream_proxy_scheme scheme=socks5:// setting=HTTP_PROXY"},
		{"HTTPS_PROXY", "http://proxy.corp:http", "upstream_proxy_invalid setting=HTTPS_PROXY"},
	} {
		env = map[string]string{c.env: c.value}
		if err := prepare(false); msg.NamesOf(err) != c.want {
			t.Errorf("%s=%s: %v, want %s", c.env, c.value, msg.NamesOf(err), c.want)
		}
	}
}

// A proxy that needs starting again is stopped, killed if SIGTERM doesn't
// end it; one that won't end fails the start, never kept with its old
// settings.
func TestPrepareStopsTheOldProxy(t *testing.T) {
	d, st := sandbox(t, "base")
	p, f := testProxy(t, d)
	p.Tick = 20 * time.Millisecond
	f.stubborn = true
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	old, _ := p.PID(st.Name())
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if pid, ok := p.PID(st.Name()); !ok || pid == old || daemon.Alive(old) {
		t.Errorf("the old proxy %d: alive %v; now %d", old, daemon.Alive(old), pid)
	}
	p.Signal = func(int, syscall.Signal) error { return nil } // signals that never arrive
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); msg.NamesOf(err) != "proxy_stop_failed vm="+st.Name() {
		t.Errorf("a proxy that won't stop: %v", msg.NamesOf(err))
	}
}

func TestPrepareCAFails(t *testing.T) {
	d, st := sandbox(t, "base")
	p, _ := testProxy(t, d)
	p.InitCA = func(string) error { return errors.New("disk full") }
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); msg.NamesOf(err) != "proxy_ca_failed dir="+d.CA() {
		t.Errorf("Prepare = %v", msg.NamesOf(err))
	}
}

// A launch with network policy warns of each line of the VM's policy files
// that isn't a rule, a broken deny as one.
func TestPrepareWarnsPolicy(t *testing.T) {
	d, st := sandbox(t, "base")
	p, _ := testProxy(t, d)
	var warned msg.Recorder
	p.Warn = warned.Add
	f := d.VM(st.Name()).Policy()
	if err := os.WriteFile(f, []byte("ok.test\n! bad.test\na b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	want := []string{"policy_broken_deny file=" + f + " line=2 rule=! bad.test", "policy_not_rule file=" + f + " line=3 rule=a b"}
	if !slices.Equal(warned.Names(), want) {
		t.Errorf("warned %q; want %q", warned.Names(), want)
	}
}

// Two devvm runs writing a VM's proxy files at once each write them whole:
// neither fails, and no temporary file is left.
func TestWriteUpstreamAtOnce(t *testing.T) {
	d, st := sandbox(t, "base")
	p, _ := testProxy(t, d)
	run := d.VM(st.Name()).Run()
	if err := os.MkdirAll(run, 0o700); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := range 4 {
		wg.Go(func() {
			for range 50 {
				errs <- p.writeUpstream(st.Name(), []byte("cas "+strconv.Itoa(i)), []byte("up "+strconv.Itoa(i)))
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("writeUpstream: %v", err)
		}
	}
	es, _ := os.ReadDir(run)
	var left []string
	for _, e := range es {
		left = append(left, e.Name())
	}
	if want := []string{"proxy.cas", "proxy.upstream"}; !slices.Equal(left, want) {
		t.Errorf("the run folder holds %q; want %q", left, want)
	}
	up := p.run(st.Name(), ".upstream")
	b, err := os.ReadFile(up) //nolint:gosec // the test's own folder
	if fi, serr := os.Stat(up); err != nil || serr != nil || fi.Mode().Perm() != 0o600 || !regexp.MustCompile(`\Aup [0-3]\z`).Match(b) {
		t.Errorf("upstream file %q (%v), mode %v (%v)", b, err, fi, serr)
	}
}

// A proxy file that can't be written fails the start and leaves no
// temporary file behind.
func TestPrepareWriteFails(t *testing.T) {
	d, st := sandbox(t, "base")
	p, _ := testProxy(t, d)
	// A folder where the password goes: renaming over it fails.
	auth := d.VM(st.Name()).Proxy(".auth")
	if err := os.MkdirAll(filepath.Join(auth, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	var le *os.LinkError
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); !errors.As(err, &le) || le.New != auth {
		t.Fatalf("Prepare: %v; want the password's rename failing", err)
	}
	es, _ := os.ReadDir(filepath.Dir(auth))
	for _, e := range es {
		if strings.Contains(e.Name(), "tmp") {
			t.Errorf("left in the run folder: %s", e.Name())
		}
	}
}

// PID names only a process running as the VM's proxy.
func TestPID(t *testing.T) {
	d, st := sandbox(t, "base")
	p, f := testProxy(t, d)
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); err != nil {
		t.Fatal(err)
	}
	pid, ok := p.PID(st.Name())
	if !ok {
		t.Fatal("no proxy")
	}
	f.args[pid] = []string{"/bin/sh", "_proxy", "-vm", "devvm-other-0123456789abc9"}
	if _, ok := p.PID(st.Name()); ok {
		t.Error("another VM's proxy counts")
	}
	f.args[pid] = []string{"/bin/sleep", "30"}
	if _, ok := p.PID(st.Name()); ok {
		t.Error("another program counts")
	}
	// Stopping it signals nothing: the pid is another program's now.
	var signalled []int
	p.Signal = func(pid int, _ syscall.Signal) error { signalled = append(signalled, pid); return nil }
	if !p.Stop(st.Name()) || len(signalled) != 0 {
		t.Errorf("Stop of another program's pid: signalled %v", signalled)
	}
	// Nor does a pid file that names no pid, or this test's own.
	for _, pidFile := range []string{"x\n", "0\n", "-1\n", strconv.Itoa(os.Getpid()) + "\n"} {
		if err := os.WriteFile(d.VM(st.Name()).Proxy(".pid"), []byte(pidFile), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := p.PID(st.Name()); ok || !p.Stop(st.Name()) || len(signalled) != 0 {
			t.Errorf("pid file %q: a proxy, or signalled %v", pidFile, signalled)
		}
	}
}

// Stop stops a VM's proxy; its files stay in its folder, which goes
// with the VM.
func TestStopKeepsFiles(t *testing.T) {
	d, st := sandbox(t, "base")
	p, _ := testProxy(t, d)
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); err != nil {
		t.Fatal(err)
	}
	pid, _ := p.PID(st.Name())
	if !p.Stop(st.Name()) || daemon.Alive(pid) {
		t.Error("the proxy still runs")
	}
	if _, err := os.Stat(d.VM(st.Name()).Proxy(".auth")); err != nil {
		t.Errorf("its files: %v", err)
	}
}

// RefreshSecrets writes the secret files again only of VMs whose proxy has
// them, so a token saved since gets its binding at once.
func TestRefreshSecrets(t *testing.T) {
	d, st := sandbox(t, "base", "node")
	p, _ := testProxy(t, d)
	ctx := t.Context()
	if err := st.Set(secrets.KeySecrets, "github"); err != nil {
		t.Fatal(err)
	}
	other, err := state.CreateVM(d, "devvm-other-0123456789abc9", state.NewVM{Image: "base", Tools: []string{"node"}, Dir: "/o", Backend: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Set(secrets.KeySecrets, "github"); err != nil {
		t.Fatal(err)
	}
	salt, _ := Salt(st)
	if err := p.Secrets.WriteFiles(ctx, st, salt); err != nil {
		t.Fatal(err)
	}
	if err := p.Secrets.Save(ctx, "github", "ghp_testfake"); err != nil {
		t.Fatal(err)
	}
	// A bindings file whose VM devvm no longer has is left alone.
	if err := state.WriteFile(d.VM("devvm-gone-0123456789abc8").Secrets(), []byte("[]\n")); err != nil {
		t.Fatal(err)
	}
	if err := p.RefreshSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(d.VM(st.Name()).Secrets())
	if !strings.Contains(string(data), `"devvm-proxy-managed-github-`+salt+`"`) || strings.Contains(string(data), "ghp_testfake") {
		t.Errorf("bindings = %s", data)
	}
	if _, err := os.Stat(d.VM(other.Name()).Secrets()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a VM without a proxy got files: %v", err)
	}
	if data, _ := os.ReadFile(d.VM("devvm-gone-0123456789abc8").Secrets()); string(data) != "[]\n" {
		t.Errorf("a gone VM's = %s", data)
	}
	if err := p.RefreshSecrets(ctx, other.Name()); err != nil {
		t.Fatal(err)
	}
}

// A running VM whose proxy couldn't start is stopped, and the reason
// returned; a stop that fails is returned instead.
func TestUnproxied(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	why := msg.ProxyStartFailed("/log", vmV)
	f := backendtest.New("fake", nil)
	f.Instances[vmV] = backend.Instance{Name: vmV, Status: backend.StatusRunning}
	var warned msg.Recorder
	p := &Proxy{Warn: warned.Add}
	if err := p.Unproxied(t.Context(), f, vmV, why); msg.NamesOf(err) != "proxy_start_failed log=/log vm="+vmV ||
		f.Instances[vmV].Status != backend.StatusStopped {
		t.Errorf("Unproxied = %v, VM %s", msg.NamesOf(err), f.Instances[vmV].Status)
	}
	if !slices.Equal(warned.Names(), []string{"net_proxy_down_stopped vm=" + vmV}) {
		t.Errorf("warned %q", warned.Names())
	}
	// Interrupted (its context cancelled), it still stops the VM.
	f.Instances[vmV] = backend.Instance{Name: vmV, Status: backend.StatusRunning}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cs := ctxStopper{Fake: f}
	if err := p.Unproxied(ctx, &cs, vmV, why); msg.NamesOf(err) != "proxy_start_failed log=/log vm="+vmV || cs.err != nil ||
		f.Instances[vmV].Status != backend.StatusStopped {
		t.Errorf("interrupted: %v, stop's context %v, VM %s", msg.NamesOf(err), cs.err, f.Instances[vmV].Status)
	}
	stopErr := errors.New("stop failed")
	f.Errs = map[string]error{"Stop": stopErr}
	warned.Reset()
	if err := p.Unproxied(t.Context(), f, vmV, why); !errors.Is(err, stopErr) ||
		!slices.Equal(warned.Names(), []string{"proxy_start_failed log=/log vm=" + vmV}) {
		t.Errorf("a failed stop: %v, warned %q", err, warned.Names())
	}
}

// ctxStopper is a fake backend that notes whether Stop's context was done.
type ctxStopper struct {
	*backendtest.Fake
	err error
}

func (c *ctxStopper) Stop(ctx context.Context, vm string) error {
	c.err = ctx.Err()
	return c.Fake.Stop(ctx, vm)
}

// After a start: enforced is ok; a direct route, or a VM started outside
// devvm without a terminal (or still after a restart), stops the VM.
func TestSelfCheck(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, tc := range []struct {
		name                         string
		unconfined, direct, tty, yes bool
		fixed                        bool // the restart confines it
		want, check                  string
		stops, restarts              bool
	}{
		{name: "enforced", check: "ok"},
		{name: "a direct route", direct: true, want: "net_direct_route vm=" + vmV, check: "unenforced", stops: true},
		{name: "unconfined, no terminal", unconfined: true, want: "net_unenforced_stop vm=" + vmV, check: "unenforced", stops: true},
		{name: "unconfined, restarted", unconfined: true, tty: true, yes: true, fixed: true, check: "ok", stops: true, restarts: true},
		{name: "unconfined, restarted, still", unconfined: true, tty: true, yes: true, want: "net_unenforced_stop vm=" + vmV, check: "unenforced", stops: true, restarts: true},
		{name: "unconfined, declined", unconfined: true, tty: true, check: "unenforced"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := paths.Dirs{Data: t.TempDir()}
			st, err := state.CreateVM(d, vmV, state.NewVM{Image: "base"})
			if err != nil {
				t.Fatal(err)
			}
			_ = st.Set(KeyNet, "on")
			f := backendtest.New("fake", nil)
			f.Instances[vmV] = backend.Instance{Name: vmV, Status: backend.StatusRunning}
			f.Unconfined = tc.unconfined
			f.ExecFunc = func(_ string, argv []string, _ backend.ExecOpts) error {
				if tc.direct && slices.Equal(argv, directArgv) {
					return nil
				}
				return errors.New("curl: (7) failed")
			}
			p := &Proxy{Warn: func(msg.M) {}, Confirm: func(msg.M) bool { return tc.yes }, Terminal: func() bool { return tc.tty }}
			restarted := false
			err = p.SelfCheck(t.Context(), f, st, func(context.Context) error {
				restarted = true
				f.Unconfined = !tc.fixed
				return f.Start(t.Context(), vmV, backend.Confinement{Net: backend.NetProxy})
			})
			if msg.NamesOf(err) != tc.want {
				t.Errorf("SelfCheck = %v, want %q", msg.NamesOf(err), tc.want)
			}
			if st.Get(KeyCheck) != tc.check {
				t.Errorf("net_check = %q", st.Get(KeyCheck))
			}
			if got := slices.Contains(f.Methods(), "Stop"); got != tc.stops || restarted != tc.restarts {
				t.Errorf("stopped %v, restarted %v: %q", got, restarted, f.Methods())
			}
			if tc.want != "" && f.Instances[vmV].Status != backend.StatusStopped {
				t.Error("the VM it doesn't hold is left running")
			}
		})
	}
	// Without network policy, nothing is checked.
	d := paths.Dirs{Data: t.TempDir()}
	st, _ := state.CreateVM(d, vmV, state.NewVM{Image: "base"})
	f := backendtest.New("fake", nil)
	if err := (&Proxy{}).SelfCheck(t.Context(), f, st, nil); err != nil || len(f.Calls()) > 0 {
		t.Errorf("no policy: %v, %q", err, f.Methods())
	}
}

// What ls shows: running VMs are checked on the host; a failed self-check
// stands either way.
func TestListed(t *testing.T) {
	for _, tc := range []struct {
		name, net, check    string
		running, unconfined bool
		want                string
		asks                bool
	}{
		{name: "unfinished", want: ""},
		{name: "off", net: "off", running: true, want: "off"},
		{name: "on, stopped", net: "on", want: "on"},
		{name: "on, stopped, unconfined", net: "on", unconfined: true, want: "on"},
		{name: "on, running", net: "on", running: true, want: "on", asks: true},
		{name: "on, running elsewhere", net: "on", running: true, unconfined: true, want: "unenforced", asks: true},
		{name: "failed self-check", net: "on", check: "unenforced", running: true, want: "unenforced"},
		{name: "failed self-check, stopped", net: "on", check: "unenforced", want: "unenforced"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := paths.Dirs{Data: t.TempDir()}
			st, _ := state.CreateVM(d, vmV, state.NewVM{Image: "base"})
			for k, v := range map[string]string{KeyNet: tc.net, KeyCheck: tc.check} {
				if v != "" {
					_ = st.Set(k, v)
				}
			}
			f := backendtest.New("fake", nil)
			f.Unconfined = tc.unconfined
			got, err := Listed(t.Context(), f, st, tc.running)
			if err != nil || got != tc.want {
				t.Errorf("Listed = %q, %v; want %q", got, err, tc.want)
			}
			if asked := slices.Contains(f.Methods(), "Confined"); asked != tc.asks {
				t.Errorf("asked the backend: %v", asked)
			}
		})
	}
}

// Only a VM with the nix layer gets the shared Nix cache, with the cap
// nix-cache.size gives, or its default.
func TestNixCacheArgs(t *testing.T) {
	if settings.NixCacheDefault != proxy.NixCacheDefault {
		t.Errorf("the cap's default: settings' %d, the proxy's %d", settings.NixCacheDefault, proxy.NixCacheDefault)
	}
	d, st := sandbox(t, "base")
	var warned msg.Recorder
	p := &Proxy{Dirs: d, Warn: warned.Add}
	if got := p.nixCacheArgs(st); got != nil {
		t.Errorf("base: %q", got)
	}
	if err := st.Set(state.KeyImage, "pg+nix"); err != nil {
		t.Fatal(err)
	}
	want := []string{"-nix-cache", d.NixCache(), "-nix-cache-max", strconv.FormatInt(settings.NixCacheDefault, 10)}
	if got := p.nixCacheArgs(st); !slices.Equal(got, want) {
		t.Errorf("node+nix: %q, want %q", got, want)
	}
	if _, err := settings.Save(d, settings.Values{Retention: map[string]string{settings.RetainNixCacheSize: "2G"}}); err != nil {
		t.Fatal(err)
	}
	if got := p.nixCacheArgs(st); got[3] != strconv.FormatInt(2<<30, 10) {
		t.Errorf("nix-cache.size=2G: %q", got)
	}
	if err := os.MkdirAll(d.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.ConfigFile(), []byte("[retention]\nnix-cache.size = \"lots\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := p.nixCacheArgs(st); got[3] != want[3] || len(warned.Names()) != 1 || !strings.HasPrefix(warned.Names()[0], "retention_unreadable") {
		t.Errorf("unreadable: %q, warned %q", got, warned.Names())
	}
}
