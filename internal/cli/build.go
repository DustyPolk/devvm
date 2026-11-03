package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
)

// newBuildCmd is devvm build IMAGE.
func newBuildCmd(u *ui.UI) *cobra.Command {
	var f imageFlags
	cmd := &cobra.Command{
		Use:   "build IMAGE",
		Short: "Build an image and the images under it, unless they're built",
		Long: "Build an image and the images under it, unless they're built. " +
			"IMAGE is base, or layers joined with + bottom first, so foo+bar is bar on foo on base. " +
			"An image holds no shared tools: a VM gets those when devvm makes it, or with devvm tools.",
		Example: examples(
			"devvm build docker", "build base and the docker layer on it",
			"devvm build foo+bar", "build base, foo and foo+bar",
			"devvm build base --backend smolvm", "build base on the smolvm backend",
		),
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return imageCmd(cmd.Context(), u, f, args[0], build.Builds.Build)
		},
	}
	f.add(cmd)
	return cmd
}

// newRebuildCmd is devvm rebuild IMAGE.
func newRebuildCmd(u *ui.UI) *cobra.Command {
	var f imageFlags
	cmd := &cobra.Command{
		Use:   "rebuild IMAGE",
		Short: "Build an image again, and delete the old one",
		Long: "Build an image again from its parent as it is, then delete the stale images " +
			"no VM uses, the old one and those built on it among them. " +
			"When images are built on it, devvm asks first in a terminal.",
		Example: examples(
			"devvm rebuild foo", "build foo again with the newest packages",
			"devvm rebuild base --force", "rebuild base without asking, as a script must",
		),
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return imageCmd(cmd.Context(), u, f, args[0], build.Builds.Rebuild)
		},
	}
	f.add(cmd)
	addForceFlag(cmd.Flags(), &f.force)
	return cmd
}

// imageFlags are the flags of the commands that build or delete an image
// by its name: the backend it's on and the container image its base
// starts from; rebuild's --force too.
type imageFlags struct {
	backend, from string
	force         bool
}

func (f *imageFlags) add(cmd *cobra.Command) {
	addBackendFlag(cmd.Flags(), &f.backend)
	addFromFlag(cmd.Flags(), &f.from)
}

// imageCmd runs do on the builds of the backend new VMs get, once the
// name is one and the backend can run.
func imageCmd(ctx context.Context, u *ui.UI, f imageFlags, name string, do func(build.Builds, context.Context, string) error) error {
	b, err := builder(u, f.backend)
	if err == nil {
		b.From, b.Force = fromRef(f.from), f.force
		err = from.Check(b.From)
	}
	if err == nil {
		err = b.CheckLayers(image.Normalize(name))
	}
	if err == nil {
		err = b.Backend.Check(ctx)
	}
	if err == nil {
		err = do(build.Builds{Env: b}, ctx, name)
	}
	fail(u, err, func(reason string) msg.M { return msg.BuildFailed(name, reason) })
	return nil
}

// builder returns what builds images on the backend new VMs get: flag's (--backend),
// else the default.
func builder(u *ui.UI, flag string) (*build.Env, error) {
	dirs, err := paths.FromEnv()
	if err != nil {
		return nil, err
	}
	reg, err := newRegistry(dirs, assets.FS, u)
	if err != nil {
		return nil, err
	}
	c, err := reg.For(dirs, new(state.VM), flag)
	if err != nil {
		return nil, err
	}
	return newBuilder(u, dirs, c.Backend, reg.Review, newTools(dirs, assets.FS, u, reg.Host), ""), nil
}

// newBuilder returns what builds images on be, its features checked
// against rv, whose base starts from fromRef, printing and asking through
// u.
func newBuilder(u *ui.UI, dirs paths.Dirs, be backend.Backend, rv backend.Review, t *tools.Env, fromRef string) *build.Env {
	return &build.Env{
		Dirs: dirs, Assets: assets.FS, Backend: be, Review: rv, Tools: t, From: fromRef,
		Info: u.Info, Warn: u.Warn, Say: u.Say, Confirm: func(m msg.M) bool { return u.Confirm(m) },
		Pick: pickFeatures(u), Terminal: u.StdinIsTerminal, LogNote: u.LogNote, FreeSpace: thisHost().FreeSpace,
	}
}

// pickFeatures returns what asks which features base gets, in u's
// terminal; without one, they stay as they are.
func pickFeatures(u *ui.UI) func([]build.Feature) []build.Feature {
	return func(fs []build.Feature) []build.Feature {
		choices := make([]ui.Choice, len(fs))
		for i, f := range fs {
			choices[i] = ui.Choice{Name: f.Name, Desc: f.Desc, On: f.On}
		}
		picked := u.Pick(msg.FeaturesHelp(), []ui.Section{{Choices: choices}})[0].Choices
		out := make([]build.Feature, len(picked))
		for i, c := range picked {
			out[i] = build.Feature{Name: c.Name, Desc: c.Desc, On: c.On}
		}
		return out
	}
}

// fail ends the run on an error, through u: with the run log's end when
// its details are there, with what failed (an error that isn't a message,
// as what's reason) otherwise, quietly once interrupted.
func fail(u *ui.UI, err error, what func(reason string) msg.M) {
	if err == nil {
		return
	}
	if e, ok := errors.AsType[*backend.LoggedError](err); ok {
		u.DieLogged(e.Msg)
	}
	if m, ok := errors.AsType[msg.M](err); ok {
		u.Die(m)
	}
	u.Die(what(err.Error()).Because(err))
}
