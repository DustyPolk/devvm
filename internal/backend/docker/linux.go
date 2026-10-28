package docker

// devvm's Linux build, which network policy's relays and Docker Desktop's
// builds run in containers (egress.go). On Linux it's devvm itself.
// Elsewhere (macOS) it's found beside devvm or in ../libexec (a build from
// source: the Homebrew formula's, a test build's), or else fetched: a
// release of devvm has its own version's Linux build's address and that
// build's SHA-256 set in it when it's built (registry's linuxURL and
// linuxSHA256), and fetches it from the release's Linux archive the first
// time a Docker VM needs it, into devvm's data folder, checking it against
// that SHA-256 before it's used. A build without them fetches nothing.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

// linuxMax is the most a Linux build, or its archive, may be.
const linuxMax = 256 << 20

// linuxFetchTime is how long a fetch may take.
const linuxFetchTime = 10 * time.Minute

// errUnfetched is a release's Linux build not fetched yet, which only a
// Docker VM's start fetches.
var errUnfetched = errors.New("devvm's Linux build isn't fetched yet")

// linuxCopy is where devvm's data folder keeps devvm's Linux build: the
// copy Docker mounts, and a fetched one.
func linuxCopy(d paths.Dirs, goarch string) string {
	return filepath.Join(d.Docker(), "bin", "devvm-linux-"+goarch)
}

// linuxFinder finds devvm's Linux build for this machine's processor.
type linuxFinder struct {
	Self, GOOS, GOARCH string
	// URL is the release's Linux archive, and SHA256 its devvm's SHA-256,
	// hex: both "" in a build that has none to fetch.
	URL, SHA256 string
	// Copy is where a fetched one is kept (linuxCopy).
	Copy string
	// Info shows what's being done.
	Info func(msg.M)
}

// find returns the path of devvm's Linux build: on Linux devvm itself
// (Self, its links resolved); else devvm-linux-<arch> beside it or in
// ../libexec, built as this devvm was; else, for a release, its own
// version's, kept in devvm's data folder, fetched first when it isn't
// there and fetch is set (errUnfetched without).
func (f linuxFinder) find(ctx context.Context, fetch bool) (string, error) {
	exe, err := filepath.EvalSymlinks(f.Self)
	if err != nil {
		return "", err
	}
	if f.GOOS == "linux" {
		return exe, nil
	}
	name := "devvm-linux-" + f.GOARCH
	for _, p := range []string{filepath.Join(filepath.Dir(exe), name), filepath.Join(filepath.Dir(exe), "..", "libexec", name)} {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if !sameBuild(p, f.GOARCH) {
			return "", msg.DockerLinuxBuildOther(p)
		}
		return p, nil
	}
	if f.URL == "" || f.SHA256 == "" {
		return "", msg.DockerLinuxBuildMissing(filepath.Join(filepath.Dir(exe), name))
	}
	if sum, err := fileSum(f.Copy); err == nil && strings.EqualFold(sum, f.SHA256) && sameBuild(f.Copy, f.GOARCH) {
		return f.Copy, nil
	}
	if !fetch {
		return "", errUnfetched
	}
	if err := f.fetch(ctx); err != nil {
		return "", err
	}
	if !sameBuild(f.Copy, f.GOARCH) {
		_ = os.Remove(f.Copy)
		return "", msg.DockerLinuxBuildOther(f.Copy)
	}
	return f.Copy, nil
}

// fetch downloads the release's Linux archive and keeps its devvm as
// Copy, once its SHA-256 is the one this devvm has: the archive is read
// as it comes, devvm written to a temporary file beside Copy, which is
// renamed into place only when it matches.
func (f linuxFinder) fetch(ctx context.Context) error {
	f.Info(msg.DockerLinuxBuildFetching(f.URL))
	ctx, cancel := context.WithTimeout(ctx, linuxFetchTime)
	defer cancel()
	failed := msg.DockerLinuxBuildFetchFailed(f.URL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return failed.Because(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failed.Because(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return failed.Because(fmt.Errorf("%s: %s", f.URL, resp.Status))
	}
	if err := os.MkdirAll(filepath.Dir(f.Copy), 0o700); err != nil {
		return err
	}
	tmp := f.Copy + ".fetch"
	defer func() { _ = os.Remove(tmp) }()
	sum, err := extractDevvm(io.LimitReader(resp.Body, linuxMax), tmp)
	if err != nil {
		return failed.Because(err)
	}
	if !strings.EqualFold(sum, f.SHA256) {
		return msg.DockerLinuxBuildMismatch(f.URL)
	}
	if err := os.Chmod(tmp, 0o700); err != nil { //nolint:gosec // a program, the user's alone

		return err
	}
	return os.Rename(tmp, f.Copy)
}

// extractDevvm writes the archive's devvm (a regular file named devvm, at
// its top or in its one folder) to dst, mode 600, and returns its SHA-256.
func extractDevvm(r io.Reader, dst string) (string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", errors.New("no devvm in the archive")
		} else if err != nil {
			return "", err
		}
		name := path.Clean(strings.TrimPrefix(hdr.Name, "./"))
		if hdr.Typeflag != tar.TypeReg || path.Base(name) != "devvm" || strings.Count(name, "/") > 1 {
			continue
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // devvm's own folder
		if err != nil {
			return "", err
		}
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(tr, linuxMax+1))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return "", err
		}
		if n > linuxMax {
			return "", errors.New("devvm in the archive is too large")
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
}

// fileSum is a file's SHA-256, as hex.
func fileSum(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // devvm's own build
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile copies src to dst whole, executable, through a temporary file
// renamed over it.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src) //nolint:gosec // devvm's own build
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700) //nolint:gosec // devvm's own folder
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// pinFlags is the start of the -X flags that set a release's Linux
// build's address and SHA-256, which only its macOS builds have.
const pinFlags = "github.com/w3cj/devvm/internal/backend/registry.linux"

// sameBuild reports whether the file at path is devvm built for linux and
// goarch as this one was: the same Go, version, commit and flags, but the
// Linux build's address and SHA-256.
func sameBuild(path, goarch string) bool {
	theirs, err := buildinfo.ReadFile(path)
	ours, ok := debug.ReadBuildInfo()
	if err != nil || !ok {
		return false
	}
	setting := func(bi *debug.BuildInfo, key string) string {
		for _, s := range bi.Settings {
			if s.Key == key {
				return s.Value
			}
		}
		return ""
	}
	if setting(theirs, "GOOS") != "linux" || setting(theirs, "GOARCH") != goarch ||
		theirs.GoVersion != ours.GoVersion || theirs.Path != ours.Path || theirs.Main.Version != ours.Main.Version {
		return false
	}
	for _, k := range []string{"vcs.revision", "vcs.modified", "-trimpath", "-tags"} {
		if setting(theirs, k) != setting(ours, k) {
			return false
		}
	}
	return withoutPin(setting(theirs, "-ldflags")) == withoutPin(setting(ours, "-ldflags"))
}

// withoutPin is -ldflags without the -X flags that set the Linux build's
// address and SHA-256.
func withoutPin(ldflags string) string {
	var out []string
	fs := strings.Fields(strings.Trim(ldflags, `"`))
	for i := 0; i < len(fs); i++ {
		if fs[i] == "-X" && i+1 < len(fs) && strings.HasPrefix(fs[i+1], pinFlags) {
			i++
			continue
		}
		out = append(out, fs[i])
	}
	return strings.Join(out, " ")
}
