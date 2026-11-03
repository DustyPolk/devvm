package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
)

// testStore is a store in a temporary sandbox, its Keychain a map and its
// environment env.
func testStore(t *testing.T, keychain bool, env map[string]string) (*Store, map[string]string) {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	root := t.TempDir()
	d := paths.Dirs{Home: filepath.Join(root, "home"), Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config")}
	s := NewStore(d, func(m msg.M) { t.Log(m.Names()) })
	kc := map[string]string{}
	s.Keychain = keychain
	s.KeychainGet = func(_ context.Context, service, account string) (string, error) {
		v, ok := kc[service+"/"+account]
		if !ok {
			return "", platform.ErrNoSecret
		}
		return v, nil
	}
	s.KeychainSet = func(_ context.Context, service, account, label, value string) error {
		if label != "devvm "+account {
			t.Errorf("label %q for %s", label, account)
		}
		kc[service+"/"+account] = value
		return nil
	}
	s.KeychainDelete = func(_ context.Context, service, account string) error {
		if _, ok := kc[service+"/"+account]; !ok {
			return platform.ErrNoSecret
		}
		delete(kc, service+"/"+account)
		return nil
	}
	s.Getenv = func(k string) string { return env[k] }
	return s, kc
}

// A token is kept without whitespace; one that's empty, would end the
// Keychain's command line or a header, or for claude isn't a setup-token
// token, is refused.
func TestToken(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, tc := range []struct{ name, given, want, err string }{
		{"github", " ghp_x \n", "ghp_x", ""},
		{"github", "ghp_ x\t", "ghp_x", ""},
		{"claude", "sk-ant-oat01-abc\r\n", "sk-ant-oat01-abc", ""},
		{"github", "  \n", "", "no_token"},
		{"github", `ghp_x" -A -T "`, "", "token_quotes"},
		{"github", `ghp_x\`, "", "token_quotes"},
		{"github", "ghp_\x00x", "", "token_quotes"},
		{"github", "ghp_\x7fx", "", "token_quotes"},
		{"claude", "some-browser-code", "", "not_setup_token"},
	} {
		v, err := Token(tc.name, tc.given)
		if v != tc.want || msg.NamesOf(err) != tc.err {
			t.Errorf("Token(%s, %q) = %q, %v", tc.name, tc.given, v, msg.NamesOf(err))
		}
	}
}

// Save and Delete: the Keychain under devvm's service, else a file only
// the user reads in a folder only the user opens; only devvm's names.
func TestSaveDelete(t *testing.T) {
	ctx := t.Context()
	s, kc := testStore(t, true, map[string]string{"DEVVM_SECRET_SERVICE": "devvm-test"}) //nolint:gosec // a service name
	if err := s.Save(ctx, "github", "ghp_x"); err != nil || kc["devvm-test/github"] != "ghp_x" {
		t.Errorf("Save: %v, %q", err, kc)
	}
	if v, _ := s.Value(ctx, "github"); v != "ghp_x" {
		t.Errorf("Value after Save = %q", v)
	}
	if err := s.Delete(ctx, "github"); err != nil || len(kc) != 0 {
		t.Errorf("Delete: %v, %q", err, kc)
	}
	if err := s.Delete(ctx, "github"); !errors.Is(err, ErrNone) {
		t.Errorf("Delete again: %v", err)
	}
	s.KeychainSet = func(context.Context, string, string, string, string) error { return errors.New("denied") }
	if err := s.Save(ctx, "claude", "sk-ant-oat01-x"); msg.NamesOf(err) != "keychain_save_failed secret=claude" {
		t.Errorf("a failed save: %v", msg.NamesOf(err))
	}
	for _, name := range []string{"../../x", "nope"} {
		if err := s.Save(ctx, name, "v"); msg.NamesOf(err) != "no_secret secret="+msg.OneLine(name) {
			t.Errorf("Save(%s): %v", name, msg.NamesOf(err))
		}
		if err := s.Delete(ctx, name); !errors.Is(err, ErrNone) {
			t.Errorf("Delete(%s): %v", name, err)
		}
	}

	f, _ := testStore(t, false, nil)
	// One made too open is made the user's alone.
	if err := os.MkdirAll(f.Dirs.Secrets(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(ctx, "github", "ghp_file"); err != nil {
		t.Fatal(err)
	}
	if v, _ := f.Value(ctx, "github"); v != "ghp_file" {
		t.Errorf("file Value = %q", v)
	}
	for path, want := range map[string]os.FileMode{f.Dirs.Secrets(): 0o700, f.File("github"): 0o600} {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v, %v", path, fi.Mode(), err)
		}
	}
	if err := f.Delete(ctx, "github"); err != nil {
		t.Errorf("file Delete: %v", err)
	}
	if err := f.Delete(ctx, "github"); !errors.Is(err, ErrNone) {
		t.Errorf("file Delete again: %v", err)
	}
}

// Bind adds a secret to a VM's own list once, and takes it out; claude
// stays with an image that has it.
func TestBind(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	st, err := state.CreateVM(d, "vm1", state.NewVM{Image: "base", Tools: []string{"node", "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		name string
		use  bool
		want string
	}{{"github", true, "github"}, {"github", true, "github"}, {"claude", true, "github claude"}, {"github", false, "claude"}, {"claude", false, ""}} {
		if err := Bind(st, step.name, step.use); err != nil || st.Get(KeySecrets) != step.want {
			t.Errorf("Bind(%s, %v): %q, %v", step.name, step.use, st.Get(KeySecrets), err)
		}
	}
	if !slices.Equal(Names(st), []string{"claude"}) || !FromTools(st, "claude") || FromTools(st, "github") {
		t.Errorf("Names = %q", Names(st))
	}
}

func TestValue(t *testing.T) {
	ctx := t.Context()
	s, kc := testStore(t, true, map[string]string{})
	if _, err := s.Value(ctx, "github"); !errors.Is(err, ErrNone) {
		t.Errorf("no value: %v", err)
	}
	kc["devvm/github"] = "ghp_x\n"
	if v, err := s.Value(ctx, "github"); v != "ghp_x" || err != nil {
		t.Errorf("Keychain: %q, %v", v, err)
	}
	// DEVVM_SECRET_SERVICE keeps test secrets apart, and is plain.
	env := map[string]string{}
	s.Getenv = func(k string) string { return env[k] }
	env["DEVVM_SECRET_SERVICE"] = "devvm-test"
	if _, err := s.Value(ctx, "github"); !errors.Is(err, ErrNone) {
		t.Errorf("another service's: %v", err)
	}
	env["DEVVM_SECRET_SERVICE"] = "x -a y"
	if _, err := s.Value(ctx, "github"); msg.NamesOf(err) != "bad_secret_service" {
		t.Errorf("a service with spaces: %v", msg.NamesOf(err))
	}
	// A claude token in the environment wins.
	delete(env, "DEVVM_SECRET_SERVICE")
	env["CLAUDE_CODE_OAUTH_TOKEN"] = "sk-ant-oat01-env"
	kc["devvm/claude"] = "sk-ant-oat01-kc"
	if v, _ := s.Value(ctx, "claude"); v != "sk-ant-oat01-env" {
		t.Errorf("claude = %q", v)
	}
	// Without a Keychain, a file only the user reads.
	f, _ := testStore(t, false, nil)
	if err := os.MkdirAll(f.Dirs.Secrets(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.File("github"), []byte("ghp_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if v, err := f.Value(ctx, "github"); v != "ghp_file" || err != nil {
		t.Errorf("file: %q, %v", v, err)
	}
	if _, err := f.Value(ctx, "claude"); !errors.Is(err, ErrNone) {
		t.Errorf("no file: %v", err)
	}
}

// Get: a known secret's value, a login's through the login helper, and
// nothing else (a name is never a path).
func TestGet(t *testing.T) {
	s, kc := testStore(t, true, nil)
	kc["devvm/github"] = "ghp_x"
	var calls []string
	s.Login = func(args []string, out io.Writer) error {
		calls = append(calls, strings.Join(args, " "))
		_, err := io.WriteString(out, "value\n")
		return err
	}
	ctx := t.Context()
	if v, err := s.Get(ctx, "github"); v != "ghp_x" || err != nil {
		t.Errorf("github = %q, %v", v, err)
	}
	for _, tc := range [][2]string{
		{"codex-key", "codex " + s.Dirs.Home + "/.codex/auth.json"},
		{"codex", "codex " + s.Dirs.Home + "/.codex/auth.json"},
		{"opencode-anthropic", "opencode " + s.Dirs.Home + "/.local/share/opencode/auth.json"},
		{"claude-mcp-0123456789ab", "claude " + s.Dirs.Home + "/.claude.json"},
	} {
		calls = nil
		v, err := s.Get(ctx, tc[0])
		want := "-login " + strings.Replace(tc[1], " ", " -login-file ", 1) + " -login-get " + tc[0]
		if v != "value" || err != nil || !slices.Equal(calls, []string{want}) {
			t.Errorf("Get(%s) = %q, %v; calls %q", tc[0], v, err, calls)
		}
	}
	for _, name := range []string{"claude", "../../etc/passwd", "nope"} {
		if _, err := s.Get(ctx, name); msg.NamesOf(err) != `no_secret secret=`+msg.OneLine(name) {
			t.Errorf("Get(%s) = %v", name, msg.NamesOf(err))
		}
	}
}

// A VM uses the secrets bound to it, and claude when its image has it;
// its logins are its agents' with a login file.
func TestNamesAndLogins(t *testing.T) {
	s, _ := testStore(t, true, nil)
	d := paths.Dirs{Data: t.TempDir()}
	st, err := state.CreateVM(d, "vm1", state.NewVM{Image: "base", Tools: []string{"node", "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := Names(st); !slices.Equal(got, []string{"claude"}) {
		t.Errorf("Names = %q", got)
	}
	_ = st.Set(KeySecrets, "github")
	if got := Names(st); !slices.Equal(got, []string{"github", "claude"}) {
		t.Errorf("Names = %q", got)
	}
	if got := s.Logins([]string{"node", "codex", "claude"}); got != nil {
		t.Errorf("Logins without files = %q", got)
	}
	for _, k := range loginKinds {
		f := LoginFile(s.Dirs.Home, k)
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.Logins([]string{"node", "codex", "claude"}); !slices.Equal(got, []string{"codex", "claude"}) {
		t.Errorf("Logins = %q", got)
	}
	if got := s.Logins([]string{"node"}); got != nil {
		t.Errorf("Logins without agents = %q", got)
	}
}

// The proxy's view of a VM's secrets: bindings for those with a value and
// for its logins (no values), and their hosts on 443.
func TestWriteFiles(t *testing.T) {
	s, kc := testStore(t, true, nil)
	kc["devvm/github"] = "ghp_secret"
	d := s.Dirs
	st, err := state.CreateVM(d, "vm1", state.NewVM{Image: "base", Tools: []string{"node", "codex", "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Set(KeySecrets, "github")
	auth := LoginFile(s.Dirs.Home, "codex")
	if err := os.MkdirAll(filepath.Dir(auth), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auth, []byte(`{"OPENAI_API_KEY":"sk-codex-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	salt := strings.Repeat("ab", 16)
	if err := s.WriteFiles(t.Context(), st, salt); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(BindingsFile(d, "vm1"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") {
		t.Errorf("a value in the bindings: %s", data)
	}
	var bs []binding
	if err := json.Unmarshal(data, &bs); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range bs {
		got = append(got, b.Name+" "+strings.Join(b.Hosts, ",")+" "+b.Header+" "+b.Placeholder)
	}
	// claude has no value: no binding, but its host still comes with it.
	want := []string{
		"github api.github.com,github.com,codeload.github.com,raw.githubusercontent.com Authorization devvm-proxy-managed-github-" + salt,
	}
	if len(got) != 2 || got[0] != want[0] || !strings.HasPrefix(got[1], "codex-key api.openai.com Authorization ") || !strings.Contains(got[1], salt) {
		t.Errorf("bindings = %q", got)
	}
	pol, err := os.ReadFile(PolicyFile(d, "vm1"))
	if err != nil {
		t.Fatal(err)
	}
	wantPol := "api.github.com:443\ngithub.com:443\ncodeload.github.com:443\nraw.githubusercontent.com:443\napi.anthropic.com:443\napi.openai.com:443\n"
	if string(pol) != wantPol {
		t.Errorf("policy = %q, want %q", pol, wantPol)
	}
	for _, f := range []string{BindingsFile(d, "vm1"), PolicyFile(d, "vm1")} {
		if fi, err := os.Stat(f); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v", f, fi.Mode(), err)
		}
	}
	// A refresh keeps the logins' bindings and hosts as written, without
	// reading any login file, and follows the secrets: unbound, github's
	// binding and hosts go.
	s.Login = func([]string, io.Writer) error { t.Error("a refresh read a login"); return errors.New("no") }
	_ = st.Set(KeySecrets, "")
	if err := s.RefreshFiles(t.Context(), st, salt); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(BindingsFile(d, "vm1"))
	if strings.Contains(string(data), `"github"`) || !strings.Contains(string(data), `"codex-key"`) {
		t.Errorf("refreshed bindings = %s", data)
	}
	if pol, _ := os.ReadFile(PolicyFile(d, "vm1")); string(pol) != "api.anthropic.com:443\napi.openai.com:443\n" {
		t.Errorf("refreshed policy = %q", pol)
	}
	// No secrets at all: an empty list.
	none, err := state.CreateVM(d, "vm2", state.NewVM{Image: "base", Tools: []string{"node"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFiles(t.Context(), none, salt); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(BindingsFile(d, "vm2")); string(data) != "[]\n" {
		t.Errorf("no bindings = %q", data)
	}
}

// The VM's copies: a login's with placeholders ending in its salt, the MCP
// servers whose secrets a plain copy would carry, and settings files with
// secret-looking values taken out.
func TestVMCopies(t *testing.T) {
	s, _ := testStore(t, false, nil)
	write := func(kind, data string) {
		t.Helper()
		f := LoginFile(s.Dirs.Home, kind)
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.MCPSecrets(); got != nil {
		t.Errorf("MCPSecrets without ~/.claude.json = %q", got)
	}
	write("claude", `{"mcpServers":{"plain":{"command":"x"},"a b":{"command":"y","env":{"K":"v"}}}}`)
	if got := s.MCPSecrets(); !slices.Equal(got, []string{"a b"}) {
		t.Errorf("MCPSecrets = %q", got)
	}
	write("codex", `{"OPENAI_API_KEY":"sk-codex-secret"}`)
	salt := strings.Repeat("cd", 16)
	data, err := s.VMLogin("codex", salt)
	if err != nil || strings.Contains(string(data), "sk-codex-secret") || !strings.Contains(string(data), salt) {
		t.Errorf("VMLogin = %s, %v", data, err)
	}
	out, keys, err := Redact("json", []byte(`{"env":{"API_TOKEN":"tok-secret","NAME":"x"}}`))
	if err != nil || strings.Contains(string(out), "tok-secret") || !slices.Equal(keys, []string{"env.API_TOKEN"}) {
		t.Errorf("Redact = %s, %q, %v", out, keys, err)
	}
	if _, _, err := Redact("toml", []byte("x = \"\"\"\nmulti\n\"\"\"\n")); err == nil {
		t.Error("Redact took a multi-line TOML string")
	}
}

// Two saves of one secret at once each write it whole: neither fails, and
// the file holds one of their values, with no temporary file left.
func TestSaveAtOnce(t *testing.T) {
	ctx := t.Context()
	s, _ := testStore(t, false, nil)
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := range 4 {
		wg.Go(func() {
			for range 50 {
				errs <- s.Save(ctx, "github", "ghp_"+strconv.Itoa(i))
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	if v, _ := s.Value(ctx, "github"); !slices.Contains([]string{"ghp_0", "ghp_1", "ghp_2", "ghp_3"}, v) {
		t.Errorf("Value = %q", v)
	}
	if left := dirNames(t, s.Dirs.Secrets()); !slices.Equal(left, []string{"github"}) {
		t.Errorf("the secrets folder holds %q", left)
	}
	if fi, err := os.Stat(s.File("github")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the secret's file: %v, %v", fi, err)
	}
	// A save that never finished left a temporary file: Delete takes it too.
	if err := os.WriteFile(filepath.Join(s.Dirs.Secrets(), ".github.tmp-123"), []byte("ghp_x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "github"); err != nil {
		t.Fatal(err)
	}
	if left := dirNames(t, s.Dirs.Secrets()); len(left) > 0 {
		t.Errorf("after Delete, the secrets folder holds %q", left)
	}
}

// A VM's files that can't be written leave no temporary file behind.
func TestWriteFilesFailing(t *testing.T) {
	s, kc := testStore(t, true, nil)
	kc["devvm/github"] = "ghp_secret"
	st, err := state.CreateVM(s.Dirs, "vm1", state.NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Set(KeySecrets, "github")
	// A folder where the bindings go: renaming over it fails.
	in := filepath.Join(BindingsFile(s.Dirs, "vm1"), "x")
	if err := os.MkdirAll(in, 0o700); err != nil {
		t.Fatal(err)
	}
	var le *os.LinkError
	if err := s.WriteFiles(t.Context(), st, strings.Repeat("ab", 16)); !errors.As(err, &le) || le.New != BindingsFile(s.Dirs, "vm1") {
		t.Fatalf("WriteFiles over a folder: %v; want its rename failing", err)
	}
	if left := dirNames(t, s.Dirs.Run()); !slices.Equal(left, []string{filepath.Base(BindingsFile(s.Dirs, "vm1"))}) {
		t.Errorf("the run folder holds %q", left)
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}
