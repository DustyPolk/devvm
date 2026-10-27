package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
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
			"by the name images have them by: each version, the disk it takes, how many VMs use it, and " +
			"whether it's the tools lock's version, in use, unused, or the lock's and not built yet.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := builder(u, g.backend)
			var imgs []listedImage
			var ts []tools.Listed
			if err == nil {
				b.From = g.fromRef()
				imgs, ts, err = listImages(cmd.Context(), u, b)
			}
			fail(u, err, msg.ImagesFailed)
			if err := writeImages(u.Stdout(), imgs, ts); err != nil {
				return err
			}
			if n := len(staleImages(imgs)); n > 0 {
				u.Note(msg.ImagesStaleNote(n))
			}
			if size := toolsSize(tools.Unused(ts)); size > 0 {
				u.Note(msg.ImagesUnused(humanSize(size)))
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
		Args: cobra.MinimumNArgs(1),
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

// toolsSize is the disk the tool images take.
func toolsSize(ts []tools.Listed) int64 {
	var n int64
	for _, t := range ts {
		n += t.Size
	}
	return n
}

// listImages lists the images on every backend, and the shared tools for
// the base new VMs get on b (its backend and From). A backend that can't
// list fails it only when a VM's state names that backend.
func listImages(ctx context.Context, u *ui.UI, b *build.Env) ([]listedImage, []tools.Listed, error) {
	defer u.Status(msg.ReadingImages())()
	if err := from.Check(b.From); err != nil {
		return nil, nil, err
	}
	reg, err := registry.New(b.Dirs, b.Assets, u)
	if err != nil {
		return nil, nil, err
	}
	var imgs []listedImage
	for _, be := range reg.All() {
		on := build.Images{Env: newBuilder(u, b.Dirs, be, reg.Review, b.Tools, "")}
		ls, err := on.Listing(ctx)
		if err != nil {
			if used, uerr := state.Backends(b.Dirs); uerr != nil {
				return nil, nil, uerr
			} else if !used[be.Name()] {
				continue
			}
			if cerr := be.Check(ctx); cerr != nil {
				return nil, nil, cerr
			}
			return nil, nil, err
		}
		for _, l := range ls {
			imgs = append(imgs, listedImage{Listed: l, Backend: be.Name(), images: on})
		}
	}
	// Current for the default backend's new VMs, or any backend's.
	tg, err := b.ToolTarget()
	if err != nil {
		return nil, nil, err
	}
	tgs := []tools.Target{tg}
	for _, be := range reg.All() {
		t, err := newBuilder(u, b.Dirs, be, reg.Review, b.Tools, b.From).ToolTarget()
		if err != nil {
			return nil, nil, err
		}
		tgs = append(tgs, t)
	}
	inUse, err := vm.ToolsInUse(b.Dirs)
	if err != nil {
		return nil, nil, err
	}
	ts, err := tools.Share{Env: b.Tools}.List(tgs, inUse)
	return imgs, ts, err
}

// writeImages writes the images, then the shared tools, to out as tables,
// each with a header; a value one lacks is -.
func writeImages(out io.Writer, imgs []listedImage, ts []tools.Listed) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	row := func(vals ...string) {
		for i, v := range vals {
			vals[i] = cmp.Or(v, "-")
		}
		_, _ = fmt.Fprintln(w, strings.Join(vals, "\t"))
	}
	row("IMAGE", "FROM", "DIGEST", "KEY", "STATE", "SIZE", "BACKEND")
	for _, l := range imgs {
		size := ""
		if l.Sized {
			size = humanSize(l.Size)
		}
		row(l.Name, l.Origin, l.DigestID, l.Key, l.State, size, l.Backend)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out)
	row("NAME", "TOOL", "VERSION", "SIZE", "VMS", "STATE")
	for _, t := range ts {
		size, vms := "", ""
		if t.Built {
			size, vms = humanSize(t.Size), strconv.Itoa(t.VMs)
		}
		row(t.Name, t.Tool, t.Version, size, vms, t.State)
	}
	return w.Flush()
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
