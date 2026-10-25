// Package secrets keeps tokens on the host for VMs with network policy: a
// VM holds a placeholder of its own, and its proxy puts the real value into
// requests for the token's hosts. Values live in the macOS Keychain, or in
// files only the user can read elsewhere. Agents' logins (Codex, opencode,
// Claude Code's MCP servers) stay in their own files on the host, read
// through the proxy's login helper.
package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// KeySecrets is the VM state key listing the secrets bound to it,
// space-separated (claude comes with its image).
const KeySecrets = "secrets"

// Secret is a token devvm keeps on the host: the hosts it's for, the header
// the proxy puts it in, and the environment variable a VM finds its
// placeholder in.
type Secret struct {
	Name   string
	Hosts  []string
	Header string
	Env    string
	prefix string // the placeholder's start; a VM's salt ends it
}

// Known are the secrets devvm keeps, by name.
var Known = []Secret{
	{Name: "claude", Hosts: []string{"api.anthropic.com"}, Header: "Authorization", Env: "CLAUDE_CODE_OAUTH_TOKEN", prefix: "sk-ant-oat01-devvm-proxy-managed"},
	{
		Name: "github", Hosts: []string{"api.github.com", "github.com", "codeload.github.com", "raw.githubusercontent.com"},
		Header: "Authorization", Env: "GH_TOKEN", prefix: "devvm-proxy-managed-github",
	},
}

// Lookup returns the known secret by that name.
func Lookup(name string) (Secret, bool) {
	i := slices.IndexFunc(Known, func(s Secret) bool { return s.Name == name })
	if i < 0 {
		return Secret{}, false
	}
	return Known[i], true
}

// Placeholder is what a VM with that salt gets in place of the secret.
func (s Secret) Placeholder(salt string) string { return s.prefix + "-" + salt }

// ErrNone is Value's error for a secret with no value saved.
var ErrNone = errors.New("no value saved")

// loginKinds are the agents whose login files stay on the host, in order.
var loginKinds = []string{"codex", "opencode", "claude"}

// LoginFile is an agent's login file under home: Codex's and opencode's
// auth.json, and for claude ~/.claude.json (its MCP servers' secrets).
func LoginFile(home, kind string) string {
	switch kind {
	case "codex":
		return filepath.Join(home, ".codex", "auth.json")
	case "opencode":
		return filepath.Join(home, ".local", "share", "opencode", "auth.json")
	}
	return filepath.Join(home, ".claude.json")
}

// Store reads secrets' values and agents' logins. NewStore sets every
// field; tests change them.
type Store struct {
	Dirs paths.Dirs
	Home string
	// Keychain: values are in the Keychain (KeychainGet), else in files.
	Keychain       bool
	KeychainGet    func(ctx context.Context, service, account string) (string, error)
	KeychainSet    func(ctx context.Context, service, account, label, value string) error
	KeychainDelete func(ctx context.Context, service, account string) error
	Getenv         func(string) string
	// Login runs the proxy's login helper (devvm _proxy -login …) with
	// args, its output to out.
	Login func(args []string, out io.Writer) error
	// Warn prints a warning (ui.Warn).
	Warn func(msg.M)
}

// NewStore returns the store for devvm's folders and the user's home.
func NewStore(d paths.Dirs, home string) *Store {
	s := &Store{
		Dirs: d, Home: home, Keychain: platform.HasKeychain, Getenv: os.Getenv,
		KeychainGet: platform.KeychainGet, KeychainSet: platform.KeychainSet, KeychainDelete: platform.KeychainDelete,
		Warn: ui.Warn,
	}
	s.Login = func(args []string, out io.Writer) error {
		kept := func(agent string, names []string) { LoginKept(s.Warn, agent, names) }
		return proxy.Main(args, proxy.IO{Stdin: strings.NewReader(""), Stdout: out, Stderr: io.Discard, Kept: kept})
	}
	return s
}

// LoginKept warns, through warn, about what of an agent's login file goes
// into the VM's copy as it is.
func LoginKept(warn func(msg.M), agent string, names []string) {
	list := strings.Join(names, ", ")
	if agent == "codex" {
		warn(msg.CodexLoginKept(list))
		return
	}
	warn(msg.OpencodeLoginKept(list))
}

