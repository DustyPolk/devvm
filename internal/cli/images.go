package cli

import (
	"cmp"
	"context"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newImagesCmd is devvm images, and devvm images rm.
func newImagesCmd(u *ui.UI, g *globals) *cobra.Command {
	c := &cobra.Command{
		Use:   "images",
		Short: "List the images and the shared tools",
		Long: "List the images on every backend, by name: what each one's base started from and its " +
			"digest, its key, whether it's current, stale (its inputs changed since) or unfinished (a " +
			"build that didn't finish), the disk it takes and its backend. Then the shared tool images, " +
			"by the name images have them by: each version, the backend and C library it's built for, the " +
			"disk it takes, how many VMs use it, and whether it's the tools lock's version, in use, unused, " +
			"or the lock's and not built yet. Then what they take in all, and what the shared Nix cache " +
			"takes of its cap.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := builder(u, g.backend)
			var imgs []listedImage
			var ts []tools.Listed
			if err == nil {
				b.From = g.fromRef()
				var l imageListing
				if l, err = listImages(cmd.Context(), u, b); err == nil {
					imgs, ts, err = l.images, l.tools, l.failUnlisted()
				}
			}
			fail(u, err, msg.ImagesFailed)
			target, err := toolTargets(u, b)
			fail(u, err, msg.ImagesFailed)
			if err := writeImages(u.Table(), imgs, ts, target); err != nil {
				return err
			}
			if n := len(staleImages(imgs)); n > 0 {
				u.Note(msg.ImagesStaleNote(n))
			}
			if size := toolsSize(tools.Unused(ts)); size > 0 {
				u.Note(msg.ImagesUnused(humanSize(size)))
			}
			u.Note(msg.ImagesTotal(humanSize(imagesSize(imgs) + toolsSize(ts))))
			if n, size, err := proxy.NixCacheUsage(b.Dirs.NixCache()); err == nil && n > 0 {
				u.Note(msg.ImagesNixCache(humanSize(nixCacheCap(u, b.Dirs)), humanSize(size)))
			}
			return nil
		},
	}
	c.AddCommand(&cobra.Command{
		Use:   "rm IMAGE|KEY...",
		Short: "Delete images, by name or by key",
		Long: "Delete images, with their records: each by its name (base, foo+bar), or by the key " +
			"devvm images shows, for a stale or unfinished one; with --from, a name is its image on a base " +
			"from that container image. Every one must be built, or nothing is " +
			"deleted. VMs made from an image keep working, except on Linux hosts, where an image whose " +
			"disk VMs use is refused. Shared tools aren't images.",
		Args: minArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			dirs, err := paths.FromEnv()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return joined(toComplete, (&build.Env{Dirs: dirs}).Layers()), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := builder(u, g.backend)
			if err == nil {
				b.From = g.fromRef()
				err = from.Check(b.From)
			}
			if err == nil {
				err = b.Backend.Check(cmd.Context())
			}
			if err == nil {
				err = build.Images{Env: b}.Delete(cmd.Context(), args)
			}
			fail(u, err, msg.ImagesRmFailed)
			return nil
		},
	})
	return c
}

// listedImage is an image, its backend's name and the images on it.
type listedImage struct {
	build.Listed
	Backend string
	images  build.Images
}

// staleImages returns the images that aren't current: stale, or
// unfinished.
func staleImages(imgs []listedImage) []listedImage {
	var out []listedImage
	for _, l := range imgs {
		if l.State != build.StateCurrent {
			out = append(out, l)
		}
	}
	return out
}

// imagesSize is the disk the images take, those whose size is known.
func imagesSize(imgs []listedImage) int64 {
	var n int64
	for _, l := range imgs {
		n += l.Size
	}
	return n
}

// toolTargets returns what tells the backend and C library a tool image
// is built for (toolTarget): its record, else a VM's that has it.
func toolTargets(u *ui.UI, b *build.Env) (func(tools.Listed) (string, string), error) {
	reg, err := registry.New(b.Dirs, b.Assets, u)
	if err != nil {
		return nil, err
	}
	vms, err := vmToolTargets(b.Dirs)
	if err != nil {
		return nil, err
	}
	return func(t tools.Listed) (string, string) { return toolTarget(reg, t, vms) }, nil
}

// toolsSize is the disk the tool images take.
func toolsSize(ts []tools.Listed) int64 {
	var n int64
	for _, t := range ts {
		n += t.Size
	}
	return n
}

// unlisted is a backend a VM's state names that couldn't list its images,
// and why: its Check's error, else its listing's.
type unlisted struct {
	backend string
	err     error
}

// imageListing is what listImages found: the images on every backend
// that listed them, the shared tools, and the backends a VM uses that
// couldn't list.
type imageListing struct {
	images   []listedImage
	tools    []tools.Listed
	unlisted []unlisted
}

