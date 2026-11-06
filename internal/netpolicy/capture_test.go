package netpolicy

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/state"
)

// Every VM's proxy captures logins at OpenAI's and Claude's token endpoints, on every tier,
// whatever its image holds: its config names them for Codex, opencode and Claude Code. In a Codex VM, the capture is
// Codex's, whatever sends it, and it saves the login in devvm's store, gives the VM its
// placeholders, and writes the VM's bindings again, so the proxy puts the login in at once.
func TestCapturer(t *testing.T) {
	d, st := sandbox(t, "base", "codex")
	p, _ := testProxy(t, d)
	both := []proxy.Capture{
		{Agent: "codex", Host: "auth.openai.com", Path: "/oauth/token"},
		{Agent: "opencode", Host: "auth.openai.com", Path: "/oauth/token"},
		{Agent: "claude", Host: "platform.claude.com", Path: "/v1/oauth/token"},
		{Agent: "claude", Host: "console.anthropic.com", Path: "/v1/oauth/token"},
	}
	for _, vm := range []*state.VM{st, newVM(t, d, "devvm-plain-0123456789abc4", "base")} {
		for _, tier := range []Tier{TierAllowlist, TierInternet, TierNone, TierOpen} {
			cfg, err := p.config(t.Context(), vm, tier, "k3y", "")
			if err != nil || !slices.Equal(cfg.Captures, both) {
				t.Errorf("%s, %s: captures %+v, %v", vm.Name(), tier, cfg.Captures, err)
			}
		}
	}
	salt, err := Salt(t.Context(), st)
	must(t, err)
	c := Capturer{Dirs: d, Secrets: p.Secrets}
	req := []byte(url.Values{"grant_type": {"authorization_code"}, "code": {"c"}}.Encode())
	x := proxy.Exchange{VM: st.Name(), Agents: []string{"codex", "opencode"}, UserAgent: "Bun/1.2", ContentType: "application/x-www-form-urlencoded", Body: req}
	if b, answer, err := c.Request(t.Context(), x); err != nil || string(b) != string(req) || answer != nil {
		t.Fatalf("Request = %q, %q, %v", b, answer, err)
	}
	// A JWT-shaped token, as OpenAI's are.
	tok := "eyJhbGciOiJub25lIn0.eyJleHAiOjF9.sig"
	reply, _ := json.Marshal(map[string]string{"id_token": tok, "access_token": tok + "-at", "refresh_token": "rt-real"})
	out, agent, err := c.Response(t.Context(), x, reply)
	if err != nil || agent != "codex" || strings.Contains(string(out), "rt-real") || strings.Contains(string(out), tok) {
		t.Fatalf("Response = %s, %q, %v", out, agent, err)
	}
	if b, err := os.ReadFile(secrets.LoginFile(d, "codex")); err != nil || !strings.Contains(string(b), "rt-real") {
		t.Errorf("the store's login: %s, %v", b, err)
	}
	var vm map[string]any
	must(t, json.Unmarshal(out, &vm))
	l, err := proxy.ReadBindings(d.VM(st.Name()).Secrets())
	if err != nil || len(l.Bindings) != 1 || l.Bindings[0].Name != "codex" || l.Bindings[0].Placeholder != vm["access_token"] {
		t.Fatalf("bindings %+v, %v; the VM's access token %v", l.Bindings, err, vm["access_token"])
	}
	// The VM's own placeholder: its claims end in the VM's salt.
	parts := strings.Split(l.Bindings[0].Placeholder, ".")
	if claims, err := base64.RawURLEncoding.DecodeString(parts[1]); err != nil || !strings.Contains(string(claims), salt) {
		t.Errorf("the placeholder's claims %s, %v; want the salt %s", claims, err, salt)
	}
	if pol, _ := os.ReadFile(d.VM(st.Name()).SecretsPolicy()); !strings.Contains(string(pol), "auth.openai.com:443\n") {
		t.Errorf("the VM's allowlist = %q", pol)
	}
	// No VM by that name, or one with no placeholders yet: nothing is captured.
	other := newVM(t, d, vmV, "base", "codex")
	for _, vm := range []string{"devvm-gone-0123456789abc5", other.Name()} {
		x.VM = vm
		if _, _, err := c.Request(t.Context(), x); err == nil {
			t.Errorf("%s: Request went on", vm)
		}
		if _, _, err := c.Response(t.Context(), x, reply); err == nil {
			t.Errorf("%s: Response saved", vm)
		}
	}
	// Claude Code's login, from a VM whose image hasn't Claude Code: nothing is captured, so it
	// can't replace the login every VM with Claude Code uses.
	x.VM, x.Agents, x.ContentType, x.Body = st.Name(), []string{"claude"}, "application/json", []byte(`{"grant_type":"authorization_code","code":"c"}`)
	if _, _, err := c.Request(t.Context(), x); err == nil {
		t.Error("Claude Code's login from a VM without it went on")
	}
	claudeReply := []byte(`{"access_token":"sk-ant-oat01-x","refresh_token":"r","scope":"user:inference"}`)
	if _, _, err := c.Response(t.Context(), x, claudeReply); err == nil {
		t.Error("Claude Code's login from a VM without it was saved")
	}
	cl := newVM(t, d, "devvm-claude-0123456789abc6", "base", "claude")
	_, err = Salt(t.Context(), cl)
	must(t, err)
	x.VM = cl.Name()
	if _, agent, err := c.Response(t.Context(), x, claudeReply); err != nil || agent != "claude" {
		t.Errorf("Claude Code's login from its VM: %q, %v", agent, err)
	}
	// An agent devvm keeps no logins of.
	x.VM, x.Agents = st.Name(), []string{"aider"}
	if _, _, err := c.Request(t.Context(), x); msg.NamesOf(err) != "no_secret secret=aider" {
		t.Errorf("another agent: %v", msg.NamesOf(err))
	}
}
