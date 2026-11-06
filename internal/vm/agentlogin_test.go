package vm

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/secrets"
)

// What devvm agent login says the store saved: a new ChatGPT login, with
// the account's email when its tokens name one; an API key, the same
// login as before, or none at all, fails.
func TestAgentLoginSaved(t *testing.T) {
	root := t.TempDir()
	d := paths.Dirs{Home: filepath.Join(root, "home"), Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config")}
	var got msg.Recorder
	fake := &msg.Fake{OnAny: got.Add}
	store := secrets.NewStore(d, fake)
	store.ExcludeFromBackup = func(context.Context, string) error { return nil }
	e := AgentLogin{&Env{Dirs: d, Reporter: fake}}
	const vm = "devvm-p-00000000000000"
	notSaved := func(err error) bool {
		m, ok := errors.AsType[msg.M](err)
		return ok && m.ID() == "agent_login_not_saved"
	}

	if err := e.saved(store, "codex", vm, store.LoginSum("codex")); !notSaved(err) {
		t.Errorf("no login: %v", err)
	}
	before := store.LoginSum("codex")
	jwt := func(claims string) string {
		return "e30." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".sig"
	}
	write(t, d.AgentLogins("codex"), map[string]string{".codex/auth.json": `{"tokens": {"id_token": "` +
		jwt(`{"email":"a@b.test"}`) + `", "access_token": "` + jwt(`{}`) + `"}}`})
	if err := e.saved(store, "codex", vm, before); err != nil {
		t.Fatal(err)
	}
	if err := e.saved(store, "codex", vm, store.LoginSum("codex")); !notSaved(err) {
		t.Errorf("the same login: %v", err)
	}
	write(t, d.AgentLogins("opencode"), map[string]string{".local/share/opencode/auth.json": `{"openai": {"type": "oauth", "access": "opaque"}}`})
	if err := e.saved(store, "opencode", vm, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveKey(t.Context(), "codex", "", "sk-test"); err != nil {
		t.Fatal(err)
	}
	if err := e.saved(store, "codex", vm, before); !notSaved(err) {
		t.Errorf("an API key: %v", err)
	}
	want := []string{"agent_login_saved_for agent=codex email=a@b.test vm=" + vm, "agent_login_saved agent=opencode vm=" + vm}
	if names := got.Names(); !slices.Equal(names, want) {
		t.Errorf("said %q, want %q", names, want)
	}
}

// Each agent's own ChatGPT login: Codex's kept in its file, opencode's
// for OpenAI, in a browser or with a code.
func TestLoginArgv(t *testing.T) {
	for _, c := range []struct {
		o    AgentLoginOptions
		want []string
	}{
		{AgentLoginOptions{Agent: "codex"}, []string{"codex", "-c", `cli_auth_credentials_store="file"`, "login"}},
		{AgentLoginOptions{Agent: "codex", Device: true}, []string{"codex", "-c", `cli_auth_credentials_store="file"`, "login", "--device-auth"}},
		{AgentLoginOptions{Agent: "opencode"}, []string{"opencode", "auth", "login", "openai", "--method", "chatgpt-browser"}},
		{AgentLoginOptions{Agent: "opencode", Device: true}, []string{"opencode", "auth", "login", "openai", "--method", "chatgpt-headless"}},
		{AgentLoginOptions{Agent: "claude"}, []string{"env", "-u", "CLAUDE_CODE_OAUTH_TOKEN", "claude", "auth", "login", "--claudeai"}},
		{AgentLoginOptions{Agent: "claude", SetupToken: true}, []string{"env", "-u", "CLAUDE_CODE_OAUTH_TOKEN", "claude", "setup-token"}},
	} {
		if got := loginArgv(c.o); !slices.Equal(got, c.want) {
			t.Errorf("loginArgv(%+v) = %q, want %q", c.o, got, c.want)
		}
	}
}
