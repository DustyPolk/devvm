package lima

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ubuntu"
)

var keyPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

// BuildImage builds the image's instance, devvm-img-<key>, which is then
// the image: base is a VM made from Ubuntu's cloud image with base.yaml's
// settings (with spec.From, from that image converted into a disk:
// convert), a layer a clone of its parent's instance. Neither mounts
// anything from the host. It starts confined as spec.Net asks (no port
// forwards, and on macOS a sandbox), runs the scripts in order, and stops.
//
// Until it's built, an interrupted run deletes the instance, and so does a
// failed create or clone: a cleanup on the ui stack, which runs after the
// caller has shown the run log's end. Once it exists, a failure keeps it,
// stopped, for inspection; the next build deletes it (it has no record).
func (l *Lima) BuildImage(ctx context.Context, spec backend.ImageSpec) (backend.Built, error) {
	if spec.From != nil && spec.Parent != "" {
		return backend.Built{}, errors.New("lima: only base starts from a container image")
	}
	if spec.Net != backend.NetInternet && spec.Net != backend.NetNone {
		return backend.Built{}, fmt.Errorf("lima: builds are confined, not started with network %d", spec.Net)
	}
	for _, k := range []string{spec.Key, spec.Parent} {
		if k != "" && !keyPattern.MatchString(k) {
			return backend.Built{}, fmt.Errorf("lima: invalid image key %q", k)
		}
	}
	inst := ImagePrefix + spec.Key
	if _, err := l.inspect(ctx, inst); err == nil {
		return backend.Built{}, fmt.Errorf("lima: %s exists already", inst)
	} else if !errors.Is(err, backend.ErrNotFound) {
		return backend.Built{}, err
	}
	parent := ImagePrefix + spec.Parent
	if spec.Parent != "" {
		// Its parent stays as it is: stopped, as clones need.
		in, err := l.inspect(ctx, parent)
		if errors.Is(err, backend.ErrNotFound) {
			return backend.Built{}, msg.ImageMissing(spec.Parent)
		} else if err != nil {
			return backend.Built{}, err
		}
		if backend.Status(in.Status) != backend.StatusStopped {
			if err := l.stop(ctx, parent); err != nil {
				return backend.Built{}, err
			}
		}
	}

	var created atomic.Bool // an interrupt's cleanups may run while this goes on
	remove := l.AddCleanup(func(ctx context.Context) {
		if !created.Load() || l.Interrupted() {
			l.unfinished(ctx, inst)
		}
	})
	// Its steps: create or clone, configure and start (Start's edit is
	// always there for a build: its ports, and on macOS its ssh port),
	// each script, stop; with --from, the conversion's first, and an
	// image only Docker has its export.
	steps := 4 + len(spec.Scripts)
	if spec.From != nil {
		steps += ociSteps
		if !strings.Contains(spec.From.Digest, "@") {
			steps++
		}
	}
	ctx = l.WithSteps(ctx, steps)
	var built backend.Built
	var conv *converted
	if spec.Parent == "" {
		size, err := l.buildSize()
		if err != nil {
			return backend.Built{}, err
		}
		if spec.MemoryGiB > 0 {
			size.MemoryGiB = spec.MemoryGiB
		}
		if spec.From == nil {
			img, err := l.createFromUbuntu(ctx, inst, size)
			if err != nil {
				return backend.Built{}, err
			}
			built = backend.Built{Origin: img.String(), Libc: "gnu"}
		} else {
			if conv, err = l.convert(ctx, spec.From, size); err != nil {
				return backend.Built{}, err
			}
			// base.yaml's settings, booting the converted disk.
			images := []limaImage{{
				Location: filepath.Join(conv.dir, "disk.raw"), Arch: limaArch(l.Arch),
				Kernel: &limaKernel{Location: filepath.Join(conv.dir, "kernel"), Cmdline: conv.cmdline},
				Initrd: &limaFile{Location: filepath.Join(conv.dir, "initrd")},
			}}
			assets, err := l.assets()
			if err != nil {
				return backend.Built{}, err
			}
			if err := l.Runner.Run(ctx, msg.Creating(inst), l.limactl(ctx, createArgs(inst, images, size, assets)...)); err != nil {
				return backend.Built{}, backend.Logged(msg.VMCreateFailed(inst))
			}
			built = backend.Built{Origin: spec.From.Digest, Libc: conv.libc}
		}
	} else if err := l.Runner.Run(ctx, msg.CloningImage(image.Parent(spec.Image)),
		l.limactl(ctx, "clone", "--tty=false", "--mount-none", parent, inst)); err != nil {
		return backend.Built{}, backend.Logged(msg.VMCreateFailed(inst))
	}
	created.Store(true)
	// A failed start leaves it stopped, with its console logs.
	if err := l.Start(ctx, inst, backend.Confinement{Net: spec.Net}); err != nil {
		return backend.Built{}, err
	}
	if conv != nil {
		// Lima copied the converted files in as it started.
		conv.done(ctx)
	}
	// Once it's running, a failure stops it (unless the run was
	// interrupted: then it's deleted).
	failed := func(err error) (backend.Built, error) {
		if ctx.Err() == nil {
			_ = l.stop(ctx, inst)
		}
		return backend.Built{}, err
	}
	for _, s := range spec.Scripts {
		if err := l.runScript(ctx, inst, s); err != nil {
			return failed(err)
		}
	}
	// Flushed first: a guest that ignores Lima's stop request is stopped
	// hard.
	if err := l.Runner.Run(ctx, msg.M{}, l.limactl(ctx, "shell", "--workdir", "/", inst, "sync")); err != nil {
		return failed(backend.Logged(msg.StopFailed(inst)))
	}
	if err := l.stop(ctx, inst); err != nil {
		return backend.Built{}, err
	}
	if l.overlays() {
		// Project VMs' disks are overlays of this one: it never changes
		// again.
		if fi, err := os.Stat(l.disk(inst)); err == nil {
			if err := os.Chmod(l.disk(inst), fi.Mode().Perm()&^0o222); err != nil {
				return backend.Built{}, err
			}
		}
	}
	remove()
	return built, nil
}

