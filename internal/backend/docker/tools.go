package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// Containers can't mount the shared tools' images (no loop devices
// without --privileged), so on Docker each tool image is unpacked into one
// volume of devvm's (toolsVolume), a folder per image, named as the
// image is; project VMs mount it read-only where other backends' mount
// the share, and assets/guest/tools.sh links each folder where it would
// mount the image. The images stay in the share, as the record of what's
// built (and what the volume is made from again when it's gone);
// toolsManifest lists what's unpacked.
//
// Unpacking runs in a container of devvm's with no network, from a VM's
// image, never in the tools build's container: what the build ran never
// sees the volume.

// toolsManifest lists the tool images unpacked in the tools volume.
func (d *Docker) toolsManifest() string { return filepath.Join(d.Dirs.Docker(), "tools.json") }

func (d *Docker) readManifest() ([]string, error) {
	var files []string
	b, err := os.ReadFile(d.toolsManifest())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return files, json.Unmarshal(b, &files)
}

func (d *Docker) writeManifest(files []string) error {
	if err := os.MkdirAll(d.Dirs.Docker(), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(files)
	if err != nil {
		return err
	}
	return state.WriteFile(d.toolsManifest(), append(b, '\n'))
}

// BuildTools makes the tools build's container from base's image, with
// spec.Out its only mount, starts it with the network spec.Net asks for,
// runs the script with its arguments as the VM's user, and deletes it;
// then unpacks each image it wrote into the tools volume (unpack). A
// container left by an earlier run is deleted first, and the container
// goes however the run ends (a cleanup on the ui stack).
func (d *Docker) BuildTools(ctx context.Context, spec backend.ToolsSpec) error {
	if err := d.checkTools(spec); err != nil {
		return err
	}
	if _, err := d.inspectImage(ctx, d.imageRef(spec.Base)); err != nil {
		return msg.ImageMissing(spec.Base)
	}
	out, err := bindAt(spec.Out, spec.OutAt, true)
	if err != nil {
		return err
	}
	cpus, mem, _, err := d.baseSize()
	if err != nil {
		return err
	}
	name := d.toolsName()
	if err := d.removeIfThere(ctx, name); err != nil {
		return err
	}
	// A failure leaves it to the cleanup, which runs once the caller has
	// shown the run log's end.
	done := d.AddCleanup(func(ctx context.Context) { _ = d.removeIfThere(ctx, name) })
	// Its steps: create, start, the script, delete, the unpacking.
	ctx = d.WithSteps(ctx, 5)
	e := egress{d}
	ctx, netw, lifeline, undoNet, err := e.buildNet(ctx, name, spec.Net)
	if err != nil {
		return err
	}
	undo := d.AddCleanup(undoNet)
	c := config{Hostname: "devvm-tools", Net: netw, Mounts: []string{out}, CapDrop: buildCaps(name, netw)}
	if err := d.create(ctx, msg.CloningImage(image.Base), name, d.imageRef(spec.Base), map[string]string{kindLabel: kindTools}, c, cpus, mem); err != nil {
		return err
	}
	if err := e.startBuild(ctx, name, lifeline); err != nil {
		return err
	}
	if err := e.buildSudo(ctx, name, true); err != nil {
		return err
	}
	args, err := d.userArgs(ctx, name, "/", true, false, append([]string{"bash", "-s", "--"}, spec.Args...))
	if err != nil {
		return err
	}
	cmd := d.docker(ctx, args...)
	cmd.Stdin = bytes.NewReader(spec.Script.Data)
	if err := d.Runner.Run(ctx, msg.BuildingToolsIn(name), cmd); err != nil {
		return backend.Logged(msg.ToolsBuildFailed())
	}
	if err := d.removeContainer(ctx, msg.Deleting(name), name); err != nil {
		return err
	}
	done()
	undo()
	undoNet(ctx)
	var files []string
	for i := 2; i < len(spec.Args); i += 3 {
		f := spec.Args[i]
		// A plain file, as the share takes it: a link could name a host
		// file.
		if fi, err := os.Lstat(filepath.Join(spec.Out, f)); err != nil || !fi.Mode().IsRegular() {
			return msg.ToolBadOutput(f)
		}
		files = append(files, f)
	}
	return d.unpack(ctx, d.imageRef(spec.Base), spec.Out, files, nil)
}

// checkTools checks what BuildTools is asked for.
func (d *Docker) checkTools(spec backend.ToolsSpec) error {
	if err := checkBuildNet(d.Caps(), d.toolsName(), spec.Net); err != nil {
		return err
	}
	if spec.Format != d.ToolFormat() {
		return fmt.Errorf("docker: tool images are %s, not %q", d.ToolFormat(), spec.Format)
	}
	if !keyPattern.MatchString(spec.Base) {
		return fmt.Errorf("docker: invalid image key %q", spec.Base)
	}
	if !filepath.IsAbs(spec.Out) || !filepath.IsAbs(spec.OutAt) {
		return fmt.Errorf("docker: the tools build's folders must be absolute: %q at %q", spec.Out, spec.OutAt)
	}
	return nil
}

// syncTools brings the tools volume up to the share before a VM that
// mounts it is made or starts: what the manifest lists that the share no
// longer has (devvm gc deleted it) goes, and when the volume itself is
// gone, it's made again with what the share still has of the manifest.
// img is an image of devvm's to unpack with.
func (d *Docker) syncTools(ctx context.Context, img string) error {
	files, err := d.readManifest()
	if err != nil {
		return err
	}
	var keep, gone []string
	for _, f := range files {
		if fi, err := os.Lstat(filepath.Join(d.Dirs.Tools(), f)); err == nil && fi.Mode().IsRegular() {
			keep = append(keep, f)
		} else {
			gone = append(gone, f)
		}
	}
	ok, err := d.volumeExists(ctx, d.toolsVolume())
	if err != nil {
		return err
	}
	if !ok {
		if err := d.writeManifest(nil); err != nil {
			return err
		}
		if len(keep) == 0 {
			return d.createVolume(ctx, d.toolsVolume())
		}
		return d.unpack(ctx, img, d.Dirs.Tools(), keep, nil)
	}
	if len(gone) == 0 {
		return nil
	}
	return d.unpack(ctx, img, d.Dirs.Tools(), nil, gone)
}

// unpack unpacks the tool images add, from the host folder dir, into the
// tools volume (made first when it's missing), and deletes those in drop
// from it, in a throwaway container of devvm's from img, with no network
// (unpack.sh); then records them in the manifest.
func (d *Docker) unpack(ctx context.Context, img, dir string, add, drop []string) error {
	if len(add) == 0 && len(drop) == 0 {
		return nil
	}
	script, err := d.script("docker/unpack.sh")
	if err != nil {
		return err
	}
	if err := d.ensureVolume(ctx, d.toolsVolume()); err != nil {
		return err
	}
	src, err := bindAt(dir, shareAt, false)
	if err != nil {
		return err
	}
	name := d.unpackName()
	if err := d.removeIfThere(ctx, name); err != nil {
		return err
	}
	args := []string{
		"container", "run", "--rm", "-i", "--name", name,
		"--label", LabelKey + "=" + d.Data, "--label", kindLabel + "=" + kindTools,
		"--network", "none", "--user", "0:0", "--entrypoint", "sh",
		"--mount", src, "--mount", volume(d.toolsVolume(), storeAt, true), img, "-s", "--", shareAt, storeAt,
	}
	for _, f := range add {
		args = append(args, "+"+f)
	}
	for _, f := range drop {
		args = append(args, "-"+f)
	}
	cmd := d.docker(ctx, args...)
	cmd.Stdin = bytes.NewReader(script)
	if err := d.Runner.Run(ctx, msg.UnpackingTools(), cmd); err != nil {
		return backend.Logged(msg.DockerToolsFailed(d.toolsVolume()))
	}
	files, err := d.readManifest()
	if err != nil {
		return err
	}
	files = slices.DeleteFunc(files, func(f string) bool { return slices.Contains(drop, f) || slices.Contains(add, f) })
	return d.writeManifest(append(files, add...))
}
