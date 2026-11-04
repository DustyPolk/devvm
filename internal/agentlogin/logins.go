// Package agentlogin is agents' files on the host as VMs with network policy see them. Codex
// (~/.codex/auth.json) and opencode (~/.local/share/opencode/auth.json) keep their logins there, and
// Claude Code its MCP servers' secrets (~/.claude.json). Such a VM gets its own copy of each file
// with placeholders instead of keys and tokens (File.VMCopy), and the proxy puts the real ones
// into requests for each provider's hosts (File.Bindings), looking each value up in its own process
// (File.Get).
// OAuth access tokens are refreshed here, on the host, and written back to the host's file, so the
// host's login keeps working: a VM never holds a refresh token, so it can't rotate one away.
// Agents' settings files get their secret-looking values taken out (Redact).
//
// Formats (checked against upstream, 2026-10): Codex keeps OPENAI_API_KEY or tokens{id_token,
// access_token, refresh_token, account_id} plus last_refresh; it decodes id_token's claims and
// access_token's exp (never the signatures) and refreshes 5 minutes before exp. opencode keeps one
// entry per provider: {"type":"api","key"} or {"type":"oauth","access","refresh","expires" (ms)}.
package agentlogin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/w3cj/devvm/internal/fetch"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
)

const (
	// OpenAITokenURL is OpenAI's OAuth token endpoint, which refreshes Codex's and opencode's
	// ChatGPT logins, and openaiClientID Codex's public client ID (opencode uses the same one).
	OpenAITokenURL = "https://auth.openai.com/oauth/token" //nolint:gosec // a URL, not a secret
	openaiClientID = "app_EMoamEEZ73f0CkXaXp7hrann"        //nolint:gosec // a public client ID, not a secret
	// Refresh this long before an access token expires: the proxy caches values for a while.
	refreshMargin = 10 * time.Minute
	// How long a token refresh may take.
	refreshTimeout = 30 * time.Second
	// The most of a token endpoint's reply read.
	replyMax = 1 << 20
	// farFuture is 2099-01-01 UTC, in Unix seconds: when the VM's placeholders expire.
	farFuture = 4070908800
)

// The agents whose files a File is.
const (
	Codex    = "codex"
	Opencode = "opencode"
	Claude   = "claude"
)

// The errors of Get that logging in again with the agent on the host fixes.
var (
	// ErrCodexNoKey: Codex's login on the host has no API key.
	ErrCodexNoKey = errors.New("codex isn't logged in with an API key")
	// ErrCodexNoChatGPT: Codex's login on the host has no ChatGPT login.
	ErrCodexNoChatGPT = errors.New("codex isn't logged in with ChatGPT")
)

// RefreshError is Get's error for an OAuth login whose refresh on the host failed: Agent's (Codex
// or Opencode), and for opencode its Provider's. Logging in again with the agent fixes it.
type RefreshError struct {
	Agent, Provider string
	Err             error
}

func (e *RefreshError) Error() string { return "refreshing the login: " + e.Err.Error() }
func (e *RefreshError) Unwrap() error { return e.Err }

// File is one agent's file on the host.
type File struct {
	Kind string // Codex, Opencode or Claude
	Path string
	// Salt is the VM's key, which ends every placeholder, so each VM's are its own: a proxy puts a
	// value in only for the placeholders bound for its VM. Empty: none.
	Salt string
	// TokenURL is where OAuth tokens are refreshed: OpenAITokenURL when empty.
	TokenURL string
}

// salted: a placeholder made the VM's own.
func salted(ph, salt string) string {
	if salt == "" {
		return ph
	}
	return ph + "-" + salt
}

// opencode providers devvm knows: where an API key or access token goes. Logins for other providers
// stay in the VM's copy as they are (a local model server's key, say).
type provider struct {
	hosts  []string
	header string
}

var opencodeAPI = map[string]provider{
	"anthropic":  {[]string{"api.anthropic.com"}, "x-api-key"},
	"openai":     {[]string{"api.openai.com"}, "Authorization"},
	"openrouter": {[]string{"openrouter.ai"}, "Authorization"},
}

var opencodeOAuth = map[string]provider{
	"openai":         {[]string{"chatgpt.com"}, "Authorization"},
	"github-copilot": {[]string{"api.githubcopilot.com"}, "Authorization"},
}

// fileMax is the most of an agent's file read: ~/.claude.json keeps every project's history, so it
// grows.
const fileMax = 64 << 20

