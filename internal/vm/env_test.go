package vm

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// testUI is devvm's UI for these tests: it prints nothing, fails the test
// on a question, runs commands as they are, their output dropped, counts
// no steps and keeps no cleanups.
type testUI struct{ t *testing.T }

func (testUI) Info(msg.M)                {}
func (testUI) Warn(msg.M)                {}
func (testUI) Note(msg.M)                {}
func (testUI) Say(msg.M)                 {}
func (u testUI) Ask(m msg.M) bool        { u.t.Errorf("asked %s", m.Names()); return false }
func (u testUI) Confirm(q ...msg.M) bool { u.t.Errorf("asked %v", q); return false }
func (testUI) StdinIsTerminal() bool     { return false }
func (testUI) LogNote()                  {}
func (testUI) LogWriter() io.Writer      { return io.Discard }
func (testUI) Status(msg.M) func()       { return func() {} }
func (testUI) Interrupted() bool         { return false }
func (testUI) Stdout() io.Writer         { return io.Discard }
func (testUI) Stderr() io.Writer         { return io.Discard }

func (testUI) LoggedCmd(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }

func (testUI) AddCleanup(func(context.Context)) func() { return func() {} }

// fakeDefault is a test registry's default backend: the fake, which
// config.toml or settings.json would name.
func fakeDefault() (name, file string, err error) { return "fake", "", nil }

// upEnv is up on a fake backend, in a temporary sandbox, with a project
// folder named web.
func upEnv(t *testing.T, caps backend.Caps) (*Env, *backendtest.Fake, *[]backend.Command) {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// The shared tools' versions come from the stub mise: the fixture lock.
	t.Setenv("MISE_STUB_LOCK", filepath.Join(repo, "testdata", "mise.lock"))
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config"), Lima: filepath.Join(root, "lima")}
	tl := tools.New(d, assets.FS, testUI{t}, platform.Host{})
	tl.FreeSpace = func(string) (uint64, error) { return 1 << 40, nil }
	tl.Mise = filepath.Join(repo, "testdata", "stubs", "mise")
	dir := filepath.Join(root, "web")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f := backendtest.New("fake", caps)
	f.ShellCmd = backend.Command{Path: "/bin/shell", Args: []string{"shell", dir}}
	reg, err := backend.NewRegistry(f)
	if err != nil {
		t.Fatal(err)
	}
	reg.DefaultBackend = fakeDefault
	var execs []backend.Command
	d.Home = filepath.Join(root, "home")
	e := &Env{
		Dirs: d, Assets: assets.FS, Registry: reg, Tools: tl, Proxy: testProxy(t, d), Dir: dir, OS: "darwin",
		Host: func() (Host, error) { return Host{CPUs: 8, MemoryGiB: 32}, nil },
		Protector: func(dir string) *gitprotect.Protector {
			p := gitprotect.New(dir, d.Home, quiet)
			p.OS = "darwin" // as the Env's
			p.Chflags = func(context.Context, string, string) error { return nil }
			return p
		},
		Exec:   func(c backend.Command) error { execs = append(execs, c); return nil },
		Getenv: func(string) string { return "" },
		HostTZ: func() string { return "Europe/Paris" },
		GitConfig: func(_ context.Context, _, key string) string {
			return map[string]string{"user.name": "A User", "user.email": "a@example.com"}[key]
		},
		// The user's own agent: one that holds keys.
		SSHAdd: func(context.Context, ...string) int { return 0 },
		Info:   quiet, Warn: quiet, Note: quiet, Say: quiet,
		Ask: unasked(t), Confirm: unasked(t),
		PickFeatures: func(fs []build.Feature) []build.Feature { return fs },
		Terminal:     func() bool { return false }, Interactive: func() bool { return false }, CanPick: func() bool { return false },
		Answer: func(m msg.M) string { t.Errorf("asked %s", m.Names()); return "" },
		Stdout: io.Discard, LogNote: func() {}, LogWriter: testUI{t}.LogWriter, LoggedCmd: testUI{t}.LoggedCmd,
		AddCleanup: testUI{t}.AddCleanup, Interrupted: testUI{t}.Interrupted,
		PortInUse: func(context.Context, int) bool { return false },
	}
	// devvm sets SSH_AUTH_SOCK as a VM's agent mode says; the test's own
	// is put back.
	t.Setenv("SSH_AUTH_SOCK", "/user/agent")
	e.Environ = []string{"SSH_AUTH_SOCK=/user/agent"}
	e.Agent = sshagent.New(d, e.mounts, testUI{t})
	e.Agent.Run = (&agentCalls{}).run
	return e, f, &execs
}

