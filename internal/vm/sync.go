package vm

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// syncCmd unpacks what the host sends into a folder of its own in the VM,
// runs its sync.sh, and removes it.
const syncCmd = `d=$(mktemp -d); tar -xf - -C "$d" --warning=no-unknown-keyword --warning=no-timestamp && bash "$d/sync.sh"; rc=$?; rm -rf "$d"; exit $rc`

// syncer sends a running VM its setup.
type syncer struct{ *Env }

// sync sets up a running VM on every launch: devvm's guest/sync.sh runs in it,
// with what it needs sent over as a tar stream, staged in memory (it holds
// credentials): the host's settings, the agents' config, the VM's tokens
// or their placeholders, and its shared tools. With network policy its
// proxy's address, devvm's CA, the logins' copies with placeholders, and
// the agents' settings files with their keys taken out.
func (e syncer) sync(ctx context.Context, b backend.Backend, st *state.VM, slug string, files []string, conf backend.Confinement) error {
	vm := st.Name()
	s, err := e.stage(ctx, st, slug, files, b.ProxyURL(conf))
	if err != nil {
		return err
	}
	data, err := s.tar()
	if err != nil {
		return err
	}
	out := e.LogWriter()
	err = b.Exec(ctx, vm, []string{"bash", "-c", syncCmd}, backend.ExecOpts{Stdin: bytes.NewReader(data), Stdout: out, Stderr: out})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return backend.Logged(msg.SyncFailed(vm))
	}
	return nil
}

// stage returns what sync sends; see devvm's guest/sync.sh for each file.
func (e syncer) stage(ctx context.Context, st *state.VM, slug string, files []string, proxyURL string) (*stage, error) {
	vm, img, net := st.Name(), st.Get(state.KeyImage), netpolicy.On(st)
	store := e.Proxy.Secrets
	s := &stage{}
	for _, name := range []string{"sync.sh", "tools.sh"} {
		data, err := fs.ReadFile(e.Assets, "guest/"+name)
		if err != nil {
			return nil, err
		}
		s.put(name, 0o755, data)
	}
	s.put("tools", 0o644, tools.Mounts(files))
	s.put("host_home", 0o644, []byte(e.Home+"\n"))

	// The host's settings, for every VM.
	lang := e.Getenv("LC_ALL")
	if lang == "" {
		lang = e.Getenv("LANG")
	}
	if !strings.HasSuffix(lang, ".UTF-8") {
		lang = ""
	}
	tz := e.Getenv("TZ")
	if tz == "" {
		tz = e.HostTZ()
	}
	vars := [][2]string{
		{"DEVVM_HOSTNAME", slug},
		{"DEVVM_TZ", tz},
		{"DEVVM_LANG", lang},
		{"DEVVM_GIT_NAME", e.GitConfig(ctx, st.Get(state.KeyDir), "user.name")},
		{"DEVVM_GIT_EMAIL", e.GitConfig(ctx, st.Get(state.KeyDir), "user.email")},
		{"DEVVM_PROXY", proxyURL},
	}
	// The devcontainer.json's variables first: devvm's own win.
	dcEnv, err := devcontainerEnv(st)
	if err != nil {
		return nil, err
	}
	var env bytes.Buffer
	env.WriteString(dcEnv)
	if lang != "" {
		env.WriteString("export LANG=" + shellQuote(lang) + "\n")
	}

	// The agents' config: folders to link to their mounts, files copied.
	// Never their history.
	agents := agentsOf(img)
	var links bytes.Buffer
	for _, a := range agents {
		for _, rel := range agentDirs(e.Home, a) {
			links.WriteString(rel + "\n")
		}
		for _, rel := range agentFiles(e.Home, a) {
			if err := s.copy("home/"+rel, filepath.Join(e.Home, rel)); err != nil {
				return nil, err
			}
		}
	}
	if slices.Contains(agents, "claude") {
		if err := s.copy("claude.json", secrets.LoginFile(e.Home, "claude")); err != nil {
			return nil, err
		}
		// With network policy the copy holds placeholders (below).
		if s.has("claude.json") && !net {
			if servers := store.MCPSecrets(); len(servers) > 0 {
				e.Warn(msg.ClaudeMcpSecretsCopied(strings.Join(servers, " "), vm))
			}
		}
	}

	// Tokens: a placeholder when the proxy puts the value in, else the
	// value itself.
	var salt string
	if net {
		var err error
		if salt, err = netpolicy.Salt(st); err != nil {
			return nil, err
		}
	}
	for _, name := range secrets.Names(st) {
		sec, ok := secrets.Lookup(name)
		if !ok {
			continue
		}
		v, err := store.Value(ctx, name)
		if errors.Is(err, secrets.ErrNone) {
			switch name {
			case "claude":
				e.Warn(msg.NoClaudeToken(vm))
			case "github":
				e.Warn(msg.NoGithubToken(vm))
			}
			continue
		} else if err != nil {
			return nil, err
		}
		if name == "claude" && !strings.HasPrefix(v, "sk-ant-oat01-") {
			e.Warn(msg.ClaudeTokenInvalid())
			continue
		}
		if net {
			v = sec.Placeholder(salt)
		}
		env.WriteString("export " + sec.Env + "=" + shellQuote(v) + "\n")
		if name == "github" {
			vars = append(vars, [2]string{"DEVVM_GITHUB", "1"})
		}
	}

	if net {
		if err := e.stageNet(s, store, vm, img, salt); err != nil {
			return nil, err
		}
	}
	var vmFile bytes.Buffer
	for _, kv := range vars {
		vmFile.WriteString(kv[0] + "=" + shellQuote(kv[1]) + "\n")
	}
	s.put("vm", 0o644, vmFile.Bytes())
	s.put("env", 0o600, env.Bytes())
	s.put("links", 0o644, links.Bytes())
	return s, nil
}

