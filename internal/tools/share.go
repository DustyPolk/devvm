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
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/trust"
)

// shareLock is the share's lock file, in devvm's data folder beside the
// share, which VMs mount.
func (t *Env) shareLock() string { return filepath.Join(t.Dirs.Data, "tools.lock") }

// LockShare holds the share's lock, for a build writing tool images or a
// swap of the lock, until unlock is called or the process ends, however
// it ends: it's an OS lock on an open file. Another devvm holding it is
// waited for until ctx ends, saying so after ShareNote.
func (t *Env) LockShare(ctx context.Context) (unlock func(), err error) {
	if err := os.MkdirAll(t.Dirs.Tools(), 0o700); err != nil {
		return nil, err
	}
	return platform.LockFileWaiting(ctx, t.shareLock(), t.ShareNote, func() { t.Info(msg.ShareWaiting(t.Dirs.Tools())) })
}

// Built reports whether a tool image is in the share with the record
// devvm wrote when it moved it in (FILE.json).
func (t Share) Built(file string) bool {
	if fi, err := os.Stat(filepath.Join(t.Dirs.Tools(), file)); err != nil || !fi.Mode().IsRegular() {
		return false
	}
	_, err := state.ReadToolImage(t.Dirs, file)
	return err == nil
}

// Build builds the tool images these files name that the share lacks, in
// a throwaway VM made from base's image (key base) on b, its features
// checked against rv, in tg's format. The VM runs upstream installers, so
// it never sees the share: it writes into a folder of its own, and each
// image is checked to be a plain file, moved into the share and its hash
// recorded.
func (t Share) Build(ctx context.Context, rv backend.Review, b backend.Backend, base string, tg Target, files []string) error {
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
	if err := rv.Require(b, Label(todo), spec.Features()...); err != nil {
		return err
	}
	unlock, err := t.LockShare(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	lock, err := Versions(t).Lock(ctx)
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
	// The build VM is the backend's; the images come out into devvm's data
	// folder, whatever the backend. Both grow with the tools built.
	space, label := backend.Space{Free: t.FreeSpace}, Label(build)
	if err := space.Check(b, label, func(f backend.Floors) uint64 { return f.ForTools(len(build)) }); err != nil {
		return err
	}
	if err := space.CheckDir(t.Dirs.Data, label, backend.ToolImages(len(build))); err != nil {
		return err
	}
	t.Info(msg.BuildingTools(Label(build)))
	t.LogNote()
	return t.batch(ctx, b, base, tg, lock, build)
}

// batch builds these tool images in one throwaway VM with a folder of its
// own, for tg, then moves them into the share.
func (t Share) batch(ctx context.Context, b backend.Backend, base string, tg Target, lock Lock, files []string) error {
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
	config, err := t.config()
	if err != nil {
		return err
	}
	script, err := fs.ReadFile(t.Assets, "shared-tools/build.sh")
	if err != nil {
		return err
	}
	for name, data := range map[string][]byte{"mise.toml": config, "mise.lock": lock} {
		if err := state.WriteFile(filepath.Join(build, name), data); err != nil {
			return err
		}
	}
	var args []string
	tools, versions := split(files)
	for i, f := range files {
		args = append(args, tools[i], versions[i], f)
	}
	// The user's CAs, trusted in the build VM, which is deleted: a tool
	// image's key holds none of them.
	cas, err := trust.Read(t.Dirs.TrustedCAs())
	if err != nil {
		return err
	}
	ca, err := cas.Build(t.Assets)
	if err != nil {
		return err
	}
	spec := backend.ToolsSpec{
		Base: base, Out: out, OutAt: ShareAt, Args: args, Format: tg.Format, Net: backend.NetInternet,
		Script: cas.Script(backend.Script{Name: "build.sh", Data: script}), Trust: ca,
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
		if err := os.Rename(src, filepath.Join(t.Dirs.Tools(), f)); err != nil {
			return err
		}
		// Its file name doesn't say what it's built for: the record does.
		rec := state.ToolImage{SHA256: sum, Libc: tg.Libc, Format: tg.Format, Backend: b.Name(), Built: t.Now().UTC().Truncate(time.Second)}
		if err := state.WriteToolImage(t.Dirs, f, rec); err != nil {
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

// Delete deletes tool images from the share, with their records. Only
// tool image names are taken, nothing else in the share.
func (t Share) Delete(files []string) error {
	for _, f := range files {
		if _, _, ok := image.ParseToolFile(f); !ok || filepath.Base(f) != f {
			return fmt.Errorf("not a tool image: %q", f)
		}
	}
	for _, f := range files {
		for _, name := range []string{filepath.Join(t.Dirs.Tools(), f), t.Dirs.ToolRecord(f)} {
			if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