// quiet prints nothing.
func quiet(msg.M) {}

// unasked fails the test on a question it doesn't answer itself
// (answers).
func unasked(t *testing.T) func(msg.M) bool {
	return func(m msg.M) bool {
		t.Errorf("asked %s", m.Names())
		return false
	}
}

// agentCalls stands in for devvm's own agent, which runs and holds a key.
type agentCalls struct{ calls []string }

func (a *agentCalls) run(_ context.Context, _ bool, argv ...string) (string, int) {
	a.calls = append(a.calls, strings.Join(argv, " "))
	if len(argv) == 2 && argv[0] == "ssh-add" && argv[1] == "-l" {
		return "256 SHA256:fp k (ED25519)\n", 0
	}
	return "", 0
}

// testProxy runs each VM's "proxy" as a sleep that says it listens, with
// no Keychain and no backups.
func testProxy(t *testing.T, d paths.Dirs) *netpolicy.Proxy {
	t.Helper()
	store := secrets.NewStore(d, quiet)
	store.Keychain, store.Getenv = false, func(string) string { return "" }
	p := netpolicy.NewProxy(d, assets.FS, "/devvm", store, testUI{t})
	p.Terminal = func() bool { return false }
	var mu sync.Mutex
	args := map[int][]string{}
	p.Start = func(c daemon.Cmd) (int, error) {
		path, a := c.Path, c.Args
		cmd := exec.CommandContext(t.Context(), "sleep", "30")
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
		mu.Lock()
		defer mu.Unlock()
		args[cmd.Process.Pid] = append([]string{path}, a...)
		return cmd.Process.Pid, daemon.WriteStatus(c.Status, cmd.Process.Pid, a[2])
	}
	p.Args = func(pid int) ([]string, error) {
		mu.Lock()
		defer mu.Unlock()
		return args[pid], nil
	}
	p.Listening = func(context.Context, int) bool { return false }
	p.ExcludeFromBackup = func(context.Context, string) error { return nil }
	p.Tick = time.Millisecond
	return p
}

func allNative() backend.Caps {
	caps := backend.Caps{}
	for _, f := range backend.Features {
		caps[f] = backend.Native
	}
	return caps
}

// existingVM makes the folder's VM with up, and returns its state.
func existingVM(t *testing.T, e *Env, o Options) *state.VM {
	t.Helper()
	if err := (Launch{e}).Up(t.Context(), o); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	vm := vmName(t, e)
	t.Cleanup(func() { e.Proxy.Stop(vm) }) // its proxy, if any: the test's sleep
	st, err := state.ReadVM(e.Dirs, vm)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func vmName(t *testing.T, e *Env) string {
	t.Helper()
	es, err := os.ReadDir(e.Dirs.VMs())
	if err != nil || len(es) == 0 {
		t.Fatalf("no VM state: %v", err)
	}
	return es[0].Name()
}

// answers gives e's questions a terminal and these answers.
func answers(t *testing.T, e *Env, yes bool) *[]string {
	t.Helper()
	var asked []string
	e.Terminal = func() bool { return true }
	e.Confirm = func(m msg.M) bool { asked = append(asked, m.Names()); return yes }
	e.Ask = e.Confirm
	return &asked
}

// call returns the fake's last call of a method.
func call(f *backendtest.Fake, method string) backendtest.Call {
	var last backendtest.Call
	for _, c := range f.Calls() {
		if c.Method == method {
			last = c
		}
	}
	return last
}

// staged returns the files the VM's setup was sent (Exec's input).
func staged(t *testing.T, f *backendtest.Fake) map[string]string {
	t.Helper()
	c := call(f, "Exec")
	if argv, _ := c.Args[1].([]string); !slices.Equal(argv, []string{"bash", "-c", syncCmd}) {
		t.Fatalf("Exec argv = %q", argv)
	}
	o, _ := c.Args[2].(backend.ExecOpts)
	files := map[string]string{}
	r := tar.NewReader(o.Stdin)
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return files
		} else if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		files[h.Name] = string(data)
	}
}
