package netpolicy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/state"
)

func names(err error) string {
	if m, ok := err.(msg.M); ok { //nolint:errorlint // messages aren't wrapped
		return m.Names()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

var repo = func() string {
	r, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	return r
}()

// sandbox returns devvm's folders in a temporary folder, and a VM's state
// there with that image.
func sandbox(t *testing.T, img string) (paths.Dirs, *state.VM) {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root) // the run log
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config"), Lima: filepath.Join(root, "lima")}
	st, err := state.CreateVM(d, "devvm-web-0123456789abcd", state.NewVM{Image: img, Dir: "/w", Backend: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	return d, st
}

func TestPolicyFiles(t *testing.T) {
	d := paths.Dirs{Data: "/d", Config: "/c", Lima: "/l"}
	got := PolicyFiles(d, "/r", "vm1", "node+claude")
	want := []string{
		"/r/layers/base.policy", "/r/tools/node.policy", "/r/tools/claude.policy",
		"/c/policy", "/d/vms/vm1.policy", "/d/run/vm1.secrets.policy",
	}
	if !slices.Equal(got, want) {
		t.Errorf("PolicyFiles = %q, want %q", got, want)
	}
	// A custom layer's allowlist is the user's, next to its script.
	got = PolicyFiles(d, "/r", "vm1", "node+pg")
	if got[2] != "/c/layers/pg.policy" {
		t.Errorf("a custom layer's policy = %q", got[2])
	}
	if got := PolicyFiles(d, "/r", "vm1", "base"); len(got) != 4 {
		t.Errorf("base's PolicyFiles = %q", got)
	}
}

// Every image part devvm has comes with its allowlist.
func TestEveryPartHasAPolicy(t *testing.T) {
	for _, p := range append([]string{"base"}, append(image.Toolchains, image.Agents...)...) {
		d := paths.Dirs{Data: "/d", Config: "/c"}
		f := PolicyFiles(d, repo, "vm1", p)
		if p != "base" {
			f = f[1:]
		}
		if _, err := os.Stat(f[0]); err != nil {
			t.Errorf("%s has no policy: %v", p, err)
		}
	}
}

// Each VM has its own random key (its proxy's password) and salt (the end
// of its placeholders), kept.
func TestKeyAndSalt(t *testing.T) {
	d, a := sandbox(t, "base")
	b, err := state.CreateVM(d, "devvm-api-0123456789abcd", state.NewVM{Image: "base", Dir: "/a", Backend: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	hex32 := regexp.MustCompile(`^[0-9a-f]{32}$`)
	ka, _ := Key(a)
	kb, _ := Key(b)
	sa, _ := Salt(a)
	sb, _ := Salt(b)
	for _, v := range []string{ka, kb, sa, sb} {
		if !hex32.MatchString(v) {
			t.Errorf("%q isn't 32 hex digits", v)
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

// The flag wins; without one and without a terminal, off; in a terminal,
// the answer.
func TestChoose(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	swapQuestions(t)
	tty, answer, asked := false, false, 0
	terminal = func() bool { return tty }
	ask = func(msg.M) bool { asked++; return answer }
	if !Choose(true, false, "vm") || Choose(false, true, "vm") || Choose(false, false, "vm") || asked != 0 {
		t.Error("without a terminal: not the flags, or off")
	}
	tty = true
	if Choose(false, false, "vm") || asked != 1 {
		t.Error("no answered yes")
	}
	answer = true
	if !Choose(false, false, "vm") || !Choose(true, false, "vm") || asked != 2 {
		t.Error("yes answered no, or a flag asked")
	}
}

// swapQuestions puts the terminal and the questions back after the test.
func swapQuestions(t *testing.T) {
	t.Helper()
	tm, a, c := terminal, ask, confirm
	t.Cleanup(func() { terminal, ask, confirm = tm, a, c })
}

// usedPorts lists the ports VMs have; freePort avoids them.
func TestFreePort(t *testing.T) {
	d, st := sandbox(t, "base")
	if err := st.Set(KeySSHPort, "23456"); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(KeyProxyPort, "34567"); err != nil {
		t.Fatal(err)
	}
	used, err := usedPorts(d)
	if err != nil || !used[23456] || !used[34567] || len(used) != 2 {
		t.Errorf("usedPorts = %v, %v", used, err)
	}
	// And the ports VMs forward, wanted or set: all and none name none.
	for k, v := range map[string]string{keyPorts: "5173,40000-40002", keyPortsSet: "41000, bad,9-1"} {
		if err := st.Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	used, err = usedPorts(d)
	if err != nil || len(used) != 7 || !used[5173] || !used[40000] || !used[40002] || !used[41000] {
		t.Errorf("usedPorts with forwards = %v, %v", used, err)
	}
	if err := st.Set(keyPorts, "all"); err != nil {
		t.Fatal(err)
	}
	if got := forwarded(st); len(got) != 1 || !got[41000] {
		t.Errorf("forwarded with all = %v", got)
	}
	p := &Proxy{Dirs: d, Listening: func(port int) bool { return port%2 == 0 }}
	for range 200 {
		port, err := p.freePort()
		if err != nil || port < 20000 || port >= 60000 || port%2 == 0 || used[port] {
			t.Fatalf("freePort = %d, %v", port, err)
		}
	}
}

// proxies is a fake start: each "proxy" is a sleep, which says it listens
// unless quiet.
type proxies struct {
	mu    sync.Mutex
	quiet bool
	args  map[int][]string
	n     int
}

func (f *proxies) start(t *testing.T) func(string, []string, *os.File) (int, error) {
	return func(path string, args []string, out *os.File) (int, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		cmd := exec.CommandContext(context.Background(), "sleep", "30")
		if f.quiet {
			cmd = exec.CommandContext(context.Background(), "true")
		}
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		pid := cmd.Process.Pid
		if f.quiet {
			_ = cmd.Wait()
			_, _ = fmt.Fprintf(out, "devvm proxy: listen tcp %s: bind: address already in use\n", args[2])
		} else {
			// Reaped once it ends, as a detached process is.
			done := make(chan struct{})
			go func() { _ = cmd.Wait(); close(done) }()
			t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
			_, _ = fmt.Fprintf(out, "devvm proxy: listening on %s for %s\n", args[2], args[4])
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

func testProxy(t *testing.T, d paths.Dirs) (*Proxy, *proxies) {
	t.Helper()
	store := secrets.NewStore(d, t.TempDir())
	store.Keychain, store.Getenv = false, func(string) string { return "" }
	f := &proxies{args: map[int][]string{}}
	p := NewProxy(d, repo, "/bin/sh", store)
	p.Start, p.Args, p.Tick = f.start(t), f.argsOf, time.Millisecond
	p.Listening = func(int) bool { return false }
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
	d, st := sandbox(t, "node+claude")
	p, f := testProxy(t, d)
	lima := backendtest.New("lima", nil)
	lima.LifelineFile = "/lima/vm/ha.pid"
	var excluded []string
	p.OS = "darwin"
	p.ExcludeFromBackup = func(_ context.Context, dir string) error { excluded = append(excluded, dir); return nil }
	conf, err := p.Prepare(t.Context(), lima, st, false)
	if err != nil {
		t.Fatal(names(err))
	}
	key := st.Get(KeyProxyKey)
	sp, _ := strconv.Atoi(st.Get(KeySSHPort))
	pp, _ := strconv.Atoi(st.Get(KeyProxyPort))
	if conf.Net != backend.NetProxy || conf.ProxyUser != "devvm" || conf.ProxyPassword != key || key == "" ||
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
	for _, want := range []string{
		"/bin/sh _proxy -listen 127.0.0.1:" + strconv.Itoa(pp) + " -vm " + st.Name() + " -policy " + repo + "/layers/base.policy,",
		" -log " + d.Logs() + "/" + st.Name() + ".jsonl -lifeline /lima/vm/ha.pid -ca " + d.CA() + "/ca.pem -cakey " + d.CA() + "/ca.key",
		" -secrets " + d.Run() + "/" + st.Name() + ".secrets.json -secret-cmd /bin/sh -auth-file " + d.Run() + "/" + st.Name() + ".proxy.auth",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("the proxy's arguments %q lack %q", args, want)
		}
	}
	// The key and salt are in no process's arguments.
	if strings.Contains(args, key) || strings.Contains(args, st.Get(KeyProxySalt)) {
		t.Errorf("the proxy's arguments hold its key or salt: %q", args)
	}
	// A backend without a decider gets none.
	if conf.Decider != "" || conf.DeciderToken != "" || strings.Contains(args, "-decider") || st.Get(KeyDeciderPort) != "" {
		t.Errorf("a decider without a backend that uses one: %+v, %q", conf, args)
	}
	auth := filepath.Join(d.Run(), st.Name()+".proxy.auth")
	b, err := os.ReadFile(auth) //nolint:gosec // the test's own folder
	if err != nil || strings.TrimSpace(string(b)) != key {
		t.Errorf("auth file = %q, %v", b, err)
	}
	for path, mode := range map[string]os.FileMode{
		auth: 0o600, d.Run(): 0o700, d.Logs(): 0o700, ProxyLog(d, st.Name()): 0o600,
		secrets.BindingsFile(d, st.Name()): 0o600, d.CA(): 0o700,
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
	if want := "proxy_start_failed log=" + ProxyLog(d, st.Name()) + " vm=" + st.Name(); names(err) != want {
		t.Errorf("Prepare = %v, want %s", names(err), want)
	}
}

// On a backend with a decider, the proxy decides too: on a port of its
// own, taking over the proxy's address in the VM, with a token in a file
// (never in its arguments) that's new at each start and kept while the VM
// runs.
func TestPrepareDecider(t *testing.T) {
	d, st := sandbox(t, "base")
	p, f := testProxy(t, d)
	b := lifelined()
	b.Decides = true
	conf, err := p.Prepare(t.Context(), b, st, false)
	if err != nil {
		t.Fatal(names(err))
	}
	dp := st.Get(KeyDeciderPort)
	tok := st.Get(KeyDeciderToken)
	if dp == "" || dp == st.Get(KeyProxyPort) || conf.Decider != "127.0.0.1:"+dp ||
		!regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(tok) || conf.DeciderToken != tok {
		t.Fatalf("confinement = %+v, state port %s token %s", conf, dp, tok)
	}
	pid, _ := p.PID(st.Name())
	args := strings.Join(f.args[pid], " ")
	file := filepath.Join(d.Run(), st.Name()+".proxy.decider")
	want := " -decider 127.0.0.1:" + dp + " -decider-to proxy.test:" + st.Get(KeyProxyPort) + " -decider-token-file " + file
	if !strings.Contains(args, want) || strings.Contains(args, tok) {
		t.Errorf("the proxy's arguments %q: want %q, and not the token", args, want)
	}
	if b, err := os.ReadFile(file); err != nil || strings.TrimSpace(string(b)) != tok { //nolint:gosec // the test's own folder
		t.Errorf("token file = %q, %v", b, err)
	} else if fi, _ := os.Stat(file); fi.Mode().Perm() != 0o600 {
		t.Errorf("token file mode %v", fi.Mode().Perm())
	}
	// Running, it keeps its token (its backend holds it); started again,
	// it gets a new one, on the same port.
	if conf, err = p.Prepare(t.Context(), b, st, true); err != nil || conf.DeciderToken != tok {
		t.Errorf("running: %v, token %s, want %s", err, conf.DeciderToken, tok)
	}
	if conf, err = p.Prepare(t.Context(), b, st, false); err != nil || conf.DeciderToken == tok || conf.Decider != "127.0.0.1:"+dp {
		t.Errorf("started again: %v, %+v", err, conf)
	}
	// A running VM whose proxy is gone, with the decider's port taken.
	p.Stop(st.Name())
	p.Listening = func(port int) bool { return strconv.Itoa(port) == dp }
	if _, err := p.Prepare(t.Context(), b, st, true); names(err) != "proxy_port_taken port="+dp+" vm="+st.Name() {
		t.Errorf("decider port taken: %v", names(err))
	}
}

// A running VM keeps its proxy, unless the proxy is from another devvm
// binary, holds a CA since replaced, or its allowlist files changed. A
// stopped VM's always starts again.
func TestPrepareRestartsTheProxy(t *testing.T) {
	d, st := sandbox(t, "node")
	p, f := testProxy(t, d)
	prepare := func(running bool) {
		t.Helper()
		if _, err := p.Prepare(t.Context(), lifelined(), st, running); err != nil {
			t.Fatal(names(err))
		}
	}
	prepare(false)
	pp := st.Get(KeyProxyPort)
	prepare(true)
	if f.count() != 1 {
		t.Fatalf("a running VM's proxy started again: %d starts", f.count())
	}
	for i, change := range []func(){
		func() { _ = os.WriteFile(filepath.Join(d.Run(), st.Name()+".proxy.bin"), []byte("old\n"), 0o600) },
		func() {
			_ = os.WriteFile(filepath.Join(d.CA(), "ca.pem"), []byte("rotated\n"), 0o600)
			p.InitCA = func(string) error { return nil }
		},
		func() { _ = st.Set(state.KeyImage, "node+claude") },
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
	prepare(false)
	if f.count() != 5 {
		t.Errorf("a stopped VM's proxy didn't start again: %d starts", f.count())
	}
	// A running VM whose proxy is gone, with its port taken by another
	// program.
	p.Stop(st.Name())
	p.Listening = func(port int) bool { return strconv.Itoa(port) == pp }
	if _, err := p.Prepare(t.Context(), lifelined(), st, true); names(err) != "proxy_port_taken port="+pp+" vm="+st.Name() {
		t.Errorf("port taken: %v", names(err))
	}
	// Stopped, it moves.
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); err != nil || st.Get(KeyProxyPort) == pp {
		t.Errorf("stopped with its port taken: %v, port %s", err, st.Get(KeyProxyPort))
	}
}

func TestPrepareCAFails(t *testing.T) {
	d, st := sandbox(t, "base")
	p, _ := testProxy(t, d)
	p.InitCA = func(string) error { return errors.New("disk full") }
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); names(err) != "proxy_ca_failed dir="+d.CA() {
		t.Errorf("Prepare = %v", names(err))
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
	f.args[pid] = []string{"/bin/sh", "_proxy", "-vm", "devvm-other-0123456789abcd"}
	if _, ok := p.PID(st.Name()); ok {
		t.Error("another VM's proxy counts")
	}
	f.args[pid] = []string{"/bin/sleep", "30"}
	if _, ok := p.PID(st.Name()); ok {
		t.Error("another program counts")
	}
}

// Forget stops a VM's proxy and removes what it left; another VM's stay.
func TestForget(t *testing.T) {
	d, st := sandbox(t, "base")
	p, _ := testProxy(t, d)
	if _, err := p.Prepare(t.Context(), lifelined(), st, false); err != nil {
		t.Fatal(err)
	}
	pid, _ := p.PID(st.Name())
	other := filepath.Join(d.Run(), "devvm-other-0123456789abcd.sb")
	for _, f := range []string{VMPolicyFile(d, st.Name()), other} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.Forget(st.Name()); err != nil {
		t.Fatal(err)
	}
	left, _ := filepath.Glob(filepath.Join(d.Run(), st.Name()+".*"))
	if _, err := os.Stat(VMPolicyFile(d, st.Name())); !errors.Is(err, os.ErrNotExist) || len(left) > 0 {
		t.Errorf("left: %q, policy %v", left, err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("another VM's file: %v", err)
	}
	if alive(pid) {
		t.Error("the proxy still runs")
	}
}

// RefreshSecrets writes the secret files again only of VMs whose proxy has
// them, so a token saved since gets its binding at once.
func TestRefreshSecrets(t *testing.T) {
	d, st := sandbox(t, "node")
	p, _ := testProxy(t, d)
	ctx := t.Context()
	if err := st.Set(secrets.KeySecrets, "github"); err != nil {
		t.Fatal(err)
	}
	other, err := state.CreateVM(d, "devvm-other-0123456789abcd", state.NewVM{Image: "node", Dir: "/o", Backend: "fake"})
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
	if err := os.WriteFile(secrets.BindingsFile(d, "devvm-gone-0123456789abcd"), []byte("[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.RefreshSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(secrets.BindingsFile(d, st.Name()))
	if !strings.Contains(string(data), `"devvm-proxy-managed-github-`+salt+`"`) || strings.Contains(string(data), "ghp_testfake") {
		t.Errorf("bindings = %s", data)
	}
	if _, err := os.Stat(secrets.BindingsFile(d, other.Name())); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a VM without a proxy got files: %v", err)
	}
	if data, _ := os.ReadFile(secrets.BindingsFile(d, "devvm-gone-0123456789abcd")); string(data) != "[]\n" {
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
	why := msg.ProxyStartFailed("/log", "V")
	f := backendtest.New("fake", nil)
	f.Instances["V"] = backend.Instance{Name: "V", Status: backend.StatusRunning}
	if err := Unproxied(t.Context(), f, "V", why); names(err) != "proxy_start_failed log=/log vm=V" ||
		f.Instances["V"].Status != backend.StatusStopped {
		t.Errorf("Unproxied = %v, VM %s", names(err), f.Instances["V"].Status)
	}
	// Interrupted (its context cancelled), it still stops the VM.
	f.Instances["V"] = backend.Instance{Name: "V", Status: backend.StatusRunning}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cs := ctxStopper{Fake: f}
	if err := Unproxied(ctx, &cs, "V", why); names(err) != "proxy_start_failed log=/log vm=V" || cs.err != nil ||
		f.Instances["V"].Status != backend.StatusStopped {
		t.Errorf("interrupted: %v, stop's context %v, VM %s", names(err), cs.err, f.Instances["V"].Status)
	}
	stopErr := errors.New("stop failed")
	f.Errs = map[string]error{"Stop": stopErr}
	if err := Unproxied(t.Context(), f, "V", why); !errors.Is(err, stopErr) {
		t.Errorf("a failed stop: %v", err)
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
	swapQuestions(t)
	for _, tc := range []struct {
		name                         string
		unconfined, direct, tty, yes bool
		fixed                        bool // the restart confines it
		want, check                  string
		stops, restarts              bool
	}{
		{name: "enforced", check: "ok"},
		{name: "a direct route", direct: true, want: "net_direct_route vm=V", check: "unenforced", stops: true},
		{name: "unconfined, no terminal", unconfined: true, want: "net_unenforced_stop vm=V", check: "unenforced", stops: true},
		{name: "unconfined, restarted", unconfined: true, tty: true, yes: true, fixed: true, check: "ok", stops: true, restarts: true},
		{name: "unconfined, restarted, still", unconfined: true, tty: true, yes: true, want: "net_unenforced_stop vm=V", check: "unenforced", stops: true, restarts: true},
		{name: "unconfined, declined", unconfined: true, tty: true, check: "unenforced"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := paths.Dirs{Data: t.TempDir()}
			st, err := state.CreateVM(d, "V", state.NewVM{Image: "base"})
			if err != nil {
				t.Fatal(err)
			}
			_ = st.Set(KeyNet, "on")
			f := backendtest.New("fake", nil)
			f.Instances["V"] = backend.Instance{Name: "V", Status: backend.StatusRunning}
			f.Unconfined = tc.unconfined
			f.ExecFunc = func(_ string, argv []string, _ backend.ExecOpts) error {
				if tc.direct && slices.Equal(argv, directArgv) {
					return nil
				}
				return errors.New("curl: (7) failed")
			}
			terminal = func() bool { return tc.tty }
			confirm = func(msg.M) bool { return tc.yes }
			restarted := false
			err = SelfCheck(t.Context(), f, st, func(context.Context) error {
				restarted = true
				f.Unconfined = !tc.fixed
				return f.Start(t.Context(), "V", backend.Confinement{Net: backend.NetProxy})
			})
			if names(err) != tc.want {
				t.Errorf("SelfCheck = %v, want %q", names(err), tc.want)
			}
			if st.Get(KeyCheck) != tc.check {
				t.Errorf("net_check = %q", st.Get(KeyCheck))
			}
			if got := slices.Contains(f.Methods(), "Stop"); got != tc.stops || restarted != tc.restarts {
				t.Errorf("stopped %v, restarted %v: %q", got, restarted, f.Methods())
			}
			if tc.want != "" && f.Instances["V"].Status != backend.StatusStopped {
				t.Error("the VM it doesn't hold is left running")
			}
		})
	}
	// Without network policy, nothing is checked.
	d := paths.Dirs{Data: t.TempDir()}
	st, _ := state.CreateVM(d, "V", state.NewVM{Image: "base"})
	f := backendtest.New("fake", nil)
	if err := SelfCheck(t.Context(), f, st, nil); err != nil || len(f.Calls()) > 0 {
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
			st, _ := state.CreateVM(d, "V", state.NewVM{Image: "base"})
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
