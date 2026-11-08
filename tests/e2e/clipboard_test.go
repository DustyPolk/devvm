//go:build e2e

package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/clipboard"
)

// clip is the clipboard section's VM and its stand-in for this machine's
// clipboard: never the real one.
type clip struct {
	s       *S
	dir, vm string
	env     []string // PATH with the stand-in readers first, and a Wayland display for them
	img     string   // the stand-in clipboard's image: there, or not
	calls   string   // each reader's command line, one a line
	want    string   // the SHA-256 of the image as devvm sends it (clipboard.Sanitize)
}

// clipMarker is text in the image's own metadata, which never reaches the VM.
const clipMarker = "devvm-e2e-clip-marker"

// clipboardSection: clipboard images (internals/design.md, "Clipboard
// images"), each check the end-to-end proof of a row of its threat check.
// One small VM on the allowlist tier; a stand-in for this machine's
// clipboard readers (wl-paste, xclip, osascript) first on PATH for the
// devvm that starts its link session, which never reads your clipboard.
func clipboardSection(s *S) {
	s.Section("clipboard images")
	c := &clip{s: s.Needing(backend.FeatureLink)}
	c.dir, _ = s.NewProject("clip")
	c.vm, _, _ = s.Launch(c.dir, "exit", "--network", "allowlist", "--no-git-protect", "--cpus", "2", "--memory", "2", "--ports", "none")
	s.Check("the stand-in clipboard, an image with text in its metadata", func() error { return clipFake(c) })
	s.Check("devvm clipboard images: warned of, on at once, with xclip and wl-paste in the VM devvm's", func() error { return clipOn(c) })
	s.Check("an agent's paste: the types, then the image as devvm sanitized it, with one read of this machine's clipboard", func() error { return clipPaste(c) })
	s.Check("its socket is mode 600 in a folder of mode 700; nothing on this machine's loopback; the VM's proxy refuses its address", func() error { return clipConfined(c) })
	s.Check("text and an empty clipboard: nothing, and text is never asked of this machine", func() error { return clipNoImage(c) })
	s.Check("root in the VM sending anything else: closed with nothing, the clipboard never read, refused in devvm network log", func() error { return clipHostile(c) })
	s.Check("devvm clipboard off: the stand-ins and the address gone, and the session with them", func() error { return clipOff(c) })
	_, _ = s.Devvm(c.dir, "rm", "--force", "--no-export")
	s.Check("devvm rm leaves no clipboard socket", func() error {
		if f := clipSock(c); Exists(f) {
			return errors.New(f + " stays")
		}
		return nil
	})
}

// clipFake writes the stand-in readers and the image: a PNG with a tEXt
// chunk, which devvm's PNG, encoded again from its pixels, doesn't have.
func clipFake(c *clip) error {
	s := c.s
	bin := filepath.Join(s.sb.work, "clipbin")
	c.img, c.calls = filepath.Join(s.sb.work, "clip.png"), filepath.Join(s.sb.work, "clip-calls")
	for _, f := range []string{bin, c.img, c.calls} {
		madeFile(s.sb, f)
	}
	if err := os.MkdirAll(bin, 0o700); err != nil {
		return err
	}
	script := "#!/bin/sh\nimg='" + c.img + "'\necho \"$(basename \"$0\") $*\" >> '" + c.calls + "'\n" + `case "$(basename "$0") $*" in
"wl-paste --list-types"|"xclip -selection clipboard -t TARGETS -o") [ -f "$img" ] && echo image/png ;;
"wl-paste --no-newline --type image/png"|"xclip -selection clipboard -t image/png -o") [ -f "$img" ] && cat "$img" ;;
osascript*) [ -f "$img" ] && printf '«data PNGf%s»\n' "$(od -An -tx1 -v "$img" | tr -d ' \n' | tr a-f A-F)" || { echo "(-1700)" >&2; exit 1; } ;;
*) exit 1 ;;
esac
`
	for _, n := range []string{"wl-paste", "xclip", "osascript"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(script), 0o700); err != nil { //nolint:gosec // G306: the stand-in runs
			return err
		}
	}
	img, err := clipImage()
	if err != nil {
		return err
	}
	clean, err := clipboard.Sanitize(img)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(clean)
	c.want = hex.EncodeToString(sum[:])
	path := ""
	for _, kv := range s.sb.env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = v
		}
	}
	c.env = []string{"PATH=" + bin + ":" + path, "WAYLAND_DISPLAY=devvm-e2e", "DISPLAY="}
	return os.WriteFile(c.img, img, 0o600)
}

