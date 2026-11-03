package lima

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
)

// ToolsVM is the throwaway VM shared tools are built in.
const ToolsVM = "devvm-tools-build"

// BuildTools clones base's instance into ToolsVM, with spec.Out its only
// mount, starts it confined as spec.Net asks, runs the script with its
// arguments, and deletes it. A ToolsVM left by an earlier run is deleted
// first, and the VM goes however the run ends (a cleanup on the ui stack).
func (l *Lima) BuildTools(ctx context.Context, spec backend.ToolsSpec) error {
	if spec.Net != backend.NetInternet && spec.Net != backend.NetNone {
		return fmt.Errorf("lima: builds are confined, not started with network %d", spec.Net)
	}
	if spec.Format != l.ToolFormat() {
		return fmt.Errorf("lima: tool images are build.sh's own, not %q", spec.Format)
	}
	if !image.IsKey(spec.Base) {
		return fmt.Errorf("lima: invalid image key %q", spec.Base)
	}
	if !filepath.IsAbs(spec.Out) || !filepath.IsAbs(spec.OutAt) {
		return fmt.Errorf("lima: the tools build's folders must be absolute: %q at %q", spec.Out, spec.OutAt)
	}
	base := ImagePrefix + spec.Base
	in, err := l.inspect(ctx, base)
	if errors.Is(err, backend.ErrNotFound) {
		return msg.ImageMissing(spec.Base)
	} else if err != nil {
		return err
	}
	if backend.Status(in.Status) != backend.StatusStopped {
		if err := l.stop(ctx, base); err != nil {
			return err
		}
	}
	if err := l.deleteIfThere(ctx, ToolsVM); err != nil {
		return err
	}
	mounts, err := mountsSet([]mount{{Location: spec.Out, MountPoint: ptr(spec.OutAt), Writable: ptr(true)}})
	if err != nil {
		return err
	}
	// A failure leaves it to the cleanup, which runs once the caller has
	// shown the run log's end.
	done := l.AddCleanup(func(ctx context.Context) { _ = l.deleteIfThere(ctx, ToolsVM) })
	// Its steps: clone, configure (when Start edits it) and start, the
	// script (the user's CAs before it), delete.
	steps := 4
	if cloneEdits(l, in.Config, spec.Net) {
		steps++
	}
	if !spec.Trust.None() {
		steps++
	}
	ctx = l.WithSteps(ctx, steps)
	if err := l.Runner.Run(ctx, msg.CloningImage("base"), l.limactl(ctx, toolsArgs(base, mounts)...)); err != nil {
		return backend.Logged(msg.VMCreateFailed(ToolsVM))
	}
	// Behind a company proxy, through a proxy of devvm's (buildproxy.go).
	conf, hp, proxyDone, err := buildProxy(ctx, l, ToolsVM, spec.Net)
	if err != nil {
		return err
	}
	if err := l.Start(ctx, ToolsVM, conf); err != nil {
		return err
	}
	if err := l.runScript(ctx, ToolsVM, spec.Trust.Add, true); err != nil {
		return err
	}
	cmd := l.limactl(ctx, append([]string{"shell", "--workdir", "/", ToolsVM, "bash", "-s", "--"}, spec.Args...)...)
	// The build's proxy for its commands, never written into the VM.
	cmd.Stdin = bytes.NewReader(withProxy(hp, spec.Script).Data)
	if err := l.Runner.Run(ctx, msg.BuildingToolsIn(ToolsVM), cmd); err != nil {
		return backend.Logged(msg.ToolsBuildFailed())
	}
	if err := l.delete(ctx, ToolsVM); err != nil {
		return err
	}
	proxyDone(true)
	done()
	return nil
}

// toolsArgs are limactl clone's arguments for ToolsVM: a clone of base's
// instance with mounts (mountsSet's) and nothing else from the host.
func toolsArgs(base, mounts string) []string {
	return []string{"clone", "--tty=false", "--mount-none", "--set", mounts, base, ToolsVM}
}

// ToolFormat is build.sh's own: Lima's guests mount compressed EROFS.
func (l *Lima) ToolFormat() string { return "" }

// deleteIfThere deletes a VM if devvm's Lima home has it.
func (l *Lima) deleteIfThere(ctx context.Context, vm string) error {
	if _, err := l.inspect(ctx, vm); errors.Is(err, backend.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	return l.delete(ctx, vm)
}
