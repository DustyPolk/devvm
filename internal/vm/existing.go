package vm

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"slices"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// Restart stops the folder's VM, then opens it as Up does: what changes
// only when it starts (its mounts, size, ports) applies.
func (e Launch) Restart(ctx context.Context, o Options) error {
	t, err := finder(e).target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	if err := t.b.Stop(ctx, t.f.Name); err != nil {
		return err
	}
	return e.open(ctx, o, t)
}

// restartNow reports whether the user agrees to restart the VM vm, which
// is running, so that a change that applies only when it starts applies
// at once (restart_now). It asks only where a question with a fallback
// is asked (CanAsk: a terminal, without --no-input); else it's no, and
// the caller says when the change applies.
func (e *Env) restartNow(vm string) bool {
	return e.CanAsk != nil && e.CanAsk() && e.Confirm(msg.RestartNow(vm))
}

// restartOr restarts the folder's VM vm, which is running, as devvm
// restart does once the user agrees (restartNow), so that a setting just
// changed applies now; otherwise say(later) tells the user when it
// applies.
func (e *Env) restartOr(ctx context.Context, vm string, say func(msg.M), later msg.M) error {
	if !e.restartNow(vm) {
		say(later)
		return nil
	}
	return Launch{e}.Restart(ctx, Options{})
}

// open opens a shell in a VM that exists, made for this folder. Flags for
// new VMs are ignored, with a warning. On the way it warns about images
// and tools newer than the VM's (--clone on a VM that mounts the folder is
// refused); then it launches the VM (launch) with its agent folder mounts
// matched to the host's, sets it up and opens the shell, as for a new VM.
func (e Launch) open(ctx context.Context, o Options, t *found) error {
	vm, st, b := t.f.Name, t.st, t.b
	if o.Clone && !isClone(st) {
		return msg.CloneNeedsNewVM(vm)
	}
	if o.From != "" && o.From != st.From {
		e.Warn(msg.FromIgnored(vm))
	}
	builder := e.Builder(b, st.From)
	if err := (build.Builds{Env: builder}).WarnIfOutdated(st.Chain); err != nil {
		return err
	}
	tg, err := e.toolTarget(b, st)
	if err != nil {
		return err
	}
	if err := (tools.VMs{Env: e.Tools}).WarnNewer(ctx, tools.Names(st), tools.Files(st), tg); err != nil {
		return err
	}
	if err := (tools.Versions{Env: e.Tools}).RefreshDaily(); err != nil {
		return err
	}
	e.warnIgnored(o, vm)
	conf, err := e.launch(ctx, t, upOpts{agentDirs: true})
	if err != nil {
		return err
	}
	if err := e.setUpVM(ctx, b, st, t.f, conf); err != nil {
		return err
	}
	cmd, err := b.Shell(ctx, vm, t.shellDir(ctx, e.LogWriter()))
	if err != nil {
		return err
	}
	return e.Exec(cmd)
}

// shellDir is the folder in a running VM that its shell and editors open
// in: its project folder, else, for a parent folder's VM, the folder devvm
// runs in (t.sub), at the same path in the VM. A --clone VM's clone is at
// the parent's path: it's the subfolder when the clone has it, else the
// clone's top. The check's errors go to log.
func (t *found) shellDir(ctx context.Context, log io.Writer) string {
	if t.sub == "" {
		return t.f.Dir
	}
	if isClone(t.st) {
		err := t.b.Exec(ctx, t.f.Name, []string{"test", "-d", t.sub}, backend.ExecOpts{Stderr: log})
		if err != nil {
			return t.f.Dir
		}
	}
	return t.sub
}

// launch takes a VM that exists, made for this folder, through what each
// launch of it does before its setup: the folder's devcontainer.json
// compared with the one it was made from, then what every start of it
// does (starter.bringUp). It returns the confinement the VM runs with.
func (e Launch) launch(ctx context.Context, t *found, o upOpts) (backend.Confinement, error) {
	devcontainers(e).check(t.st, t.f.Dir)
	return starter(e).bringUp(ctx, t, o)
}