// stageNet adds what a VM with network policy gets: devvm's CA, its
// logins' copies with placeholders ending in its salt, and its agents'
// settings files with the values under secret-looking keys taken out (one
// that can't be read that way isn't sent).
func (e syncer) stageNet(s *stage, store *secrets.Store, vm, img, salt string) error {
	ca, err := os.ReadFile(filepath.Join(e.Dirs.CA(), "ca.pem"))
	if err != nil {
		return err
	}
	s.put("ca.pem", 0o644, ca)
	for _, kind := range store.Logins(img) {
		data, err := store.VMLogin(kind, salt)
		if err != nil {
			return msg.LoginCopyFailed(kind, err.Error(), vm).Because(err)
		}
		name := "claude.json" // merged by sync.sh, not copied
		if kind != "claude" {
			rel, err := filepath.Rel(store.Home, secrets.LoginFile(store.Home, kind))
			if err != nil {
				return err
			}
			name = "home/" + filepath.ToSlash(rel)
		}
		s.put(name, 0o600, data)
	}
	for _, f := range slices.Clone(s.files) {
		rel, ok := strings.CutPrefix(f.name, "home/")
		if !ok {
			continue
		}
		format, ok := settingsFormat(rel)
		if !ok {
			continue
		}
		shown := msg.Shown(filepath.Join(e.Home, rel))
		out, keys, err := secrets.Redact(format, f.data)
		if err != nil {
			s.drop(f.name)
			e.Warn(msg.ConfigNotCopied(shown, vm))
			continue
		}
		f.data = out
		s.add(f)
		if len(keys) > 0 {
			e.Note(msg.ConfigRedacted(shown, strings.Join(keys, " "), vm))
		}
	}
	return nil
}

// settingsFormat returns the format of an agent's settings file, by its
// path relative to the home folder: Claude Code's settings.json, Codex's
// config.toml, opencode's JSON, JSONC and TOML files.
func settingsFormat(rel string) (string, bool) {
	ext := filepath.Ext(rel)
	switch {
	case rel == ".claude/settings.json":
	case rel == ".codex/config.toml":
	case filepath.Dir(rel) == opencodeDir && (ext == ".json" || ext == ".jsonc" || ext == ".toml"):
	default:
		return "", false
	}
	if ext == ".toml" {
		return "toml", true
	}
	return "json", true
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// hostTZ is the host's timezone: what /etc/localtime links to, else
// /etc/timezone.
func hostTZ() string {
	if l, err := os.Readlink("/etc/localtime"); err == nil {
		if _, tz, ok := strings.Cut(l, "zoneinfo/"); ok {
			return tz
		}
	}
	b, _ := os.ReadFile("/etc/timezone")
	return strings.TrimSpace(string(b))
}

// gitConfig is git's setting for key in dir, or "".
func gitConfig(ctx context.Context, dir, key string) string {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", key) //nolint:gosec // devvm's own arguments
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// stageFile is one file sync sends.
type stageFile struct {
	name  string
	mode  int64
	mtime time.Time
	data  []byte
}

// stage is the files sync sends, in order; a name added again replaces
// its file.
type stage struct{ files []stageFile }

func (s *stage) index(name string) int {
	return slices.IndexFunc(s.files, func(f stageFile) bool { return f.name == name })
}

func (s *stage) has(name string) bool { return s.index(name) >= 0 }

func (s *stage) add(f stageFile) {
	if i := s.index(f.name); i >= 0 {
		s.files[i] = f
		return
	}
	s.files = append(s.files, f)
}

func (s *stage) put(name string, mode int64, data []byte) {
	s.add(stageFile{name: name, mode: mode, mtime: time.Now(), data: data})
}

func (s *stage) drop(name string) {
	if i := s.index(name); i >= 0 {
		s.files = slices.Delete(s.files, i, i+1)
	}
}

// copy adds the host file at path, with its mode and time, if it's a
// file (or a link to one).
func (s *stage) copy(name, path string) error {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return nil //nolint:nilerr // nothing to copy
	}
	data, err := os.ReadFile(path) //nolint:gosec // the user's own agent config
	if err != nil {
		return err
	}
	s.add(stageFile{name: name, mode: int64(fi.Mode().Perm()), mtime: fi.ModTime(), data: data})
	return nil
}

// tar returns the files as a tar stream.
func (s *stage) tar() ([]byte, error) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, f := range s.files {
		h := &tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.data)), ModTime: f.mtime, Typeflag: tar.TypeReg}
		if err := w.WriteHeader(h); err != nil {
			return nil, err
		}
		if _, err := w.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
