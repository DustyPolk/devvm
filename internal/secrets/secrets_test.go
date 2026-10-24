package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
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

// testStore is a store in a temporary sandbox, its Keychain a map and its
// environment env.
func testStore(t *testing.T, keychain bool, env map[string]string) (*Store, map[string]string) {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	root := t.TempDir()
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config")}
	s := NewStore(d, filepath.Join(root, "home"))
	kc := map[string]string{}
	s.Keychain = keychain
	s.KeychainGet = func(_ context.Context, service, account string) (string, error) {
		v, ok := kc[service+"/"+account]
		if !ok {
			return "", platform.ErrNoSecret
		}
		return v, nil
	}
	s.Getenv = func(k string) string { return env[k] }
	return s, kc
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
	if _, err := s.Value(ctx, "github"); names(err) != "bad_secret_service" {
		t.Errorf("a service with spaces: %v", names(err))
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
		{"codex-key", "codex " + s.Home + "/.codex/auth.json"},
		{"codex", "codex " + s.Home + "/.codex/auth.json"},
		{"opencode-anthropic", "opencode " + s.Home + "/.local/share/opencode/auth.json"},
		{"claude-mcp-0123456789ab", "claude " + s.Home + "/.claude.json"},
	} {
		calls = nil
		v, err := s.Get(ctx, tc[0])
		want := "-login " + strings.Replace(tc[1], " ", " -login-file ", 1) + " -login-get " + tc[0]
		if v != "value" || err != nil || !slices.Equal(calls, []string{want}) {
			t.Errorf("Get(%s) = %q, %v; calls %q", tc[0], v, err, calls)
		}
	}
	for _, name := range []string{"claude", "../../etc/passwd", "nope"} {
		if _, err := s.Get(ctx, name); names(err) != `no_secret secret=`+msg.Shown(name) {
			t.Errorf("Get(%s) = %v", name, names(err))
		}
	}
}

// A VM uses the secrets bound to it, and claude when its image has it;
// its logins are its agents' with a login file.
func TestNamesAndLogins(t *testing.T) {
	s, _ := testStore(t, true, nil)
	d := paths.Dirs{Data: t.TempDir()}
	st, err := state.CreateVM(d, "vm1", state.NewVM{Image: "node+claude"})
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
	if got := s.Logins("node+codex+claude"); got != nil {
		t.Errorf("Logins without files = %q", got)
	}
	for _, k := range loginKinds {
		f := LoginFile(s.Home, k)
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.Logins("node+codex+claude"); !slices.Equal(got, []string{"codex", "claude"}) {
		t.Errorf("Logins = %q", got)
	}
	if got := s.Logins("node"); got != nil {
		t.Errorf("Logins without agents = %q", got)
	}
}

// The proxy's view of a VM's secrets: bindings for those with a value and
// for its logins (no values), and their hosts on 443.
func TestWriteFiles(t *testing.T) {
	s, kc := testStore(t, true, nil)
	kc["devvm/github"] = "ghp_secret"
	d := s.Dirs
	st, err := state.CreateVM(d, "vm1", state.NewVM{Image: "node+codex+claude"})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Set(KeySecrets, "github")
	auth := LoginFile(s.Home, "codex")
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
	// No secrets at all: an empty list.
	none, err := state.CreateVM(d, "vm2", state.NewVM{Image: "node"})
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
