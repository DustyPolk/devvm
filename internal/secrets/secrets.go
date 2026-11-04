// Package secrets keeps tokens on the host for VMs with network policy: a
// VM holds a placeholder of its own, and its proxy puts the real value into
// requests for the token's hosts. Values live in the macOS Keychain, or in
// files only the user can read elsewhere. Agents' logins (Codex, opencode,
// Claude Code's MCP servers) stay in their own files on the host, read
// with agentlogin.
package secrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/agentlogin"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// Section is the name of the secrets section in a VM's state.
const Section = "secrets"

// Bound is a VM's secrets section: the secrets bound to it, as asked
// (claude comes with its shared tools).
type Bound struct {
	Names []string `json:"names,omitempty"`
}

// Of returns a VM's secrets section.
func Of(st *state.VM) (Bound, error) { return state.Section[Bound](st, Section) }

// Secret is a token devvm keeps on the host: the hosts it's for, the header
// the proxy puts it in, and the environment variable a VM finds its
// placeholder in. A HostOnly one is devvm's own, which no VM is bound to:
// the company proxy's password (proxy.UpstreamSecret), which devvm's
// proxies send only to the company proxy.
type Secret struct {
	Name     string
	Hosts    []string
	Header   string
	Env      string
	HostOnly bool
	prefix   string // the placeholder's start; a VM's salt ends it
}

