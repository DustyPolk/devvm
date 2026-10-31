// Package tools keeps the shared tools: the toolchains and agents image
// names hold (node, claude), which aren't image layers. Each tool version
// is one read-only image in the share, built once in a throwaway VM and
// mounted into every VM that uses it, where mise runs it.
//
// Which versions is per user: devvm's shared-tools/mise.toml says which
// releases (node 24, the latest claude), and the host's mise resolves it
// into the tools lock,
// with each download's checksum per platform. Builds install exactly the
// lock's versions, checked against those checksums. The lock is resolved
// on first use and again when mise.toml changes; once a day a
// launch resolves it again in the background, so VMs can move to newer
// versions. A VM lists its tool images in its state, so a newer version
// never changes a VM that has one.
//
// Env is what the shared tools run with; Versions keeps the lock (resolving
// it, the daily check, the image files an image name's tools are); Share
// builds, lists and deletes the share's tool images; VMs moves a VM to
// newer tools.
package tools

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
)

// ShareAt is where VMs have the share, read-only, and where the tools
// build has its output folder.
const ShareAt = "/mnt/devvm-tools"

// checkEvery is how often the lock is resolved again.
const checkEvery = 24 * time.Hour

// Env is what one user's shared tools run with, shared by Versions, Share
// and VMs; its methods are what they share: the platforms, devvm's
// mise.toml, a tool's image file and the share's lock. New sets every
// field; tests change them.
type Env struct {
	Dirs paths.Dirs
	// Assets holds devvm's own files (assets.FS): shared-tools/mise.toml
	// and shared-tools/build.sh.
	Assets fs.FS
	// Self is devvm's binary, which the daily check runs.
	Self string
	// OS and Arch are the host's, as Go names them.
	OS, Arch string
	Getenv   func(string) string
	Client   *http.Client
	Now      func() time.Time
	// Detach starts the daily check, which outlives the run.
	Detach func(path string, args []string, out *os.File) (int, error)
	// ShareWait is how long the share's lock is waited for between tries.
	ShareWait time.Duration
	// What it prints: Warn says where the run log is too.
	Info, Warn func(msg.M)
	// LogNote says where the run log is, once a run; Status shows a status
	// line until stop; LoggedCmd runs a command into the run log.
	LogNote   func()
	Status    func(label msg.M) (stop func())
	LoggedCmd func(ctx context.Context, step msg.M, cmd *exec.Cmd) error
	// AddCleanup adds to the run's cleanup stack.
	AddCleanup func(f func(context.Context)) (remove func())
	// FreeSpace returns the free space on the volume holding a path
	// (platform.FreeSpace), checked before a build; nil checks none.
	FreeSpace func(path string) (uint64, error)

	// The host's mise: its download's URL (a format taking the version
	// and platform) and checksums, by platform.
	miseURL  string
	miseSums map[string]string
}

// Versions is the tools lock: resolving it with the host's mise, the daily
// check, and the image files of an image name's tools at its versions.
type Versions struct{ *Env }

// Share is the share's tool images: building what it lacks, listing and
// deleting them.
type Share struct{ *Env }

// VMs is a VM's shared tools against the lock: whether newer ones are
// there, and moving the VM to them.
type VMs struct{ *Env }

// UI is where the shared tools print, the run log and status line their
// slow steps show on, and the run's cleanup stack.
type UI interface {
	Info(msg.M)
	Warn(msg.M)
	LogNote()
	Status(label msg.M) (stop func())
	LoggedCmd(ctx context.Context, step msg.M, cmd *exec.Cmd) error
	AddCleanup(f func(context.Context)) (remove func())
}

