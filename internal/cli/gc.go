package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
)

// newGCCmd is devvm gc.
func newGCCmd(u *ui.UI, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "gc",
		Short: "Delete stale images and unused tool images",
		Long: "Delete what devvm images lists as stale or unfinished, on every backend, with their " +
			"records, and the shared tool images it lists as unused. It lists them first, and in a " +
			"terminal asks. VMs are never deleted; on Linux hosts, an image whose disk VMs use stays.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fail(u, gc(cmd.Context(), u, g), msg.GCFailed)
			return nil
		},
	}
}

// gc lists what's stale or unused, asks in a terminal, and deletes it. It
// holds the share's lock throughout, so no build moves a tool image in and
// no update swaps the lock meanwhile.
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
	imgs, ts, err := listImages(ctx, u, b)
	if err != nil {
		return err
	}
	stale, unused := staleImages(imgs), tools.Unused(ts)
	if len(stale)+len(unused) == 0 {
		u.Info(msg.NothingToClean())
		return nil
	}
	if len(stale) > 0 {
		u.Say(msg.GCImages())
		for _, l := range stale {
			if l.Name == "" {
				u.Say(msg.GCUnfinished(l.Key))
			} else {
				u.Say(msg.GCImage(l.Name, l.Key))
			}
		}
	}
	if len(unused) > 0 {
		u.Say(msg.GCTools(len(unused), humanSize(toolsSize(unused))))
	}
	if u.StdinIsTerminal() && !u.Confirm(msg.GCConfirm()) {
		u.Info(msg.NothingDeleted())
		return nil
	}
	if err := deleteStale(ctx, u, stale); err != nil {
		return err
	}
	if len(unused) == 0 {
		return nil
	}
	files := make([]string, 0, len(unused))
	for _, t := range unused {
		files = append(files, t.File)
	}
	if err := b.Tools.Delete(files); err != nil {
		return err
	}
	u.Info(msg.DeletedTools(len(files)))
	return nil
}

// deleteStale deletes these images, each on its backend. One the backend
// refuses with a message (on Linux hosts, an image whose disk VMs use)
// stays, saying so.
func deleteStale(ctx context.Context, u *ui.UI, imgs []listedImage) error {
	for _, l := range imgs {
		if err := l.builder.DeleteKey(ctx, l.Key); err != nil {
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