// VMLogin returns the VM's copy of an agent's login file: placeholders
// ending in salt in place of its keys and tokens.
func (s *Store) VMLogin(kind, salt string) ([]byte, error) {
	var out bytes.Buffer
	err := s.Login([]string{"-login", kind, "-login-file", LoginFile(s.Home, kind), "-login-vm", "-login-salt", salt}, &out)
	return out.Bytes(), err
}

// MCPSecrets returns the MCP servers in ~/.claude.json whose settings hold
// secrets; none when it can't be read.
func (s *Store) MCPSecrets() []string {
	var out bytes.Buffer
	if s.Login([]string{"-login", "claude", "-login-file", LoginFile(s.Home, "claude"), "-login-secrets"}, &out) != nil {
		return nil
	}
	return lines(out.String())
}

// Redact returns an agent's settings file (format json, for JSONC too, or
// toml) with the values under secret-looking keys replaced, and those
// keys' paths. A file it can't read that way is an error.
func Redact(format string, data []byte) ([]byte, []string, error) {
	var out, paths bytes.Buffer
	if err := proxy.Main([]string{"-redact", format}, proxy.IO{Stdin: bytes.NewReader(data), Stdout: &out, Stderr: &paths}); err != nil {
		return nil, nil, err
	}
	return out.Bytes(), lines(paths.String()), nil
}

// lines returns s's lines that aren't blank, trimmed.
func lines(s string) []string {
	var ls []string
	for l := range strings.Lines(s) {
		if l = strings.TrimSpace(l); l != "" {
			ls = append(ls, l)
		}
	}
	return ls
}

var serviceName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Service is the Keychain service secrets are saved under:
// DEVVM_SECRET_SERVICE, which keeps test secrets apart, else devvm.
func (s *Store) Service() (string, error) {
	svc := s.Getenv("DEVVM_SECRET_SERVICE")
	if svc == "" {
		return "devvm", nil
	}
	if !serviceName.MatchString(svc) {
		return "", msg.BadSecretService()
	}
	return svc, nil
}

// File is where a secret's value is kept without a Keychain.
func (s *Store) File(name string) string { return filepath.Join(s.Dirs.Secrets(), name) }

// Value returns a known secret's value, or ErrNone. A claude token in
// devvm's environment (CLAUDE_CODE_OAUTH_TOKEN) wins: the proxy inherits it
// from the devvm that started it.
func (s *Store) Value(ctx context.Context, name string) (string, error) {
	if _, ok := Lookup(name); !ok {
		return "", ErrNone
	}
	if name == "claude" {
		if v := s.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); v != "" {
			return v, nil
		}
	}
	var v string
	if s.Keychain {
		svc, err := s.Service()
		if err != nil {
			return "", err
		}
		if v, err = s.KeychainGet(ctx, svc, name); errors.Is(err, platform.ErrNoSecret) {
			return "", ErrNone
		} else if err != nil {
			return "", err
		}
	} else {
		b, err := os.ReadFile(s.File(name))
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNone
		} else if err != nil {
			return "", err
		}
		v = string(b)
	}
	if v = strings.TrimSpace(v); v == "" {
		return "", ErrNone
	}
	return v, nil
}

// Token returns a token as given for the secret (pasted, or a line on
// stdin) without its whitespace. It refuses an empty one, one with a quote,
// a backslash or a control character (they'd end the Keychain's command
// line, or a request's header), and for claude anything but a setup-token
// token.
func Token(name, given string) (string, error) {
	v := strings.Join(strings.Fields(given), "")
	switch {
	case v == "":
		return "", msg.NoToken()
	case strings.ContainsFunc(v, func(r rune) bool { return r == '"' || r == '\\' || r < 0x20 || r == 0x7f }):
		return "", msg.TokenQuotes()
	case name == "claude" && !strings.HasPrefix(v, "sk-ant-oat01-"):
		return "", msg.NotSetupToken()
	}
	return v, nil
}

