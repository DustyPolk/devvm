package cli

import (
	"cmp"
	"context"
	"strconv"

	"github.com/spf13/cobra"

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
func newImagesCmd(u *ui.UI) *cobra.Command {
	var (
		asJSON bool
		only   string
	)
	c := &cobra.Command{
		Use:   "images",
		Short: "List the images and the shared tools",
		Long: "List the images on every backend, with whether each is current, stale, unfinished or being built and the disk it takes, " +
			"then the shared tools devvm has built, with how many VMs have each. " +
			"It ends with what they take in all.",
		Example: examples(
			"devvm images", "list the images and the shared tools",
			"devvm images --backend lima", "only the lima backend's",
			"devvm images --json", "the same lists as JSON, for scripts",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := builder(u, only)
			var imgs []listedImage
			var ts []tools.Listed
			if err == nil {
				b.From = fromRef("")
				var l imageListing
				if l, err = listImages(cmd.Context(), u, b); err == nil {
					l = l.only(only)
					imgs, ts, err = l.images, l.tools, l.failUnlisted()
				}
			}
			fail(u, err, msg.ImagesFailed)
			ts = toolsOn(only, ts)
			nix := nixCacheOf(u, b.Dirs)
			if only != "" {
				nix = nixCacheJSON{}
			}
			if asJSON {
				err = printJSON(u, imagesJSON(imgs, ts, nix))
			} else {
				err = writeImages(u.Table(), imgs, ts)
			}
			if err != nil {
				return err
			}
			imagesNotes(u, imgs, ts, nix)
			return nil
		},
	}
	addBackendFilterFlag(c.Flags(), &only)
	jsonFlag(c, &asJSON)
	c.AddCommand(newImagesRmCmd(u))
	return c
}

// newImagesRmCmd is devvm images rm IMAGE|KEY....
func newImagesRmCmd(u *ui.UI) *cobra.Command {
	var (
		f     imageFlags
		force bool
	)
	c := &cobra.Command{
		Use:   "rm IMAGE|KEY...",
		Short: "Delete images, by name or by key",
		Long: "Delete images, each by its name or by the key devvm images shows, after asking in a terminal. " +
			"Every one must be built, or devvm deletes nothing. " +
			"VMs made from an image keep working, except on Linux hosts, where devvm refuses an image whose disk VMs use.",
		Example: examples(
			"devvm images rm foo+bar", "delete the foo+bar image, after asking",
			"devvm images rm 7a76021c46ef", "delete a stale image by its key",
			"devvm images rm base --from node:24", "delete base made from node:24",
		),
		Args: minArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			dirs, err := paths.FromEnv()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return joined(toComplete, (&build.Env{Dirs: dirs}).Layers()), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := builder(u, f.backend)
			if err == nil {
				b.From = fromRef(f.from)
				err = from.Check(b.From)
			}
			if err == nil {
				err = b.Backend.Check(cmd.Context())
			}
			ok := false
			if err == nil {
				ok, err = confirmed(u, force, "devvm images rm", func() bool { return u.Ask(msg.ImagesRmConfirm(shownArgs(args))) })
			}
			if err == nil && ok {
				err = build.Images{Env: b}.Delete(cmd.Context(), args)
			} else if err == nil {
				u.Info(msg.NothingDeleted())
			}
			fail(u, err, msg.ImagesRmFailed)
			return nil
		},
	}
	f.add(c)
	addForceFlag(c.Flags(), &force)
	return c
}

// only is the listing of the backend named alone, or all of it when name
// is "". Shared tool images are left to toolsOn, which knows their
// targets.
func (l imageListing) only(name string) imageListing {
	if name == "" {
		return l
	}
	out := imageListing{tools: l.tools}
	for _, i := range l.images {
		if i.Backend == name {
			out.images = append(out.images, i)
		}
	}
	for _, x := range l.unlisted {
		if x.backend == name {
			out.unlisted = append(out.unlisted, x)
		}
	}
	return out
}

// toolsOn is the shared tool images the backend named built, as their
// records say, or all of them when name is "". One without a record is
// never one.
func toolsOn(name string, ts []tools.Listed) []tools.Listed {
	if name == "" {
		return ts
	}
	var out []tools.Listed
	for _, t := range ts {
		if t.Backend == name {
			out = append(out, t)
		}
	}
	return out
}

// nixCacheJSON is what the shared Nix cache holds: its files, the bytes
// they take, and its cap in bytes.
type nixCacheJSON struct {
	Files int   `json:"files"`
	Size  int64 `json:"size"`
	Cap   int64 `json:"cap"`
}

