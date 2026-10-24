package tools

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
)

// shareTries is how many times the share's lock is tried, ShareWait apart,
// before devvm gives up; it says it's waiting after waitNote tries.
const (
	shareTries = 600
	waitNote   = 3
)

func (t *Tools) shareLock() string { return filepath.Join(t.Dirs.Tools(), ".lock") }

// LockShare holds the share's lock, for a build writing tool images or a
// swap of the lock, until unlock is called or the run ends. Another devvm
// holding it is waited for.
func (t *Tools) LockShare(ctx context.Context) (unlock func(), err error) {
	if err := os.MkdirAll(t.Dirs.Tools(), 0o700); err != nil {
		return nil, err
	}
	for i := 1; ; i++ {
		err := os.Mkdir(t.shareLock(), 0o700)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if i >= shareTries {
			return nil, msg.ShareLockHeld(t.Dirs.Tools())
		}
		if i == waitNote {
			t.Info(msg.ShareWaiting(t.Dirs.Tools()))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(t.ShareWait):
		}
	}
	remove := ui.AddCleanup(func(context.Context) { _ = os.Remove(t.shareLock()) })
	return func() {
		_ = os.Remove(t.shareLock())
		remove()
	}, nil
}

// Built reports whether a tool image is in the share with the hash devvm
// recorded when it moved it in (FILE.sha256).
func (t *Tools) Built(file string) bool {
	for _, p := range []string{file, file + ".sha256"} {
		if fi, err := os.Stat(filepath.Join(t.Dirs.Tools(), p)); err != nil || !fi.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// Build builds the tool images these files name that the share lacks, in
// a throwaway VM made from base's image (key base) on b. The VM runs
// upstream installers, so it never sees the share: it writes into a folder
// of its own, and each image is checked to be a plain file, moved into the
// share and its hash recorded.
func (t *Tools) Build(ctx context.Context, b backend.Backend, base, libc string, files []string) error {
	var todo []string
	for _, f := range files {
		if !t.Built(f) {
			todo = append(todo, f)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	spec := backend.ToolsSpec{Net: backend.NetInternet}
	if err := backend.Require(b, label(todo), spec.Features()...); err != nil {
		return err
	}
	unlock, err := t.LockShare(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	lock, err := t.Lock(ctx)
	if err != nil {
		return err
	}
	var rest, goes []string
	for _, f := range todo {
		if t.Built(f) {
			continue // another devvm built it while this one waited
		}
		tool, v, ok := image.ParseToolFile(f)
		if !ok {
			return msg.ToolBadOutput(f)
		}
		// build.sh installs the lock's version: the lock may have moved
		// on while this one waited.
		if cur, err := t.File(lock, tool, libc); err != nil {
			return err
		} else if cur != f {
			return msg.ToolsChanged()
		}
		if t.noDownload(lock, tool, libc) {
			return msg.ToolUnbuilt(tool, v)
		}
		// Go builds alone: devvm's proxy, which runs on the host, is
		// compiled with its image, so no other tool's installer may share
		// its VM and folder before its hash is recorded.
		if tool == "go" {
			goes = append(goes, f)
		} else {
			rest = append(rest, f)
		}
	}
	if len(rest)+len(goes) == 0 {
		return nil
	}
	t.Info(msg.BuildingTools(label(append(rest, goes...))))
	ui.LogNote()
	for _, batch := range [][]string{rest, goes} {
		if len(batch) == 0 {
			continue
		}
		if err := t.batch(ctx, b, base, lock, batch); err != nil {
			return err
		}
	}
	return nil
}

// batch builds these tool images in one throwaway VM with a folder of its
// own, then moves them into the share.
func (t *Tools) batch(ctx context.Context, b backend.Backend, base string, lock Lock, files []string) error {
	out, err := os.MkdirTemp(t.Dirs.Data, "tools-build.")
	if err != nil {
		return err
	}
	remove := ui.AddCleanup(func(context.Context) { _ = os.RemoveAll(out) })
	defer func() {
		_ = os.RemoveAll(out)
		remove()
	}()
	if out, err = filepath.EvalSymlinks(out); err != nil {
		return err
	}
	build := filepath.Join(out, ".build")
	if err := os.Mkdir(build, 0o700); err != nil {
		return err
	}
	config, err := os.ReadFile(t.configFile())
	if err != nil {
		return err
	}
	for name, data := range map[string][]byte{"mise.toml": config, "mise.lock": lock} {
		if err := os.WriteFile(filepath.Join(build, name), data, 0o600); err != nil {
			return err
		}
	}
	var args []string
	tools, versions := split(files)
	for i, f := range files {
		args = append(args, tools[i], versions[i], f)
	}
	spec := backend.ToolsSpec{
		Base: base, Out: out, OutAt: ShareAt, Args: args, Net: backend.NetInternet,
		Script: backend.Script{Name: "build.sh", Path: filepath.Join(t.Root, "tools", "build.sh")},
	}
	if err := b.BuildTools(ctx, spec); err != nil {
		return err
	}
	// The VM is gone: nothing changes out now. Plain files only (a link
	// could name a host file). Only the images leave out; their hashes are
	// written in the share, never through a name the VM could have
	// planted.
	for _, f := range files {
		src := filepath.Join(out, f)
		if fi, err := os.Lstat(src); err != nil || !fi.Mode().IsRegular() {
			return msg.ToolBadOutput(f)
		}
		sum, err := image.SHA256File(src)
		if err != nil {
			return err
		}
		dst := filepath.Join(t.Dirs.Tools(), f)
		if err := os.Rename(src, dst); err != nil {
			return err
		}
		if err := os.Remove(dst + ".sha256"); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		h, err := os.OpenFile(dst+".sha256", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the share, devvm's own
		if err != nil {
			return err
		}
		_, err = h.WriteString(sum + "\n")
		if cerr := h.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Garbage returns the tool images in the share that are neither current
// (the lock's versions, for a base with libc) nor in use (listed by a VM),
// and with tmp the temporary files of interrupted builds too (only with
// the share's lock held).
func (t *Tools) Garbage(ctx context.Context, libc string, inUse []string, tmp bool) ([]string, error) {
	entries, err := os.ReadDir(t.Dirs.Tools())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	all, err := t.All()
	if err != nil {
		return nil, err
	}
	lock, err := t.Lock(ctx)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, tool := range all {
		f, err := t.File(lock, tool, libc)
		if err != nil {
			return nil, err
		}
		live[f] = true
	}
	for _, f := range inUse {
		live[f] = true
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if !e.Type().IsRegular() {
			continue
		}
		if strings.HasSuffix(n, ".erofs") && !live[n] || tmp && strings.HasSuffix(n, ".tmp") {
			out = append(out, filepath.Join(t.Dirs.Tools(), n))
		}
	}
	return out, nil
}
