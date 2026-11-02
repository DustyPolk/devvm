package tools

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
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

// Built reports whether a tool image is in the share with the hash devvm
// recorded when it moved it in (FILE.sha256).
func (t Share) Built(file string) bool {
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
		if err := os.WriteFile(filepath.Join(build, name), data, 0o600); err != nil {
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
		dst := filepath.Join(t.Dirs.Tools(), f)
		if err := os.Rename(src, dst); err != nil {
			return err
		}
		if err := writeNew(dst+".sha256", sum+"\n"); err != nil {
			return err
		}
		if err := writeNew(dst+targetExt, tg.record()); err != nil {
			return err
		}
	}
	return nil
}

// targetExt is the file beside a tool image naming what it's built for
// (Target.record): its file name doesn't say.
const targetExt = ".target"

// record is a target as its file beside a tool image has it.
func (tg Target) record() string { return "libc=" + tg.Libc + "\nformat=" + tg.Format + "\n" }

// readTarget reads what a tool image in the share is built for; ok is
// false when there's no record (one built before they were kept) or it
// can't be read.
func (t Share) readTarget(file string) (tg Target, ok bool) {
	data, err := os.ReadFile(filepath.Join(t.Dirs.Tools(), file+targetExt)) //nolint:gosec // the share, devvm's own
	if err != nil {
		return Target{}, false
	}
	libc, format := "", ""
	for line := range strings.SplitSeq(string(data), "\n") {
		if v, found := strings.CutPrefix(line, "libc="); found {
			libc = v
		} else if v, found := strings.CutPrefix(line, "format="); found {
			format = v
		}
	}
	if libc != "gnu" && libc != "musl" {
		return Target{}, false
	}
	return Target{Libc: libc, Format: format}, true
}

// writeNew writes a file in the share afresh: one there is removed first,
// so it's never written through a link.
func writeNew(path, data string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	h, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the share, devvm's own
	if err != nil {
		return err
	}
	_, err = h.WriteString(data)
	if cerr := h.Close(); err == nil {
		err = cerr
	}
	return err
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
func (t Share) Delete(files []string) error {
	for _, f := range files {
		if _, _, ok := image.ParseToolFile(f); !ok || filepath.Base(f) != f {
			return fmt.Errorf("not a tool image: %q", f)
		}
	}
	for _, f := range files {
		p := filepath.Join(t.Dirs.Tools(), f)
		for _, name := range []string{p, p + ".sha256", p + targetExt, p + idleExt} {
			if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