// limaImage is an entry in a Lima config's images: a disk image, or a
// disk with the kernel and initrd Lima boots directly.
type limaImage struct {
	Location string      `json:"location"`
	Arch     string      `json:"arch"`
	Digest   string      `json:"digest,omitempty"`
	Kernel   *limaKernel `json:"kernel,omitempty"`
	Initrd   *limaFile   `json:"initrd,omitempty"`
}

type limaKernel struct {
	Location string `json:"location"`
	Cmdline  string `json:"cmdline"`
}

type limaFile struct {
	Location string `json:"location"`
}

// createFromUbuntu creates a VM from Ubuntu's newest cloud image, with
// base.yaml's settings and nothing from the host mounted, or the mounts a
// --set in opts gives it (--set applies after --mount-none). It returns
// the image it used. It gets size's CPUs and memory (buildSize's).
func (l *Lima) createFromUbuntu(ctx context.Context, vm string, size backend.Size, opts ...string) (ubuntu.Image, error) {
	img, err := l.Ubuntu(ctx)
	if err != nil {
		return ubuntu.Image{}, err
	}
	assets, err := l.assets()
	if err != nil {
		return ubuntu.Image{}, err
	}
	images := []limaImage{{Location: img.URL, Arch: limaArch(l.Arch), Digest: img.Digest}}
	if err := l.Runner.Run(ctx, msg.CreatingFromUbuntu(vm), l.limactl(ctx, createArgs(vm, images, size, assets, opts...)...)); err != nil {
		return ubuntu.Image{}, backend.Logged(msg.VMCreateFailed(vm))
	}
	return img, nil
}

// createArgs are limactl create's arguments for a VM booting images, with
// nothing from the host mounted but what opts' --set gives it, size's CPUs
// and memory, and the rest of base.yaml's settings (from devvm's files,
// written out in assets).
func createArgs(vm string, images []limaImage, size backend.Size, assets string, opts ...string) []string {
	args := append([]string{"create", "--tty=false", "--mount-none", "--name=" + vm, "--set", ".images = " + yqJSON(images)}, opts...)
	return append(args, "--cpus="+strconv.Itoa(size.CPUs), "--memory="+gibString(size.MemoryGiB), filepath.Join(assets, "base.yaml"))
}

// limaArch is Lima's name for a Go architecture.
func limaArch(goarch string) string {
	switch goarch {
	case "arm64":
		return "aarch64"
	case "amd64":
		return "x86_64"
	}
	return goarch
}

// runScript runs a script in the VM, fed to bash on its standard input.
func (l *Lima) runScript(ctx context.Context, vm string, s backend.Script) error {
	cmd := l.limactl(ctx, "shell", vm, "bash", "-s")
	cmd.Stdin = bytes.NewReader(s.Data)
	if err := l.Runner.Run(ctx, msg.RunningScript(s.Name, vm), cmd); err != nil {
		return backend.Logged(msg.ScriptFailed(s.Name, vm))
	}
	return nil
}

// unfinished deletes an image's instance whose build didn't finish: on an
// interrupt, or when its create or clone failed.
func (l *Lima) unfinished(ctx context.Context, inst string) {
	if _, err := l.inspect(ctx, inst); err != nil {
		return // never created, or Lima can't say: nothing devvm can delete
	}
	if l.Interrupted() {
		l.Info(msg.InterruptedImage(inst))
	} else {
		l.Info(msg.UnfinishedImage(inst))
	}
	_ = l.delete(ctx, inst)
}

var (
	cpusLine   = regexp.MustCompile(`(?m)^cpus:\s*([0-9]+)\s*$`)
	memoryLine = regexp.MustCompile(`(?m)^memory:\s*([0-9.]+)GiB\s*$`)
)

// buildSize is base's CPUs and memory: base.yaml's, fitted to the host
// (backend.FitCPUs, backend.FitMemory), so a host with fewer CPUs gets
// its own count and one that base.yaml's memory would leave less than 2
// GiB gets half its memory. Images built from base, and their VMs, keep
// them.
func (l *Lima) buildSize() (backend.Size, error) {
	data, err := fs.ReadFile(l.Assets, "base.yaml")
	if err != nil {
		return backend.Size{}, err
	}
	c := cpusLine.FindSubmatch(data)
	if c == nil {
		return backend.Size{}, errors.New("lima: base.yaml sets no CPUs")
	}
	cpus, err := strconv.Atoi(string(c[1]))
	if err != nil {
		return backend.Size{}, err
	}
	m := memoryLine.FindSubmatch(data)
	if m == nil {
		return backend.Size{}, errors.New("lima: base.yaml sets no memory in GiB")
	}
	mem, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return backend.Size{}, err
	}
	host, err := l.Memory()
	if err != nil {
		return backend.Size{}, err
	}
	return backend.Size{CPUs: backend.FitCPUs(cpus, l.CPUs()), MemoryGiB: backend.FitMemory(mem, host)}, nil
}
