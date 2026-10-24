package tools

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
)

// Remove takes shared tools out of a VM made from imageName with these
// tool files: it returns the VM's image name and tool files without them.
// What the VM installed itself stays. A custom layer ran its script into
// the VM's disk, so it can't be removed, and neither can anything the VM
// lacks.
func Remove(vm, imageName string, files []string, names string) (string, []string, error) {
	have := strings.Split(imageName, "+")
	gone := strings.Split(names, "+")
	var drop []string
	for _, n := range gone {
		if !slices.Contains(have, n) {
			return "", nil, msg.VMLacks(strings.Join(have, " "), n, vm)
		}
		if !image.IsTool(n) {
			return "", nil, msg.CustomLayerRemove(n, vm)
		}
		drop = append(drop, image.GroupTools(n)...)
	}
	rest := []string{image.Base}
	for _, h := range have {
		if !slices.Contains(gone, h) {
			rest = append(rest, h)
		}
	}
	var kept []string
	tools, _ := split(files)
	for i, f := range files {
		if !slices.Contains(drop, tools[i]) {
			kept = append(kept, f)
		}
	}
	return image.Normalize(strings.Join(rest, "+")), kept, nil
}

// Newer returns the tools of an image name at the lock's versions that a
// VM with these tool files doesn't have, as "claude 2.1.295".
func (t *Tools) Newer(ctx context.Context, imageName string, have []string, libc string) ([]string, error) {
	files, err := t.Files(ctx, image.ToolsOf(imageName), libc)
	if err != nil {
		return nil, err
	}
	var newer []string
	for _, f := range files {
		if !slices.Contains(have, f) {
			newer = append(newer, label([]string{f}))
		}
	}
	return newer, nil
}

// WarnNewer warns when there are newer shared tools than a VM's: moving
// to them is cheap, as nothing in the VM's disk changes. It never changes
// the VM.
func (t *Tools) WarnNewer(ctx context.Context, imageName string, have []string, libc string) error {
	newer, err := t.Newer(ctx, imageName, have, libc)
	if err != nil {
		return err
	}
	if len(newer) > 0 {
		t.Warn(msg.NewerTools(strings.Join(newer, ", ")))
	}
	return nil
}

// Update moves a VM to the newest shared tools for its image name: it
// checks upstream first (unless there's no lock yet: resolving it is
// checking), builds what the share lacks on b from base's image (key
// base), and returns the VM's new tool files. changed is false when they
// were the newest already.
func (t *Tools) Update(ctx context.Context, b backend.Backend, base, vm, imageName string, have []string, libc string) (files []string, changed bool, err error) {
	if _, err := os.Stat(t.Dirs.ToolLock()); err == nil {
		t.Check(ctx)
	}
	if files, err = t.Files(ctx, image.ToolsOf(imageName), libc); err != nil {
		return nil, false, err
	}
	if slices.Equal(files, have) {
		t.Info(msg.ToolsNewest(vm))
		return files, false, nil
	}
	if err := t.Build(ctx, b, base, libc, files); err != nil {
		return nil, false, err
	}
	t.Info(msg.ToolsSwitch(vm))
	return files, true, nil
}