// warnIgnored warns about the flags only new VMs take, on the VM vm.
func (e Launch) warnIgnored(o Options, vm string) {
	if o.Toolchains != nil || o.Agents != nil || o.Layers != nil {
		e.Warn(msg.PartsFlagsIgnored(vm))
	}
	if o.NetworkPolicy || o.NoNetworkPolicy {
		e.Warn(msg.NetFlagIgnored())
	}
	if o.NoGitProtect {
		e.Warn(msg.GitProtectFlagIgnored())
	}
	if o.Size != (Size{}) {
		e.Warn(msg.SizeFlagsIgnored())
	}
	if o.Ports != "" {
		e.Warn(msg.PortsFlagIgnored())
	}
	if o.SSHAgent {
		e.Warn(msg.SSHAgentFlagIgnored())
	}
	if o.NoDevcontainer {
		e.Warn(msg.NoDevcontainerFlagIgnored())
	}
}

// toolTarget is what a VM's tool images are built for: its base's C
// library, with a --from gnu base's glibc version, for musl the digest
// its --from image has as last recorded, and its backend's format.
func (e *Env) toolTarget(b backend.Backend, st *state.VM) (tools.Target, error) {
	t, _ := tools.Of(st)
	tg := tools.Target{Libc: t.Made.Libc, Glibc: t.Made.Glibc, Format: b.ToolFormat()}
	if tg.Libc == "musl" {
		digest, err := from.Recorded(e.Dirs, st.From)
		if err != nil {
			return tools.Target{}, err
		}
		tg.Base = digest
	}
	return tg, nil
}

// buildTarget is toolTarget for building the VM's tools with builder: a
// musl VM's base is built first, its digest looked up, as its tool images
// build in that base as it is now and their keys hash its digest.
func (e Launch) buildTarget(ctx context.Context, builder *build.Env, b backend.Backend, st *state.VM) (tools.Target, error) {
	if t, _ := tools.Of(st); t.Made.Libc == "musl" {
		if err := (build.Builds{Env: builder}).Build(ctx, image.Base); err != nil {
			return tools.Target{}, err
		}
	}
	return e.toolTarget(b, st)
}

// buildTools builds the tool images the share lacks of files, then checks
// that each runs on tg's base (tools.Share.Runs). They build in a base as
// it is now, built first if it isn't: for gnu, devvm's own Ubuntu base on
// builder's backend, whatever builder's base starts from, as any VM may
// mount them, and for musl builder's own, as their keys hash its digest.
func (e Launch) buildTools(ctx context.Context, builder *build.Env, tg tools.Target, files []string) error {
	share := tools.Share{Env: e.Tools}
	if slices.ContainsFunc(files, func(f string) bool { return !share.Built(f) }) {
		if tg.Libc != "musl" && builder.From != "" {
			own, err := e.ownBase(builder.Backend)
			if err != nil {
				return err
			}
			builder = own
		}
		if err := (build.Builds{Env: builder}).Build(ctx, image.Base); err != nil {
			return err
		}
		base, err := builder.Key(image.Base)
		if err != nil {
			return err
		}
		if err := share.Build(ctx, builder.Review, builder.Backend, base, tg, files); err != nil {
			return err
		}
	}
	return share.Runs(files, tg)
}

// ownBase is what builds devvm's own Ubuntu base on b, which a --from
// VM's gnu tool images build in. When it isn't built yet, it says that
// it builds it first (tools_need_own_base), before the build says it
// builds base.
func (e Launch) ownBase(b backend.Backend) (*build.Env, error) {
	own := e.Builder(b, "")
	key, err := own.Key(image.Base)
	if err != nil {
		return nil, err
	}
	if _, err := state.ReadImage(e.Dirs, key); errors.Is(err, fs.ErrNotExist) {
		e.Info(msg.ToolsNeedOwnBase())
	}
	return own, nil
}
