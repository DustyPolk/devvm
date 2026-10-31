package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// spaceTimeout bounds what devvm space asks each backend, so a backend
// that hangs (Docker's engine starting) never holds it up.
const spaceTimeout = 30 * time.Second

// What devvm space's KEPT column says of each kind of thing.
const (
	keptRemade   = "remade"    // devvm makes it again when it's needed
	keptOnlyCopy = "only copy" // nothing else has it: a VM's work, an export
	keptLog      = "log"       // a record nothing needs
	keptState    = "state"     // what devvm knows of its VMs and images
	keptLeftover = "leftover"  // what devvm keeps for a VM its backend no longer has
)

// newSpaceCmd is devvm space.
func newSpaceCmd(u *ui.UI, g *globals) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "space",
		Short: "Show what devvm keeps on this machine and the space it takes",
		Long: "Show each kind of thing devvm keeps on this machine, on every backend, with how many there " +
			"are, the disk they take, whether devvm can make them again, what devvm gc frees of them now " +
			"and what frees the rest, then a total. The backends' own caches come last and apart, because " +
			"they can also hold your own downloads. It changes nothing and starts no VM.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := space(cmd.Context(), u, g)
			fail(u, err, msg.SpaceFailed)
			for _, b := range r.Unknown {
				u.Note(msg.SpaceBackendUnknown(b))
			}
			for _, s := range r.Strays {
				u.Note(msg.SpaceStray(s.Backend, s.Data, s.Name))
			}
			if asJSON {
				enc := json.NewEncoder(u.Stdout())
				enc.SetIndent("", "  ")
				return enc.Encode(r)
			}
			if err := writeSpace(u.Stdout(), r); err != nil {
				return err
			}
			if len(r.Caches) > 0 {
				u.Note(msg.SpaceCachesNote())
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print it as JSON, for scripts")
	c.AddCommand(&cobra.Command{
		Use:   "set KEY=VALUE...",
		Short: "Change the retention settings: what devvm deletes on its own",
		Long: "Change the retention settings, each KEY=VALUE, and KEY= to turn one off: logs.runs (the run " +
			"logs kept, 20 when off), exports.age and exports.count (the exports older than an age, or " +
			"beyond the newest count, deleted), tools.unused-age (current shared tool images no VM has had " +
			"for that long deleted) and orphans.age (what devvm keeps for VMs gone from their backend for " +
			"that long, deleted by devvm gc --retention). Ages are 30d, 2w or 12h. Every setting is off " +
			"unless set. A bad key or value is refused, and nothing is written. devvm applies them once a " +
			"day, and devvm gc --retention at once; they never delete a VM, an image or a tool any VM has.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dirs, err := paths.FromEnv()
			if err == nil {
				_, err = state.SetRetention(dirs, args)
				if err != nil {
					if _, corrupt := errors.AsType[*state.CorruptError](err); !corrupt {
						err = msg.RetentionBad(err.Error())
					}
				}
			}
			fail(u, err, msg.SpaceFailed)
			u.Info(msg.RetentionSaved(dirs.Retention()))
			return nil
		},
	})
	return c
}

// spaceRow is one kind of thing devvm keeps, as devvm space shows it.
type spaceRow struct {
	What    string `json:"what"`
	Backend string `json:"backend,omitempty"`
	Count   int    `json:"count"`
	Size    int64  `json:"size"`
	Sized   bool   `json:"sized"` // Size is known
	Kept    string `json:"kept"`  // one of the kept constants
	GC      int64  `json:"gc"`    // what devvm gc frees of it now
	Rest    string `json:"rest"`  // what frees the rest
	// Retain is the retention setting that applies to it, as written, ""
	// when there's none or it's off; RetainFrees what it would free now.
	Retain      string `json:"retain,omitempty"`
	RetainFrees int64  `json:"retain_frees,omitempty"`
}

// retainCell is how the RETAIN column shows a row's setting: off, or its
// value and what it would free now.
func (s spaceRow) retainCell() string {
	if s.Retain == "" {
		return ""
	}
	return s.Retain + " (" + humanSize(s.RetainFrees) + ")"
}

// cacheRow is one of a backend's own caches.
type cacheRow struct {
	Backend string `json:"backend"`
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Count   int    `json:"count"`
	Size    int64  `json:"size"`
	Sized   bool   `json:"sized"`
}

// spaceReport is what devvm space shows.
type spaceReport struct {
	Rows    []spaceRow `json:"devvm"`
	Total   int64      `json:"total"`
	GC      int64      `json:"gc"`
	Caches  []cacheRow `json:"backends"`
	Unknown []string   `json:"unknown,omitempty"` // backends that didn't answer
	// Strays are VMs with devvm's label it has no state for: another
	// devvm's, or this one's whose state is gone. devvm never deletes them.
	Strays []strayVM `json:"strays,omitempty"`
}

