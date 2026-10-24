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
	Keychain    bool
	KeychainGet func(ctx context.Context, service, account string) (string, error)
	Getenv      func(string) string
	// Login runs the proxy's login helper (devvm _proxy -login …) with
	// args, its output to out.
	Login func(args []string, out io.Writer) error
}

// NewStore returns the store for devvm's folders and the user's home.
func NewStore(d paths.Dirs, home string) *Store {
	return &Store{
		Dirs: d, Home: home, Keychain: platform.HasKeychain, KeychainGet: platform.KeychainGet, Getenv: os.Getenv,
		Login: func(args []string, out io.Writer) error {
			return proxy.Main(args, proxy.IO{Stdin: strings.NewReader(""), Stdout: out, Stderr: io.Discard})
		},
	}
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
	if slices.Contains(agents(st.Get(state.KeyImage)), "claude") && !slices.Contains(names, "claude") {
		names = append(names, "claude")
	}
	return names
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
	// A login the helper can't read has no bindings.
	for _, kind := range s.Logins(st.Get(state.KeyImage)) {
		args := []string{"-login", kind, "-login-file", LoginFile(s.Home, kind), "-login-bindings", "-login-salt", salt}
		var out bytes.Buffer
		var bs []json.RawMessage
		if s.Login(args, &out) == nil && json.Unmarshal(out.Bytes(), &bs) == nil {
			list = append(list, bs...)
		}
		out.Reset()
		if s.Login(append(args, "-login-hosts"), &out) == nil {
			hosts = append(hosts, strings.Fields(out.String())...)
		}
	}
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
