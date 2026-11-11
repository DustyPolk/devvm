package vm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/platform"
	"github.com/devvmhq/devvm/internal/settings"
)

// Paste is devvm paste: an image, or one copied file, from this machine's
// clipboard into the folder's VM's paste folder (config.toml's paste-dir),
// its path in the VM then copied to the clipboard for the agent; devvm
// paste rm, which deletes what devvm paste left there; and the launch's
// offer to do the same (offerRm). Its connections to the VM carry no SSH
// agent, and the only thing it reads back from the VM is a count. It's a
// type of its own because Lifecycle and Net are at the structure test's
// limit of methods.
type Paste struct{ *Env }

// PasteSent is what devvm paste sent: the VM, the file's path in it, which
// devvm chose on this machine, its kind (image or file), an image's type
// (image/png…, "" for a file), a copied file's path on this machine (""
// for an image), its size, and whether the path reached the clipboard.
// It's empty when you said no to sending a file.
type PasteSent struct {
	VM, Path string
	Kind     string
	Type     string
	Source   string
	Bytes    int64
	Copied   bool
}

// The kinds of a PasteSent.
const (
	PasteImage = "image"
	PasteFile  = "file"
)

// pasteMax is the most devvm paste sends, an image or a file, and
// pasteMaxText how its message says it.
const (
	pasteMax     = 64 << 20
	pasteMaxText = "64 MiB"
)

// pasteCountMax bounds what the VM prints for a count.
const pasteCountMax = 32

// pastePayload is what one devvm paste sends, chosen once from the
// clipboard: the bytes, the file's name in the VM, which devvm picks from
// a fixed set of characters, its kind, an image's type and a copied
// file's path on this machine.
type pastePayload struct {
	data       []byte
	name       string
	kind, mime string
	source     string
}

// Send reads the clipboard and puts what's there into the folder's VM,
// which must be running, as a new file in its paste folder, then copies
// the file's path. It reads the clipboard before it touches the VM, so an
// empty one fails fast. A copied file is sent only after a question
// showing its path on this machine, [y/N], answered by sendFile
// (--send-file), because the clipboard can name any file of yours; a no
// sends nothing and returns an empty PasteSent. A copy that fails is a
// warning, as the file is already in the VM, and the path is shown.
func (e Paste) Send(ctx context.Context, sendFile bool) (PasteSent, error) {
	t, err := e.finder.target(ctx, need{has: hasMade})
	if err != nil {
		return PasteSent{}, err
	}
	vm := t.f.Name
	if t.in.Status != backend.StatusRunning {
		return PasteSent{}, msg.VMNotRunning(vm)
	}
	dir, err := settings.PasteDir(e.Dirs)
	if err != nil {
		return PasteSent{}, err
	}
	clip, err := e.ReadClip(ctx, pasteMax)
	if err != nil {
		return PasteSent{}, clipError(err)
	}
	p, err := pastePayloadOf(clip, time.Now())
	if err != nil {
		return PasteSent{}, err
	}
	if p.kind == PasteFile && !sendFile {
		yes, err := e.Ask(msg.PasteFileAsk(p.source, vm).AnsweredBy("--send-file"))
		if err != nil || !yes {
			return PasteSent{}, err
		}
	}
	path := dir + "/" + p.name
	if err := e.run(ctx, t, []string{"put", dir, p.name}, bytes.NewReader(p.data), nil); err != nil {
		if ctx.Err() != nil {
			return PasteSent{}, ctx.Err()
		}
		return PasteSent{}, backend.Logged(msg.PasteNotSent(path, vm))
	}
	sent := PasteSent{VM: vm, Path: path, Kind: p.kind, Type: p.mime, Source: p.source, Bytes: int64(len(p.data))}
	if err := e.CopyText(ctx, path); err != nil {
		e.Warn(msg.PasteCopyFailed(path).Because(err))
	} else {
		sent.Copied = true
	}
	return sent, nil
}

