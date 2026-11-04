package vm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/devcontainer"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/shell"
	"github.com/w3cj/devvm/internal/state"
)

// devcontainerUnreadable is the sum of a file that can't be read now.
const devcontainerUnreadable = "unreadable"

// commandScript runs a devcontainer.json's command, its arguments, in a
// login shell: the VM's PATH, and its variables (~/.profile).
const commandScript = `exec "$@"`

// devcontainers reads a project's devcontainer.json for a new VM, and runs
// its commands in the VM.
type devcontainers struct{ *Env }

// find reads the folder's devcontainer.json, never following a link out
// of it: its path in the folder ("" for none) and its contents.
func (e devcontainers) find(dir string) (string, []byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = root.Close() }()
	return devcontainer.Find(rootFS{root})
}

// rootFS is a folder's files, as os.Root opens them (never through a
// link out of it), opened without waiting: a FIFO the VM made in place of
// a file opens at once, and Find refuses it.
type rootFS struct{ root *os.Root }

func (f rootFS) Open(name string) (fs.File, error) {
	return f.root.OpenFile(name, os.O_RDONLY|platform.NoBlock, 0)
}

// prepare reads the folder's devcontainer.json for a new VM, unless
// --no-devcontainer, and changes o as it says: its image is --from's (one
// its registry gives anyone, pinned), its features' layers go under
// IMAGE's, its forwardPorts add to the default ports. What's given on the
// command line wins: --from, --ports. It says what it ignores.
func (e devcontainers) prepare(ctx context.Context, o *Options, vm, dir string) error {
	if o.NoDevcontainer {
		return nil
	}
	file, data, err := e.find(dir)
	switch {
	case err != nil:
		return msg.DevcontainerUnreadable(file).Because(err)
	case file == "":
		return nil
	}
	p, err := devcontainer.Parse(file, data, dir)
	if err != nil {
		return err
	}
	e.Info(msg.DevcontainerUsing(file, vm))
	for _, m := range p.Notes {
		e.Warn(m)
	}
	switch {
	case p.Image == "":
	case o.FromSet:
		e.Warn(msg.DevcontainerFlagWins(file, "from", "image"))
	default:
		if err := from.Check(p.Image); err != nil {
			return err
		}
		pinned, err := e.PublicImage(ctx, p.Image)
		if errors.Is(err, from.ErrNotFound) {
			return msg.DevcontainerImageNotPublic(file, p.Image).Because(err)
		} else if err != nil {
			return err
		}
		o.From = pinned
	}
	if p.Ports != "" && o.Ports != "" {
		e.Warn(msg.DevcontainerFlagWins(file, "ports", "forwardPorts"))
		p.Ports = ""
	}
	o.dc = &p
	return nil
}

// record sets what a new VM is made from in its state, before it's
// written: the file and what it maps to of the VM's own (its variables,
// its commands), or none.
func (e devcontainers) record(st *state.VM, o Options) error {
	d := devcontainerInVM{Off: o.NoDevcontainer}
	if p := o.dc; p != nil && !o.NoDevcontainer {
		d.File, d.Sum, d.Env, d.Create, d.Start = p.File, p.Sum, p.Env, p.Create, p.Start
	}
	return state.SetSection(st, devcontainerSection, d)
}

// check warns, once a launch, when the folder's devcontainer.json isn't
// the one the VM was made from (changed, gone, or new): nothing in it
// changes an existing VM.
func (e devcontainers) check(st *state.VM, dir string) {
	was := sectionOf[devcontainerInVM](st, devcontainerSection)
	if was.Off {
		return
	}
	file, data, err := e.find(dir)
	sum := ""
	switch {
	case err != nil:
		sum = devcontainerUnreadable
	case file != "":
		sum = devcontainer.Sum(data)
	}
	switch {
	case file == was.File && sum == was.Sum:
	case was.File == "":
		e.Warn(msg.DevcontainerNew(file, st.Name()))
	default:
		e.Warn(msg.DevcontainerChanged(was.File, st.Name()))
	}
}

// run runs a VM's devcontainer.json commands, once it's set up: those
// that run once (onCreateCommand, updateContentCommand,
// postCreateCommand) at its first, then postStartCommand once each start,
// at the first launch that finds it started. They run in the VM, as its
// user, in the project folder; one that fails is said, and those after it
// in its group don't run.
func (e devcontainers) run(ctx context.Context, b backend.Backend, st *state.VM, dir string) error {
	d := sectionOf[devcontainerInVM](st, devcontainerSection)
	if len(d.Create) > 0 && !d.Made.Created {
		// Recorded first: a run cut short never runs them again.
		if err := update(ctx, st, devcontainerSection, func(d *devcontainerInVM) error { d.Made.Created = true; return nil }); err != nil {
			return err
		}
		if err := e.runAll(ctx, b, st.Name(), dir, d.Create); err != nil {
			return err
		}
	}
	if len(d.Start) == 0 {
		return nil
	}
	run, err := b.Running(ctx, st.Name())
	if err != nil || run.ID == "" {
		return err
	}
	id := run.ID
	sum := sha256.Sum256([]byte(id))
	started := hex.EncodeToString(sum[:16])
	if d.Made.Started == started {
		return nil
	}
	if err := update(ctx, st, devcontainerSection, func(d *devcontainerInVM) error { d.Made.Started = started; return nil }); err != nil {
		return err
	}
	return e.runAll(ctx, b, st.Name(), dir, d.Start)
}

// runAll runs commands in the VM in order, their output in the run log,
// until one fails.
func (e devcontainers) runAll(ctx context.Context, b backend.Backend, vm, dir string, cmds []devcontainer.Command) error {
	for _, c := range cmds {
		e.Info(msg.RunningScript(c.Name, vm))
		e.LogNote()
		out := e.LogWriter()
		argv := append([]string{"bash", "-lc", commandScript, "devvm"}, c.Argv...)
		if err := b.Exec(ctx, vm, argv, backend.ExecOpts{Dir: dir, Stdout: out, Stderr: out}); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			e.Warn(msg.DevcontainerCommandFailed(c.Name, vm))
			return nil
		}
	}
	return nil
}

// devcontainerEnv is the exports of a VM's devcontainer.json variables,
// for its environment file; a name that isn't one is left out.
func devcontainerEnv(st *state.VM) (string, error) {
	d, err := state.Section[devcontainerInVM](st, devcontainerSection)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, v := range d.Env {
		if shell.EnvName(v.Name) {
			out.WriteString("export " + v.Name + "=" + v.Value + "\n")
		}
	}
	return out.String(), nil
}