// Save saves a known secret's value (one Token returned): in the Keychain,
// else in a file only the user can read.
func (s *Store) Save(ctx context.Context, name, value string) error {
	if _, ok := Lookup(name); !ok {
		return msg.NoSecret(msg.Shown(name))
	}
	if !s.Keychain {
		if err := os.MkdirAll(s.Dirs.Secrets(), 0o700); err != nil {
			return err
		}
		if err := os.Chmod(s.Dirs.Secrets(), 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
			return err
		}
		return writeFile(s.File(name), []byte(value+"\n"))
	}
	svc, err := s.Service()
	if err != nil {
		return err
	}
	if s.KeychainSet(ctx, svc, name, "devvm "+name, value) != nil {
		return msg.KeychainSaveFailed(name)
	}
	return nil
}

// Delete deletes a known secret's saved value: ErrNone when there's none.
func (s *Store) Delete(ctx context.Context, name string) error {
	if _, ok := Lookup(name); !ok {
		return ErrNone
	}
	if !s.Keychain {
		if err := os.Remove(s.File(name)); errors.Is(err, os.ErrNotExist) {
			return ErrNone
		} else if err != nil {
			return err
		}
		return nil
	}
	svc, err := s.Service()
	if err != nil {
		return err
	}
	if err := s.KeychainDelete(ctx, svc, name); errors.Is(err, platform.ErrNoSecret) {
		return ErrNone
	} else if err != nil {
		return err
	}
	return nil
}

// loginOf returns the login kind a value's name belongs to: codex and
// codex-key are Codex's, opencode-* opencode's, claude-mcp-* Claude Code's
// MCP servers'.
func loginOf(name string) (string, bool) {
	switch {
	case name == "codex" || name == "codex-key":
		return "codex", true
	case strings.HasPrefix(name, "opencode-"):
		return "opencode", true
	case strings.HasPrefix(name, "claude-mcp-"):
		return "claude", true
	}
	return "", false
}

// Get returns the value the proxy asks for by name: a login's, through the
// login helper (which refreshes an OAuth token when it's due), or a known
// secret's. Anything else is no_secret.
func (s *Store) Get(ctx context.Context, name string) (string, error) {
	if kind, ok := loginOf(name); ok {
		var out bytes.Buffer
		if err := s.Login([]string{"-login", kind, "-login-file", LoginFile(s.Home, kind), "-login-get", name}, &out); err != nil {
			return "", msg.ProxyFailed(err.Error())
		}
		return strings.TrimSpace(out.String()), nil
	}
	v, err := s.Value(ctx, name)
	if errors.Is(err, ErrNone) {
		return "", msg.NoSecret(msg.Shown(name))
	}
	return v, err
}

// Names returns the secrets a VM uses: those bound to it, then claude when
// its image has claude.
func Names(st *state.VM) []string {
	names := strings.Fields(st.Get(KeySecrets))
	if FromImage(st, "claude") && !slices.Contains(names, "claude") {
		names = append(names, "claude")
	}
	return names
}

// FromImage reports whether a VM uses the secret because of its image:
// claude, when the image has claude.
func FromImage(st *state.VM, name string) bool {
	return name == "claude" && slices.Contains(agents(st.Get(state.KeyImage)), "claude")
}

// Bind binds a secret to a VM (use), or unbinds it: claude stays with a
// VM whose image has claude.
func Bind(st *state.VM, name string, use bool) error {
	names := slices.DeleteFunc(strings.Fields(st.Get(KeySecrets)), func(n string) bool { return n == name })
	if use {
		names = append(names, name)
	}
	return st.Set(KeySecrets, strings.Join(names, " "))
}

// agents returns the agents an image name holds.
func agents(img string) []string {
	var as []string
	for p := range strings.SplitSeq(img, "+") {
		if image.IsAgent(p) {
			as = append(as, p)
		}
	}
	return as
}

