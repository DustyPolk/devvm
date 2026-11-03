package cli

import (
	"context"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/settings"
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
	// tools is --tool-images: also the current tool images no VM has, of
	// toolNames when there are any.
	tools     bool
	toolNames []string
	// recent is --recent, only with --tool-images: also the current tool images
	// no VM has that were built in the last hour, while no VM is being
	// made.
	recent bool
	// force is --force: gc deletes without asking, which it needs without
	// a terminal.
	force bool
	// making is set with --tool-images when a VM is being made: gc keeps every
	// current tool image no VM has, and says so.
	making bool
	// history is --history: also the exports of agents' history, only
	// those older than --older-than when it's given.
	history   bool
	olderThan string
	age       time.Duration
	// orphans is --orphans: also what devvm keeps for VMs their backend
	// no longer has.
	orphans bool
	// caches is --caches: then devvm's share of the backends' own
	// caches, each backend asked for apart, only in a terminal.
	caches bool
	// retention is --retention: then the retention settings, at once.
	retention bool
	// nixCache is --nix-cache: also the whole shared Nix cache, not only
	// what's over its cap.
	nixCache bool
}

// newGCCmd is devvm gc.
func newGCCmd(u *ui.UI) *cobra.Command {
	var o gcOpts
	c := &cobra.Command{
		Use:   "gc [--tool-images [--recent] [TOOL...]]",
		Short: "Delete stale images and unused tool images",
		Long: "Delete the stale and unfinished images on every backend, the shared tools devvm images lists as unused, " +
			"the files devvm itself no longer needs, and the shared Nix cache's least recently used files while it's over its cap, " +
			"after listing them and asking in a terminal. " +
			"Its flags delete more, such as --tool-images for the current shared tool images no VM has. " +
			"gc never deletes a VM.",
		Example: examples(
			"devvm gc", "delete stale images and unused tools, after asking",
			"devvm gc --tool-images", "also the current shared tool images no VM has",
			"devvm gc --history --older-than 30d", "also the history exports 30 days old or more", "devvm gc --force", "delete without asking, as a script must",
		),
		Args: func(_ *cobra.Command, args []string) error { return o.checkArgs(args) },
		RunE: func(cmd *cobra.Command, args []string) error {
			o.toolNames = args
			fail(u, gc(cmd.Context(), u, o), msg.GCFailed)
			return nil
		},
	}
	f := c.Flags()
	addBackendFilterFlag(f, &o.only)
	f.BoolVar(&o.tools, "tool-images", false, "also delete the current shared tool images no VM has, or only those of the TOOLs named")
	f.BoolVar(&o.recent, "recent", false, "with --tool-images, also delete those built in the last hour")
	addForceFlag(f, &o.force)
	f.BoolVar(&o.history, "history", false, "also delete the saved exports of agents' history")
	f.StringVar(&o.olderThan, "older-than", "", "with --history, only those older than `DURATION`, such as 30d")
	f.BoolVar(&o.nixCache, "nix-cache", false, "also delete the whole shared Nix cache")
	f.BoolVar(&o.orphans, "orphans", false, "also delete what devvm keeps for VMs their backend no longer has")
	f.BoolVar(&o.caches, "caches", false, "then free devvm's share of the backends' own caches, asking for each backend")
	f.BoolVar(&o.retention, "retention", false, "then apply the retention settings")
	return c
}

// checkArgs takes tool names, by the names images have them by, only
// with --tool-images, and an age only with --history.
func (o *gcOpts) checkArgs(args []string) error {
	if o.olderThan != "" {
		if !o.history {
			return msg.GCOlderThanNeedsHistory()
		}
		age, err := state.ParseDuration(o.olderThan)
		if err != nil {
			return err
		}
		o.age = age
	}
	if len(args) > 0 && !o.tools {
		return msg.GCNamesNeedTools(args[0])
	}
	if o.recent && !o.tools {
		return msg.GCRecentNeedsTools()
	}
	for _, a := range args {
		if !slices.Contains(image.Toolchains, a) && !slices.Contains(image.Agents, a) {
			return msg.UnknownTool(a, strings.Join(slices.Concat(image.Toolchains, image.Agents), " "))
		}
	}
	return nil
}

