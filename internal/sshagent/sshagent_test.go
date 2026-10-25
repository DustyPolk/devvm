package sshagent

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// fakeAgent stands in for ssh-agent, ssh-add and ssh-keygen: a key file's
// contents stand for the key, held as fingerprint SHA256:fp-<contents>.
type fakeAgent struct {
	up    bool
	held  []string // the contents of the keys it holds
	calls []string // ssh-agent and ssh-add calls, a key as key:<contents>
}

func (f *fakeAgent) run(_ context.Context, _ bool, argv ...string) (string, int) {
	switch argv[0] {
	case "ssh-agent":
		f.calls = append(f.calls, strings.Join(argv, " "))
		f.up = true
		return "", 0
	case "ssh-keygen":
		data, err := os.ReadFile(argv[len(argv)-1])
		if err != nil || len(data) == 0 {
			return "", 1
		}
		return "256 SHA256:fp-" + strings.TrimSpace(string(data)) + " c (ED25519)\n", 0
	}
	if !f.up {
		return "", 2
	}
	if argv[1] == "-l" {
		if len(f.held) == 0 {
			return "The agent has no identities.\n", 1
		}
		var b strings.Builder
		for _, k := range f.held {
			b.WriteString("256 SHA256:fp-" + k + " " + k + " comment (ED25519)\n")
		}
		return b.String(), 0
	}
	data, err := os.ReadFile(argv[len(argv)-1])
	if err != nil {
		return "", 1
	}
	k := strings.TrimSpace(string(data))
	f.calls = append(f.calls, strings.Join(append(slices.Clone(argv[:len(argv)-1]), "key:"+k), " "))
	f.held = append(f.held, k)
	return "", 0
}

type said struct{ lines []string }

func (s *said) add(prefix string) func(msg.M) {
	return func(m msg.M) { s.lines = append(s.lines, prefix+m.Names()) }
}

// said is the agent's UI: what it prints, by name, and no run log.
func (s *said) Info(m msg.M)         { s.add("==> ")(m) }
func (s *said) Note(m msg.M)         { s.add("")(m) }
func (s *said) Warn(m msg.M)         { s.add("devvm: ")(m) }
func (s *said) LogWriter() io.Writer { return io.Discard }

// testAgent is devvm's agent on a fake, in a sandbox, with no VMs unless
// mounts is set; tmp is a folder outside them for keys.
func testAgent(t *testing.T) (a *Agent, f *fakeAgent, out *said, tmp string, mounts *[]Mount) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f, out, mounts = &fakeAgent{}, &said{}, new([]Mount)
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config")}
	a = New(d, func(context.Context) ([]Mount, error) { return *mounts, nil }, out)
	a.Run = f.run
	a.Getenv = func(string) string { return "" }
	a.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	tmp = filepath.Join(root, "keys")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	return a, f, out, tmp, mounts
}

