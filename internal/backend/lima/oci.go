package lima

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
)

// OCIVM is the throwaway VM a container image is converted in (--from).
const OCIVM = "devvm-oci-build"

// convertedSize is the converted disk's size; Lima grows it to base.yaml's.
const convertedSize = "20G"

// ociSteps is the conversion's steps: the build VM's create, configure
// and start, the conversion, and its delete.
const ociSteps = 5

// converted is a container image converted into what Lima boots
// directly, in a folder of devvm's own (out/vm). done removes the folder
// and lets the next conversion go: once base's VM has started, which
// copies the files in.
type converted struct {
	dir, cmdline, libc string
	done               func(context.Context)
}

// convert turns spec.From into a disk, kernel and initrd, without Docker
// (unless only Docker has the image): a throwaway VM from Ubuntu's cloud
// image runs oci/build.sh, from devvm's files written out and mounted
// read-only, where crane pulls exactly the recorded digest and bootify.sh
// adds what a container lacks. The image's own code runs as
// root in that VM, so everything it writes is untrusted: it gets only a
// subfolder of devvm's temporary folder, and what comes out is checked
// before Lima sees it. One conversion runs at a time. The build VM and
// the folder go however the run ends. With the user's CAs (trust, a
// build's Trust.Add), the build VM trusts them, and build.sh has the
// image's chroot trust them while bootify.sh runs, with devvm's guest
// folder (ca.sh) mounted read-only too.
func (l *Lima) convert(ctx context.Context, f *backend.From, size backend.Size, trust backend.Script) (*converted, error) {
	if f.Digest == "" {
		return nil, msg.NoDigest(f.Ref)
	}
	if !from.Valid(f.Ref) || !from.Valid(f.Digest) {
		return nil, msg.BadFrom(msg.Shown(f.Ref))
	}
	tmp, err := os.MkdirTemp("", "devvm-oci.")
	if err != nil {
		return nil, err
	}
	// Its real path: macOS's temporary folder is under the /var link.
	out, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	var unlock func()
	cleanup := func(ctx context.Context) {
		_ = l.deleteIfThere(ctx, OCIVM)
		_ = os.RemoveAll(out)
		if unlock != nil {
			unlock()
		}
	}
	remove := l.AddCleanup(cleanup)
	vmOut := filepath.Join(out, "vm")
	if err := os.Mkdir(vmOut, 0o700); err != nil {
		return nil, err
	}
	// One at a time: each uses OCIVM, and starts by deleting it.
	if unlock, err = l.lockOCI(ctx); err != nil {
		return nil, err
	}
	ref := f.Digest
	if !strings.Contains(ref, "@") {
		// Docker's ID: an image only Docker has.
		if ref, err = l.exportDocker(ctx, f, vmOut); err != nil {
			return nil, err
		}
	}

	l.Info(msg.ConvertingInVM(f.Ref, OCIVM))
	if err := l.deleteIfThere(ctx, OCIVM); err != nil {
		return nil, err
	}
	assets, err := l.assets()
	if err != nil {
		return nil, err
	}
	oci, guest := filepath.Join(assets, "oci"), filepath.Join(assets, "guest")
	ms := []mount{{Location: vmOut, Writable: ptr(true)}, {Location: oci, Writable: ptr(false)}}
	if len(trust.Data) > 0 {
		ms = append(ms, mount{Location: guest, Writable: ptr(false)})
	}
	mounts, err := mountsSet(ms)
	if err != nil {
		return nil, err
	}
	if _, err := l.createFromUbuntu(ctx, OCIVM, size, "--set", mounts); err != nil {
		return nil, err
	}
	// crane pulls from the registry, behind a company proxy through a
	// proxy of devvm's (buildproxy.go).
	conf, hp, proxyDone, err := buildProxy(ctx, l, OCIVM, backend.NetInternet)
	if err != nil {
		return nil, err
	}
	if err := l.Start(ctx, OCIVM, conf); err != nil {
		return nil, err
	}
	if err := l.runScript(ctx, OCIVM, trust, true); err != nil {
		return nil, err
	}
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	script := filepath.Join(oci, "build.sh")
	args := []string{
		"shell", "--workdir", "/", OCIVM, "sudo", "bash", script, oci, vmOut, ref, f.Ref, u.Username,
		strconv.Itoa(os.Getuid()), "/home/" + u.Username + ".guest", convertedSize,
	}
	if len(trust.Data) > 0 {
		args = append(args, filepath.Join(guest, "ca.sh"))
	}
	cmd := l.limactl(ctx, args...)
	// With the build's proxy, its variables come first on the input, and
	// the script runs with its arguments after them.
	if pre := proxyExports(hp); pre != nil {
		at := slices.Index(args, "bash") + 1
		cmd = l.limactl(ctx, slices.Concat(args[:at], []string{"-s", "--"}, args[at:])...)
		cmd.Stdin = bytes.NewReader(append(pre, "exec bash \"$@\"\n"...))
	}
	if err := l.Runner.Run(ctx, msg.Converting(f.Ref), cmd); err != nil {
		return nil, backend.Logged(msg.ConvertFailed(f.Ref))
	}
	if err := l.delete(ctx, OCIVM); err != nil {
		return nil, err
	}
	proxyDone(true)
	// The build VM is gone, so nothing changes them now.
	c := &converted{dir: vmOut}
	if c.libc, c.cmdline, err = checkConverted(vmOut, f.Ref); err != nil {
		return nil, err
	}
	c.done = func(ctx context.Context) {
		cleanup(ctx)
		remove()
	}
	return c, nil
}

