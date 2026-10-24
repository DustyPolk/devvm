package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, v any) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "auth.json")
	b, _ := json.Marshal(v)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// A fake token endpoint: records the request, answers with new tokens.
func tokenServer(t *testing.T, reply map[string]any) *string {
	t.Helper()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = r.Header.Get("Content-Type") + " " + string(b)
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	old := openaiTokenURL
	openaiTokenURL = srv.URL
	t.Cleanup(func() { openaiTokenURL = old })
	return &got
}

func jwtWith(exp time.Time, extra map[string]any) string {
	c := map[string]any{"exp": exp.Unix()}
	for k, v := range extra {
		c[k] = v
	}
	return fakeJWT(c) + "real-signature"
}

var openaiAuth = map[string]any{"chatgpt_account_id": "acct-1", "chatgpt_plan_type": "pro"}

func TestCodexVMCopyHoldsOnlyPlaceholders(t *testing.T) {
	access := jwtWith(time.Now().Add(time.Hour), map[string]any{"https://api.openai.com/auth": openaiAuth})
	p := writeFile(t, map[string]any{
		"OPENAI_API_KEY": "sk-real",
		"tokens": map[string]any{
			"id_token":     jwtWith(time.Now(), map[string]any{"email": "a@b", "https://api.openai.com/auth": openaiAuth}),
			"access_token": access, "refresh_token": "rt-real", "account_id": "acct-1",
		},
		"last_refresh": "2026-01-01T00:00:00Z", "auth_mode": "chatgpt",
	})
	out, err := loginCmd("codex", p, true, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"sk-real", "rt-real", access} {
		if strings.Contains(out, secret) {
			t.Errorf("the VM's copy holds %q:\n%s", secret, out)
		}
	}
	var m map[string]any
	must(t, json.Unmarshal([]byte(out), &m))
	tk := m["tokens"].(map[string]any)
	// Codex decodes these: the account claims survive, and the access token doesn't expire.
	if c := jwtClaims(tk["id_token"].(string)); c["email"] != "a@b" || c["https://api.openai.com/auth"] == nil {
		t.Errorf("id_token claims = %v", c)
	}
	if exp, ok := jwtExp(tk["access_token"].(string)); !ok || exp.Before(time.Now().AddDate(10, 0, 0)) {
		t.Errorf("access_token exp = %v %v", exp, ok)
	}
	if tk["account_id"] != "acct-1" || m["auth_mode"] != "chatgpt" {
		t.Errorf("other fields changed: %v", m)
	}
	// The bindings name exactly the placeholders in the copy.
	bs, _ := loginCmd("codex", p, false, true, "")
	if !strings.Contains(bs, `"codex-key"`) || !strings.Contains(bs, tk["access_token"].(string)) {
		t.Errorf("bindings = %s", bs)
	}
}

// A tokens object without an access token still has its other tokens scrubbed.
func TestCodexVMScrubsTokensWithoutAnAccessToken(t *testing.T) {
	p := writeFile(t, map[string]any{"tokens": map[string]any{"refresh_token": "rt-real", "id_token": "id-real"}, "extra": "x"})
	out, err := loginCmd("codex", p, true, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "rt-real") || strings.Contains(out, "id-real") {
		t.Errorf("the VM's copy holds a token:\n%s", out)
	}
	if got := codexOthers(map[string]any{"tokens": 1, "extra": "x", "OPENAI_API_KEY": "k"}); !slices.Equal(got, []string{"extra"}) {
		t.Errorf("codexOthers = %v", got)
	}
}

