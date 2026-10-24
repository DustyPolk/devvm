package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backends"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// newBuildCmd is devvm build IMAGE.
func newBuildCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "build IMAGE",
		Short: "Build an image and the images under it, unless they're built",
		Long: "Build an image and the images under it, unless they're built. IMAGE is base, " +
			"or your layers joined with +, bottom first (foo+bar is bar on foo on base).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return imageCmd(cmd.Context(), g, args[0], (*build.Builder).Build)
		},
	}
}

// newRebuildCmd is devvm rebuild IMAGE.
func newRebuildCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "rebuild IMAGE",
		Short: "Build an image again, and delete the old one",
		Long: "Build an image again, from its parent as it is, and delete the stale images " +
			"no VM uses, the old one and those built on it among them.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return imageCmd(cmd.Context(), g, args[0], (*build.Builder).Rebuild)
		},
	}
}

// imageCmd runs do on the builder of the backend new VMs get, once the
// name is one and the backend can run.
func imageCmd(ctx context.Context, g *globals, name string, do func(*build.Builder, context.Context, string) error) error {
	b, err := builder(g.backend)
	if err == nil {
		err = b.CheckLayers(build.Normalize(name))
	}
	if err == nil {
		err = b.Backend.Check(ctx)
	}
	if err == nil {
		err = do(b, ctx, name)
	}
	fail(err, func(reason string) msg.M { return msg.BuildFailed(name, reason) })
	return nil
}

// builder returns a builder on the backend new VMs get: flag's (--backend),
// else the default.
func builder(flag string) (*build.Builder, error) {
	dirs, err := paths.FromEnv()
	if err != nil {
		return nil, err
	}
	root, err := paths.Root()
	if err != nil {
		return nil, err
	}
	reg, err := backends.Registry(dirs)
	if err != nil {
		return nil, err
	}
	c, err := reg.For(dirs, new(state.VM), flag)
	if err != nil {
		return nil, err
	}
	return &build.Builder{Dirs: dirs, Root: root, Backend: c.Backend}, nil
}

// fail ends the run on an error: with the run log's end when its details
// are there, with what failed (an error that isn't a message, as what's
// reason) otherwise, quietly once interrupted.
func fail(err error, what func(reason string) msg.M) {
	if err == nil {
		return
	}
	if e, ok := errors.AsType[*backend.LoggedError](err); ok {
		ui.DieLogged(e.Msg)
	}
	if m, ok := errors.AsType[msg.M](err); ok {
		ui.Die(m)
	}
	ui.Die(what(err.Error()))
}