// clipImage is a 32x16 PNG with clipMarker in a tEXt chunk before its end.
func clipImage() ([]byte, error) {
	m := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for x := range 32 {
		m.Set(x, x/2, color.RGBA{200, 30, 30, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		return nil, err
	}
	raw := b.Bytes()
	end := len(raw) - 12 // IEND: length, type, CRC
	data := append([]byte("tEXt"), "Comment\x00"+clipMarker...)
	var chunk []byte
	chunk = binary.BigEndian.AppendUint32(chunk, uint32(len(data)-4)) //nolint:gosec // G115: a few bytes
	chunk = append(chunk, data...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(data))
	return append(append(append([]byte{}, raw[:end]...), chunk...), raw[end:]...), nil
}

func clipOn(c *clip) error {
	s := c.s
	r, err := s.Run(Cmd{Args: []string{s.sb.devvm, "clipboard", "images"}, Dir: c.dir, Env: append(c.env, "DEVVM_MESSAGES=names")})
	if err != nil {
		return err
	}
	if err := said(r, "devvm: clipboard_on_risk vm="+c.vm); err != nil {
		return err
	}
	if !r.Has("==> setting_applied vm=" + c.vm) {
		return errors.New("not applied at once:\n" + r.Out)
	}
	r, err = s.InVM(c.vm, "readlink /usr/local/bin/xclip /usr/local/bin/wl-paste; cat /etc/devvm/clipboard; command -v xclip wl-paste")
	if err != nil {
		return err
	}
	if got, want := out(r), "devvm-clip\ndevvm-clip\n127.77.255.255 7737\n/usr/local/bin/xclip\n/usr/local/bin/wl-paste"; got != want {
		return fmt.Errorf("in the VM: %q, want %q", got, want)
	}
	return s.Until(30*time.Second, func() error {
		if !Exists(clipSock(c)) {
			return errors.New("no socket at " + clipSock(c))
		}
		return nil
	})
}

// clipReads is how many times the stand-ins read the clipboard: a list of
// its types (Linux) or osascript (macOS), each a read's first command.
func clipReads(c *clip) int {
	b, _ := os.ReadFile(c.calls)
	n := 0
	for line := range strings.SplitSeq(string(b), "\n") {
		if strings.HasPrefix(line, "osascript") || line == "wl-paste --list-types" || line == "xclip -selection clipboard -t TARGETS -o" {
			n++
		}
	}
	return n
}

func clipPaste(c *clip) error {
	s := c.s
	before := clipReads(c)
	// Claude Code's check and save, then pi's, as their releases run them.
	r, err := s.InVM(c.vm, `set -e
xclip -selection clipboard -t TARGETS -o 2>/dev/null | grep -E "image/(png|jpeg|jpg|gif|webp|bmp)"
xclip -selection clipboard -t image/png -o > /tmp/a.png
wl-paste --list-types
wl-paste --type image/png --no-newline > /tmp/b.png
sha256sum /tmp/a.png /tmp/b.png | cut -d" " -f1
grep -c `+clipMarker+` /tmp/a.png || true`)
	if err != nil {
		return err
	}
	if got, want := out(r), "image/png\nimage/png\n"+c.want+"\n"+c.want+"\n0"; got != want {
		return fmt.Errorf("the paste: %q, want %q", got, want)
	}
	if n := clipReads(c) - before; n != 1 {
		return fmt.Errorf("%d reads of the clipboard for one paste", n)
	}
	r, err = s.Devvm(c.dir, "network", "log")
	if err != nil {
		return err
	}
	if !strings.Contains(r.Out, "image sent") {
		return errors.New("devvm network log doesn't say an image was sent:\n" + r.Out)
	}
	return nil
}

func clipConfined(c *clip) error {
	s := c.s
	f, dir := clipSock(c), filepath.Dir(clipSock(c))
	Note(s, "%s: %s; %s: %s", f, modeOf(f), dir, modeOf(dir))
	if fi, err := os.Stat(f); err != nil || fi.Mode().Type() != os.ModeSocket || modeOf(f) != "600" || modeOf(dir) != "700" {
		return fmt.Errorf("not a socket of mode 600 in a folder of mode 700: %s", f)
	}
	if Reaches(s, Local(clipboard.Port)) {
		return fmt.Errorf("something on this machine's loopback answers at %d", clipboard.Port)
	}
	r, err := s.InVM(c.vm, `curl -s -o /dev/null -w '%{http_code}' --max-time 10 http://127.77.255.255:7737/ || true`)
	if err != nil {
		return err
	}
	if got := out(r); got != "403" {
		return fmt.Errorf("through the VM's proxy: %q, want 403", got)
	}
	return nil
}

func clipNoImage(c *clip) error {
	s := c.s
	if err := os.Remove(c.img); err != nil {
		return err
	}
	time.Sleep(clipboard.Fresh + time.Second) // the last read is kept that long
	before := clipReads(c)
	r, err := s.InVM(c.vm, `xclip -selection clipboard -o; echo "text $?"; xclip -selection clipboard -t image/png -o | wc -c; wl-paste --list-types; echo "list $?"`)
	if err != nil {
		return err
	}
	if got, want := out(r), "text 1\n0\nlist 1"; got != want {
		return fmt.Errorf("with no image: %q, want %q", got, want)
	}
	b, _ := os.ReadFile(c.calls)
	if strings.Contains(string(b), "-t text") || strings.Contains(string(b), "UTF8_STRING") {
		return errors.New("text asked of this machine:\n" + string(b))
	}
	if clipReads(c) == before {
		return errors.New("the image asked for, but the clipboard never read")
	}
	return nil
}

func clipHostile(c *clip) error {
	s := c.s
	time.Sleep(clipboard.Fresh + time.Second)
	before := clipReads(c)
	r, err := s.InVM(c.vm, `for req in 'hello' 'IMAGE/PNG' 'types extra' "$(head -c 200 /dev/zero | tr '\0' a)"; do
  sudo bash -c 'exec 3<>/dev/tcp/127.77.255.255/7737; printf "%s\n" "$1" >&3; cat <&3 | wc -c' _ "$req"
done
sudo bash -c 'exec 3<>/dev/tcp/127.77.255.255/7737; sleep 4; cat <&3 | wc -c'`)
	if err != nil {
		return err
	}
	if got := strings.Fields(out(r)); strings.Join(got, " ") != "0 0 0 0 0" {
		return fmt.Errorf("answers: %q, want nothing to each", out(r))
	}
	if n := clipReads(c) - before; n != 0 {
		return fmt.Errorf("%d reads of the clipboard for requests that aren't one", n)
	}
	r, err = s.Devvm(c.dir, "network", "log")
	if err != nil {
		return err
	}
	if strings.Count(r.Out, "clipboard image: bad request") < 5 {
		return errors.New("devvm network log doesn't refuse each:\n" + r.Out)
	}
	return nil
}

func clipOff(c *clip) error {
	s := c.s
	r, err := s.DevvmNames(c.dir, "clipboard", "off")
	if err != nil {
		return err
	}
	if !r.Has("==> setting_applied vm=" + c.vm) {
		return errors.New("not applied at once:\n" + r.Out)
	}
	r, err = s.InVM(c.vm, "ls /usr/local/bin/xclip /usr/local/bin/wl-paste /usr/local/bin/devvm-clip /etc/devvm/clipboard 2>/dev/null | wc -l")
	if err != nil {
		return err
	}
	if got := out(r); got != "0" {
		return fmt.Errorf("%s of the stand-ins and the address stay", got)
	}
	return s.Until(30*time.Second, func() error {
		if Exists(clipSock(c)) || lkSession(s, c.vm) != "" {
			return errors.New("the session or its socket stays")
		}
		return nil
	})
}

// clipSock is the clipboard's socket on this machine, named as a link's.
func clipSock(c *clip) string {
	return lkSock(c.s, c.vm, "127.77.255.255", fmt.Sprint(clipboard.Port))
}
