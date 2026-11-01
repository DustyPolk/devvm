package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// retainEvery is how often devvm's commands apply the retention settings
// on their own (devvm gc --retention applies them at once).
const retainEvery = 24 * time.Hour

// retainLockAge is how old the retention lock is when the devvm holding
// it is taken to have died.
const retainLockAge = 10 * time.Minute

// newRetentionCmd is devvm retention [KEY=VALUE...]: the retention
// settings, shown bare.
func newRetentionCmd(u *ui.UI) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "retention [KEY=VALUE...]",
		Short: "Show or change the retention settings: what devvm deletes on its own",
		Long: "Show each retention setting and its value, off when it isn't set, or change them, each " +
			"KEY=VALUE, and KEY=off to turn one off: logs.runs (the run logs kept, 20 when off), history.age " +
			"and history.count (the exports of agents' history older than an age, or beyond the newest count, deleted), " +
			"tools.unused-age (current shared tool images no VM has had for that long deleted), orphans.age " +
			"(what devvm keeps for VMs gone from their backend for that long, deleted by devvm gc " +
			"--retention) and nix-cache.size (the shared Nix cache's cap, 10G when off). Ages are durations, such as 30m, " +
			"12h, 30d or 2w, and sizes 500M, 20G or 1T. Every setting is off until you set it. devvm refuses a bad key " +
			"or value, or one with no value, and then writes nothing. devvm applies the settings once a day, " +
			"and devvm gc --retention applies them at once. They never delete a VM, an image or a tool any " +
			"VM has.",
		Args: showsOnly(func(_ *cobra.Command, args []string) error {
			if err := state.CheckRetention(args); err != nil {
				// A bad duration is bad_duration, as for every flag.
				if m, ok := errors.AsType[msg.M](err); ok {
					return m
				}
				return msg.RetentionBad(err.Error()).Because(err)
			}
			return nil
		}),
		RunE: func(_ *cobra.Command, args []string) error {
			dirs, err := paths.FromEnv()
			fail(u, err, msg.RetentionSettingsFailed)
			if len(args) == 0 {
				r, err := state.ReadRetention(dirs)
				fail(u, err, msg.RetentionSettingsFailed)
				if asJSON {
					return printJSON(u, retentionJSON(r))
				}
				_, err = u.Stdout().Write([]byte(retentionShown(r)))
				return err
			}
			_, err = state.SetRetention(dirs, args)
			fail(u, err, msg.RetentionSettingsFailed)
			u.Info(msg.RetentionSaved(dirs.Retention()))
			return nil
		},
	}
	jsonFlag(cmd, &asJSON)
	return cmd
}

// retentionShown is every retention setting as KEY=VALUE lines, in the
// order devvm space shows them, off for one that isn't set.
func retentionShown(r state.Retention) string {
	var b strings.Builder
	for _, key := range state.RetentionKeys() {
		b.WriteString(key + "=" + retentionValue(r, key) + "\n")
	}
	return b.String()
}

// retentionValue is a retention setting's value as written, off when it
// isn't set.
func retentionValue(r state.Retention, key string) string {
	if v := r.Raw[key]; v != "" {
		return v
	}
	return state.RetainOff
}

// retentionOutput is devvm retention --json: every setting by its key,
// with its value as written, off when it isn't set.
type retentionOutput struct {
	Settings map[string]string `json:"settings"`
}

func retentionJSON(r state.Retention) retentionOutput {
	out := retentionOutput{Settings: map[string]string{}}
	for _, key := range state.RetentionKeys() {
		out.Settings[key] = retentionValue(r, key)
	}
	return out
}

// retained is what retention deleted.
type retained struct{ exports, tools, orphans, nixFiles int }

func (r retained) any() bool { return r.exports+r.tools+r.orphans+r.nixFiles > 0 }