// New returns the shared tools in devvm's folders, with devvm's own files
// (assets), on u.
func New(d paths.Dirs, assets fs.FS, u UI) *Env {
	self, err := os.Executable()
	if err == nil {
		if r, rerr := filepath.EvalSymlinks(self); rerr == nil {
			self = r
		}
	}
	return &Env{
		Dirs: d, Assets: assets, Self: self, OS: runtime.GOOS, Arch: runtime.GOARCH,
		Getenv: os.Getenv, Client: newClient(), Now: time.Now, Detach: platform.StartDetached,
		ShareWait: time.Second, miseURL: miseURL, miseSums: miseSHA256,
		Info: u.Info, Warn: func(m msg.M) { u.Warn(m); u.LogNote() },
		LogNote: u.LogNote, Status: u.Status, LoggedCmd: u.LoggedCmd, AddCleanup: u.AddCleanup,
		FreeSpace: platform.FreeSpace,
	}
}

// MiseArch is mise's name for a Go architecture: the host's, and so its
// VMs'.
func MiseArch(goarch string) string {
	if goarch == "amd64" {
		return "x64"
	}
	return "arm64"
}

// platforms are the platforms the lock resolves: the VMs', glibc and musl.
func (t *Env) platforms() []string {
	a := MiseArch(t.Arch)
	return []string{"linux-" + a, "linux-" + a + "-musl"}
}

// platform is the platform of a VM whose base has libc.
func (t *Env) platform(libc string) string {
	p := "linux-" + MiseArch(t.Arch)
	if libc == "musl" {
		p += "-musl"
	}
	return p
}

// config is devvm's shared-tools/mise.toml.
func (t *Env) config() ([]byte, error) { return fs.ReadFile(t.Assets, "shared-tools/mise.toml") }

// lockConfig is the mise.toml the lock was resolved from.
func (t Versions) lockConfig() string { return t.Dirs.ToolLock() + ".toml" }

// checkedFile is touched whenever the lock is resolved.
func (t Versions) checkedFile() string { return t.Dirs.ToolLock() + ".checked" }

// Lock returns the tools lock, resolving it first when there's none yet or
// shared-tools/mise.toml changed since (a devvm upgrade).
func (t Versions) Lock(ctx context.Context) (Lock, error) {
	want, err := t.config()
	if err != nil {
		return nil, err
	}
	lock, lerr := os.ReadFile(t.Dirs.ToolLock())
	from, ferr := os.ReadFile(t.lockConfig())
	if lerr == nil && ferr == nil && bytes.Equal(from, want) {
		return Lock(lock), nil
	}
	t.Info(msg.ResolvingTools())
	if err := t.Resolve(ctx, t.Dirs.ToolLock()); err != nil {
		if _, ok := errors.AsType[*backend.LoggedError](err); ok {
			return nil, backend.Logged(msg.ToolsNeedResolve())
		}
		return nil, err
	}
	if err := t.touchChecked(); err != nil {
		return nil, err
	}
	lock, err = os.ReadFile(t.Dirs.ToolLock())
	return Lock(lock), err
}

func (t Versions) touchChecked() error {
	now := t.Now()
	err := os.Chtimes(t.checkedFile(), now, now)
	if errors.Is(err, fs.ErrNotExist) {
		err = os.WriteFile(t.checkedFile(), nil, 0o600)
		if err == nil {
			err = os.Chtimes(t.checkedFile(), now, now)
		}
	}
	return err
}

// CheckDue reports whether the daily check is due: there's a lock, not
// resolved in the last day. Never with DEVVM_NO_TOOL_CHECK set.
func (t Versions) CheckDue() bool {
	if t.Getenv("DEVVM_NO_TOOL_CHECK") != "" {
		return false
	}
	if _, err := os.Stat(t.Dirs.ToolLock()); err != nil {
		return false
	}
	fi, err := os.Stat(t.checkedFile())
	return err != nil || t.Now().Sub(fi.ModTime()) >= checkEvery
}

// checkLog is where the daily check's output goes.
func (t Versions) checkLog() string { return filepath.Join(t.Dirs.Logs(), "tools-check.log") }