// strayVM is a VM with devvm's label that this devvm has no state for.
type strayVM struct {
	Backend string `json:"backend"`
	Name    string `json:"name"`
	Data    string `json:"data"` // the data folder its label names
}

// spaceEnv is what devvm space reads: devvm's folders, its backends and
// the shared tools.
type spaceEnv struct {
	u     *ui.UI
	b     *build.Env
	reg   *backend.Registry
	inUse map[string]int
	// vms is each backend's VMs, by name, for those that answered.
	vms map[string]map[string]bool
	ret state.Retention // the retention settings
	now time.Time
}

// space reads what devvm keeps. Nothing it runs changes anything, and
// each backend gets spaceTimeout.
func space(ctx context.Context, u *ui.UI, g *globals) (spaceReport, error) {
	defer u.Status(msg.ReadingSpace())()
	var r spaceReport
	b, err := builder(u, g.backend)
	if err != nil {
		return r, err
	}
	reg, err := registry.New(b.Dirs, b.Assets, u)
	if err != nil {
		return r, err
	}
	inUse, err := vm.ToolsInUse(b.Dirs)
	if err != nil {
		return r, err
	}
	ret, err := state.ReadRetention(b.Dirs)
	if err != nil {
		return r, err
	}
	e := spaceEnv{u: u, b: b, reg: reg, inUse: inUse, vms: map[string]map[string]bool{}, ret: ret, now: time.Now()}
	used, err := state.Backends(b.Dirs)
	if err != nil {
		return r, err
	}
	for _, be := range reg.All() {
		rows, caches, ok := e.backendRows(ctx, be)
		if !ok {
			if used[be.Name()] {
				r.Unknown = append(r.Unknown, be.Name())
			}
			continue
		}
		r.Rows = append(r.Rows, rows...)
		r.Caches = append(r.Caches, caches...)
		r.Strays = append(r.Strays, strays(ctx, be)...)
	}
	toolRows, err := e.toolRows(ctx)
	if err != nil {
		return r, err
	}
	r.Rows = append(r.Rows, toolRows...)
	dataRows, err := e.dataRows()
	if err != nil {
		return r, err
	}
	r.Rows = append(r.Rows, dataRows...)
	for _, row := range r.Rows {
		r.Total += row.Size
		r.GC += row.GC
	}
	return r, nil
}

// backendRows are a backend's VMs and images, and its caches; ok is false
// when it can't be asked (not installed, not running, too slow).
func (e spaceEnv) backendRows(ctx context.Context, be backend.Backend) (rows []spaceRow, caches []cacheRow, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, spaceTimeout)
	defer cancel()
	if be.Check(ctx) != nil {
		return nil, nil, false
	}
	vms, err := be.List(ctx)
	if err != nil {
		return nil, nil, false
	}
	ls, err := build.Images{Env: newBuilder(e.u, e.b.Dirs, be, e.reg.Review, e.b.Tools, "")}.Listing(ctx)
	if err != nil {
		return nil, nil, false
	}
	names := map[string]bool{}
	for _, in := range vms {
		names[in.Name] = true
	}
	e.vms[be.Name()] = names
	var usage backend.Usage
	if r, isReporter := be.(backend.UsageReporter); isReporter {
		usage, _ = r.Usage(ctx)
	}
	vmRow := spaceRow{What: "VMs", Backend: be.Name(), Count: len(vms), Sized: true, Kept: keptOnlyCopy, Rest: "devvm rm"}
	for _, in := range vms {
		n, sized := usage.VMs[in.Name]
		vmRow.Size += n
		vmRow.Sized = vmRow.Sized && sized
	}
	imgRow := spaceRow{What: "images", Backend: be.Name(), Count: len(ls), Sized: true, Kept: keptRemade, Rest: "gc --backend " + be.Name()}
	for _, l := range ls {
		imgRow.Size += l.Size
		imgRow.Sized = imgRow.Sized && l.Sized
		if l.State != build.StateCurrent {
			imgRow.GC += l.Size
		}
	}
	for _, c := range usage.Caches {
		if c.Count == 0 {
			continue
		}
		caches = append(caches, cacheRow{Backend: be.Name(), Name: c.Name, Path: c.Path, Count: c.Count, Size: c.Size, Sized: c.Sized})
	}
	return nonEmpty(vmRow, imgRow), caches, true
}

