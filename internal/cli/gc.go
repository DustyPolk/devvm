package cli

import (
	"context"
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
)

// newGCCmd is devvm gc.
func newGCCmd(u *ui.UI, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "gc",
		Short: "Delete stale images and unused tool images",
		Long: "Delete what devvm images lists as stale or unfinished, on every backend, with their " +
			"records, the shared tool images it lists as unused, the copies of devvm's own files " +
			"older versions wrote out, and the older versions of the mise devvm runs on this machine. " +
			"It lists them first, and in a terminal asks. VMs are never deleted; on Linux hosts, an " +
			"image whose disk VMs use stays.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fail(u, gc(cmd.Context(), u, g), msg.GCFailed)
			return nil
		},
	}
}

// gcPlan is what devvm gc deletes, gathered before it asks.
type gcPlan struct {
	images []listedImage  // stale and unfinished images, each with its backend's
	tools  []tools.Listed // unused tool images
	assets []string       // old copies of devvm's files no VM's proxy was started with
	mise   []string       // older versions of devvm's own mise
	share  tools.Share    // where the tool images are
}

// gc lists what's stale or unused, asks in a terminal, and deletes it. It
// holds the share's lock throughout, so no build moves a tool image in
// and no update swaps the lock meanwhile.
func gc(ctx context.Context, u *ui.UI, g *globals) error {
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
	p, err := planGC(ctx, u, b)
	if err != nil {
		return err
	}
	if p.empty() {
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
func planGC(ctx context.Context, u *ui.UI, b *build.Env) (gcPlan, error) {
	p := gcPlan{share: tools.Share{Env: b.Tools}}
	imgs, ts, err := listImages(ctx, u, b)
	if err != nil {
		return p, err
	}
	p.images, p.tools = staleImages(imgs), tools.Unused(ts)
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

func (p gcPlan) empty() bool {
	return len(p.images)+len(p.tools)+len(p.assets)+len(p.mise) == 0
}

// show lists what gc deletes.
func (p gcPlan) show(u *ui.UI) {
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
	if len(p.assets) > 0 {
		u.Say(msg.GCAssets(len(p.assets)))
	}
	if len(p.mise) > 0 {
		u.Say(msg.GCMise(len(p.mise), humanSize(dirsSize(p.mise))))
	}
}

// run deletes what's planned.
func (p gcPlan) run(ctx context.Context, u *ui.UI) error {
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
	if len(p.tools) == 0 {
		return nil
	}
	files := make([]string, 0, len(p.tools))
	for _, t := range p.tools {
		files = append(files, t.File)
	}
	if err := p.share.Delete(files); err != nil {
		return err
	}
	u.Info(msg.DeletedTools(len(files)))
	return nil
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