// Rm deletes what devvm paste left in the folder's VM's paste folder,
// which must be running: it counts first, asks unless force ([y/N],
// answered by --force), then deletes and says how many. With nothing to
// delete it says so.
func (e Paste) Rm(ctx context.Context, force bool) error {
	t, err := e.finder.target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	vm := t.f.Name
	if t.in.Status != backend.StatusRunning {
		return msg.VMNotRunning(vm)
	}
	dir, err := settings.PasteDir(e.Dirs)
	if err != nil {
		return err
	}
	n, err := e.count(ctx, t, "count", dir)
	if err != nil {
		return err
	}
	if n == 0 {
		e.Say(msg.PasteRmNone(dir, vm))
		return nil
	}
	if !force {
		yes, err := e.Ask(msg.PasteRmAsk(n, dir, vm).AnsweredBy("--force"))
		if err != nil || !yes {
			return err
		}
	}
	return e.rm(ctx, t, dir)
}

// offerRm is a launch's offer, once it started t's VM, which was stopped,
// to delete what devvm paste left in its paste folder: it asks, [y/N],
// when there's anything, and where it can't ask it keeps them and says
// nothing, as devvm paste rm deletes them later. A count that fails is
// in the run log, and the launch goes on.
func (e Paste) offerRm(ctx context.Context, t *target) error {
	dir, err := settings.PasteDir(e.Dirs)
	if err != nil {
		return err
	}
	n, err := e.count(ctx, t, "count", dir)
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		return nil //nolint:nilerr // the count's failure is in the run log; the launch goes on without the offer
	case n == 0:
		return nil
	}
	yes, err := e.Ask(msg.PasteLaunchAsk(n, dir, t.f.Name).AnsweredBy("devvm paste rm"))
	if err != nil {
		return nil //nolint:nilerr // the question's refusal: the files stay for devvm paste rm
	}
	if !yes {
		return nil
	}
	return e.rm(ctx, t, dir)
}

// rm deletes what devvm paste left in dir in t's VM and says how many.
func (e Paste) rm(ctx context.Context, t *target, dir string) error {
	n, err := e.count(ctx, t, "rm", dir)
	if err != nil {
		return err
	}
	e.Info(msg.PasteRmDone(n, dir, t.f.Name))
	return nil
}

// count runs paste.sh's cmd, count or rm, on dir in t's VM and reads the
// number it prints, the one thing devvm reads back from the VM: a whole
// number within pasteCountMax bytes, or the command's failure
// (paste_count_failed, paste_rm_not_done).
func (e Paste) count(ctx context.Context, t *target, cmd, dir string) (int, error) {
	out := &bounded{max: pasteCountMax}
	err := e.run(ctx, t, []string{cmd, dir}, nil, out)
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	n := -1
	if err == nil && !out.over {
		n, err = strconv.Atoi(strings.TrimSpace(out.String()))
	}
	if err != nil || n < 0 {
		m := msg.PasteCountFailed(dir, t.f.Name)
		if cmd == "rm" {
			m = msg.PasteRmNotDone(dir, t.f.Name)
		}
		return 0, backend.Logged(m)
	}
	return n, nil
}

// run runs paste.sh (assets/guest/paste.sh) in t's running VM with args
// after its command name, the script itself an argument of sh -c, so
// nothing of it is left on disk, with stdin as its input and stdout
// where its output goes (nil for none). The VM's stderr goes to the run
// log. The connection carries no SSH agent: it's the backend's Exec as
// it is, never target.exec.
func (e Paste) run(ctx context.Context, t *target, args []string, stdin io.Reader, stdout io.Writer) error {
	script, err := fs.ReadFile(e.Assets, "guest/paste.sh")
	if err != nil {
		return err
	}
	return t.b.Exec(ctx, t.f.Name, pasteArgv(string(script), args), backend.ExecOpts{Stdin: stdin, Stdout: stdout, Stderr: e.vmLog()})
}

// pasteArgv is the command that runs paste.sh, the script, with args
// after its command name: sh -c's script, named devvm-paste.
func pasteArgv(script string, args []string) []string {
	return append([]string{"sh", "-c", script, "devvm-paste"}, args...)
}

