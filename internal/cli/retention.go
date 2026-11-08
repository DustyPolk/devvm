package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/platform"
	"github.com/devvmhq/devvm/internal/settings"
	"github.com/devvmhq/devvm/internal/state"
	"github.com/devvmhq/devvm/internal/vm"
)

// retainEvery is how often devvm's commands apply the retention settings
// on their own (devvm gc --retention applies them at once).
const retainEvery = 24 * time.Hour

// newRetentionCmd is devvm retention [KEY=VALUE...]: the retention
// settings, shown bare.
func newRetentionCmd(a *app) *cobra.Command {
	u := a.u
	var asJSON bool
	cmd := &cobra.Command{
		Use:         "retention [KEY=VALUE...]",
		Annotations: about{docs: "using/settings#retention", group: groupSpace}.annotations(),
		Short:       "Show or change what devvm deletes on its own",
		Long: "Show the retention settings, which say what devvm deletes on its own, or change them with KEY=VALUE, " +
			"and KEY=off turns one off. The keys are logs.runs, history.age, history.count, tools.unused-age, " +
			"orphans.age and nix-cache.size, and every setting is off until you set it. " +
			"devvm applies them at most once a day, except orphans.age, which only devvm gc --retention applies, and they never delete a VM, an image or a shared tool any VM has.",
		Example: examples(
			"devvm retention", "show the settings",
			"devvm retention history.age=90d tools.unused-age=30d", "delete exports and unused tools once that old",
			"devvm retention history.age=off", "turn a setting off",
			"devvm gc --retention", "apply the settings now",
		),
		Args: showsOnly(func(_ *cobra.Command, args []string) error {
			if _, err := settings.ParseRetention(args); err != nil {
				// A bad duration is bad_duration, as for every flag.
				if m, ok := errors.AsType[msg.M](err); ok {
					return m
				}
				return msg.RetentionBad(err.Error()).Because(err)
			}
			return nil
		}),
		RunE: func(_ *cobra.Command, args []string) error {
			dirs, err := a.dirs()
			if err != nil {
				return failed(err, msg.RetentionSettingsFailed)
			}
			if len(args) == 0 {
				def, err := settings.Read(dirs)
				if err != nil {
					return failed(err, msg.RetentionSettingsFailed)
				}
				if asJSON {
					return printJSON(u, retentionJSON(def.Retention))
				}
				_, err = u.Stdout().Write([]byte(retentionShown(dirs, def.Retention)))
				return err
			}
			change, _ := settings.ParseRetention(args) // checked by Args
			_, err = settings.Save(dirs, settings.Values{Retention: change})
			if err != nil {
				return failed(err, msg.RetentionSettingsFailed)
			}
			u.Info(msg.RetentionSaved(dirs.Settings()))
			return nil
		},
	}
	jsonFlag(cmd, &asJSON)
	return cmd
}

// retentionShown is every retention setting as KEY=VALUE lines, in the
// order devvm space shows them, off for one that isn't set, each followed
// by where it came from.
func retentionShown(d paths.Dirs, r settings.Retention) string {
	var b strings.Builder
	for _, key := range settings.RetentionKeys() {
		b.WriteString(key + "=" + retentionValue(r, key) + " # " + r.Source(key).Label(d) + "\n")
	}
	return b.String()
}

// retentionValue is a retention setting's value as written, off when it
// isn't set.
func retentionValue(r settings.Retention, key string) string {
	if v := r.Raw[key]; v != "" {
		return v
	}
	return settings.RetainOff
}

// retentionOutput is devvm retention --json: every setting by its key,
// with its value as written, off when it isn't set, and where each came
// from (a settings.Source).
type retentionOutput struct {
	Settings map[string]string `json:"settings"`
	Sources  map[string]string `json:"sources"`
}

func retentionJSON(r settings.Retention) retentionOutput {
	out := retentionOutput{Settings: map[string]string{}, Sources: map[string]string{}}
	for _, key := range settings.RetentionKeys() {
		out.Settings[key] = retentionValue(r, key)
		out.Sources[key] = string(r.Source(key))
	}
	return out
}

// retainedReport is the one line that says what retention deleted.
func retainedReport(r vm.Retained) msg.M {
	return msg.RetentionDeleted(strconv.Itoa(r.Exports), strconv.Itoa(r.NixFiles), strconv.Itoa(r.Orphans), strconv.Itoa(r.Tools))
}

// autoRetain applies the retention settings once a day, at the start of
// a command (vm.Housekeeping.Retain): never without a retention file,
// never in two devvms at once (one holding the lock is waited for by
// nobody), and again at once when the clock went back. VMs' state of VMs
// gone from their backend waits for devvm gc --retention, which asks the
// backends. logs.runs is the run log's own (KeepRunLogs).
func autoRetain(ctx context.Context, a *app) {
	u := a.u
	d, err := a.dirs()
	if err != nil {
		return
	}
	def, err := settings.Read(d)
	if err != nil {
		u.Warn(msg.RetentionUnreadable().Because(err))
		return
	}
	r := def.Retention
	u.KeepRunLogs(r.RunLogs)
	if r.Off() || !retainDue(d, time.Now()) {
		return
	}
	unlock, ok := lockRetention(d)
	if !ok {
		return
	}
	defer unlock()
	env, err := a.vmEnv()
	if err != nil {
		return
	}
	b, err := a.builder()
	if err != nil {
		return
	}
	// A build holding the share's lock isn't waited for: the tools wait
	// for the next day's run.
	if b.Tools.ShareBusy() {
		r.ToolsAge = 0
	}
	done, err := vm.Housekeeping{Env: env}.Retain(ctx, b, r, false)
	touchRetention(d)
	if err != nil {
		u.Warn(msg.RetentionFailed().Because(err))
		return
	}
	if done.Any() {
		u.Note(retainedReport(done)) // on stderr: the command's data stays apart
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

// lockRetention holds retention's lock (paths.Dirs.RetentionLock) without waiting;
// false when another devvm holds it. It's an OS file lock, so a devvm
// that crashed or was killed never leaves it held.
func lockRetention(d paths.Dirs) (unlock func(), ok bool) {
	if err := os.MkdirAll(d.Data, 0o700); err != nil {
		return nil, false
	}
	unlock, ok, err := platform.TryLockFile(d.RetentionLock())
	if err != nil || !ok {
		return nil, false
	}
	return unlock, true
}

// skipsRetention reports whether a command never applies retention on its
// own: those that skip the layout check (skipsLayout), gc (which has
// --retention), space (which shows what the settings would free) and
// retention (which shows and changes them).
func skipsRetention(cmd *cobra.Command) bool {
	path := strings.Fields(cmd.CommandPath())
	return skipsLayout(cmd) || len(path) >= 2 && slices.Contains([]string{"gc", "space", "retention"}, path[1])
}