// Known are the secrets devvm keeps, by name.
var Known = []Secret{
	{Name: "claude", Hosts: []string{"api.anthropic.com"}, Header: "Authorization", Env: "CLAUDE_CODE_OAUTH_TOKEN", prefix: "sk-ant-oat01-devvm-proxy-managed"},
	{
		Name: "github", Hosts: []string{"api.github.com", "github.com", "codeload.github.com", "raw.githubusercontent.com"},
		Header: "Authorization", Env: "GH_TOKEN", prefix: "devvm-proxy-managed-github",
	},
	{Name: proxy.UpstreamSecret, HostOnly: true},
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
var loginKinds = []string{agentlogin.Codex, agentlogin.Opencode, agentlogin.Claude}

// LoginFile is an agent's login file under home: Codex's and opencode's
// auth.json, and for claude ~/.claude.json (its MCP servers' secrets).
func LoginFile(home, kind string) string {
	switch kind {
	case agentlogin.Codex:
		return filepath.Join(home, ".codex", "auth.json")
	case agentlogin.Opencode:
		return filepath.Join(home, ".local", "share", "opencode", "auth.json")
	}
	return filepath.Join(home, ".claude.json")
}

// Store reads secrets' values and agents' logins. NewStore sets every
// field; tests change them.
type Store struct {
	Dirs paths.Dirs // its Home holds the agents' login files
	// Keychain: values are in the Keychain (KeychainGet), else in files.
	Keychain       bool
	KeychainGet    func(ctx context.Context, service, account string) (string, error)
	KeychainSet    func(ctx context.Context, service, account, label, value string) error
	KeychainDelete func(ctx context.Context, service, account string) error
	Getenv         func(string) string
	// Warn prints a warning.
	Warn func(msg.M)
}

// NewStore returns the store for devvm's folders and the user's home,
// warning through warn.
func NewStore(d paths.Dirs, warn func(msg.M)) *Store {
	return &Store{
		Dirs: d, Keychain: platform.HasKeychain, Getenv: os.Getenv,
		KeychainGet: platform.KeychainGet, KeychainSet: platform.KeychainSet, KeychainDelete: platform.KeychainDelete,
		Warn: warn,
	}
}

// login is an agent's login file in the user's home, its placeholders
// ending in salt.
func (s *Store) login(kind, salt string) agentlogin.File {
	return agentlogin.File{Kind: kind, Path: LoginFile(s.Dirs.Home, kind), Salt: salt}
}

// VMLogin returns the VM's copy of an agent's login file: placeholders
// ending in salt in place of its keys and tokens. It warns about what of
// the file the copy holds as it is.
func (s *Store) VMLogin(kind, salt string) ([]byte, error) {
	data, kept, err := s.login(kind, salt).VMCopy()
	if len(kept) > 0 {
		list := strings.Join(kept, ", ")
		if kind == agentlogin.Codex {
			s.Warn(msg.CodexLoginKept(list))
		} else {
			s.Warn(msg.OpencodeLoginKept(list))
		}
	}
	return data, err
}

// MCPSecrets returns the MCP servers in ~/.claude.json whose settings hold
// secrets; none when it can't be read.
func (s *Store) MCPSecrets() []string {
	servers, err := agentlogin.MCPSecrets(LoginFile(s.Dirs.Home, agentlogin.Claude))
	if err != nil {
		return nil
	}
	return servers
}

// Redact returns an agent's settings file (format json, for JSONC too, or
// toml) with the values under secret-looking keys replaced, and those
// keys' paths. A file it can't read that way is an error.
func Redact(format string, data []byte) ([]byte, []string, error) {
	return agentlogin.Redact(format, data)
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

// ProxyPassword returns the value of name, the secret config.toml's
// company proxy takes its password from (proxy.UpstreamSecret): its
// Value, or proxy_password_missing while it isn't saved. It is
// settings.CompanyProxy's Password.
func (s *Store) ProxyPassword(ctx context.Context, name string) (string, error) {
	v, err := s.Value(ctx, name)
	if errors.Is(err, ErrNone) {
		return "", msg.ProxyPasswordMissing()
	}
	return v, err
}

// Token returns a token as given for the secret (pasted, or a line on
// stdin) without its whitespace. It refuses an empty one, one with a quote,
// a backslash or a control character (they'd end the Keychain's command
// line, or a request's header), and for claude anything but a setup-token
// token. The company proxy's password (proxy.UpstreamSecret) loses only
// the whitespace around it: a password may hold spaces.
func Token(name, given string) (string, error) {
	v := strings.Join(strings.Fields(given), "")
	if name == proxy.UpstreamSecret {
		v = strings.TrimSpace(given)
	}
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
		return msg.NoSecret(name)
	}
	if !s.Keychain {
		if err := state.PrivateDir(s.Dirs.Secrets()); err != nil {
			return err
		}
		return state.WriteFile(s.File(name), []byte(value+"\n"))
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
		// A save that never finished leaves its value in a temporary file
		// (state.WriteFile's, .<name>.tmp-*): gone with the secret.
		tmps, _ := filepath.Glob(filepath.Join(s.Dirs.Secrets(), "."+name+".tmp-*"))
		for _, f := range tmps {
			if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
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

// Get returns a binding's value for the VM's proxy, which runs it in its
// own process (proxy.IO.Secret): a login's (proxy.KindLogin), from its
// agent's file (agentlogin.File.Get, which refreshes an OAuth token when
// it's due), or a known secret's (proxy.KindSecret). Anything else is
// no_secret.
func (s *Store) Get(ctx context.Context, b proxy.Binding) (string, error) {
	switch b.Kind {
	case proxy.KindLogin:
		if !slices.Contains(loginKinds, b.Agent) {
			return "", msg.NoSecret(b.Name)
		}
		f := s.login(b.Agent, "")
		v, err := f.Get(ctx, b.Name)
		if err != nil {
			return "", loginFailed(f.Path, err)
		}
		return strings.TrimSpace(v), nil
	case proxy.KindSecret:
		v, err := s.Value(ctx, b.Name)
		if errors.Is(err, ErrNone) {
			return "", msg.NoSecret(b.Name)
		}
		return v, err
	}
	return "", msg.NoSecret(b.Name)
}

// loginFailed is the message for a login's lookup that failed: what
// logging in again with its agent on this machine fixes, or the login file
// at path that can't be read.
func loginFailed(path string, err error) msg.M {
	var re *agentlogin.RefreshError
	switch {
	case errors.Is(err, agentlogin.ErrCodexNoKey):
		return msg.CodexNoKey().Because(err)
	case errors.Is(err, agentlogin.ErrCodexNoChatGPT):
		return msg.CodexNoChatgpt().Because(err)
	case errors.As(err, &re) && re.Agent == agentlogin.Codex:
		return msg.CodexRefreshFailed(re.Err.Error()).Because(err)
	case errors.As(err, &re):
		return msg.OpencodeRefreshFailed(re.Provider, re.Err.Error()).Because(err)
	}
	return msg.LoginReadFailed(path, err.Error()).Because(err)
}

// Names returns the secrets a VM uses: those bound to it, then claude when
// it has Claude Code.
// A section that can't be read (the folder's VM's is checked as it's
// found) binds none, and a HostOnly secret is never a VM's, whatever its
// section says: its value would reach the VM, or its proxy's bindings.
func Names(st *state.VM) []string {
	b, _ := Of(st)
	names := slices.DeleteFunc(slices.Clone(b.Names), func(n string) bool {
		s, ok := Lookup(n)
		return ok && s.HostOnly
	})
	if FromTools(st, "claude") && !slices.Contains(names, "claude") {
		names = append(names, "claude")
	}
	return names
}

// FromTools reports whether a VM uses the secret because of its shared
// tools: claude, when it has Claude Code.
func FromTools(st *state.VM, name string) bool {
	return name == "claude" && slices.Contains(tools.Names(st), "claude")
}

// Bind binds a secret to a VM (use), or unbinds it, in st alone: inside
// st.Update. claude stays with a VM that has Claude Code.
func Bind(st *state.VM, name string, use bool) error {
	b, err := Of(st)
	if err != nil {
		return err
	}
	b.Names = slices.DeleteFunc(b.Names, func(n string) bool { return n == name })
	if use {
		b.Names = append(b.Names, name)
	}
	return state.SetSection(st, Section, b)
}

// Logins returns the login kinds a VM with these shared tools uses: its
// agents' (codex, opencode, claude) that have a login file on the host.
func (s *Store) Logins(tools []string) []string {
	as := image.AgentsOf(tools)
	var kinds []string
	for _, k := range loginKinds {
		if !slices.Contains(as, k) {
			continue
		}
		if fi, err := os.Stat(LoginFile(s.Dirs.Home, k)); err == nil && fi.Mode().IsRegular() {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

// WriteFiles writes the proxy's view of a VM's secrets: the bindings of
// those with a value saved and of its agents' logins, and the hosts they
// all imply, on 443 only (where the proxy puts values in). The proxy reads
// both again when they change.
func (s *Store) WriteFiles(ctx context.Context, st *state.VM, salt string) error {
	var logins []proxy.Binding
	// A login that can't be read has no bindings.
	for _, kind := range s.Logins(tools.Names(st)) {
		bs, err := s.login(kind, salt).Bindings()
		if err != nil {
			continue
		}
		logins = append(logins, bs...)
	}
	return s.writeFiles(ctx, st, salt, logins)
}

// RefreshFiles writes a VM's secret files again for its secrets as they
// are now, keeping its logins' bindings as WriteFiles last wrote them:
// they come from the login files in the home of the devvm that started
// its proxy.
func (s *Store) RefreshFiles(ctx context.Context, st *state.VM, salt string) error {
	l, err := proxy.ReadBindings(s.Dirs.VM(st.Name()).Secrets())
	if err != nil {
		return err
	}
	logins := slices.DeleteFunc(l.Bindings, func(b proxy.Binding) bool { return b.Kind != proxy.KindLogin })
	return s.writeFiles(ctx, st, salt, logins)
}

// writeFiles writes the VM's secret files: the bindings of its secrets
// with a value saved, then its logins', and the allowlist they imply,
// every bound secret's hosts (saved or not) and its logins', on 443.
func (s *Store) writeFiles(ctx context.Context, st *state.VM, salt string, logins []proxy.Binding) error {
	var list []proxy.Binding
	var hosts []string
	for _, name := range Names(st) {
		sec, ok := Lookup(name)
		if !ok {
			continue
		}
		hosts = append(hosts, sec.Hosts...)
		if _, err := s.Value(ctx, name); errors.Is(err, ErrNone) {
			continue
		} else if err != nil {
			return err
		}
		list = append(list, proxy.Binding{
			Name: name, Kind: proxy.KindSecret, Hosts: sec.Hosts, Header: sec.Header, Placeholder: sec.Placeholder(salt),
		})
	}
	for _, b := range logins {
		hosts = append(hosts, b.Hosts...)
	}
	list = append(list, logins...)
	// The VM's secret bindings, for its proxy (Config.Secrets): names,
	// kinds, hosts, headers and placeholders, never values; and the
	// allowlist they imply.
	files := s.Dirs.VM(st.Name())
	if err := state.PrivateDir(files.Run()); err != nil {
		return err
	}
	if err := state.WriteFile(files.Secrets(), proxy.BindingList{Bindings: list}.Encode()); err != nil {
		return err
	}
	var pol []byte
	for _, l := range proxy.InterceptLines(hosts) {
		pol = append(pol, l+"\n"...)
	}
	return state.WriteFile(files.SecretsPolicy(), pol)
}
