package cli

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// gcOpts are what devvm gc is asked for besides its default run, which
// each option adds to.
type gcOpts struct {
	// only is --backend as passed (never the default backend file): that
	// backend alone.
	only string
	// tools is --tools: also the current tool images no VM has, of
	// toolNames when there are any.
	tools     bool
	toolNames []string
	// exports is --exports: also the exports, only those older than
	// --older-than when it's given.
	exports   bool
	olderThan string
	age       time.Duration
	// orphans is --orphans: also what devvm keeps for VMs their backend
	// no longer has.
	orphans bool
	// caches is --caches: then devvm's share of the backends' own
	// caches, each backend asked for apart, only in a terminal.
	caches bool
}

// newGCCmd is devvm gc.
func newGCCmd(u *ui.UI, g *globals) *cobra.Command {
	var o gcOpts
	c := &cobra.Command{
		Use:   "gc [--tools [TOOL...]]",
		Short: "Delete stale images and unused tool images",
		Long: "Delete what devvm images lists as stale or unfinished, on every backend, with their " +
			"records, the shared tool images it lists as unused, the copies of devvm's own files " +
			"older versions wrote out, and the older versions of the mise devvm runs on this machine. " +
			"With --backend, only that backend's images, its current ones too, and its unused tool " +
			"images; without it, every backend's, whatever the default backend is. With --tools, also " +
			"the current tool images no VM has, on every backend and C library, or only those of the " +
			"TOOLs named (node, claude). With --exports, also the agents' history devvm export and " +
			"devvm rm saved, the only copy of a deleted VM's, or only those older than --older-than " +
			"(30d, 2w, 12h). With --orphans, also what devvm keeps for VMs deleted outside it, as devvm rm " +
			"does, for each VM whose backend answers that it no longer has it. With --caches, then " +
			"what devvm's builds put in the backends' own caches that nothing of devvm's needs (Lima's " +
			"Ubuntu downloads, smolvm's disks of devvm's packs, Docker volumes of devvm's no VM has), " +
			"each backend listed and asked for apart, and only in a terminal. It lists them first, and in a terminal asks. VMs are never " +
			"deleted; on Linux hosts, an image whose disk VMs use stays, and so does a smolvm or " +
			"Docker image a VM was made from.",
		Args: func(_ *cobra.Command, args []string) error { return o.checkArgs(args) },
		RunE: func(cmd *cobra.Command, args []string) error {
			o.only, o.toolNames = g.backend, args
			fail(u, gc(cmd.Context(), u, g, o), msg.GCFailed)
			return nil
		},
	}
	c.Flags().BoolVar(&o.tools, "tools", false, "also delete the current shared tool images no VM has; with TOOL names, only theirs")
	c.Flags().BoolVar(&o.exports, "exports", false, "also delete the saved exports of agents' history")
	c.Flags().StringVar(&o.olderThan, "older-than", "", "with --exports, only those older than `AGE`: 30d, 2w or 12h")
	c.Flags().BoolVar(&o.orphans, "orphans", false, "also delete what devvm keeps for VMs their backend no longer has, as devvm rm does")
	c.Flags().BoolVar(&o.caches, "caches", false, "then free devvm's share of the backends' own caches, asking for each backend; only in a terminal")
	return c
}

// checkArgs takes tool names, by the names images have them by, only
// with --tools, and an age only with --exports.
func (o *gcOpts) checkArgs(args []string) error {
	if o.olderThan != "" {
		if !o.exports {
			return msg.GCOlderThanNeedsExports()
		}
		age, err := state.ParseAge(o.olderThan)
		if err != nil {
			return msg.GCBadAge(o.olderThan)
		}
		o.age = age
	}
	if len(args) > 0 && !o.tools {
		return msg.GCNamesNeedTools(args[0])
	}
	for _, a := range args {
		if !slices.Contains(image.Toolchains, a) && !slices.Contains(image.Agents, a) {
			return msg.GCUnknownTool(a, strings.Join(slices.Concat(image.Toolchains, image.Agents), " "))
		}
	}
	return nil
}

// gcPlan is what devvm gc deletes, gathered before it asks.
type gcPlan struct {
	images  []listedImage  // stale and unfinished images, each with its backend's
	tools   []tools.Listed // unused tool images
	current []tools.Listed // with --tools, current tool images no VM has
	exports []savedExport  // with --exports, the exports old enough
	// with --orphans, the VMs their backend no longer has, and what
	// forgets them
	orphans   vm.LeftoverReport
	leftovers vm.Leftovers
	assets    []string    // old copies of devvm's files no VM's proxy was started with
	mise      []string    // older versions of devvm's own mise
	share     tools.Share // where the tool images are
}

