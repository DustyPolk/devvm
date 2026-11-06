package agentlogin

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// An API key saved: Codex's login becomes an API-key login, its ChatGPT
// login gone; opencode's gets the provider's entry beside the others. A
// provider devvm doesn't put keys in for, or a key that isn't one line,
// is refused, and nothing is written.
func TestSetKey(t *testing.T) {
	if got := KeyProviders(Opencode); !slices.Equal(got, []string{"anthropic", "openai", "openrouter"}) {
		t.Errorf("KeyProviders(opencode) = %v", got)
	}
	if got := KeyProviders(Codex); got != nil {
		t.Errorf("KeyProviders(codex) = %v", got)
	}

	c := storeFile(t, Codex, ".codex/auth.json")
	must(t, c.lockStoreWrite(map[string]any{"tokens": map[string]any{"access_token": "at"}}))
	must(t, c.SetKey("", "sk-test"))
	m := readStore(t, c)
	if str(m, "OPENAI_API_KEY") != "sk-test" || str(m, "auth_mode") != "apikey" || m["tokens"] != nil {
		t.Errorf("Codex's login: %v", m)
	}
	if bs, _ := c.Bindings(); len(bs) != 1 || bs[0].Name != "codex-key" {
		t.Errorf("Codex's bindings: %v", bs)
	}

	o := storeFile(t, Opencode, "auth.json")
	must(t, o.lockStoreWrite(map[string]any{"openai": map[string]any{"type": "oauth", "access": "a", "refresh": "r", "expires": 1}}))
	must(t, o.SetKey("anthropic", "sk-ant"))
	m = readStore(t, o)
	if e := obj(m, "anthropic"); str(e, "type") != "api" || str(e, "key") != "sk-ant" || str(obj(m, "openai"), "refresh") != "r" {
		t.Errorf("opencode's login: %v", m)
	}

	for _, bad := range []struct {
		f             File
		provider, key string
	}{
		{c, "anthropic", "k"}, {o, "", "k"}, {o, "groq", "k"}, {c, "", " "}, {c, "", "a\nb"},
	} {
		if err := bad.f.SetKey(bad.provider, bad.key); err == nil {
			t.Errorf("SetKey(%s, %q, %q) took it", bad.f.Kind, bad.provider, bad.key)
		}
	}
	if str(readStore(t, c), "OPENAI_API_KEY") != "sk-test" {
		t.Error("a refused key changed Codex's login")
	}
}

// opencode's import of a key is opencode 2's auth export entry: devvm's
// ID, the key's placeholder, which is the one the VM's copy of auth.json
// holds and the proxy binds, never the key; a provider devvm doesn't put
// keys in for is refused.
func TestOpencodeKeyImport(t *testing.T) {
	o := storeFile(t, Opencode, "auth.json")
	must(t, o.lockStoreWrite(map[string]any{}))
	must(t, o.SetKey("anthropic", "sk-ant-secret"))
	o.Salt = "s4lt"
	bs, err := o.Bindings()
	if err != nil || len(bs) != 1 {
		t.Fatalf("bindings: %v %v", bs, err)
	}
	data, err := OpencodeKeyImport("anthropic", "s4lt")
	if err != nil {
		t.Fatal(err)
	}
	var got []struct {
		ID, IntegrationID, Label string
		Active                   bool
		Value                    struct{ Type, Key string }
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		ID, IntegrationID, Label string
		Active                   bool
		Value                    struct{ Type, Key string }
	}{{"cred_devvm_anthropic", "anthropic", "devvm", true, struct{ Type, Key string }{"key", bs[0].Placeholder}}}
	if !slices.Equal(got, want) || strings.Contains(string(data), "sk-ant-secret") {
		t.Errorf("OpencodeKeyImport = %s, want the placeholder %s", data, bs[0].Placeholder)
	}
	if _, err := OpencodeKeyImport("groq", "s4lt"); err == nil {
		t.Error("OpencodeKeyImport(groq) took it")
	}
}

// lockStoreWrite writes m as f's login, its folder made.
func (f File) lockStoreWrite(m map[string]any) error {
	unlock, err := f.lockStore()
	if err != nil {
		return err
	}
	defer unlock()
	return writeJSON(f.Path, m)
}

// The ChatGPT login a store's file holds, with its account's email: from
// Codex's id_token, or opencode's access token's profile; none for an
// API key or no file.
func TestChatGPT(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	c := storeFile(t, Codex, ".codex/auth.json")
	if _, ok, err := c.ChatGPT(); ok || err != nil {
		t.Errorf("no file: %v, %v", ok, err)
	}
	must(t, c.lockStoreWrite(map[string]any{"tokens": map[string]any{
		"id_token": jwtWith(exp, map[string]any{"email": "a@b.test"}), "access_token": jwtWith(exp, nil),
	}}))
	if e, ok, err := c.ChatGPT(); e != "a@b.test" || !ok || err != nil {
		t.Errorf("Codex: %q, %v, %v", e, ok, err)
	}
	must(t, c.SetKey("", "sk-test"))
	if _, ok, _ := c.ChatGPT(); ok {
		t.Error("an API key read as a ChatGPT login")
	}

	o := storeFile(t, Opencode, "auth.json")
	must(t, o.lockStoreWrite(map[string]any{"openai": map[string]any{
		"type":   "oauth",
		"access": jwtWith(exp, map[string]any{"https://api.openai.com/profile": map[string]any{"email": "o@b.test"}}),
	}}))
	if e, ok, err := o.ChatGPT(); e != "o@b.test" || !ok || err != nil {
		t.Errorf("opencode: %q, %v, %v", e, ok, err)
	}
	must(t, o.lockStoreWrite(map[string]any{"openai": map[string]any{"type": "oauth", "access": "opaque"}}))
	if e, ok, _ := o.ChatGPT(); e != "" || !ok {
		t.Errorf("opencode without claims: %q, %v", e, ok)
	}
}