// Values from agents' login files are cached briefly: a logout on the host stops them soon.
func TestLoginValuesCacheBriefly(t *testing.T) {
	for name, want := range map[string]time.Duration{
		"claude": valueTTL, "github": valueTTL, "codex": loginTTL,
		"codex-key": loginTTL, "opencode-anthropic": loginTTL, "claude-mcp-abc": loginTTL,
	} {
		if got := ttlOf(name); got != want {
			t.Errorf("ttlOf(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCodexGetRefreshesOnTheHostWhenDue(t *testing.T) {
	got := tokenServer(t, map[string]any{"access_token": "at-new", "refresh_token": "rt-new", "id_token": "id-new"})
	p := writeFile(t, map[string]any{"tokens": map[string]any{
		"access_token": jwtWith(time.Now().Add(time.Minute), nil), "refresh_token": "rt-old", "id_token": "x",
	}})
	v, err := loginCmd("codex", p, false, false, "codex")
	if err != nil || v != "at-new" {
		t.Fatalf("value = %q, %v", v, err)
	}
	if !strings.HasPrefix(*got, "application/json ") || !strings.Contains(*got, `"refresh_token":"rt-old"`) ||
		!strings.Contains(*got, openaiClientID) {
		t.Errorf("refresh request = %s", *got)
	}
	m, _ := readJSON(p)
	if tk := obj(m, "tokens"); str(tk, "refresh_token") != "rt-new" || str(tk, "access_token") != "at-new" || str(m, "last_refresh") == "" {
		t.Errorf("host file = %v", m)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", st.Mode())
	}
}

func TestCodexGetLeavesAFreshTokenAlone(t *testing.T) {
	got := tokenServer(t, map[string]any{"access_token": "at-new"})
	access := jwtWith(time.Now().Add(time.Hour), nil)
	p := writeFile(t, map[string]any{"tokens": map[string]any{"access_token": access, "refresh_token": "rt"}})
	before, _ := os.ReadFile(p) //nolint:gosec // the test's own file
	if v, err := loginCmd("codex", p, false, false, "codex"); err != nil || v != access {
		t.Fatalf("value = %q, %v", v, err)
	}
	after, _ := os.ReadFile(p) //nolint:gosec // the test's own file
	if *got != "" || string(before) != string(after) {
		t.Errorf("refreshed or rewrote a fresh login: %s", *got)
	}
}

func TestOpencodeLogins(t *testing.T) {
	got := tokenServer(t, map[string]any{"access_token": "at-new", "refresh_token": "rt-new", "expires_in": 7200})
	p := writeFile(t, map[string]any{
		"anthropic":      map[string]any{"type": "api", "key": "sk-ant-real"},
		"openai":         map[string]any{"type": "oauth", "access": jwtWith(time.Now(), map[string]any{"https://api.openai.com/auth": openaiAuth}), "refresh": "rt-old", "expires": time.Now().UnixMilli()},
		"github-copilot": map[string]any{"type": "oauth", "access": "gho_real", "refresh": "gho_real", "expires": 0},
		"llama-cpp":      map[string]any{"type": "api", "key": "local"},
	})
	out, err := loginCmd("opencode", p, true, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"sk-ant-real", "rt-old", "gho_real"} {
		if strings.Contains(out, secret) {
			t.Errorf("the VM's copy holds %q", secret)
		}
	}
	if !strings.Contains(out, `"local"`) {
		t.Errorf("an unknown provider's login should stay as it is:\n%s", out)
	}
	if v, _ := loginCmd("opencode", p, false, false, "opencode-anthropic"); v != "sk-ant-real" {
		t.Errorf("anthropic = %q", v)
	}
	if v, _ := loginCmd("opencode", p, false, false, "opencode-github-copilot"); v != "gho_real" {
		t.Errorf("copilot = %q", v)
	}
	// openai's access token has expired: refreshed here, as a form, and written back.
	if v, err := loginCmd("opencode", p, false, false, "opencode-openai"); err != nil || v != "at-new" {
		t.Fatalf("openai = %q, %v", v, err)
	}
	if !strings.HasPrefix(*got, "application/x-www-form-urlencoded ") || !strings.Contains(*got, "refresh_token=rt-old") {
		t.Errorf("refresh request = %s", *got)
	}
	m, _ := readJSON(p)
	if e := obj(m, "openai"); str(e, "refresh") != "rt-new" || str(e, "access") != "at-new" {
		t.Errorf("host file = %v", e)
	}
	// Logins copied as they are, the note's list: wellknown ones too (they hold a token).
	m["mcp"] = map[string]any{"type": "wellknown", "key": "k", "token": "t"}
	others := opencodeOthers(m)
	if !slices.Contains(others, "llama-cpp") || !slices.Contains(others, "mcp") {
		t.Errorf("others = %v", others)
	}
}

// With -login-salt, every placeholder ends in the VM's key, so two VMs' differ.
func TestLoginSalt(t *testing.T) {
	p := writeFile(t, map[string]any{
		"OPENAI_API_KEY": "sk-real",
		"tokens":         map[string]any{"access_token": jwtWith(time.Now().Add(time.Hour), nil), "refresh_token": "rt"},
	})
	t.Cleanup(func() { loginSalt = "" })
	phs := map[string]bool{}
	for _, salt := range []string{"aaa", "bbb"} {
		loginSalt = salt
		out, err := loginCmd("codex", p, false, true, "")
		if err != nil {
			t.Fatal(err)
		}
		var bs []binding
		must(t, json.Unmarshal([]byte(out), &bs))
		for _, b := range bs {
			if phs[b.Placeholder] {
				t.Errorf("%s: two VMs share placeholder %q", b.Name, b.Placeholder)
			}
			phs[b.Placeholder] = true
		}
		vm, _ := loginCmd("codex", p, true, false, "")
		for _, b := range bs {
			if !strings.Contains(vm, b.Placeholder) {
				t.Errorf("salt %s: the VM's copy lacks %s's placeholder", salt, b.Name)
			}
		}
	}
	if len(phs) != 4 {
		t.Errorf("placeholders: %v", phs)
	}
}

// ~/.claude.json: MCP servers' env values and headers never reach the VM; an HTTPS server's headers
// become bindings for its host, and -login-get returns the real value.
func TestClaudeMCP(t *testing.T) {
	p := writeFile(t, map[string]any{
		"oauthAccount": map[string]any{"emailAddress": "a@b"},
		"projects":     map[string]any{"/x": map[string]any{"mcpServers": map[string]any{"p": map[string]any{"env": map[string]any{"K": "proj-secret"}}}}},
		"mcpServers": map[string]any{
			"remote": map[string]any{"type": "http", "url": "https://mcp.example.com/mcp", "headers": map[string]any{"Authorization": "Bearer remote-secret"}},
			"plain":  map[string]any{"type": "http", "url": "http://mcp.example.net/", "headers": map[string]any{"X-Key": "plain-secret"}},
			"local":  map[string]any{"command": "npx", "args": []any{"srv"}, "env": map[string]any{"API_KEY": "env-secret"}},
			"open":   map[string]any{"command": "srv"},
		},
	})
	vm, err := loginCmd("claude", p, true, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"remote-secret", "plain-secret", "env-secret", "proj-secret", "projects"} {
		if strings.Contains(vm, s) {
			t.Errorf("the VM's copy holds %q:\n%s", s, vm)
		}
	}
	if !strings.Contains(vm, `"a@b"`) || !strings.Contains(vm, `"npx"`) {
		t.Errorf("the VM's copy lost settings:\n%s", vm)
	}
	out, _ := loginCmd("claude", p, false, true, "")
	var bs []binding
	must(t, json.Unmarshal([]byte(out), &bs))
	if len(bs) != 1 || bs[0].Header != "Authorization" || !slices.Equal(bs[0].Hosts, []string{"mcp.example.com"}) || !strings.Contains(vm, bs[0].Placeholder) {
		t.Fatalf("bindings %+v", bs)
	}
	if v, err := loginCmd("claude", p, false, false, bs[0].Name); err != nil || v != "Bearer remote-secret" {
		t.Errorf("get %s: %q %v", bs[0].Name, v, err)
	}
	loginSecrets = true
	t.Cleanup(func() { loginSecrets = false })
	if s, _ := loginCmd("claude", p, false, true, ""); s != "local\nplain\nremote" {
		t.Errorf("servers with secrets: %q", s)
	}
}

// One lookup at a time per login folder: a second waits until the first has written back.
func TestLockLogin(t *testing.T) {
	p := filepath.Join(t.TempDir(), "auth.json")
	unlock, err := lockLogin(p)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan bool)
	go func() {
		u, err := lockLogin(p)
		if err == nil {
			u()
		}
		got <- true
	}()
	select {
	case <-got:
		t.Fatal("a second lock didn't wait")
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	<-got
}

// An MCP server's host goes into the allowlist: only a DNS name, on 443.
func TestClaudeHostIsANameOnly(t *testing.T) {
	for u, want := range map[string]string{
		"https://mcp.example.com/x": "mcp.example.com", "https://MCP.Example.com.:443/": "mcp.example.com",
		"https://1.2.3.4/": "", "https://[::1]/": "", "https://*.example.com/": "", "https://localhost/": "",
		"http://mcp.example.com/": "", "https://mcp.example.com:8443/": "", "https://a;b.com/": "",
	} {
		if got := claudeHost(map[string]any{"url": u}); got != want {
			t.Errorf("claudeHost(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestLoginHostsAreAllowlistLines(t *testing.T) {
	p := writeFile(t, map[string]any{
		"OPENAI_API_KEY": "sk-real",
		"tokens":         map[string]any{"access_token": jwtWith(time.Now().Add(time.Hour), nil)},
	})
	loginHosts = true
	t.Cleanup(func() { loginHosts = false })
	out, err := loginCmd("codex", p, false, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if out != "api.openai.com:443\nchatgpt.com:443" {
		t.Errorf("-login-hosts = %q", out)
	}
}

// With network policy, nothing in the VM's ~/.claude.json carries a secret: not URLs, not
// args, not top-level keys; settings and harmless values stay.
func TestClaudeVMScrubsURLsArgsAndUnknownKeys(t *testing.T) {
	p := writeFile(t, map[string]any{ //nolint:gosec // fake values
		"primaryApiKey": "sk-ant-api03-TOPSECRET", "someFutureKey": "x", "theme": "dark",
		"mcpServers": map[string]any{
			"u": map[string]any{"type": "http", "url": "https://user:DUMMYPASS@mcp.example.com/s/Ab3dEf6hIj9kLm2nOp5qRs8tUv/mcp?api_key=Q1&region=eu&x=zz9Yy8Xx7Ww6Vv5Uu4Tt3Ss2Rr1"}, //nolint:gosec // fake values
			"z": map[string]any{"type": "http", "url": "https://mcp.zapier.com/api/mcp/s/3f2c1b4a-9e8d-4c7b-a6f5-0e1d2c3b4a59/mcp"},
			"a": map[string]any{
				"command": "npx", "args": []any{
					"-y", "@scope/server-x", "--api-key", "DUMMYARG", "--token=DUMMYEQ",
					"--port", "8080", "ghp_DUMMYTOKENVALUE123", "/home/me/data",
				}, "extra": "DUMMYEXTRA",
				"oauth": map[string]any{"clientId": "cid", "clientSecret": "DUMMYCS"},
			},
		},
	})
	out, err := loginCmd("claude", p, true, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"TOPSECRET", "someFutureKey", "DUMMYPASS", "Ab3dEf6hIj9kLm2nOp5qRs8tUv", "Q1", "zz9Yy8Xx7Ww6",
		"3f2c1b4a-9e8d", "DUMMYARG", "DUMMYEQ", "ghp_DUMMY", "DUMMYEXTRA", "DUMMYCS",
	} {
		if strings.Contains(out, s) {
			t.Errorf("the VM's copy holds %q:\n%s", s, out)
		}
	}
	for _, s := range []string{`"theme": "dark"`, "region=eu", "mcp.example.com", "@scope/server-x", `"8080"`, "/home/me/data", `"cid"`, "--token=devvm-redacted"} {
		if !strings.Contains(out, s) {
			t.Errorf("the VM's copy lost %q:\n%s", s, out)
		}
	}
	loginSecrets = true
	t.Cleanup(func() { loginSecrets = false })
	if s, _ := loginCmd("claude", p, false, true, ""); s != "a\nu\nz" {
		t.Errorf("servers with secrets: %q", s)
	}
}
