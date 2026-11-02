package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// The mise devvm resolves the tools lock with, on the host: pinned, and
// checked against its checksum before it runs.
const miseVersion = "2026.10.7"

var miseSHA256 = map[string]string{
	"macos-arm64": "f5171e341518a57e8c4e9280e28443e35d66212c51164c83be76794e0a78b014",
	"macos-x64":   "c3355f0c56d1b9fe73a2ba30e034b4e483541b25b1ad812a87440abfaeec8baa",
	"linux-arm64": "c7108d85a32ba17e4747d31d4a42f39f0c134f16211e204e8ef0a49d4f518fe1",
	"linux-x64":   "6eb1b890e90818417ca34c90dbbd47881917d5cd199f31b63b062ea9c6b18d85",
}

const miseURL = "https://github.com/jdx/mise/releases/download/v%[1]s/mise-v%[1]s-%[2]s"

// downloadTimeout bounds mise's download, start to end.
const downloadTimeout = 10 * time.Minute

// newClient is the client mise is downloaded with: https only, redirects
// included.
func newClient() *http.Client {
	return &http.Client{Timeout: downloadTimeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("redirect to %s refused", req.URL.Scheme)
		}
		return nil
	}}
}

// HostMise returns the path of the host's mise: a test build's stand-in
// (Mise) when it has one, else devvm's pinned mise, downloaded once and
// checked.
func (t Versions) HostMise(ctx context.Context) (string, error) {
	if t.Mise != "" {
		return t.Mise, nil
	}
	var osName string
	switch t.OS {
	case "darwin":
		osName = "macos"
	case "linux":
		osName = "linux"
	default:
		return "", fmt.Errorf("no mise for %s", t.OS)
	}
	platform := osName + "-" + MiseArch(t.Arch)
	dir := filepath.Join(t.Dirs.Mise(), miseVersion)
	path := filepath.Join(dir, "mise")
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o100 != 0 {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	stop := t.Status(msg.DownloadingMise(miseVersion))
	defer stop()
	tmp := path + ".tmp"
	sum, err := t.download(ctx, fmt.Sprintf(t.miseURL, miseVersion, platform), tmp)
	if err != nil {
		_ = os.Remove(tmp)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", msg.MiseDownloadFailed(miseVersion)
	}
	if want := t.miseSums[platform]; want == "" || sum != want {
		_ = os.Remove(tmp)
		return "", msg.MiseChecksum(miseVersion)
	}
	if err := os.Chmod(tmp, 0o700); err != nil { //nolint:gosec // a program: the user runs it
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// download writes url's body to path and returns its SHA-256.
func (t Versions) download(ctx context.Context, url, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := t.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", url, resp.Status)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // devvm's own folder
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return hex.EncodeToString(h.Sum(nil)), err
}

// miseCmd returns mise running in dir, a folder of devvm's own, with its
// state there too. mise also reads config files in the folders above (a
// shared temporary folder: anyone's): MISE_CEILING_PATHS, which needs the
// physical path, stops it at dir.
func miseCmd(ctx context.Context, mise, dir string, args ...string) (*exec.Cmd, error) {
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, mise, args...) //nolint:gosec // devvm's own mise
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"MISE_DATA_DIR="+filepath.Join(dir, "data"),
		"MISE_CACHE_DIR="+filepath.Join(dir, "cache"),
		"MISE_STATE_DIR="+filepath.Join(dir, "state"),
		"MISE_CONFIG_DIR="+filepath.Join(dir, "config"),
		"MISE_GLOBAL_CONFIG_FILE="+filepath.Join(dir, "config", "none.toml"),
		"MISE_TRUSTED_CONFIG_PATHS="+dir,
		"MISE_CEILING_PATHS="+filepath.Dir(dir),
	)
	return cmd, nil
}

// Resolve resolves shared-tools/mise.toml to the newest matching releases, for
// every platform VMs run, into out (and out.toml, the mise.toml it's
// from). Every tool, or nothing: offline, or with a tool mise can't
// resolve, it writes nothing and returns tools_resolve_failed, with the
// details in the run log. mise's version lists come from its versions
// host (no GitHub rate limit).
func (t Versions) Resolve(ctx context.Context, out string) error {
	mise, err := t.HostMise(ctx)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "devvm-tools.")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	config, err := t.config()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "mise.toml"), config, 0o600); err != nil {
		return err
	}
	cmd, err := miseCmd(ctx, mise, tmp, "lock", "--platform", strings.Join(t.platforms(), ","))
	if err != nil {
		return err
	}
	if err := t.LoggedCmd(ctx, msg.ResolvingTools(), cmd); err != nil {
		return t.resolveFailed(ctx)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "mise.lock")) //nolint:gosec // devvm's own temporary folder
	if err != nil {
		return t.resolveFailed(ctx)
	}
	lock := Lock(data)
	names, specs := configTools(config)
	for _, n := range names {
		if len(lock.Entry(n)) == 0 {
			return t.resolveFailed(ctx)
		}
	}
	lock = t.stepBack(ctx, mise, lock, names, specs)
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return err
	}
	for _, f := range []struct {
		path string
		data []byte
	}{{out + ".toml", config}, {out, lock}} {
		if err := os.WriteFile(f.path+".tmp", f.data, 0o600); err != nil {
			return err
		}
		if err := os.Rename(f.path+".tmp", f.path); err != nil {
			return err
		}
	}
	return nil
}