// pastePayloadOf is what to send for clip, named at now: an image as it
// came, or a copied file, which may be in a project folder, which its VM
// writes. The path is cleaned, so a folder's trailing slash goes, and
// opened as named, following no link but root's (platform.OpenNoLinks:
// /tmp on macOS is one). Nothing of the path is resolved first, because
// a VM can plant a link in the project folder before the copy as well as
// swap one in after it, and either, say to a key of yours, fails rather
// than sends what it points to. The file opened is checked to be a
// regular file within pasteMax.
func pastePayloadOf(clip platform.Clip, now time.Time) (pastePayload, error) {
	stem := pasteStem(now)
	if clip.File == "" {
		return pastePayload{data: clip.Image, name: stem + "." + platform.ImageExt(clip.Type), kind: PasteImage, mime: clip.Type}, nil
	}
	file := filepath.Clean(clip.File)
	f, err := platform.OpenNoLinks(file)
	if err != nil {
		if errors.Is(err, platform.ErrLinkOnPath) {
			return pastePayload{}, msg.PasteFileLink(file)
		}
		return pastePayload{}, err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	switch {
	case err != nil:
		return pastePayload{}, err
	case !fi.Mode().IsRegular():
		return pastePayload{}, msg.PasteNotFile(file)
	case fi.Size() > pasteMax:
		return pastePayload{}, msg.PasteTooLarge(pasteMaxText)
	}
	data, err := io.ReadAll(io.LimitReader(f, pasteMax+1))
	switch {
	case err != nil:
		return pastePayload{}, err
	case len(data) > pasteMax:
		return pastePayload{}, msg.PasteTooLarge(pasteMaxText)
	}
	return pastePayload{data: data, name: stem + "-" + pasteBase(filepath.Base(file)), kind: PasteFile, source: file}, nil
}

// pasteStem is the start of a pasted file's name: paste-YYYYMMDD-HHMMSS
// and 8 hex digits at random, so two pastes in one second differ and the
// name is from a fixed set of characters.
func pasteStem(now time.Time) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return "paste-" + now.Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

// pasteBaseMax is the most of a copied file's name a pasted file keeps.
const pasteBaseMax = 64

// pasteBase is a copied file's name as a pasted file's name carries it:
// every byte outside [A-Za-z0-9._-] becomes _, at most pasteBaseMax
// bytes, never empty (file), because it goes into a shell in the VM and
// onto the clipboard.
func pasteBase(base string) string {
	b := []byte(base)
	for i, c := range b {
		safe := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
		if !safe {
			b[i] = '_'
		}
	}
	if len(b) > pasteBaseMax {
		b = b[:pasteBaseMax]
	}
	if len(b) == 0 {
		return "file"
	}
	return string(b)
}

// clipError is the message for what reading the clipboard failed with
// (platform.ReadClip's errors), or err as it is.
func clipError(err error) error {
	var many *platform.ClipManyFiles
	var missing *platform.ClipToolMissing
	var failed *platform.ClipToolFailed
	switch {
	case errors.As(err, &many):
		return msg.PasteManyFiles(many.N)
	case errors.As(err, &missing):
		return msg.PasteNeedsTool(missing.Tool)
	case errors.As(err, &failed):
		return msg.PasteToolFailed(msg.Printable(strings.TrimSpace(failed.Stderr)), failed.Tool).Because(err)
	case errors.Is(err, platform.ErrClipEmpty):
		return msg.PasteNothing()
	case errors.Is(err, platform.ErrClipFileGone):
		return msg.PasteFileGone()
	case errors.Is(err, platform.ErrClipTooLarge):
		return msg.PasteTooLarge(pasteMaxText)
	case errors.Is(err, platform.ErrClipNoSession):
		return msg.PasteNoSession()
	case errors.Is(err, errors.ErrUnsupported):
		return msg.PasteUnsupported()
	}
	return err
}