// gc lists what's stale or unused, asks in a terminal, and deletes it. It
// holds the share's lock throughout, so no build moves a tool image in
// and no update swaps the lock meanwhile.
func gc(ctx context.Context, u *ui.UI, g *globals, o gcOpts) error {
	b, err := builder(u, g.backend)
	if err != nil {
		return err
	}
	b.From = g.fromRef()
	unlock, err := b.Tools.LockShare(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if o.caches && !u.StdinIsTerminal() {
		return msg.GCCachesNeedTerminal()
	}
	if err := gcRun(ctx, u, b, o); err != nil {
		return err
	}
	if o.caches {
		return gcCaches(ctx, u, b, o.only)
	}
	return nil
}

// gcRun is gc's run but the backends' caches: it lists what it deletes,
// asks in a terminal, and deletes it.
func gcRun(ctx context.Context, u *ui.UI, b *build.Env, o gcOpts) error {
	p, err := planGC(ctx, u, b, o)
	if err != nil {
		return err
	}
	if p.empty() {
		p.showOrphans(u)
		u.Info(msg.NothingToClean())
		return nil
	}
	p.show(u)
	if u.StdinIsTerminal() && !u.Confirm(msg.GCConfirm()) {
		u.Info(msg.NothingDeleted())
		return nil
	}
	return p.run(ctx, u)
}

// planGC gathers what gc deletes: an old copy of devvm's files only when
// no VM's proxy was started with its policy files (netpolicy.AssetsInUse).
// With o.only, it gathers that backend's images, current or not, and its
// tool images, and nothing that isn't a backend's.
func planGC(ctx context.Context, u *ui.UI, b *build.Env, o gcOpts) (gcPlan, error) {
	p := gcPlan{share: tools.Share{Env: b.Tools}}
	imgs, ts, err := listImages(ctx, u, b)
	if err != nil {
		return p, err
	}
	p.images, p.tools = staleImages(imgs), tools.Unused(ts)
	if o.tools {
		p.current = p.share.NoVM(ts, o.toolNames)
	}
	if o.exports {
		es, err := exports(b.Dirs)
		if err != nil {
			return p, err
		}
		p.exports = olderThan(es, o.age, time.Now())
	}
	if o.orphans {
		env, err := newEnv(u)
		if err != nil {
			return p, err
		}
		p.leftovers = vm.Leftovers{Env: env}
		if p.orphans, err = p.leftovers.Find(ctx); err != nil {
			return p, err
		}
	}
	if o.only != "" {
		target, err := toolTargets(u, b)
		if err != nil {
			return p, err
		}
		p.images = backendImages(imgs, o.only)
		p.tools, p.current = backendTools(p.tools, o.only, target), backendTools(p.current, o.only, target)
		return p, nil
	}
	current, err := state.AssetsHash(b.Assets)
	if err != nil {
		return p, err
	}
	if p.assets, err = state.StaleAssets(b.Dirs, current, netpolicy.AssetsInUse(b.Dirs)); err != nil {
		return p, err
	}
	p.mise, err = tools.Versions{Env: b.Tools}.StaleMise()
	return p, err
}

// backendImages are a backend's images, current or not.
func backendImages(imgs []listedImage, be string) []listedImage {
	var out []listedImage
	for _, l := range imgs {
		if l.Backend == be {
			out = append(out, l)
		}
	}
	return out
}

// backendTools are the tool images built for a backend, as target tells;
// one whose backend devvm can't tell is never one.
func backendTools(ts []tools.Listed, be string, target func(tools.Listed) (string, string)) []tools.Listed {
	var out []tools.Listed
	for _, t := range ts {
		if b, _ := target(t); b == be {
			out = append(out, t)
		}
	}
	return out
}

func (p gcPlan) empty() bool {
	return len(p.images)+len(p.tools)+len(p.current)+len(p.exports)+len(p.orphans.Orphans)+len(p.assets)+len(p.mise) == 0
}

// showOrphans lists the VMs gc --orphans forgets, then says which it
// leaves alone and why.
func (p gcPlan) showOrphans(u *ui.UI) {
	if orphans := p.orphans.Orphans; len(orphans) > 0 {
		u.Say(msg.GCOrphans(len(orphans)))
		for _, o := range orphans {
			u.Say(msg.GCOrphan(o.Backend, o.Dir, o.Name))
		}
	}
	for _, b := range p.orphans.Unknown {
		u.Note(msg.GCOrphansUnknown(b))
	}
	for _, n := range p.orphans.Elsewhere {
		u.Note(msg.GCOrphanElsewhere(n))
	}
	for _, n := range p.orphans.Recent {
		u.Note(msg.GCOrphanRecent(n))
	}
}

// show lists what gc deletes.
func (p gcPlan) show(u *ui.UI) {
	p.showOrphans(u)
	if len(p.images) > 0 {
		u.Say(msg.GCImages())
		for _, l := range p.images {
			if l.Name == "" {
				u.Say(msg.GCUnfinished(l.Key))
			} else {
				u.Say(msg.GCImage(l.Name, l.Key))
			}
		}
	}
	if len(p.tools) > 0 {
		u.Say(msg.GCTools(len(p.tools), humanSize(toolsSize(p.tools))))
	}
	if len(p.current) > 0 {
		u.Say(msg.GCToolsNoVM(len(p.current), humanSize(toolsSize(p.current)), tools.Label(toolFiles(p.current))))
	}
	if len(p.assets) > 0 {
		u.Say(msg.GCAssets(len(p.assets)))
	}
	if len(p.mise) > 0 {
		u.Say(msg.GCMise(len(p.mise), humanSize(dirsSize(p.mise))))
	}
	if len(p.exports) > 0 {
		u.Say(msg.GCExports(len(p.exports), humanSize(exportsSize(p.exports))))
		for _, e := range p.exports {
			u.Say(msg.GCExport(exportDate(e.At), humanSize(e.Size), e.VM))
		}
	}
}

// run deletes what's planned.
func (p gcPlan) run(ctx context.Context, u *ui.UI) error {
	for _, o := range p.orphans.Orphans {
		if err := p.leftovers.Forget(ctx, o); err != nil {
			return err
		}
		u.Info(msg.ForgotOrphan(o.Name))
	}
	if err := deleteStale(ctx, u, p.images); err != nil {
		return err
	}
	if err := removeAll(p.assets); err != nil {
		return err
	}
	if len(p.assets) > 0 {
		u.Info(msg.DeletedAssets(len(p.assets)))
	}
	if err := removeAll(p.mise); err != nil {
		return err
	}
	if len(p.mise) > 0 {
		u.Info(msg.DeletedMise(len(p.mise)))
	}
	if err := removeAll(exportPaths(p.exports)); err != nil {
		return err
	}
	if len(p.exports) > 0 {
		u.Info(msg.DeletedExports(len(p.exports)))
	}
	if len(p.tools) > 0 {
		if err := p.share.Delete(toolFiles(p.tools)); err != nil {
			return err
		}
		u.Info(msg.DeletedTools(len(p.tools)))
	}
	if len(p.current) > 0 {
		if err := p.share.Delete(toolFiles(p.current)); err != nil {
			return err
		}
		u.Info(msg.DeletedToolsNoVM(len(p.current)))
	}
	return nil
}

// toolFiles are these tool images' files in the share.
func toolFiles(ts []tools.Listed) []string {
	files := make([]string, 0, len(ts))
	for _, t := range ts {
		files = append(files, t.File)
	}
	return files
}

// removeAll deletes these files and folders.
func removeAll(paths []string) error {
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}

// dirsSize is the disk these take.
func dirsSize(paths []string) int64 {
	var n int64
	for _, p := range paths {
		n += du(p)
	}
	return n
}

// deleteStale deletes these images, each on its backend. One the backend
// refuses with a message (on Linux hosts, an image whose disk VMs use)
// stays, saying so.
func deleteStale(ctx context.Context, u *ui.UI, imgs []listedImage) error {
	for _, l := range imgs {
		if err := l.images.DeleteKey(ctx, l.Key); err != nil {
			if m, ok := errors.AsType[msg.M](err); ok {
				u.Warn(m)
				continue
			}
			return err
		}
		if l.Name == "" {
			u.Info(msg.DeletedUnfinishedBuild(l.Key))
		} else {
			u.Info(msg.DeletedImage(l.Name, l.Key))
		}
	}
	return nil
}
