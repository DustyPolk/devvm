package tools

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
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
	remove := t.AddCleanup(func(context.Context) { _ = os.Remove(t.shareLock()) })
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
// a throwaway VM made from base's image (key base) on b, its features
// checked against rv, in tg's format. The VM runs upstream installers, so
// it never sees the share: it writes into a folder of its own, and each
// image is checked to be a plain file, moved into the share and its hash
// recorded.
func (t *Tools) Build(ctx context.Context, rv backend.Review, b backend.Backend, base string, tg Target, files []string) error {
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
	if err := rv.Require(b, label(todo), spec.Features()...); err != nil {
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
	var build []string
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
		if cur, err := t.File(lock, tool, tg); err != nil {
			return err
		} else if cur != f {
			return msg.ToolsChanged()
		}
		if t.noDownload(lock, tool, tg.Libc) {
			return msg.ToolUnbuilt(tool, v)
		}
		build = append(build, f)
	}
	if len(build) == 0 {
		return nil
	}
	t.Info(msg.BuildingTools(label(build)))
	t.LogNote()
	return t.batch(ctx, b, base, tg.Format, lock, build)
}

// batch builds these tool images in one throwaway VM with a folder of its
// own, then moves them into the share.
func (t *Tools) batch(ctx context.Context, b backend.Backend, base, format string, lock Lock, files []string) error {
	out, err := os.MkdirTemp(t.Dirs.Data, "tools-build.")
	if err != nil {
		return err
	}
	remove := t.AddCleanup(func(context.Context) { _ = os.RemoveAll(out) })
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
		Base: base, Out: out, OutAt: ShareAt, Args: args, Format: format, Net: backend.NetInternet,
		Script: backend.Script{Name: "build.sh", Path: filepath.Join(t.Root, "shared-tools", "build.sh")},
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

// Unused returns the tool images of a List that it calls unused: neither
// the lock's version nor listed by a VM. Without a lock, none is. Callers
// deleting them hold the share's lock from before the List, so no build
// or lock swap changes the share meanwhile.
func Unused(ls []Listed) []Listed {
	var out []Listed
	for _, l := range ls {
		if l.State == StateUnused && l.File != "" {
			out = append(out, l)
		}
	}
	return out
}

// Delete deletes tool images from the share, with their hashes. Only
// tool image names are taken, nothing else in the share.
func (t *Tools) Delete(files []string) error {
	for _, f := range files {
		if _, _, ok := image.ParseToolFile(f); !ok || filepath.Base(f) != f {
			return fmt.Errorf("not a tool image: %q", f)
		}
	}
	for _, f := range files {
		p := filepath.Join(t.Dirs.Tools(), f)
		for _, name := range []string{p, p + ".sha256"} {
			if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