// strays are a backend's VMs with devvm's label that this devvm has no
// state for; none when it can't say.
func strays(ctx context.Context, be backend.Backend) []strayVM {
	f, ok := be.(backend.StrayFinder)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, spaceTimeout)
	defer cancel()
	ss, err := f.Strays(ctx)
	if err != nil {
		return nil
	}
	out := make([]strayVM, 0, len(ss))
	for _, s := range ss {
		out = append(out, strayVM{Backend: be.Name(), Name: s.Name, Data: s.Data})
	}
	return out
}

// nonEmpty returns the rows that have something.
func nonEmpty(rows ...spaceRow) []spaceRow {
	return slices.DeleteFunc(rows, func(r spaceRow) bool { return r.Count == 0 })
}

// toolRows are the shared tools' builds, by backend and C library: what
// devvm gc frees of them is what devvm images lists as unused.
func (e spaceEnv) toolRows(ctx context.Context) ([]spaceRow, error) {
	ts, err := e.listTools(ctx)
	if err != nil {
		return nil, err
	}
	targets, err := vmToolTargets(e.b.Dirs)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*spaceRow{}
	var keys []string
	for _, t := range ts {
		if !t.Built {
			continue
		}
		be, libc := toolTarget(e.reg, t, targets)
		key := be + " " + libc
		row, ok := byKey[key]
		if !ok {
			row = &spaceRow{What: "tools " + cmp.Or(libc, "-"), Backend: be, Sized: true, Kept: keptRemade, Rest: "gc --tools", Retain: e.ret.Raw[state.RetainToolsAge]}
			byKey[key] = row
			keys = append(keys, key)
		}
		row.Count++
		row.Size += t.Size
		if t.State == tools.StateUnused {
			row.GC += t.Size
		}
		if e.ret.ToolsAge > 0 && e.idleFor(t) >= e.ret.ToolsAge {
			row.RetainFrees += t.Size
		}
	}
	slices.Sort(keys)
	rows := make([]spaceRow, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, *byKey[k])
	}
	return rows, nil
}

// idleFor is how long a current tool image has had no VM, as retention
// recorded it (Share.Idle); 0 when it hasn't, or a VM has it. It changes
// nothing.
func (e spaceEnv) idleFor(t tools.Listed) time.Duration {
	if t.State != tools.StateCurrent || t.VMs > 0 {
		return 0
	}
	fi, err := os.Stat(filepath.Join(e.b.Dirs.Tools(), t.File+".idle"))
	if err != nil {
		return 0
	}
	return max(e.now.Sub(fi.ModTime()), 0)
}

// listTools lists the share as devvm images and devvm gc do: current for
// the default base on every backend.
func (e spaceEnv) listTools(_ context.Context) ([]tools.Listed, error) {
	tg, err := e.b.ToolTarget()
	if err != nil {
		return nil, err
	}
	tgs := []tools.Target{tg}
	for _, be := range e.reg.All() {
		t, err := newBuilder(e.u, e.b.Dirs, be, e.reg.Review, e.b.Tools, e.b.From).ToolTarget()
		if err != nil {
			return nil, err
		}
		tgs = append(tgs, t)
	}
	return tools.Share{Env: e.b.Tools}.List(tgs, e.inUse)
}

// vmTarget is the backend and C library of a VM that has a tool image.
type vmTarget struct{ backend, libc string }

// vmToolTargets is, for each tool image a VM's state lists, that VM's
// backend and C library.
func vmToolTargets(d paths.Dirs) (map[string]vmTarget, error) {
	names, err := state.VMNames(d)
	if err != nil {
		return nil, err
	}
	out := map[string]vmTarget{}
	for _, n := range names {
		st, err := state.ReadVM(d, n)
		if err != nil {
			return nil, err
		}
		for _, f := range strings.Fields(st.Get(vm.KeyTools)) {
			out[f] = vmTarget{backend: st.Get(state.KeyBackend), libc: st.Get(vm.KeyLibc)}
		}
	}
	return out, nil
}

// toolTarget is the backend and C library a tool image is built for: from
// its record, else from a VM that has it, else unknown ("").
func toolTarget(reg *backend.Registry, t tools.Listed, vms map[string]vmTarget) (be, libc string) {
	if t.Targeted {
		return formatBackend(reg, t.Target.Format), t.Target.Libc
	}
	v := vms[t.File]
	return v.backend, v.libc
}

