package tools

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
)

// VMTool is one of a VM's shared tools, as devvm tools lists it.
type VMTool struct {
	Tool    string // its name in the lock: node, pnpm
	Version string // the VM's version of it
	// Newer is the lock's version when the VM has another build, else "";
	// Built is whether the share has that one.
	Newer string
	Built bool
}

// Show returns a VM's tools, from its tool files, in their order. The
// lock is read as it is, never resolved: without one, none is newer.
func (t VMs) Show(files []string, tg Target) ([]VMTool, error) {
	lock, err := Versions(t).ReadLock()
	if err != nil {
		return nil, err
	}
	share := Share(t)
	var out []VMTool
	for _, f := range files {
		tool, v, ok := image.ParseToolFile(f)
		if !ok {
			continue
		}
		vt := VMTool{Tool: tool, Version: v}
		// Another file is another build: a newer version, or the same
		// version from a changed lock entry or build script, as WarnNewer
		// tells it.
		if lv := lock.Version(tool); lv != "" {
			if cur, err := t.File(lock, tool, tg); err == nil && cur != f {
				vt.Newer, vt.Built = lv, share.Built(cur)
			}
		}
		out = append(out, vt)
	}
	return out, nil
}

// Remove takes shared tools out of a VM made from imageName with these
// tool files: it returns the VM's image name and tool files without them.
// What the VM installed itself stays. A custom layer ran its script into
// the VM's disk, so it can't be removed, and neither can anything the VM
// lacks.
func Remove(vm, imageName string, files []string, gone []string) (string, []string, error) {
	have := strings.Split(imageName, "+")
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
func (t VMs) Newer(ctx context.Context, imageName string, have []string, tg Target) ([]string, error) {
	files, err := Versions(t).Files(ctx, image.ToolsOf(imageName), tg)
	if err != nil {
		return nil, err
	}
	var newer []string
	for _, f := range files {
		if !slices.Contains(have, f) {
			newer = append(newer, Label([]string{f}))
		}
	}
	return newer, nil
}

// WarnNewer warns when there are newer shared tools than a VM's: moving
// to them is cheap, as nothing in the VM's disk changes. It never changes
// the VM.
func (t VMs) WarnNewer(ctx context.Context, imageName string, have []string, tg Target) error {
	newer, err := t.Newer(ctx, imageName, have, tg)
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
// checking), has build build what the share lacks of them (the VM's own,
// when they're the newest and one is gone from the share), and returns the
// VM's new tool files. changed is false when they were the newest already.
func (t VMs) Update(ctx context.Context, build func(context.Context, []string) error, vm, imageName string, have []string, tg Target) (files []string, changed bool, err error) {
	versions := Versions(t)
	if _, err := os.Stat(t.Dirs.ToolLock()); err == nil {
		versions.Check(ctx)
	}
	if files, err = versions.Files(ctx, image.ToolsOf(imageName), tg); err != nil {
		return nil, false, err
	}
	if err := build(ctx, files); err != nil {
		return nil, false, err
	}
	// The same files in any order: tools added since are listed last.
	if slices.Equal(slices.Sorted(slices.Values(files)), slices.Sorted(slices.Values(have))) {
		t.Info(msg.ToolsNewest(vm))
		return files, false, nil
	}
	t.Info(msg.ToolsSwitch(vm))
	return files, true, nil
}
