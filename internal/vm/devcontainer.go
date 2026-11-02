package vm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/devcontainer"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
)

// Keys of a VM's state for the project's devcontainer.json.
const (
	// KeyDevcontainer is the file the VM was made from: its SHA-256 and
	// path ("<sum> <file>"), devcontainerNone or devcontainerOff.
	KeyDevcontainer = "devcontainer"
	// KeyDevcontainerEnv is its variables (devcontainer.Var), JSON.
	KeyDevcontainerEnv = "devcontainer_env"
	// KeyDevcontainerCreate is the commands it runs once, JSON, until
	// they've run.
	KeyDevcontainerCreate = "devcontainer_create"
	// KeyDevcontainerStart is the commands it runs at each start, JSON.
	KeyDevcontainerStart = "devcontainer_start"
	// KeyDevcontainerStarted is the start they last ran at, hashed.
	KeyDevcontainerStarted = "devcontainer_started"
)

// KeyDevcontainer's values besides a file's.
const (
	devcontainerNone = "none" // made without one
	devcontainerOff  = "off"  // made with --no-devcontainer
)

// devcontainerUnreadable is the sum of a file that can't be read now.
const devcontainerUnreadable = "unreadable"

// commandScript runs a devcontainer.json's command, its arguments, in a
// login shell: the VM's PATH, and its variables (~/.profile).
const commandScript = `exec "$@"`

var envVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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

// record writes what a new VM was made from into its state: the file and
// what it maps to of the VM's own (its variables, its commands), or none.
func (e devcontainers) record(st *state.VM, o Options) error {
	switch {
	case o.NoDevcontainer:
		return st.Set(KeyDevcontainer, devcontainerOff)
	case o.dc == nil:
		return st.Set(KeyDevcontainer, devcontainerNone)
	}
	p := o.dc
	for _, kv := range []struct {
		key string
		v   any
		set bool
	}{
		{KeyDevcontainerEnv, p.Env, len(p.Env) > 0},
		{KeyDevcontainerCreate, p.Create, len(p.Create) > 0},
		{KeyDevcontainerStart, p.Start, len(p.Start) > 0},
	} {
		if !kv.set {
			continue
		}
		var b strings.Builder
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(kv.v); err != nil {
			return err
		}
		if err := st.Set(kv.key, strings.TrimSuffix(b.String(), "\n")); err != nil {
			return err
		}
	}
	return st.Set(KeyDevcontainer, p.Sum+" "+p.File)
}

// check warns, once a launch, when the folder's devcontainer.json isn't
// the one the VM was made from (changed, gone, or new): nothing in it
// changes an existing VM.
func (e devcontainers) check(st *state.VM, dir string) {
	was := st.Get(KeyDevcontainer)
	if was == "" || was == devcontainerOff {
		return
	}
	file, data, err := e.find(dir)
	now := devcontainerNone
	switch {
	case err != nil:
		now = devcontainerUnreadable + " " + file
	case file != "":
		now = devcontainer.Sum(data) + " " + file
	}
	if now == was {
		return
	}
	if was == devcontainerNone {
		_, nowFile, _ := strings.Cut(now, " ")
		e.Warn(msg.DevcontainerNew(nowFile, st.Name()))
		return
	}
	_, wasFile, _ := strings.Cut(was, " ")
	e.Warn(msg.DevcontainerChanged(wasFile, st.Name()))
}

// run runs a VM's devcontainer.json commands, once it's set up: those
// that run once (onCreateCommand, updateContentCommand,
// postCreateCommand) at its first, then postStartCommand once each start,
// at the first launch that finds it started. They run in the VM, as its
// user, in the project folder; one that fails is said, and those after it
// in its group don't run.
func (e devcontainers) run(ctx context.Context, b backend.Backend, st *state.VM, dir string) error {
	once, err := commands(st, KeyDevcontainerCreate)
	if err != nil {
		return err
	}
	if len(once) > 0 {
		if err := st.Set(KeyDevcontainerCreate, ""); err != nil {
			return err
		}
		if err := e.runAll(ctx, b, st.Name(), dir, once); err != nil {
			return err
		}
	}
	start, err := commands(st, KeyDevcontainerStart)
	if err != nil || len(start) == 0 {
		return err
	}
	id, err := b.StartID(ctx, st.Name())
	if err != nil || id == "" {
		return err
	}
	sum := sha256.Sum256([]byte(id))
	started := hex.EncodeToString(sum[:16])
	if st.Get(KeyDevcontainerStarted) == started {
		return nil
	}
	if err := st.Set(KeyDevcontainerStarted, started); err != nil {
		return err
	}
	return e.runAll(ctx, b, st.Name(), dir, start)
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

// commands reads a state key's commands.
func commands(st *state.VM, key string) ([]devcontainer.Command, error) {
	v := st.Get(key)
	if v == "" {
		return nil, nil
	}
	var cmds []devcontainer.Command
	if err := json.Unmarshal([]byte(v), &cmds); err != nil {
		return nil, err
	}
	return cmds, nil
}

// devcontainerEnv is the exports of a VM's devcontainer.json variables,
// for its environment file; a name that isn't one is left out.
func devcontainerEnv(st *state.VM) (string, error) {
	v := st.Get(KeyDevcontainerEnv)
	if v == "" {
		return "", nil
	}
	var vars []devcontainer.Var
	if err := json.Unmarshal([]byte(v), &vars); err != nil {
		return "", err
	}
	var out strings.Builder
	for _, v := range vars {
		if envVarName.MatchString(v.Name) {
			out.WriteString("export " + v.Name + "=" + v.Value + "\n")
		}
	}
	return out.String(), nil
}