// gcPlan is what devvm gc deletes, gathered before it asks.
type gcPlan struct {
	images  []listedImage  // stale and unfinished images, each with its backend's
	tools   []tools.Listed // unused tool images
	current []tools.Listed // with --tool-images, current tool images no VM has
	// with --tool-images but not --force, those built in the last hour, which
	// gc keeps and says so; while a VM is being made (making), all of
	// them
	recent  []tools.Listed
	making  bool
	exports []savedExport // with --history, the exports old enough
	// with --orphans, the VMs their backend no longer has, and what
	// forgets them
	orphans   vm.LeftoverReport
	leftovers vm.Leftovers
	mise      []string // older versions of devvm's own mise
	nix       nixCachePlan
	share     tools.Share // where the tool images are
	// skipped are the backends a VM uses that couldn't list their
	// images: gc leaves everything of theirs alone.
	skipped map[string]bool
}

// gc lists what's stale or unused, asks in a terminal, and deletes it. It
// holds the share's lock throughout, so no build moves a tool image in
// and no update swaps the lock meanwhile.
func gc(ctx context.Context, u *ui.UI, o gcOpts) error {
	b, err := builder(u, o.only)
	if err != nil {
		return err
	}
	b.From = fromRef("")
	skipped, err := gcLocked(ctx, u, b, o)
	if err != nil {
		return err
	}
	if o.retention {
		if err := gcRetention(ctx, u, b); err != nil {
			return err
		}
	}
	if o.caches {
		return gcCaches(ctx, u, b, o, skipped)
	}
	return nil
}