// report is the one line that says what retention deleted.
func (r retained) report() msg.M {
	return msg.RetentionDeleted(strconv.Itoa(r.exports), strconv.Itoa(r.nixFiles), strconv.Itoa(r.orphans), strconv.Itoa(r.tools))
}

// retain applies the retention settings r, each to what it names and
// nothing else: exports older than history.age, then all but the newest
// history.count; current tool images no VM has had for tools.unused-age;
// with orphans, what devvm keeps for VMs their backend hasn't had for
// orphans.age (it asks the backends); the shared Nix cache's least recently
// used files while it's over nix-cache.size (or its default). Never a VM,
// an image, or a tool image any VM's state lists. logs.runs is the run
// log's own (KeepRunLogs).
func retain(ctx context.Context, u *ui.UI, b *build.Env, r state.Retention, orphans bool) (retained, error) {
	var done retained
	now := time.Now()
	n, err := retainExports(b.Dirs, r, now)
	if err != nil {
		return done, err
	}
	done.exports = n
	if done.nixFiles, _, err = proxy.TrimNixCache(b.Dirs.NixCache(), r.NixCacheCap()); err != nil {
		return done, err
	}
	if r.ToolsAge > 0 {
		if done.tools, err = retainTools(ctx, u, b, r.ToolsAge, now); err != nil {
			return done, err
		}
	}
	if orphans && r.OrphansAge > 0 {
		if done.orphans, err = retainOrphans(ctx, u, r.OrphansAge, now); err != nil {
			return done, err
		}
	}
	return done, nil
}

// exportsToRetire are the exports history.age and history.count say to
// delete: those that old, then all but the newest count.
func exportsToRetire(es []savedExport, r state.Retention, now time.Time) []savedExport {
	var gone []savedExport
	keep := es
	if r.ExportsAge > 0 {
		keep = nil
		for _, e := range es {
			if age := now.Sub(e.At); age >= r.ExportsAge {
				gone = append(gone, e)
			} else {
				keep = append(keep, e)
			}
		}
	}
	if r.ExportsCount > 0 && len(keep) > r.ExportsCount {
		gone = append(gone, keep[:len(keep)-r.ExportsCount]...) // oldest first
	}
	return gone
}

func retainExports(d paths.Dirs, r state.Retention, now time.Time) (int, error) {
	if r.ExportsAge == 0 && r.ExportsCount == 0 {
		return 0, nil
	}
	es, err := exports(d)
	if err != nil {
		return 0, err
	}
	gone := exportsToRetire(es, r, now)
	return len(gone), removeAll(exportPaths(gone))
}

// retainTools deletes the current tool images no VM has had for age,
// holding the share's lock throughout, then the lock VMs being made hold,
// as gc --tools does. While a VM is being made it deletes none, and
// leaves their idle records as they are.
func retainTools(ctx context.Context, u *ui.UI, b *build.Env, age time.Duration, now time.Time) (int, error) {
	unlock, err := b.Tools.LockShare(ctx)
	if err != nil {
		return 0, err
	}
	defer unlock()
	unmaking, ok, err := (tools.Making{Dirs: b.Dirs}).Exclude()
	if err != nil || !ok {
		return 0, err
	}
	defer unmaking()
	ts, err := shareListing(ctx, u, b)
	if err != nil {
		return 0, err
	}
	share := tools.Share{Env: b.Tools}
	idle, err := share.Idle(ts, age, now)
	if err != nil || len(idle) == 0 {
		return 0, err
	}
	return len(idle), share.Delete(toolFiles(idle))
}

// shareListing lists the share as devvm images and gc do, without asking
// any backend: current for the default base on every backend.
func shareListing(ctx context.Context, u *ui.UI, b *build.Env) ([]tools.Listed, error) {
	reg, err := registry.New(b.Dirs, b.Assets, u)
	if err != nil {
		return nil, err
	}
	inUse, err := vm.ToolsInUse(b.Dirs)
	if err != nil {
		return nil, err
	}
	return spaceEnv{u: u, b: b, reg: reg, inUse: inUse}.listTools(ctx)
}