// nixCacheOf reads what the shared Nix cache holds; none when it can't.
func nixCacheOf(u *ui.UI, d paths.Dirs) nixCacheJSON {
	n, size, err := proxy.NixCacheUsage(d.NixCache())
	if err != nil {
		return nixCacheJSON{Cap: nixCacheCap(u, d)}
	}
	return nixCacheJSON{Files: n, Size: size, Cap: nixCacheCap(u, d)}
}

// imagesNotes says, after devvm images' tables, how many images aren't
// current, what the unused tools take, the total, and the Nix cache's
// use of its cap.
func imagesNotes(u *ui.UI, imgs []listedImage, ts []tools.Listed, nix nixCacheJSON) {
	if n := len(staleImages(imgs)); n > 0 {
		u.Note(msg.ImagesStaleNote(n))
	}
	if size := toolsSize(tools.Unused(ts)); size > 0 {
		u.Note(msg.ImagesUnused(humanSize(size)))
	}
	u.Note(msg.ImagesTotal(humanSize(imagesSize(imgs) + toolsSize(ts))))
	if nix.Files > 0 {
		u.Note(msg.ImagesNixCache(humanSize(nix.Cap), humanSize(nix.Size)))
	}
}

// imagesOutput is devvm images --json: the images on every backend, the
// shared tool images, the bytes they take in all and of that what unused
// tool images take, and the shared Nix cache. Sizes are in bytes.
type imagesOutput struct {
	Images      []imageJSON     `json:"images"`
	Tools       []toolImageJSON `json:"tools"`
	Total       int64           `json:"total"`
	UnusedTools int64           `json:"unused_tools"`
	NixCache    nixCacheJSON    `json:"nix_cache"`
}

// imageJSON is an image: "" for what an unfinished one lacks.
type imageJSON struct {
	Name    string `json:"name"`
	From    string `json:"from"`
	Digest  string `json:"digest"`
	Key     string `json:"key"`
	State   string `json:"state"`
	Size    int64  `json:"size"`
	Sized   bool   `json:"sized"`
	Backend string `json:"backend"`
}

// toolImageJSON is a shared tool image: its backend, C library, size and
// VMs only once it's built.
type toolImageJSON struct {
	Name    string `json:"name"`
	Tool    string `json:"tool"`
	Version string `json:"version"`
	Built   bool   `json:"built"`
	Backend string `json:"backend,omitempty"`
	Libc    string `json:"libc,omitempty"`
	Size    int64  `json:"size"`
	VMs     int    `json:"vms"`
	State   string `json:"state"`
}

func imagesJSON(imgs []listedImage, ts []tools.Listed, nix nixCacheJSON) imagesOutput {
	out := imagesOutput{
		Images: make([]imageJSON, 0, len(imgs)), Tools: make([]toolImageJSON, 0, len(ts)),
		Total: imagesSize(imgs) + toolsSize(ts), UnusedTools: toolsSize(tools.Unused(ts)), NixCache: nix,
	}
	for _, l := range imgs {
		digest := ""
		if l.SHA256 != "" {
			digest = "sha256:" + l.SHA256
		}
		out.Images = append(out.Images, imageJSON{
			Name: l.Name, From: l.Origin, Digest: digest, Key: l.Key, State: l.State,
			Size: l.Size, Sized: l.Sized, Backend: l.Backend,
		})
	}
	for _, t := range ts {
		j := toolImageJSON{Name: t.Name, Tool: t.Tool, Version: t.Version, Built: t.Built, State: t.State}
		if t.Built {
			j.Size, j.VMs = t.Size, t.VMs
			j.Backend, j.Libc = t.Backend, t.Target.Libc
		}
		out.Tools = append(out.Tools, j)
	}
	return out
}

// listedImage is an image, its backend's name and the images on it.
type listedImage struct {
	build.Listed
	Backend string
	images  build.Images
}

// staleImages returns the images that are stale or unfinished, never one
// another devvm is building.
func staleImages(imgs []listedImage) []listedImage {
	var out []listedImage
	for _, l := range imgs {
		if l.State == build.StateStale || l.State == build.StateUnfinished {
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
	reg, err := newRegistry(b.Dirs, b.Assets, u)
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
// that built each and the C library it's built for, as their records say,
// as tables, each with a header; a value one lacks is -. States are
// colored by what they mean, digests and keys dim.
func writeImages(tab *ui.Table, imgs []listedImage, ts []tools.Listed) error {
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
			be, libc = t.Backend, t.Target.Libc
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