// gcLocked is gcRun holding the share's lock throughout. With --tool-images it
// then tries the lock VMs being made hold (tools.Making), and holds it so
// no VM picks its tools meanwhile. While one is being made, --recent
// deletes nothing, and without it gc keeps every current tool no VM has.
func gcLocked(ctx context.Context, u *ui.UI, b *build.Env, o gcOpts) (map[string]bool, error) {
	unlock, err := b.Tools.LockShare(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if o.tools {
		unmaking, ok, err := (tools.Making{Dirs: b.Dirs}).Exclude()
		if err != nil {
			return nil, err
		}
		switch {
		case ok:
			defer unmaking()
		case o.recent:
			return nil, msg.GCRecentMaking()
		default:
			o.making = true
		}
	}
	return gcRun(ctx, u, b, o)
}

// gcRetention applies the retention settings now, the VMs' leftovers
// included, saying what it deleted in one line.
func gcRetention(ctx context.Context, u *ui.UI, b *build.Env) error {
	def, err := settings.Read(b.Dirs)
	if err != nil {
		return err
	}
	r := def.Retention
	if r.Off() {
		u.Info(msg.RetentionOff())
		return nil
	}
	unlock, ok := lockRetention(b.Dirs)
	if !ok {
		return msg.RetentionBusy()
	}
	defer unlock()
	done, err := retain(ctx, u, b, r, true)
	touchRetention(b.Dirs)
	if err != nil {
		return err
	}
	u.Info(done.report())
	return nil
}

// gcRun is gc's run but the backends' caches and retention: it lists what
// it deletes, asks in a terminal, and deletes it. It returns the backends
// it skipped, as they couldn't list their images.
func gcRun(ctx context.Context, u *ui.UI, b *build.Env, o gcOpts) (map[string]bool, error) {
	p, err := planGC(ctx, u, b, o)
	if err != nil {
		return nil, err
	}
	if p.empty() {
		p.showOrphans(u)
		if !p.showRecent(u) {
			u.Info(msg.NothingToClean())
		}
		return p.skipped, nil
	}
	p.show(u)
	p.showRecent(u)
	if ok, err := confirmed(u, o.force, "devvm gc", func() bool { return u.Confirm(msg.GCConfirm()) }); !ok {
		if err == nil {
			u.Info(msg.NothingDeleted())
		}
		return p.skipped, err
	}
	return p.skipped, p.run(ctx, u)
}

// planGC gathers what gc deletes. With o.only, it gathers that backend's images, current or not, and its
// tool images, and nothing that isn't a backend's.
func planGC(ctx context.Context, u *ui.UI, b *build.Env, o gcOpts) (gcPlan, error) {
	p := gcPlan{share: tools.Share{Env: b.Tools}}
	l, err := listImages(ctx, u, b)
	if err != nil {
		return p, err
	}
	imgs, ts := l.images, l.tools
	p.images, p.tools = staleImages(imgs), tools.Unused(ts)
	if o.tools {
		p.current, p.recent = p.share.NoVMRecent(ts, o.toolNames)
		switch {
		case o.making:
			p.current, p.recent, p.making = nil, append(p.current, p.recent...), true
		case o.recent:
			p.current, p.recent = append(p.current, p.recent...), nil
		}
	}
	p.skip(u, l.unlisted)
	if o.history {
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
		p.orphans = withoutSkipped(p.orphans, p.skipped)
	}
	if o.only != "" {
		p.images = backendImages(imgs, o.only)
		p.tools, p.current, p.recent = toolsOn(o.only, p.tools), toolsOn(o.only, p.current), toolsOn(o.only, p.recent)
		return p, nil
	}
	if p.mise, err = (tools.Versions{Env: b.Tools}).StaleMise(); err != nil {
		return p, err
	}
	p.nix, err = planNixCache(u, b.Dirs, o.nixCache)
	return p, err
}

// skip warns for each backend a VM uses that couldn't list its images and
// leaves out its shared tool images, and those without a record naming
// their backend, which may be its: gc carries on with the other backends.
func (p *gcPlan) skip(u *ui.UI, unlisted []unlisted) {
	if len(unlisted) == 0 {
		return
	}
	p.skipped = map[string]bool{}
	for _, s := range unlisted {
		u.Warn(msg.GCBackendSkipped(s.backend).Because(s.err))
		p.skipped[s.backend] = true
	}
	keep := func(ts []tools.Listed) []tools.Listed {
		var out []tools.Listed
		for _, t := range ts {
			if t.Backend != "" && !p.skipped[t.Backend] {
				out = append(out, t)
			}
		}
		return out
	}
	p.tools, p.current, p.recent = keep(p.tools), keep(p.current), keep(p.recent)
}

// withoutSkipped moves the orphans of a skipped backend to the backends
// that couldn't be asked, so their state stays.
func withoutSkipped(r vm.LeftoverReport, skipped map[string]bool) vm.LeftoverReport {
	var orphans []vm.Orphan
	for _, o := range r.Orphans {
		if !skipped[o.Backend] {
			orphans = append(orphans, o)
		} else if !slices.Contains(r.Unknown, o.Backend) {
			r.Unknown = append(r.Unknown, o.Backend)
		}
	}
	r.Orphans = orphans
	return r
}

// backendImages are a backend's images, current or not, but none another
// devvm is building.
func backendImages(imgs []listedImage, be string) []listedImage {
	var out []listedImage
	for _, l := range imgs {
		if l.Backend == be && l.State != build.StateBuilding {
			out = append(out, l)
		}
	}
	return out
}

// showRecent says which current tool images no VM has gc keeps because
// they were built in the last hour, or because a VM is being made,
// reporting whether there were any.
func (p gcPlan) showRecent(u *ui.UI) bool {
	if len(p.recent) == 0 {
		return false
	}
	label := tools.Label(toolFiles(p.recent))
	if p.making {
		u.Info(msg.GCToolsMaking(len(p.recent), label))
	} else {
		u.Info(msg.GCToolsRecent(len(p.recent), label))
	}
	return true
}

// empty reports whether gc has nothing to delete; the tool images it
// keeps for being recent aren't any.
func (p gcPlan) empty() bool {
	return len(p.images)+len(p.tools)+len(p.current)+len(p.exports)+len(p.orphans.Orphans)+len(p.mise) == 0 &&
		len(p.orphans.Links)+len(p.orphans.Sockets) == 0 && !p.nix.any()
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
	if links := p.orphans.Links; len(links) > 0 {
		u.Say(msg.GCDeadLinks(len(links)))
		for _, l := range links {
			u.Say(msg.GCDeadLink(l.Name, strconv.Itoa(l.Port), l.Target, l.Client))
		}
	}
	if n := len(p.orphans.Sockets); n > 0 {
		u.Say(msg.GCLinkSockets(n))
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
	if len(p.mise) > 0 {
		u.Say(msg.GCMise(len(p.mise), humanSize(dirsSize(p.mise))))
	}
	p.nix.show(u)
	if len(p.exports) > 0 {
		u.Say(msg.GCHistory(len(p.exports), humanSize(exportsSize(p.exports))))
		for _, e := range p.exports {
			u.Say(msg.GCHistoryExport(exportDate(e.At), humanSize(e.Size), e.VM))
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
	if len(p.orphans.Links)+len(p.orphans.Sockets) > 0 {
		if err := p.leftovers.ForgetLinks(ctx); err != nil {
			return err
		}
		if n := len(p.orphans.Sockets); n > 0 {
			u.Info(msg.DeletedLinkSockets(n))
		}
	}
	if err := deleteStale(ctx, u, p.images); err != nil {
		return err
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
		u.Info(msg.DeletedHistory(len(p.exports)))
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
	return p.nix.run(u)
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