// exportDocker saves an image only Docker has into dir, as image.tar,
// which build.sh reads for "-". The export must be the image looked up:
// it's saved by name, so the tag may have moved since.
func (l *Lima) exportDocker(ctx context.Context, f *backend.From, dir string) (string, error) {
	if !l.DockerRunning(ctx) {
		return "", msg.DockerOnly(f.Ref)
	}
	l.Info(msg.ExportingDocker(f.Ref))
	file := filepath.Join(dir, "image.tar")
	cmd := exec.CommandContext(ctx, "docker", "save", "-o", file, f.Ref) //nolint:gosec // a checked reference
	if err := l.Runner.Run(ctx, msg.ExportingDocker(f.Ref), cmd); err != nil {
		return "", backend.Logged(msg.DockerExportFailed(f.Ref))
	}
	if !exportIs(file, f.Digest) {
		return "", msg.DockerImageChanged(f.Ref)
	}
	return "-", nil
}

// maxExportJSON caps what's read of an export's manifest.json and
// index.json: a few KB.
const maxExportJSON = 16 << 20

var idPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// exportIs reports whether a docker save tarball holds the image with
// Docker ID id: its config, as Docker's own image store names it, or its
// manifest, as the containerd image store does.
func exportIs(file, id string) bool {
	if !idPattern.MatchString(id) {
		return false
	}
	h := strings.TrimPrefix(id, "sha256:")
	fh, err := os.Open(file) //nolint:gosec // devvm's own temporary folder
	if err != nil {
		return false
	}
	defer func() { _ = fh.Close() }()
	var manifest []struct{ Config string }
	var index struct {
		Manifests []struct{ Digest string }
	}
	tr := tar.NewReader(fh)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		var dst any
		switch hdr.Name {
		case "manifest.json":
			dst = &manifest
		case "index.json":
			dst = &index
		default:
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxExportJSON))
		if err == nil {
			_ = json.Unmarshal(data, dst)
		}
	}
	for _, m := range manifest {
		if m.Config == "blobs/sha256/"+h || m.Config == h+".json" {
			return true
		}
	}
	for _, m := range index.Manifests {
		if m.Digest == id {
			return true
		}
	}
	return false
}

var cmdlinePattern = regexp.MustCompile(`^[A-Za-z0-9=,./:_ -]+$`)

// maxSmallOutput caps what's read of the conversion's text files.
const maxSmallOutput = 4096

// checkConverted checks what the conversion wrote, and returns its libc
// and kernel command line. Each file must be a regular file (a link could
// point Lima or devvm at a host file); what goes into Lima's config must
// be plain text; the disk must be raw ext4, as build.sh writes it
// (another format, qcow2 or VMDK, can name host files as its backing file
// or extents).
func checkConverted(dir, ref string) (libc, cmdline string, err error) {
	bad := func(file string) error { return msg.ConvertBadOutput(file, ref) }
	for _, f := range []string{"disk.raw", "kernel", "initrd", "cmdline", "libc"} {
		if fi, err := os.Lstat(filepath.Join(dir, f)); err != nil || !fi.Mode().IsRegular() {
			return "", "", bad(f)
		}
	}
	text := func(f string) string {
		fh, err := os.Open(filepath.Join(dir, f)) //nolint:gosec // devvm's own temporary folder
		if err != nil {
			return ""
		}
		defer func() { _ = fh.Close() }()
		data, _ := io.ReadAll(io.LimitReader(fh, maxSmallOutput))
		return strings.TrimRight(string(data), "\n")
	}
	if libc = text("libc"); libc != "gnu" && libc != "musl" {
		return "", "", bad("libc")
	}
	if cmdline = text("cmdline"); !cmdlinePattern.MatchString(cmdline) {
		return "", "", bad("cmdline")
	}
	if !ext4(filepath.Join(dir, "disk.raw")) {
		return "", "", bad("disk.raw")
	}
	return libc, cmdline, nil
}

// ext4 reports whether a file starts with an ext4 superblock: its magic,
// 0xEF53, at byte 1080.
func ext4(path string) bool {
	fh, err := os.Open(path) //nolint:gosec // devvm's own temporary folder
	if err != nil {
		return false
	}
	defer func() { _ = fh.Close() }()
	magic := make([]byte, 2)
	if _, err := fh.ReadAt(magic, 1080); err != nil {
		return false
	}
	return bytes.Equal(magic, []byte{0x53, 0xef})
}

// ociWaitNote is how many looks at the lock a conversion takes before it
// says it's waiting.
const ociWaitNote = 3

// lockOCI takes the conversion lock: a folder holding its devvm's pid. It
// waits while that devvm runs, and takes the lock over from one that
// ended without unlocking (killed, a crash). The unlock leaves another
// devvm's lock alone.
func (l *Lima) lockOCI(ctx context.Context) (unlock func(), err error) {
	dir := l.Dirs.OCIBuildLock()
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return nil, err
	}
	pidFile := filepath.Join(dir, "pid")
	holder := func() int {
		data, err := os.ReadFile(pidFile) //nolint:gosec // devvm's own folder
		if err != nil {
			return 0
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		return pid
	}
	for i := 1; ; i++ {
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if pid := holder(); pid > 0 && !alive(pid) {
			_ = os.RemoveAll(dir)
			continue
		}
		if i == ociWaitNote {
			l.Info(msg.OCIWaiting())
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(l.OCIWait):
		}
	}
	me := os.Getpid()
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(me)+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return func() {
		if holder() == me {
			_ = os.RemoveAll(dir)
		}
	}, nil
}