func (t Versions) resolveFailed(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return backend.Logged(msg.ToolsResolveFailed())
}

// versionLike is a name shaped like a version: it goes into a TOML list.
var versionLike = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]*$`)

// candidates is how many older versions a step back tries.
const candidates = 5

// stepBack gives a tool whose newest entry has no download for some
// platform (a release without its files yet, a tag with none, node's musl
// builds for days after a release) the newest of its next older versions
// that has them, keeping its specifiers so it still matches mise.toml.
// Older versions come from the tool's backend (GitHub releases for aqua
// tools), not mise's versions host, which can list tags with no release.
// The next daily check tries the newest again.
func (t Versions) stepBack(ctx context.Context, mise string, lock Lock, names []string, specs map[string]string) Lock {
	for _, tool := range names {
		if lock.HasDownload(tool, t.platforms()) {
			continue
		}
		entry := lock.Entry(tool)
		v := field(entry, "version")
		older := t.olderVersions(ctx, mise, field(entry, "backend"), specs[tool], v)
		found := false
		if entries := t.lockVersions(ctx, mise, tool, older); entries != nil {
			for _, c := range older {
				one := entries.VersionEntry(tool, c)
				if one == nil || !Lock(one).HasDownload(tool, t.platforms()) {
					continue
				}
				specifiers := ""
				for _, line := range lines(entry) {
					if strings.HasPrefix(line, "specifiers = ") {
						specifiers = line
						break
					}
				}
				lock = lock.replace(tool, withSpecifiers(one, specifiers))
				t.Warn(msg.ToolSteppedBack(c, tool, v))
				found = true
				break
			}
		}
		if !found {
			t.Warn(msg.ToolNoDownload(tool, v))
		}
	}
	return lock
}

// olderVersions lists up to five versions of a tool older than v, newest
// first: those its backend lists before v (all of them when v isn't
// listed), with spec's prefix ("24") unless it's latest.
func (t Versions) olderVersions(ctx context.Context, mise, backendName, spec, v string) []string {
	dir, err := os.MkdirTemp("", "devvm-tools.")
	if err != nil {
		return nil
	}
	defer func() { _ = os.RemoveAll(dir) }()
	args := []string{"ls-remote", backendName}
	if spec != "" && spec != "latest" {
		args = append(args, spec)
	}
	cmd, err := miseCmd(ctx, mise, dir, args...)
	if err != nil {
		return nil
	}
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var vs []string
	for _, line := range lines(out) {
		if line == v {
			break
		}
		if versionLike.MatchString(line) {
			vs = append(vs, line)
		}
	}
	vs = vs[max(0, len(vs)-candidates):]
	slices.Reverse(vs)
	return vs
}

// lockVersions locks these versions of a tool, all in one lock (mise
// takes a list of versions), in a folder of its own: mise also reads the
// mise.toml of the folders above. Nil when there are none, or mise fails.
func (t Versions) lockVersions(ctx context.Context, mise, tool string, versions []string) Lock {
	if len(versions) == 0 {
		return nil
	}
	dir, err := os.MkdirTemp("", "devvm-tools.")
	if err != nil {
		return nil
	}
	defer func() { _ = os.RemoveAll(dir) }()
	list := `"` + strings.Join(versions, `","`) + `"`
	if err := os.WriteFile(filepath.Join(dir, "mise.toml"), []byte("[tools]\n"+tool+" = ["+list+"]\n"), 0o600); err != nil {
		return nil
	}
	cmd, err := miseCmd(ctx, mise, dir, "lock", "--platform", strings.Join(t.platforms(), ","))
	if err != nil {
		return nil
	}
	_ = t.LoggedCmd(ctx, msg.ResolvingTools(), cmd)           // what it locked is checked below
	data, err := os.ReadFile(filepath.Join(dir, "mise.lock")) //nolint:gosec // devvm's own temporary folder
	if err != nil {
		return nil
	}
	return Lock(data)
}