// retainOrphans forgets the VMs their backend no longer has whose state
// hasn't changed for age.
func retainOrphans(ctx context.Context, u *ui.UI, age time.Duration, now time.Time) (int, error) {
	env, err := newEnv(u)
	if err != nil {
		return 0, err
	}
	l := vm.Leftovers{Env: env}
	rep, err := l.Find(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, o := range rep.Orphans {
		if since := now.Sub(o.Changed); since < age || since < 0 {
			continue
		}
		if err := l.Forget(ctx, o); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// autoRetain applies the retention settings once a day, at the start of
// a command: never without a retention file, never in two devvms at once
// (one holding the lock is waited for by nobody), and again at once when
// the clock went back. VMs' state of VMs gone from their backend waits
// for devvm gc --retention, which asks the backends.
func autoRetain(ctx context.Context, u *ui.UI) {
	d, err := paths.FromEnv()
	if err != nil {
		return
	}
	r, err := state.ReadRetention(d)
	if err != nil {
		u.Warn(msg.RetentionUnreadable().Because(err))
		return
	}
	u.KeepRunLogs(r.RunLogs)
	if r.Off() || !retainDue(d, time.Now()) {
		return
	}
	unlock, ok := lockRetention(d)
	if !ok {
		return
	}
	defer unlock()
	b, err := builder(u, "")
	if err != nil {
		return
	}
	// A build holding the share's lock isn't waited for: the tools wait
	// for the next day's run.
	if _, err := os.Stat(filepath.Join(b.Dirs.Tools(), ".lock")); err == nil {
		r.ToolsAge = 0
	}
	done, err := retain(ctx, u, b, r, false)
	touchRetention(d)
	if err != nil {
		u.Warn(msg.RetentionFailed().Because(err))
		return
	}
	if done.any() {
		u.Note(done.report()) // on stderr: the command's data stays apart
	}
}

// retainDue reports whether retention last ran a day ago or more, never,
// or in the future (a clock that went back).
func retainDue(d paths.Dirs, now time.Time) bool {
	fi, err := os.Stat(d.RetentionRun())
	if err != nil {
		return true
	}
	since := now.Sub(fi.ModTime())
	return since >= retainEvery || since < 0
}

func touchRetention(d paths.Dirs) {
	now := time.Now()
	if err := os.Chtimes(d.RetentionRun(), now, now); errors.Is(err, fs.ErrNotExist) {
		_ = state.WriteFile(d.RetentionRun(), nil)
	}
}

// lockRetention holds retention's lock, a folder; false when another
// devvm holds it. A lock older than retainLockAge is a dead devvm's.
func lockRetention(d paths.Dirs) (unlock func(), ok bool) {
	lock := d.RetentionRun() + ".lock"
	if err := os.MkdirAll(d.Data, 0o700); err != nil {
		return nil, false
	}
	if err := os.Mkdir(lock, 0o700); err != nil {
		fi, serr := os.Stat(lock)
		if serr != nil || time.Since(fi.ModTime()) < retainLockAge {
			return nil, false
		}
		_ = os.Remove(lock)
		if os.Mkdir(lock, 0o700) != nil {
			return nil, false
		}
	}
	return func() { _ = os.Remove(lock) }, true
}

// skipsRetention reports whether a command never applies retention on its
// own: devvm's internal commands, completion, gc (which has --retention),
// space (which shows what the settings would free) and retention (which
// shows and changes them).
func skipsRetention(cmd *cobra.Command) bool {
	path := strings.Fields(cmd.CommandPath())
	if len(path) < 2 {
		return false
	}
	return strings.HasPrefix(path[1], "_") || slices.Contains([]string{"completion", "gc", "space", "retention", "version", "help", "__complete"}, path[1])
}