// formatBackend is the backend whose tool images have this format: its
// ToolFormat, or one of the same kind (plain-, dir-) with another hash,
// an older devvm's.
func formatBackend(reg *backend.Registry, format string) string {
	kind := func(f string) string { k, _, _ := strings.Cut(f, "-"); return k }
	for _, be := range reg.All() {
		if be.ToolFormat() == format {
			return be.Name()
		}
	}
	for _, be := range reg.All() {
		if f := be.ToolFormat(); f != "" && kind(f) == kind(format) {
			return be.Name()
		}
	}
	return ""
}

// dataRows are the rest of devvm's data folder: exports, logs, devvm's
// own mise, its files written out, what's left of VMs gone from their
// backends, and its other state. Each file is counted once.
func (e spaceEnv) dataRows() ([]spaceRow, error) {
	d := e.b.Dirs
	es, err := exports(d)
	if err != nil {
		return nil, err
	}
	logs, err := filesIn(d.Logs(), func(string) bool { return true })
	if err != nil {
		return nil, err
	}
	runs, err := filesIn(d.RunLogs(), func(n string) bool { return strings.HasSuffix(n, ".log") })
	if err != nil {
		return nil, err
	}
	mise, err := dirsIn(d.Mise())
	if err != nil {
		return nil, err
	}
	assets, err := dirsIn(d.Assets())
	if err != nil {
		return nil, err
	}
	stale, err := e.staleAssets()
	if err != nil {
		return nil, err
	}
	oldMise, err := tools.Versions{Env: e.b.Tools}.StaleMise()
	if err != nil {
		return nil, err
	}
	left, err := e.leftovers()
	if err != nil {
		return nil, err
	}
	other := otherState(d) - left.Size
	exportsRow := spaceRow{
		What: "exports", Count: len(es), Size: exportsSize(es), Sized: true, Kept: keptOnlyCopy, Rest: "gc --exports",
		Retain: e.retainOf(state.RetainExportsAge, state.RetainExportsCount), RetainFrees: exportsSize(exportsToRetire(es, e.ret, e.now)),
	}
	runsRow := spaceRow{
		What: "run logs", Count: runs.count, Size: runs.size, Sized: true, Kept: keptLog,
		Rest: "the newest " + strconv.Itoa(cmp.Or(e.ret.RunLogs, 20)) + " stay", Retain: e.retainOf(state.RetainRunLogs),
		RetainFrees: oldestBeyond(d.RunLogs(), e.ret.RunLogs),
	}
	return nonEmpty(
		exportsRow,
		spaceRow{What: "network logs", Count: logs.count, Size: logs.size, Sized: true, Kept: keptLog, Rest: "devvm rm"},
		runsRow,
		spaceRow{What: "mise", Count: mise.count, Size: mise.size, Sized: true, Kept: keptRemade, GC: dirsSize(oldMise), Rest: "-"},
		spaceRow{What: "devvm's files", Count: assets.count, Size: assets.size, Sized: true, Kept: keptRemade, GC: stale, Rest: "-"},
		left,
		spaceRow{What: "state", Count: 1, Size: max(other, 0), Sized: true, Kept: keptState, Rest: "-"},
	), nil
}

// staleAssets is the disk devvm gc frees of the copies of devvm's files.
func (e spaceEnv) staleAssets() (int64, error) {
	current, err := state.AssetsHash(e.b.Assets)
	if err != nil {
		return 0, err
	}
	old, err := state.StaleAssets(e.b.Dirs, current, netpolicy.AssetsInUse(e.b.Dirs))
	if err != nil {
		return 0, err
	}
	var n int64
	for _, dir := range old {
		n += du(dir)
	}
	return n, nil
}

// leftovers is what devvm keeps for VMs whose backend answered but no
// longer has them: their state, run and backend files. Their logs are
// network logs'. A VM whose backend didn't answer is never one.
func (e spaceEnv) leftovers() (spaceRow, error) {
	row := spaceRow{What: "leftover state", Sized: true, Kept: keptLeftover, Rest: "gc --orphans", Retain: e.retainOf(state.RetainOrphansAge)}
	names, err := state.VMNames(e.b.Dirs)
	if err != nil {
		return row, err
	}
	for _, n := range names {
		st, err := state.ReadVM(e.b.Dirs, n)
		if err != nil {
			return row, err
		}
		have, asked := e.vms[st.Get(state.KeyBackend)]
		if !asked || have[n] {
			continue
		}
		row.Count++
		var n64 int64
		for _, f := range leftoverFiles(e.b.Dirs, n) {
			n64 += du(f)
		}
		row.Size += n64
		if fi, err := os.Stat(e.b.Dirs.VMState(n)); err == nil && e.ret.OrphansAge > 0 && e.now.Sub(fi.ModTime()) >= e.ret.OrphansAge {
			row.RetainFrees += n64
		}
	}
	return row, nil
}