// listImages lists the images on every backend, and the shared tools for
// the base new VMs get on b (its backend and From). A backend that can't
// list is left out, and named in unlisted when a VM's state names it.
func listImages(ctx context.Context, u *ui.UI, b *build.Env) (imageListing, error) {
	defer u.Status(msg.ReadingImages())()
	var out imageListing
	if err := from.Check(b.From); err != nil {
		return out, err
	}
	reg, err := registry.New(b.Dirs, b.Assets, u)
	if err != nil {
		return out, err
	}
	for _, be := range reg.All() {
		on := build.Images{Env: newBuilder(u, b.Dirs, be, reg.Review, b.Tools, "")}
		ls, err := on.Listing(ctx)
		if err != nil {
			if used, uerr := state.Backends(b.Dirs); uerr != nil {
				return out, uerr
			} else if !used[be.Name()] {
				continue
			}
			if cerr := be.Check(ctx); cerr != nil {
				err = cerr
			}
			out.unlisted = append(out.unlisted, unlisted{backend: be.Name(), err: err})
			continue
		}
		for _, l := range ls {
			out.images = append(out.images, listedImage{Listed: l, Backend: be.Name(), images: on})
		}
	}
	// Current for the default backend's new VMs, or any backend's.
	tg, err := b.ToolTarget()
	if err != nil {
		return out, err
	}
	tgs := []tools.Target{tg}
	for _, be := range reg.All() {
		t, err := newBuilder(u, b.Dirs, be, reg.Review, b.Tools, b.From).ToolTarget()
		if err != nil {
			return out, err
		}
		tgs = append(tgs, t)
	}
	inUse, err := vm.ToolsInUse(b.Dirs)
	if err != nil {
		return out, err
	}
	out.tools, err = tools.Share{Env: b.Tools}.List(tgs, inUse)
	return out, err
}

// failUnlisted is the error of the first backend a VM uses that couldn't
// list its images, for devvm images, which lists every backend's or none.
func (l imageListing) failUnlisted() error {
	if len(l.unlisted) > 0 {
		return l.unlisted[0].err
	}
	return nil
}

// writeImages writes the images, then the shared tools with the backend
// and C library each is built for (target), as tables, each with a header;
// a value one lacks is -. States are colored by what they mean, digests
// and keys dim.
func writeImages(tab *ui.Table, imgs []listedImage, ts []tools.Listed, target func(tools.Listed) (be, libc string)) error {
	tab.Header("IMAGE", "FROM", "DIGEST", "KEY", "STATE", "SIZE", "BACKEND")
	for _, l := range imgs {
		size := ""
		if l.Sized {
			size = humanSize(l.Size)
		}
		tab.Row(cell(l.Name), cell(l.Origin), dim(l.DigestID), dim(l.Key), cellAs(l.State, imageStates), cell(size), cell(l.Backend))
	}
	if err := tab.Break(); err != nil {
		return err
	}
	tab.Header("NAME", "TOOL", "VERSION", "BACKEND", "LIBC", "SIZE", "VMS", "STATE")
	for _, t := range ts {
		size, vms, be, libc := "", "", "", ""
		if t.Built {
			size, vms = humanSize(t.Size), strconv.Itoa(t.VMs)
			be, libc = target(t)
		}
		tab.Row(cell(t.Name), cell(t.Tool), cell(t.Version), cell(be), cell(libc), cell(size), cell(vms), cellAs(t.State, toolStates))
	}
	return tab.Flush()
}

// imageStates and toolStates color devvm images' STATE columns.
func imageStates(s string) ui.Role {
	switch s {
	case build.StateCurrent:
		return ui.Good
	case build.StateStale, build.StateUnfinished:
		return ui.Attention
	}
	return ui.Plain
}

func toolStates(s string) ui.Role {
	switch s {
	case tools.StateCurrent:
		return ui.Good
	case tools.StateUnused, tools.StateNotBuilt:
		return ui.Attention
	}
	return ui.Plain
}

// cell is a table cell for v, - when it's empty; dim, one in the
// Secondary role; cellAs, one in the role roles gives its value.
func cell(v string) ui.Cell { return ui.Cell{Text: cmp.Or(v, "-")} }

func dim(v string) ui.Cell { return ui.Cell{Text: cmp.Or(v, "-"), Role: ui.Secondary} }

func cellAs(v string, roles func(string) ui.Role) ui.Cell {
	return ui.Cell{Text: cmp.Or(v, "-"), Role: roles(v)}
}

// humanSize writes a size in bytes as du -h does, in powers of 1024: 0B,
// 512K, 2.1G. Below 10, one decimal.
func humanSize(n int64) string {
	if n < 1024 {
		return strconv.FormatInt(n, 10) + "B"
	}
	f := float64(n)
	unit := ""
	for _, u := range []string{"K", "M", "G", "T", "P"} {
		f /= 1024
		unit = u
		if f < 1023.5 { // else it rounds to 1024
			break
		}
	}
	if f < 9.95 {
		return strconv.FormatFloat(f, 'f', 1, 64) + unit
	}
	return strconv.FormatFloat(f, 'f', 0, 64) + unit
}