// Logins returns the login kinds a VM with that image uses: its agents'
// (codex, opencode, claude) that have a login file on the host.
func (s *Store) Logins(img string) []string {
	as := agents(img)
	var kinds []string
	for _, k := range loginKinds {
		if !slices.Contains(as, k) {
			continue
		}
		if fi, err := os.Stat(LoginFile(s.Home, k)); err == nil && fi.Mode().IsRegular() {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

// BindingsFile is the VM's secret bindings, for its proxy (-secrets): names,
// hosts, headers and placeholders, never values.
func BindingsFile(d paths.Dirs, vm string) string {
	return filepath.Join(d.Run(), vm+".secrets.json")
}

// PolicyFile is the allowlist the VM's secrets imply: their hosts, on 443.
func PolicyFile(d paths.Dirs, vm string) string {
	return filepath.Join(d.Run(), vm+".secrets.policy")
}

// binding is one entry of the bindings file, as the proxy reads it.
type binding struct {
	Name        string   `json:"name"`
	Hosts       []string `json:"hosts"`
	Header      string   `json:"header"`
	Placeholder string   `json:"placeholder"`
}

// WriteFiles writes the proxy's view of a VM's secrets: the bindings of
// those with a value saved and of its agents' logins, and the hosts they
// all imply, on 443 only (where the proxy puts values in). The proxy reads
// both again when they change.
func (s *Store) WriteFiles(ctx context.Context, st *state.VM, salt string) error {
	var logins []json.RawMessage
	var hosts []string
	// A login the helper can't read has no bindings.
	for _, kind := range s.Logins(st.Get(state.KeyImage)) {
		args := []string{"-login", kind, "-login-file", LoginFile(s.Home, kind), "-login-bindings", "-login-salt", salt}
		var out bytes.Buffer
		var bs []json.RawMessage
		if s.Login(args, &out) == nil && json.Unmarshal(out.Bytes(), &bs) == nil {
			logins = append(logins, bs...)
		}
		out.Reset()
		if s.Login(append(args, "-login-hosts"), &out) == nil {
			hosts = append(hosts, strings.Fields(out.String())...)
		}
	}
	return s.writeFiles(ctx, st, salt, logins, hosts)
}

// RefreshFiles writes a VM's secret files again for its secrets as they
// are now, keeping its logins' bindings as WriteFiles last wrote them:
// they come from the login files in the home of the devvm that started
// its proxy.
func (s *Store) RefreshFiles(ctx context.Context, st *state.VM, salt string) error {
	data, err := os.ReadFile(BindingsFile(s.Dirs, st.Name()))
	if err != nil {
		return err
	}
	var all []json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	var logins []json.RawMessage
	var hosts []string
	for _, raw := range all {
		var b binding
		if json.Unmarshal(raw, &b) != nil {
			continue
		}
		if _, known := Lookup(b.Name); known {
			continue
		}
		logins = append(logins, raw)
		for _, h := range b.Hosts {
			if l := h + ":443"; !slices.Contains(hosts, l) {
				hosts = append(hosts, l)
			}
		}
	}
	return s.writeFiles(ctx, st, salt, logins, hosts)
}

// writeFiles writes the VM's secret files: the bindings of its secrets
// with a value saved, then its logins', and their hosts.
func (s *Store) writeFiles(ctx context.Context, st *state.VM, salt string, logins []json.RawMessage, loginHosts []string) error {
	var list []json.RawMessage
	var hosts []string
	for _, name := range Names(st) {
		sec, ok := Lookup(name)
		if !ok {
			continue
		}
		for _, h := range sec.Hosts {
			hosts = append(hosts, h+":443")
		}
		if _, err := s.Value(ctx, name); errors.Is(err, ErrNone) {
			continue
		} else if err != nil {
			return err
		}
		b, err := json.Marshal(binding{Name: name, Hosts: sec.Hosts, Header: sec.Header, Placeholder: sec.Placeholder(salt)})
		if err != nil {
			return err
		}
		list = append(list, b)
	}
	list, hosts = append(list, logins...), append(hosts, loginHosts...)
	if list == nil {
		list = []json.RawMessage{}
	}
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dirs.Run(), 0o700); err != nil {
		return err
	}
	if err := writeFile(BindingsFile(s.Dirs, st.Name()), append(data, '\n')); err != nil {
		return err
	}
	var pol []byte
	for _, h := range hosts {
		pol = append(pol, h+"\n"...)
	}
	return writeFile(PolicyFile(s.Dirs, st.Name()), pol)
}

// writeFile writes a file whole and owner-only, through a temporary file
// renamed over it.
func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