// readJSON reads an agent's file: a regular file (proxy.ReadRegular, so a FIFO or a device there
// can't hold devvm), at most fileMax, holding a JSON object.
func readJSON(path string) (map[string]any, error) {
	data, err := proxy.ReadRegular(path, fileMax)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// lockLogin holds an exclusive lock on the login file's folder while a lookup reads, refreshes and
// writes it back: VMs each have a proxy, and two refreshing with the same (rotating) refresh token
// would lose one result or get the login revoked. It returns the unlock.
func lockLogin(path string) (func(), error) {
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(d.Fd()), syscall.LOCK_EX); err != nil {
		_ = d.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(d.Fd()), syscall.LOCK_UN); _ = d.Close() }, nil
}

// writeJSON replaces path atomically, keeping it mode 600.
func writeJSON(path string, m map[string]any) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFile(path, append(data, '\n'))
}

// writeRefreshed writes a login just refreshed: the old refresh token may no longer work, so a
// failed write (a full disk for a moment, say) is tried again before giving up.
func writeRefreshed(path string, m map[string]any) error {
	var err error
	for i := 0; i < 3; i++ {
		if err = writeJSON(path, m); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func obj(m map[string]any, k string) map[string]any {
	o, _ := m[k].(map[string]any)
	return o
}

// --- JWTs (never verified: only read for claims, or made up as placeholders) ---

func jwtClaims(tok string) map[string]any {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var c map[string]any
	if json.Unmarshal(raw, &c) != nil {
		return nil
	}
	return c
}

func jwtExp(tok string) (time.Time, bool) {
	exp, ok := jwtClaims(tok)["exp"].(float64)
	return time.Unix(int64(exp), 0), ok
}

// fakeJWT: a JWT-shaped placeholder with these claims and no real signature. Deterministic, so the
// same claims give the same placeholder (bindings name it).
func fakeJWT(claims map[string]any) string {
	enc := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	return enc(map[string]string{"alg": "none", "typ": "JWT"}) + "." + enc(claims) + ".devvm-placeholder"
}

// The account claims Codex and opencode read from tokens, without anything that grants access.
func accountClaims(tok, salt string) map[string]any {
	c := jwtClaims(tok)
	out := map[string]any{"exp": farFuture, "devvm": salted("placeholder", salt)}
	if a, ok := c["https://api.openai.com/auth"]; ok {
		out["https://api.openai.com/auth"] = a
	}
	for _, k := range []string{"email", "https://api.openai.com/profile", "chatgpt_account_id"} {
		if v, ok := c[k]; ok {
			out[k] = v
		}
	}
	return out
}

// --- Codex ---

// login is one of agent's logins as a binding, of proxy.KindLogin: its value is File.Get's for name.
func login(agent, name string, hosts []string, header, placeholder string) proxy.Binding {
	return proxy.Binding{Name: name, Kind: proxy.KindLogin, Agent: agent, Hosts: hosts, Header: header, Placeholder: placeholder}
}

func codexBindings(m map[string]any, salt string) []proxy.Binding {
	var bs []proxy.Binding
	if str(m, "OPENAI_API_KEY") != "" {
		bs = append(bs, login(Codex, "codex-key", []string{"api.openai.com"}, "Authorization", salted("sk-devvm-proxy-managed-codex", salt)))
	}
	if t := obj(m, "tokens"); str(t, "access_token") != "" {
		bs = append(bs, login(Codex, "codex", []string{"chatgpt.com", "api.openai.com"}, "Authorization",
			fakeJWT(accountClaims(str(t, "access_token"), salt))))
	}
	return bs
}

// codexKnown: auth.json's keys codexVM knows. Others are copied as they are, and named.
var codexKnown = map[string]bool{"OPENAI_API_KEY": true, "tokens": true, "last_refresh": true, "auth_mode": true}

func codexOthers(m map[string]any) []string {
	var others []string
	for k := range m {
		if !codexKnown[k] {
			others = append(others, k)
		}
	}
	sort.Strings(others)
	return others
}

func codexVM(m map[string]any, salt string) map[string]any {
	bs := codexBindings(m, salt)
	t := obj(m, "tokens")
	idClaims := accountClaims(str(t, "id_token"), salt)
	// Every token goes, bound or not (a login without an access token still has the others).
	for _, k := range []string{"id_token", "refresh_token", "access_token"} {
		if str(t, k) != "" {
			t[k] = "devvm-proxy-managed"
		}
	}
	for _, b := range bs {
		switch b.Name {
		case "codex-key":
			m["OPENAI_API_KEY"] = b.Placeholder
		case "codex":
			t["id_token"] = fakeJWT(idClaims)
			t["access_token"] = b.Placeholder
			// Just refreshed, as far as the VM's Codex can tell: it never tries.
			m["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
		}
	}
	return m
}

func (f File) codexGet(ctx context.Context, name string) (string, error) {
	unlock, err := lockLogin(f.Path)
	if err != nil {
		return "", err
	}
	defer unlock()
	m, err := readJSON(f.Path)
	if err != nil {
		return "", err
	}
	switch name {
	case "codex-key":
		if k := str(m, "OPENAI_API_KEY"); k != "" {
			return k, nil
		}
		return "", ErrCodexNoKey
	case "codex":
		t := obj(m, "tokens")
		if str(t, "access_token") == "" {
			return "", ErrCodexNoChatGPT
		}
		if exp, ok := jwtExp(str(t, "access_token")); ok && time.Until(exp) > refreshMargin {
			return str(t, "access_token"), nil
		}
		nt, err := f.refresh(ctx, false, map[string]string{
			"grant_type": "refresh_token", "client_id": openaiClientID, "refresh_token": str(t, "refresh_token"),
		})
		if err != nil {
			return "", &RefreshError{Agent: Codex, Err: err}
		}
		for _, k := range []string{"id_token", "access_token", "refresh_token"} {
			if v := str(nt, k); v != "" {
				t[k] = v
			}
		}
		m["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
		return str(t, "access_token"), writeRefreshed(f.Path, m)
	}
	return "", fmt.Errorf("no Codex login %q", name)
}

// --- opencode ---

func opencodeBindings(m map[string]any, salt string) []proxy.Binding {
	var bs []proxy.Binding
	for id := range m {
		e := obj(m, id)
		var p provider
		var ok bool
		switch str(e, "type") {
		case "api":
			p, ok = opencodeAPI[id]
		case "oauth":
			p, ok = opencodeOAuth[id]
			ok = ok && str(e, "enterpriseUrl") == ""
		}
		if !ok {
			continue
		}
		ph := salted("devvm-proxy-managed-opencode-"+id, salt)
		if id == "openai" && str(e, "type") == "oauth" {
			ph = fakeJWT(accountClaims(str(e, "access"), salt)) // opencode reads the account from it
		}
		bs = append(bs, login(Opencode, "opencode-"+id, p.hosts, p.header, ph))
	}
	return bs
}

func opencodeVM(m map[string]any, salt string) map[string]any {
	for _, b := range opencodeBindings(m, salt) {
		e := obj(m, strings.TrimPrefix(b.Name, "opencode-"))
		if str(e, "type") == "api" {
			e["key"] = b.Placeholder
			continue
		}
		e["access"] = b.Placeholder
		e["refresh"] = b.Placeholder // Copilot sends this one; nothing else in the VM refreshes
		if n, _ := e["expires"].(json.Number); n.String() != "0" {
			e["expires"] = farFuture * 1000
		}
	}
	return m
}

// Providers whose logins the VM's copy still holds as they are, sorted.
func opencodeOthers(m map[string]any) []string {
	known := map[string]bool{}
	for _, b := range opencodeBindings(m, "") {
		known[strings.TrimPrefix(b.Name, "opencode-")] = true
	}
	var out []string
	for id := range m {
		if !known[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (f File) opencodeGet(ctx context.Context, name string) (string, error) {
	unlock, err := lockLogin(f.Path)
	if err != nil {
		return "", err
	}
	defer unlock()
	m, err := readJSON(f.Path)
	if err != nil {
		return "", err
	}
	id := strings.TrimPrefix(name, "opencode-")
	e := obj(m, id)
	switch str(e, "type") {
	case "api":
		return str(e, "key"), nil
	case "oauth":
		if id == "github-copilot" {
			return str(e, "refresh"), nil // a GitHub token, never refreshed
		}
		exp, _ := e["expires"].(json.Number)
		ms, _ := exp.Int64()
		if str(e, "access") != "" && time.Until(time.UnixMilli(ms)) > refreshMargin {
			return str(e, "access"), nil
		}
		nt, err := f.refresh(ctx, true, map[string]string{
			"grant_type": "refresh_token", "client_id": openaiClientID, "refresh_token": str(e, "refresh"),
		})
		if err != nil {
			return "", &RefreshError{Agent: Opencode, Provider: id, Err: err}
		}
		e["access"] = str(nt, "access_token")
		if v := str(nt, "refresh_token"); v != "" {
			e["refresh"] = v
		}
		secs := 3600.0
		if v, ok := nt["expires_in"].(float64); ok {
			secs = v
		}
		e["expires"] = time.Now().Add(time.Duration(secs) * time.Second).UnixMilli()
		return str(e, "access"), writeRefreshed(f.Path, m)
	}
	return "", fmt.Errorf("no opencode login %q", id)
}

// refresh posts an OAuth refresh request to f.TokenURL, as JSON (Codex) or a form (opencode),
// within refreshTimeout. Its caller's end doesn't cancel it: the server may already have rotated
// the refresh token, and only the reply, written back to the login file, holds the new one.
func (f File) refresh(ctx context.Context, form bool, fields map[string]string) (map[string]any, error) {
	u := f.TokenURL
	if u == "" {
		u = OpenAITokenURL
	}
	var body []byte
	ctype := "application/json"
	if form {
		v := url.Values{}
		for k, f := range fields {
			v.Set(k, f)
		}
		body, ctype = []byte(v.Encode()), "application/x-www-form-urlencoded"
	} else {
		body, _ = json.Marshal(fields)
	}
	req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ctype)
	resp, err := fetch.NewClient(refreshTimeout, fetch.HTTPSOnly).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &fetch.StatusError{URL: u, Code: resp.StatusCode, Status: resp.Status}
	}
	data, err := fetch.ReadCapped(resp.Body, replyMax)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if str(out, "access_token") == "" {
		return nil, errors.New("no access_token in the reply")
	}
	return out, nil
}

// --- Claude Code: MCP servers' secrets in ~/.claude.json ---
//
// `claude mcp add -s user` keeps a server's env values (stdio servers) and HTTP headers (remote
// servers) in ~/.claude.json, in plain text. The VM's copy gets placeholders instead. An HTTPS
// server's header becomes a binding for its host, so the proxy puts the real value in (read from
// the host's file each time); env values can't be put in by a proxy, so stdio servers that need them
// don't get them. Per-project entries (.projects) never go to VMs.

// The MCP servers, sorted by name, and each one's settings.
func claudeServers(m map[string]any) ([]string, map[string]any) {
	servers := obj(m, "mcpServers")
	names := make([]string, 0, len(servers))
	for n := range servers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, servers
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// claudeBindingName: a binding's name from its server and header (names are free text; this is
// safe in the proxy's log and as its cache's key).
func claudeBindingName(server, header string) string {
	h := sha256.Sum256([]byte(server + "\x00" + strings.ToLower(header)))
	return "claude-mcp-" + hex.EncodeToString(h[:6])
}

// claudeHost: the host of an HTTPS server on the default port (where the proxy intercepts), or "".
func claudeHost(s map[string]any) string {
	u, err := url.Parse(str(s, "url"))
	if err != nil || u.Scheme != "https" || (u.Port() != "" && u.Port() != "443") {
		return ""
	}
	// A name only: it goes into the VM's allowlist, where an address, a wildcard or a range would
	// open more than the one server.
	h := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if _, err := netip.ParseAddr(h); err != nil && proxy.HostOK(h) && strings.Contains(h, ".") {
		return h
	}
	return ""
}

func claudeBindings(m map[string]any, salt string) []proxy.Binding {
	var bs []proxy.Binding
	names, servers := claudeServers(m)
	for _, n := range names {
		s := obj(servers, n)
		host := claudeHost(s)
		if host == "" {
			continue
		}
		for _, h := range sortedKeys(obj(s, "headers")) {
			name := claudeBindingName(n, h)
			bs = append(bs, login(Claude, name, []string{host}, h, salted("devvm-proxy-managed-"+name, salt)))
		}
	}
	return bs
}

// claudeSecrets: the servers whose settings hold values the VM's copy replaces.
func claudeSecrets(m map[string]any) []string {
	var out []string
	names, servers := claudeServers(m)
	for _, n := range names {
		s := obj(servers, n)
		if len(obj(s, "env")) > 0 || len(obj(s, "headers")) > 0 || scrubURL(str(s, "url")) != str(s, "url") ||
			argsHoldSecrets(s["args"]) {
			out = append(out, n)
		}
	}
	return out
}

// What of ~/.claude.json the VM's copy keeps: settings, onboarding and account state (not
// credentials), and MCP servers. Anything else (primaryApiKey, keys a later Claude Code adds) stays
// on the host; the VM keeps its own value of it.
var claudeTopKeys = map[string]bool{
	"mcpServers": true, "hasCompletedOnboarding": true, "lastOnboardingVersion": true, "theme": true,
	"editorMode": true, "verbose": true, "autoCompactEnabled": true, "preferredNotifChannel": true,
	"messageIdleNotifThresholdMs": true, "shiftEnterKeyBindingInstalled": true, "optionAsMetaKeyInstalled": true,
	"hasAcknowledgedCostThreshold": true, "bypassPermissionsModeAccepted": true, "hasSeenTasksHint": true,
	"hasUsedBackslashReturn": true, "oauthAccount": true, "userID": true, "numStartups": true,
	"firstStartTime": true, "tipsHistory": true, "isQualifiedForDataSharing": true, "hasIdeOnboardingBeenShown": true,
	"lastReleaseNotesSeen": true, "subscriptionNoticeCount": true, "hasAvailableSubscription": true,
	"diffTool": true, "todoFeatureEnabled": true, "showExpandedTodos": true, "autoConnectIde": true,
	"autoInstallIdeExtension": true, "fallbackAvailableWarningThreshold": true,
}

// An MCP server's settings the VM's copy keeps (oauth: only its client ID and callback port).
var claudeServerKeys = map[string]bool{
	"type": true, "url": true, "command": true, "args": true, "env": true,
	"headers": true, "headersHelper": true, "oauth": true,
}

// Secret-looking names (query parameters, flags) and values (known key shapes, long random strings).
var (
	secretName  = regexp.MustCompile(`(?i)(key|token|secret|passw|pwd|auth|bearer|credential|session|sig)`)
	secretShape = regexp.MustCompile(`^(sk-|sk_|pk_|rk_|ghp_|gho_|ghs_|ghu_|github_pat_|glpat-|xox[abpr]-|AKIA|ASIA|AIza|ya29\.|eyJ)`)
	uuidShape   = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	tokenShape  = regexp.MustCompile(`^[A-Za-z0-9_\-+=~]{24,}$`)
)

// secretLike: v looks like a key or token: a known prefix, a UUID, or 24+ token characters mixing
// letters and digits.
func secretLike(v string) bool {
	if secretShape.MatchString(v) || uuidShape.MatchString(v) {
		return true
	}
	return tokenShape.MatchString(v) && strings.ContainsAny(v, "0123456789") &&
		strings.ContainsAny(strings.ToLower(v), "abcdefghijklmnopqrstuvwxyz")
}

// scrubURL: the URL without user info, and with secret-looking query values and path segments
// replaced (the VM's copy can't have a placeholder there: the proxy only fills in headers).
func scrubURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return raw
	}
	changed := false
	if u.User != nil {
		u.User = nil
		changed = true
	}
	if u.RawQuery != "" {
		q := u.Query()
		for k, vs := range q {
			for i, v := range vs {
				if v != "" && (secretName.MatchString(k) || secretLike(v)) {
					vs[i] = "devvm-redacted"
					changed = true
				}
			}
			q[k] = vs
		}
		u.RawQuery = q.Encode()
	}
	segs := strings.Split(u.Path, "/")
	for i, s := range segs {
		if secretLike(s) {
			segs[i] = "devvm-redacted"
			changed = true
		}
	}
	if !changed {
		return raw
	}
	u.Path, u.RawPath = strings.Join(segs, "/"), ""
	return u.String()
}

// scrubArgs: a server's arguments with secret values replaced: after a secret-named flag
// (--api-key X), in --flag=X with one, and any that look like a key.
func scrubArgs(v any) (any, bool) {
	as, ok := v.([]any)
	if !ok {
		return v, false
	}
	out, changed := make([]any, len(as)), false
	for i, a := range as {
		out[i] = a
		s, ok := a.(string)
		if !ok {
			continue
		}
		var prev string
		if i > 0 {
			prev, _ = as[i-1].(string)
		}
		switch {
		case strings.HasPrefix(prev, "-") && !strings.Contains(prev, "=") && secretName.MatchString(prev) && !strings.HasPrefix(s, "-"):
			out[i], changed = "devvm-redacted", true
		case strings.HasPrefix(s, "-") && strings.Contains(s, "="):
			if k, val, _ := strings.Cut(s, "="); val != "" && (secretName.MatchString(k) || secretLike(val)) {
				out[i], changed = k+"=devvm-redacted", true
			}
		case secretLike(s):
			out[i], changed = "devvm-redacted", true
		}
	}
	return out, changed
}

func argsHoldSecrets(v any) bool { _, changed := scrubArgs(v); return changed }

func claudeVM(m map[string]any, salt string) map[string]any {
	for k := range m {
		if !claudeTopKeys[k] {
			delete(m, k)
		}
	}
	names, servers := claudeServers(m)
	for _, n := range names {
		s := obj(servers, n)
		if s == nil {
			delete(servers, n)
			continue
		}
		for k := range s {
			if !claudeServerKeys[k] {
				delete(s, k)
			}
		}
		if o := obj(s, "oauth"); o != nil {
			for k := range o {
				if k != "clientId" && k != "callbackPort" {
					delete(o, k)
				}
			}
		}
		if u, ok := s["url"].(string); ok {
			s["url"] = scrubURL(u)
		}
		if a, ok := s["args"]; ok {
			s["args"], _ = scrubArgs(a)
		}
		for k := range obj(s, "env") {
			obj(s, "env")[k] = "devvm-redacted"
		}
		https := claudeHost(s) != ""
		for _, h := range sortedKeys(obj(s, "headers")) {
			if https {
				obj(s, "headers")[h] = salted("devvm-proxy-managed-"+claudeBindingName(n, h), salt)
			} else {
				obj(s, "headers")[h] = "devvm-redacted"
			}
		}
	}
	return m
}

func (f File) claudeGet(name string) (string, error) {
	m, err := readJSON(f.Path)
	if err != nil {
		return "", err
	}
	names, servers := claudeServers(m)
	for _, n := range names {
		hs := obj(obj(servers, n), "headers")
		for _, h := range sortedKeys(hs) {
			if claudeBindingName(n, h) == name {
				return str(hs, h), nil
			}
		}
	}
	return "", fmt.Errorf("no MCP server header %s in %s", name, f.Path)
}

// MCPSecrets returns the MCP servers in a ~/.claude.json at path whose settings hold values the
// VM's copy replaces (env values, headers, secret-looking URLs and arguments), sorted.
func MCPSecrets(path string) ([]string, error) {
	m, err := readJSON(path)
	if err != nil {
		return nil, err
	}
	return claudeSecrets(m), nil
}

// errKind is a File of no agent devvm knows.
func (f File) errKind() error { return fmt.Errorf("unknown login kind %q", f.Kind) }

// Bindings returns the login's secret bindings, of proxy.KindLogin with f.Kind their Agent, each
// placeholder ending in f.Salt; none for a file that holds no login devvm knows.
func (f File) Bindings() ([]proxy.Binding, error) {
	m, err := readJSON(f.Path)
	if err != nil {
		return nil, err
	}
	switch f.Kind {
	case Codex:
		return codexBindings(m, f.Salt), nil
	case Opencode:
		return opencodeBindings(m, f.Salt), nil
	case Claude:
		return claudeBindings(m, f.Salt), nil
	}
	return nil, f.errKind()
}

// VMCopy returns the VM's copy of the file, with placeholders ending in f.Salt in place of its keys
// and tokens, and what of it the copy holds as it is: Codex's auth.json keys or opencode's
// providers devvm doesn't know, sorted.
func (f File) VMCopy() (copied []byte, kept []string, err error) {
	m, err := readJSON(f.Path)
	if err != nil {
		return nil, nil, err
	}
	switch f.Kind {
	case Codex:
		kept = codexOthers(m)
		m = codexVM(m, f.Salt)
	case Opencode:
		kept = opencodeOthers(m)
		m = opencodeVM(m, f.Salt)
	case Claude:
		m = claudeVM(m, f.Salt)
	default:
		return nil, nil, f.errKind()
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return append(b, '\n'), kept, nil
}

// Get returns the value of the binding named name: from the host's file, read each time, refreshing
// an OAuth token there when it's due, under the file's lock.
func (f File) Get(ctx context.Context, name string) (string, error) {
	switch f.Kind {
	case Codex:
		return f.codexGet(ctx, name)
	case Opencode:
		return f.opencodeGet(ctx, name)
	case Claude:
		return f.claudeGet(name)
	}
	return "", f.errKind()
}