// RefreshDaily starts devvm _refresh-tools in the background once a day,
// when the check is due, so a later launch knows of newer versions. It
// doesn't wait for it.
func (t Versions) RefreshDaily() error {
	if !t.CheckDue() {
		return nil
	}
	if err := t.touchChecked(); err != nil {
		return err
	}
	if err := os.MkdirAll(t.Dirs.Logs(), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(t.Dirs.Logs(), 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
		return err
	}
	out, err := os.OpenFile(t.checkLog(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = t.Detach(t.Self, []string{"_refresh-tools"}, out)
	return err
}

// Refresh resolves the lock again, and swaps the new one in while no
// build is reading it. A failure leaves the lock as it was.
func (t Versions) Refresh(ctx context.Context) error {
	next := t.Dirs.ToolLock() + ".next"
	if err := t.Resolve(ctx, next); err != nil {
		return err
	}
	unlock, err := t.LockShare(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if err := os.Rename(next+".toml", t.lockConfig()); err != nil {
		return err
	}
	if err := os.Rename(next, t.Dirs.ToolLock()); err != nil {
		return err
	}
	return t.touchChecked()
}

// Check is Refresh for a user waiting on it: a failure is a warning, and
// the versions it has stay.
func (t Versions) Check(ctx context.Context) {
	t.Info(msg.CheckingTools())
	if err := t.Refresh(ctx); err != nil {
		if ctx.Err() == nil {
			t.Warn(msg.ToolsCheckFailed())
		}
	}
}

// Target is what a VM's tool images are built for: its base's C library
// and its backend's image format.
type Target struct {
	Libc   string // gnu or musl
	Format string // the backend's ToolFormat: "" for shared-tools/build.sh's own
}

// File returns a tool's image file at the lock's version, for a VM of
// that target.
func (t *Env) File(lock Lock, tool string, tg Target) (string, error) {
	entry := lock.Entry(tool)
	v := field(entry, "version")
	if v == "" {
		return "", msg.ToolNotLocked(t.Dirs.ToolLock(), tool)
	}
	return image.ToolFile(t.Assets, image.Tool{Name: tool, Version: v, LockEntry: entry, Libc: tg.Libc, Format: tg.Format})
}

// Files returns the image files of the tools behind each of these names
// (node is node, pnpm and bun), at the lock's versions.
func (t Versions) Files(ctx context.Context, names []string, tg Target) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	lock, err := t.Lock(ctx)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, n := range names {
		for _, tool := range image.GroupTools(n) {
			f, err := t.File(lock, tool, tg)
			if err != nil {
				return nil, err
			}
			files = append(files, f)
		}
	}
	return files, nil
}

// noDownload reports whether the lock has no download of a tool for a VM
// whose base has libc (no version had one), or builds it from source,
// which mise falls back to.
func (t Share) noDownload(lock Lock, tool, libc string) bool {
	return !lock.HasDownload(tool, []string{t.platform(libc)})
}

// split returns a VM's tool files as tool and version, keeping the order.
func split(files []string) (tools, versions []string) {
	for _, f := range files {
		tool, v, _ := image.ParseToolFile(f)
		tools, versions = append(tools, tool), append(versions, v)
	}
	return tools, versions
}

// Label lists tool files as "node 24.21.0, bun 1.4.2".
func Label(files []string) string {
	var out []string
	for _, f := range files {
		tool, v, _ := image.ParseToolFile(f)
		out = append(out, tool+" "+v)
	}
	return strings.Join(out, ", ")
}

// Mounts is a VM's tool list as the VM reads it: a "FILE TOOL VERSION"
// line per tool image.
func Mounts(files []string) []byte {
	var b bytes.Buffer
	tools, versions := split(files)
	for i, f := range files {
		if tools[i] == "" {
			continue
		}
		b.WriteString(f + " " + tools[i] + " " + versions[i] + "\n")
	}
	return b.Bytes()
}
