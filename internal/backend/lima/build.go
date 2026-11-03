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
		if k != "" && !image.IsKey(k) {
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
	edits := true // base's config is new: Start edits it
	if spec.Parent != "" {
		// Its parent stays as it is: stopped, as clones need.
		in, err := l.inspect(ctx, parent)
		if errors.Is(err, backend.ErrNotFound) {
			return backend.Built{}, msg.ImageMissing(spec.Parent)
		} else if err != nil {
			return backend.Built{}, err
		}
		edits = cloneEdits(l, in.Config, spec.Net)
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
	// Its steps: create or clone, configure (when Start edits it) and
	// start, each script, stop; with --from, the conversion's first, and
	// an image only Docker has its export.
	steps := 3 + len(spec.Scripts)
	if edits {
		steps++
	}
	if !spec.Trust.None() {
		steps += 2
	}
	// The company proxy, which the build reaches through a proxy of
	// devvm's (buildproxy.go): checked before anything is made.
	host, _, err := buildUpstream(l.Getenv, spec.Net)
	if err != nil {
		return backend.Built{}, err
	}
	if !host.None() {
		steps += 2
	}
	if spec.From != nil {
		steps += ociSteps
		if !spec.Trust.None() {
			steps++
		}
		if !strings.Contains(spec.From.Digest, "@") {
			steps++
		}
	}
	ctx = l.WithSteps(ctx, steps)
	var built backend.Built
	var conv *converted
	if spec.Parent == "" {
		want, size, err := l.buildSize()
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
			if conv, err = l.convert(ctx, spec.From, size, spec.Trust.Add); err != nil {
				return backend.Built{}, err
			}
			if err := l.create(ctx, msg.Creating(inst), inst, createArgs(inst, convertedImages(conv.dir, conv.cmdline, l.Arch), size)); err != nil {
				return backend.Built{}, err
			}
			built = backend.Built{Origin: spec.From.Digest, Libc: conv.libc}
		}
		built.Fitted = backend.Fitted(want, size)
	} else if err := l.Runner.Run(ctx, msg.CloningImage(image.Parent(spec.Image)),
		l.limactl(ctx, layerArgs(parent, inst)...)); err != nil {
		return backend.Built{}, backend.Logged(msg.VMCreateFailed(inst))
	}
	created.Store(true)
	// Its proxy ends with it, or with the run.
	conf, hp, proxyDone, err := buildProxy(ctx, l, inst, spec.Net)
	if err != nil {
		return backend.Built{}, err
	}
	// A failed start leaves it stopped, with its console logs.
	if err := l.Start(ctx, inst, conf); err != nil {
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
	if err := buildScripts(ctx, l, inst, spec, hp); err != nil {
		return failed(err)
	}
	// Flushed first: a guest that ignores Lima's stop request is stopped
	// hard.
	if err := l.Runner.Run(ctx, msg.M{}, l.limactl(ctx, "shell", "--workdir", "/", inst, "sync")); err != nil {
		return failed(backend.Logged(msg.StopFailed(inst)))
	}
	if err := l.stop(ctx, inst); err != nil {
		return backend.Built{}, err
	}
	proxyDone(true)
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

// buildScripts runs a build's scripts in its VM (inst), in order: the
// user's CAs added and sudo told to keep the build's proxy (hp) as root,
// spec's scripts with hp's variables, then both taken out again.
func buildScripts(ctx context.Context, l *Lima, inst string, spec backend.ImageSpec, hp backend.HostProxy) error {
	sudoOn, sudoOff := sudoScripts(hp)
	for _, s := range []backend.Script{spec.Trust.Add, sudoOn} {
		if err := l.runScript(ctx, inst, s, true); err != nil {
			return err
		}
	}
	for _, s := range spec.Scripts {
		if err := l.runScript(ctx, inst, withProxy(hp, s), false); err != nil {
			return err
		}
	}
	for _, s := range []backend.Script{sudoOff, spec.Trust.Remove} {
		if err := l.runScript(ctx, inst, s, true); err != nil {
			return err
		}
	}
	return nil
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

// convertedImages are a --from base's images: base.yaml's settings
// booting the converted disk in dir with its kernel (and cmdline) and
// initrd.
func convertedImages(dir, cmdline, goarch string) []limaImage {
	return []limaImage{{
		Location: filepath.Join(dir, "disk.raw"), Arch: limaArch(goarch),
		Kernel: &limaKernel{Location: filepath.Join(dir, "kernel"), Cmdline: cmdline},
		Initrd: &limaFile{Location: filepath.Join(dir, "initrd")},
	}}
}

// cloneEdits reports whether Start edits a build VM cloned from an image
// whose config is cfg, started with network n, for its step count: on
// macOS always (its ssh port, new at each start), on Linux only when the
// image's own build left its config otherwise, as a clone keeps it.
func cloneEdits(l *Lima, cfg *config, n backend.Net) bool {
	if l.OS == "darwin" {
		return true
	}
	sets, _ := l.startSets(cfg, backend.Confinement{Net: n}, 0)
	return len(sets) > 0
}

// layerArgs are limactl clone's arguments for a layer's instance (inst):
// a clone of its parent's, with nothing from the host mounted.
func layerArgs(parent, inst string) []string {
	return []string{"clone", "--tty=false", "--mount-none", parent, inst}
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
	images := []limaImage{{Location: img.URL, Arch: limaArch(l.Arch), Digest: img.Digest}}
	if err := l.create(ctx, msg.CreatingFromUbuntu(vm), vm, createArgs(vm, images, size, opts...)); err != nil {
		return ubuntu.Image{}, err
	}
	return img, nil
}

// createArgs are limactl create's arguments for a VM booting images, with
// nothing from the host mounted but what opts' --set gives it, size's CPUs
// and memory, and the rest of base.yaml's settings, which create gives it
// on its standard input (-).
func createArgs(vm string, images []limaImage, size backend.Size, opts ...string) []string {
	args := append([]string{"create", "--tty=false", "--mount-none", "--name=" + vm, "--set", ".images = " + yqJSON(images)}, opts...)
	return append(args, "--cpus="+strconv.Itoa(size.CPUs), "--memory="+gibString(size.MemoryGiB), "-")
}

// create runs limactl create with args (createArgs) as step, base.yaml
// from devvm's own files on its standard input: no copy of it on disk.
func (l *Lima) create(ctx context.Context, step msg.M, vm string, args []string) error {
	yaml, err := fs.ReadFile(l.Assets, "base.yaml")
	if err != nil {
		return err
	}
	cmd := l.limactl(ctx, args...)
	cmd.Stdin = bytes.NewReader(yaml)
	if err := l.Runner.Run(ctx, step, cmd); err != nil {
		return backend.Logged(msg.VMCreateFailed(vm))
	}
	return nil
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

// runScript runs a script in the VM, fed on its standard input to bash as
// the VM's user, or with root to sh as root: a build's Trust scripts, of
// which an empty one isn't run.
func (l *Lima) runScript(ctx context.Context, vm string, s backend.Script, root bool) error {
	cmd := l.limactl(ctx, "shell", vm, "bash", "-s")
	if root {
		if len(s.Data) == 0 {
			return nil
		}
		cmd = l.limactl(ctx, "shell", "--workdir", "/", vm, "sudo", "sh", "-s")
	}
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

// buildSize is base's CPUs and memory: base.yaml's (want), and got, them
// fitted to the host (backend.FitCPUs, backend.FitMemory), so a host with
// fewer CPUs gets its own count and one that base.yaml's memory would
// leave less than 2 GiB gets half its memory. Images built from base, and
// their VMs, keep got.
func (l *Lima) buildSize() (want, got backend.Size, err error) {
	data, err := fs.ReadFile(l.Assets, "base.yaml")
	if err != nil {
		return want, got, err
	}
	c := cpusLine.FindSubmatch(data)
	if c == nil {
		return want, got, errors.New("lima: base.yaml sets no CPUs")
	}
	if want.CPUs, err = strconv.Atoi(string(c[1])); err != nil {
		return want, got, err
	}
	m := memoryLine.FindSubmatch(data)
	if m == nil {
		return want, got, errors.New("lima: base.yaml sets no memory in GiB")
	}
	if want.MemoryGiB, err = strconv.ParseFloat(string(m[1]), 64); err != nil {
		return want, got, err
	}
	host, err := l.Memory()
	if err != nil {
		return want, got, err
	}
	got = backend.Size{CPUs: backend.FitCPUs(want.CPUs, l.CPUs()), MemoryGiB: backend.FitMemory(want.MemoryGiB, host)}
	return want, got, nil
}
