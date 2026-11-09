package lima

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/shell"
)

// remountTimeout bounds the remount and its check: a guest whose mount
// hangs mustn't hold the start.
const remountTimeout = time.Minute

// notMounted is what remountCheck prints for a folder that isn't a mount
// point.
const notMounted = "devvm-not-mounted"

// cacheChanges reports whether a start of in as w wants it changes its
// project folder mount's cache mode (projectMount, mount.same's cache).
func cacheChanges(in instance, w backend.Want) bool {
	return w.Project != "" && w.ProjectCache != "" && in.ninep() && in.backend().ProjectCache != w.ProjectCache
}

// remountProject mounts the project folder at dir in a VM just started
// again, as its /etc/fstab now has it, with the cache mode want. Lima's
// cloud-init rewrites fstab with a mount's new options at the boot after
// an edit, but systemd has mounted the folder by the old line by then, and
// a 9p share's tag stays the same, so without this the new mode would wait
// for a second boot. Nothing in the VM uses the folder yet, unless
// something it starts at boot does (a container), which keeps the old
// mount busy. Its output goes to the run log. Then the folder is checked:
// one that isn't mounted, or that the check can't see (ssh not answering),
// stops the VM and fails the start, as what's written there would land on
// the VM's disk and vanish under the share at its next mount; one mounted
// with another mode (the unmount refused, or a degraded start's old fstab)
// is warned of, and gets the new mode at the VM's next start. An interrupt
// meanwhile stops the VM too, whatever its tier.
func remountProject(ctx context.Context, l *machines, vm, dir, want string) error {
	stopIt := func(ctx context.Context) { _ = l.stop(ctx, vm) }
	done := l.AddCleanup(func(ctx context.Context) {
		if l.Interrupted() {
			stopIt(ctx)
		}
	})
	// The stop outlives the remount's timeout.
	parent := context.WithoutCancel(ctx)
	ctx, cancel := context.WithTimeout(ctx, remountTimeout)
	defer cancel()
	d := shell.Quote(dir)
	script := "umount " + d + " || exit 3; mount " + d + " || { mount " + d + "; mountpoint -q " + d + " || exit 4; }"
	_ = l.Runner.Run(ctx, msg.Remounting(vm), bounded(l.limactl(ctx, "shell", "--workdir", "/", vm, "sudo", "sh", "-c", script)))
	has, err := mountedCache(ctx, l, vm, dir)
	if !l.Interrupted() {
		// An interrupt's is the cleanup's to stop, once the command returns.
		done()
	}
	// Interrupted, the cleanup stops it: one stop, not two at once.
	stopNow := func() {
		if !l.Interrupted() {
			stopIt(parent)
		}
	}
	switch {
	case err != nil:
		stopNow()
		return msg.MountCacheUnchecked(vm).Because(err)
	case has == "":
		stopNow()
		return msg.MountCacheUnmounted(vm)
	case has != want:
		l.Warn(msg.MountCacheRemountFailed(has, vm, want))
	}
	return nil
}

// maxCheckOutput caps what's read of the guest's answer to a mount check:
// one line of /proc/self/mountinfo, or notMounted.
const maxCheckOutput = 4096

// lastMount is the awk program that prints the last line of a mountinfo
// file whose mount point, its fifth field, is $P exactly, as mountinfo
// escapes it (mountinfoPath): where mounts are stacked, the last is the
// one visible there. P comes in through the environment, which awk reads
// as it is, unlike -v, which would undo the escapes. It's POSIX, so
// busybox's awk runs it, as on an Alpine base, which has no findmnt.
const lastMount = `awk '$5 == ENVIRON["P"] { l = $0 } END { print l }'`

// mountedCache asks the guest how its folder at dir is mounted: the cache
// mode of the mount it sees there, cacheOfOptions's, "" when it isn't a
// mount point, or an error when the guest doesn't answer or its answer
// isn't a mountinfo line. Only the last line counts, and only so much of
// it is read (maxCheckOutput).
func mountedCache(ctx context.Context, l *machines, vm, dir string) (string, error) {
	d := shell.Quote(dir)
	check := "if mountpoint -q " + d + "; then P=" + shell.Quote(mountinfoPath(dir)) + " " + lastMount +
		" /proc/self/mountinfo; else echo " + notMounted + "; fi"
	cmd := bounded(l.limactl(ctx, "shell", "--workdir", "/", vm, "sh", "-c", check))
	out := &capped{max: maxCheckOutput}
	cmd.Stdout = out
	if err := l.Runner.Attach(cmd); err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	switch last {
	case notMounted:
		return "", nil
	case "":
		return "", errors.New("the guest gave no answer")
	}
	opts, ok := optionsOfMountinfo(last)
	if !ok {
		return "", errors.New("the guest's answer isn't a line of mountinfo")
	}
	return cacheOfOptions(opts), nil
}

// mountinfoPath is path as /proc/self/mountinfo writes a mount point: a
// space, a tab, a newline and a backslash as octal escapes (\040, \011,
// \012, \134), every other byte as it is.
func mountinfoPath(path string) string {
	return strings.NewReplacer(`\`, `\134`, " ", `\040`, "\t", `\011`, "\n", `\012`).Replace(path)
}

// optionsOfMountinfo is a mountinfo line's options, the mount's and then
// its file system's, joined by a comma, as findmnt's OPTIONS shows them;
// false for a line that isn't one. The optional fields between the mount
// options and the file system's type vary in number, so the file
// system's options are the third field after the "-" that ends them.
func optionsOfMountinfo(line string) (string, bool) {
	f := strings.Fields(line)
	if len(f) < 10 {
		return "", false
	}
	sep := slices.Index(f[6:], "-")
	if sep < 0 || 6+sep+3 >= len(f) {
		return "", false
	}
	return f[5] + "," + f[6+sep+3], true
}

// bounded is cmd, its output's pipes closed a second after it ends or
// its context does: limactl shell's ssh can leave a child behind that
// holds them open, which would hold the caller past its timeout.
func bounded(cmd *exec.Cmd) *exec.Cmd {
	cmd.WaitDelay = time.Second
	return cmd
}

// capped keeps the first max bytes written to it, and takes the rest
// without keeping it. Its buffer isn't embedded: io.Copy would use the
// buffer's own ReadFrom, which keeps everything.
type capped struct {
	buf bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (c *capped) String() string { return c.buf.String() }

// cacheOption finds a 9p mount's cache option in its mount options.
var cacheOption = regexp.MustCompile(`(?:^|,)cache=([^,]*)`)

// cacheOfOptions is the cache mode a 9p mount's options name
// (optionsOfMountinfo's):
// the kernel shows cache=0x5 for mmap, and no cache option at all for
// none. Any other is "unknown": the guest's text never reaches a message.
func cacheOfOptions(opts string) string {
	m := cacheOption.FindStringSubmatch(opts)
	if m == nil {
		return backend.CacheNone
	}
	switch m[1] {
	case "0x0", "0", backend.CacheNone:
		return backend.CacheNone
	case "0x5", "5", backend.CacheMmap:
		return backend.CacheMmap
	}
	return "unknown"
}