func writeKey(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readKeys(t *testing.T, a *Agent) []state.AgentKey {
	t.Helper()
	keys, err := state.ReadAgentKeys(a.Dirs)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func names(err error) string {
	if m, ok := errors.AsType[msg.M](err); ok {
		return m.Names()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

// add records the key and its fingerprint, starts the agent and loads the
// key there, kept 12 hours.
func TestAdd(t *testing.T) {
	a, f, out, tmp, _ := testAgent(t)
	key := filepath.Join(tmp, "deploy_key")
	writeKey(t, key, "deploy")
	if err := a.Add(t.Context(), key, AddOptions{}); err != nil {
		t.Fatal(names(err))
	}
	if got, want := readKeys(t, a), []state.AgentKey{{File: key, Fingerprint: "SHA256:fp-deploy"}}; !slices.Equal(got, want) {
		t.Errorf("keys = %+v, want %+v", got, want)
	}
	if want := []string{"ssh-agent -a " + a.Socket(), "ssh-add -t 12h key:deploy"}; !slices.Equal(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
	if want := []string{"==> agent_holds keys=deploy comment", "agent_holds_note"}; !slices.Equal(out.lines, want) {
		t.Errorf("said %q, want %q", out.lines, want)
	}
	if fi, err := os.Stat(a.Dirs.AgentKeys()); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("agent keys file: %v, %v", fi, err)
	}
}

// --confirm and --lifetime are kept per key and used when loading; adding
// it again replaces them. A lifetime that isn't one, and --confirm without
// an ssh-askpass to ask with, are refused.
func TestAddOptions(t *testing.T) {
	a, f, _, tmp, _ := testAgent(t)
	key := filepath.Join(tmp, "k")
	writeKey(t, key, "k")
	a.Getenv = func(k string) string { return map[string]string{"SSH_ASKPASS": "/usr/bin/true"}[k] } //nolint:gosec // a program, not a credential
	if err := a.Add(t.Context(), key, AddOptions{Confirm: true, Lifetime: "30m"}); err != nil {
		t.Fatal(names(err))
	}
	if got := readKeys(t, a); len(got) != 1 || got[0].Options != "confirm lifetime=30m" {
		t.Errorf("keys = %+v", got)
	}
	if !slices.Contains(f.calls, "ssh-add -c -t 30m key:k") {
		t.Errorf("calls = %q", f.calls)
	}
	if err := a.Add(t.Context(), key, AddOptions{Lifetime: Forever}); err != nil {
		t.Fatal(names(err))
	}
	if got := readKeys(t, a); len(got) != 1 || got[0].Options != "lifetime=forever" {
		t.Errorf("keys = %+v", got)
	}
	if f.calls[len(f.calls)-1] != "ssh-add key:k" {
		t.Errorf("calls = %q", f.calls)
	}
	if err := a.Add(t.Context(), key, AddOptions{Lifetime: "soon"}); names(err) != "agent_lifetime_invalid value=soon" {
		t.Errorf("--lifetime soon: %s", names(err))
	}
	a.Getenv = func(string) string { return "" }
	if err := a.Add(t.Context(), key, AddOptions{Confirm: true}); names(err) != "agent_confirm_needs_askpass" {
		t.Errorf("--confirm without ssh-askpass: %s", names(err))
	}
	a.LookPath = func(string) (string, error) { return "/x/ssh-askpass", nil }
	if err := a.Add(t.Context(), key, AddOptions{Confirm: true}); err != nil {
		t.Errorf("--confirm with ssh-askpass on PATH: %s", names(err))
	}
	if err := a.Add(t.Context(), filepath.Join(tmp, "none"), AddOptions{}); names(err) != "agent_key_not_found key="+filepath.Join(tmp, "none") {
		t.Errorf("missing key: %s", names(err))
	}
	if err := a.Add(t.Context(), tmp, AddOptions{}); !strings.HasPrefix(names(err), "agent_key_not_found ") {
		t.Errorf("a folder: %s", names(err))
	}
}

// After a reboot (no agent) it starts one and loads the recorded keys; a
// running agent with its keys needs nothing; a key whose lifetime ran out
// is loaded again.
func TestEnsure(t *testing.T) {
	a, f, out, tmp, _ := testAgent(t)
	deploy, other := filepath.Join(tmp, "deploy"), filepath.Join(tmp, "other")
	writeKey(t, deploy, "deploy")
	writeKey(t, other, "other")
	if err := state.WriteAgentKeys(a.Dirs, []state.AgentKey{
		{File: deploy, Fingerprint: "SHA256:fp-deploy"},
		{File: other, Fingerprint: "SHA256:fp-other", Options: "lifetime=forever"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Ensure(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if want := []string{"ssh-agent -a " + a.Socket(), "ssh-add -t 12h key:deploy", "ssh-add key:other"}; !slices.Equal(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
	f.calls = nil
	if err := a.Ensure(t.Context()); err != nil || len(f.calls) != 0 {
		t.Errorf("running, with its keys: %q, %v", f.calls, err)
	}
	f.held = slices.DeleteFunc(f.held, func(k string) bool { return k == "deploy" }) // its lifetime ran out
	if err := a.Ensure(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if want := []string{"ssh-add -t 12h key:deploy"}; !slices.Equal(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
	if len(out.lines) != 0 {
		t.Errorf("said %q", out.lines)
	}
}

// With no keys added, a new agent says it has none.
func TestEnsureNoKeys(t *testing.T) {
	a, f, out, _, _ := testAgent(t)
	if err := a.Ensure(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if !f.up || !slices.Equal(out.lines, []string{"devvm: agent_no_keys"}) {
		t.Errorf("up %v, said %q", f.up, out.lines)
	}
}

// A key is loaded only while its file holds the key that was added: not a
// link swapped to another, nor one recorded without a fingerprint.
func TestEnsurePinned(t *testing.T) {
	a, f, out, tmp, _ := testAgent(t)
	deploy, main := filepath.Join(tmp, "deploy"), filepath.Join(tmp, "main")
	writeKey(t, deploy, "deploy")
	writeKey(t, main, "main")
	if err := a.Add(t.Context(), deploy, AddOptions{}); err != nil {
		t.Fatal(names(err))
	}
	*f = fakeAgent{} // a reboot
	if err := os.Remove(deploy); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(main, deploy); err != nil { // what a VM that could write there would do
		t.Fatal(err)
	}
	out.lines = nil
	if err := a.Ensure(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if want := []string{"devvm: agent_key_changed key=" + deploy}; !slices.Equal(out.lines, want) {
		t.Errorf("said %q, want %q", out.lines, want)
	}
	if slices.Contains(f.held, "main") {
		t.Error("loaded the key the link names now")
	}
	if err := state.WriteAgentKeys(a.Dirs, []state.AgentKey{{File: main}}); err != nil {
		t.Fatal(err)
	}
	out.lines = nil
	if err := a.Ensure(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if want := []string{"devvm: agent_key_unpinned key=" + main}; !slices.Equal(out.lines, want) || slices.Contains(f.held, "main") {
		t.Errorf("said %q, held %q", out.lines, f.held)
	}
}

// A key in a folder a VM mounts is refused, a link to it too; one added
// before a VM mounted its folder isn't loaded.
func TestMountedKey(t *testing.T) {
	a, f, out, tmp, mounts := testAgent(t)
	proj := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(proj, "deploy_key")
	writeKey(t, key, "deploy")
	*mounts = []Mount{{VM: "devvm-other-0123456789abcd", Dir: "/elsewhere"}, {VM: "devvm-proj-0123456789abcd", Dir: proj}}
	want := "agent_key_mounted key=" + key + " vm=devvm-proj-0123456789abcd"
	if err := a.Add(t.Context(), key, AddOptions{}); names(err) != want {
		t.Errorf("add: %s, want %s", names(err), want)
	}
	if len(readKeys(t, a)) != 0 || len(f.held) != 0 {
		t.Error("the key was recorded or loaded")
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(key, link); err != nil {
		t.Fatal(err)
	}
	if err := a.Add(t.Context(), link, AddOptions{}); !strings.HasPrefix(names(err), "agent_key_mounted ") {
		t.Errorf("add a link to it: %s", names(err))
	}
	// A sibling folder whose name starts the same isn't inside it.
	*mounts = []Mount{{VM: "devvm-pro-0123456789abcd", Dir: filepath.Join(tmp, "pro")}}
	if err := a.Add(t.Context(), key, AddOptions{}); err != nil {
		t.Fatal(names(err))
	}
	*f = fakeAgent{}
	*mounts = []Mount{{VM: "devvm-proj-0123456789abcd", Dir: proj}}
	out.lines = nil
	if err := a.Ensure(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if !slices.Equal(out.lines, []string{"devvm: " + want}) || len(f.held) != 0 {
		t.Errorf("said %q, held %q", out.lines, f.held)
	}
}

// keys lists what the agent holds, then each key added: its file,
// fingerprint and options, its lifetime shown when it's the default.
func TestKeys(t *testing.T) {
	a, _, _, tmp, _ := testAgent(t)
	k1, k2 := filepath.Join(tmp, "a"), filepath.Join(tmp, "b")
	writeKey(t, k1, "a")
	writeKey(t, k2, "b")
	if err := a.Add(t.Context(), k1, AddOptions{}); err != nil {
		t.Fatal(names(err))
	}
	if err := a.Add(t.Context(), k2, AddOptions{Lifetime: "1h"}); err != nil {
		t.Fatal(names(err))
	}
	var b strings.Builder
	if err := a.Keys(t.Context(), &b); err != nil {
		t.Fatal(names(err))
	}
	want := "256 SHA256:fp-a a comment (ED25519)\n256 SHA256:fp-b b comment (ED25519)\n" +
		k1 + ": SHA256:fp-a lifetime=12h\n" + k2 + ": SHA256:fp-b lifetime=1h\n"
	if b.String() != want {
		t.Errorf("keys:\n%s\nwant:\n%s", b.String(), want)
	}
}

// With OpenSSH's own tools: a throwaway key, in an agent of the test's
// own, whose socket the test names. No other agent is reached.
func TestRealAgent(t *testing.T) {
	for _, tool := range []string{"ssh-agent", "ssh-add", "ssh-keygen"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " isn't installed")
		}
	}
	// /tmp keeps the socket's path short.
	root, err := os.MkdirTemp("/tmp", "dva.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if root, err = filepath.EvalSymlinks(root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data")) // the run log
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	d := paths.Dirs{Data: filepath.Join(root, "data", "devvm"), Config: filepath.Join(root, "config", "devvm")}
	if err := os.MkdirAll(d.Data, 0o700); err != nil {
		t.Fatal(err)
	}
	agent := exec.CommandContext(context.Background(), "ssh-agent", "-D", "-a", d.AgentSock()) //nolint:gosec // the test's own socket
	if err := agent.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Process.Kill(); _ = agent.Wait() })
	for range 100 {
		if _, err := os.Stat(d.AgentSock()); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	key := filepath.Join(root, "key")
	keygen := exec.CommandContext(t.Context(), "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "devvm-test-key", "-f", key) //nolint:gosec // the test's own file
	if out, err := keygen.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	var out said
	a := New(d, func(context.Context) ([]Mount, error) { return nil, nil }, &out)
	if err := a.Add(t.Context(), key, AddOptions{Lifetime: "1h"}); err != nil {
		t.Fatal(names(err))
	}
	if len(out.lines) == 0 || out.lines[0] != "==> agent_holds keys=devvm-test-key" {
		t.Errorf("said %q", out.lines)
	}
	keys := readKeys(t, a)
	if len(keys) != 1 || keys[0].File != key || !strings.HasPrefix(keys[0].Fingerprint, "SHA256:") || keys[0].Options != "lifetime=1h" {
		t.Errorf("keys = %+v", keys)
	}
	var b strings.Builder
	if err := a.Keys(t.Context(), &b); err != nil {
		t.Fatal(names(err))
	}
	if !strings.Contains(b.String(), " "+keys[0].Fingerprint+" devvm-test-key (ED25519)\n") ||
		!strings.Contains(b.String(), key+": "+keys[0].Fingerprint+" lifetime=1h\n") {
		t.Errorf("keys:\n%s", b.String())
	}
}

func TestMode(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir()}
	st, err := state.CreateVM(d, "devvm-web-0123456789abcd", state.NewVM{})
	if err != nil {
		t.Fatal(err)
	}
	for val, want := range map[string]string{"": Off, "off": Off, "on": On, "devvm": Devvm, "bogus": Off} {
		if err := st.Set(Key, val); err != nil {
			t.Fatal(err)
		}
		if got := Mode(st); got != want {
			t.Errorf("Mode(%q) = %q, want %q", val, got, want)
		}
	}
}

func TestValidLifetime(t *testing.T) {
	for s, want := range map[string]bool{"30m": true, "8h": true, "1h30m": true, "3600": true, "forever": true, "": false, "soon": false, "-1h": false, "8 h": false} {
		if got := ValidLifetime(s); got != want {
			t.Errorf("ValidLifetime(%q) = %v, want %v", s, got, want)
		}
	}
}