// oldestBeyond is the disk the run logs in dir take beyond the newest
// keep, which the next run deletes; 0 when keep is 0 (off). Their names
// sort by when they were written.
func oldestBeyond(dir string, keep int) int64 {
	if keep == 0 {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var logs []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".log") {
			logs = append(logs, e.Name())
		}
	}
	var n int64
	for i := 0; i < len(logs)-keep; i++ {
		n += du(filepath.Join(dir, logs[i]))
	}
	return n
}

// retainOf is the retention settings of these keys that are on, as
// written: 30d, or 30d, 5 for two.
func (e spaceEnv) retainOf(keys ...string) string {
	var on []string
	for _, k := range keys {
		if v := e.ret.Raw[k]; v != "" {
			on = append(on, v)
		}
	}
	return strings.Join(on, ", ")
}

// leftoverFiles are the files devvm keeps for a VM besides its logs: its
// state, its run files and its backends' records.
func leftoverFiles(d paths.Dirs, vm string) []string {
	files := []string{d.VMState(vm)}
	for _, dir := range []string{d.Run(), d.LimaVMs(), filepath.Join(d.Smolvm(), "vms"), filepath.Join(d.Docker(), "vms")} {
		m, _ := filepath.Glob(filepath.Join(dir, vm+".*"))
		files = append(files, m...)
	}
	return files
}

// otherState is the disk devvm's other state takes: its VMs' and images'
// records, run files, CA, backend records and the tools lock.
func otherState(d paths.Dirs) int64 {
	var n int64
	for _, p := range []string{
		d.VMs(), d.Images(), d.Run(), d.CA(), d.LimaVMs(), d.Docker(), d.Smolvm(),
		d.ToolLock(), d.ToolLock() + ".toml", d.ToolLock() + ".checked",
	} {
		n += du(p)
	}
	return n - du(filepath.Join(d.Smolvm(), "images"))
}

// counted is how many and how much.
type counted struct {
	count int
	size  int64
}

// filesIn counts the plain files in dir that keep picks.
func filesIn(dir string, keep func(string) bool) (counted, error) {
	return entriesIn(dir, func(e fs.DirEntry) bool { return e.Type().IsRegular() && keep(e.Name()) })
}

// dirsIn counts the folders in dir, but those of work in progress
// (.tmp-…).
func dirsIn(dir string) (counted, error) {
	return entriesIn(dir, func(e fs.DirEntry) bool { return e.IsDir() && !strings.HasPrefix(e.Name(), ".") })
}

func entriesIn(dir string, keep func(fs.DirEntry) bool) (counted, error) {
	var c counted
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	} else if err != nil {
		return c, err
	}
	for _, e := range entries {
		if keep(e) {
			c.count++
			c.size += du(filepath.Join(dir, e.Name()))
		}
	}
	return c, nil
}

// du is the disk a path takes, as platform.DiskUsage counts it: 0 for
// one that's gone or can't be read.
func du(path string) int64 {
	n, err := platform.DiskUsage(path)
	if err != nil {
		return 0
	}
	return n
}

// writeSpace writes devvm space's tables to out: devvm's own with a
// total, then the backends' caches; a value one lacks is -.
func writeSpace(out io.Writer, r spaceReport) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	row := func(vals ...string) {
		for i, v := range vals {
			vals[i] = cmp.Or(v, "-")
		}
		_, _ = fmt.Fprintln(w, strings.Join(vals, "\t"))
	}
	size := func(n int64, sized bool) string {
		if !sized {
			return ""
		}
		return humanSize(n)
	}
	gc := func(n int64) string {
		if n == 0 {
			return ""
		}
		return humanSize(n)
	}
	row("WHAT", "BACKEND", "COUNT", "SIZE", "KEPT", "GC", "FREED BY", "RETAIN")
	for _, s := range r.Rows {
		row(s.What, s.Backend, strconv.Itoa(s.Count), size(s.Size, s.Sized), s.Kept, gc(s.GC), s.Rest, s.retainCell())
	}
	row("total", "", "", humanSize(r.Total), "", gc(r.GC), "", "")
	if err := w.Flush(); err != nil {
		return err
	}
	if len(r.Caches) == 0 {
		return nil
	}
	_, _ = fmt.Fprintln(out)
	row("BACKEND", "CACHE", "COUNT", "SIZE", "PATH")
	for _, c := range r.Caches {
		row(c.Backend, c.Name, strconv.Itoa(c.Count), size(c.Size, c.Sized), c.Path)
	}
	return w.Flush()
}
