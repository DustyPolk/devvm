//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// plan is --slots' placing of the chosen sections: longest first, as
// their last complete runs measured them (else their guess), each in the
// slot with the least time so far, the lowest on a tie; link goes with
// from when both run, as link's Alpine client in the features tier starts
// from from's image. Each slot's sections are in the run's order, and a
// slot without one is left out.
func plan(o options, tdir string, all []section) map[int][]string {
	type unit struct {
		secs  time.Duration
		names []string
	}
	weight := func(s section) time.Duration {
		if t := sumOf(readTimings(filepath.Join(tdir, s.name))); t > 0 {
			return t
		}
		return time.Duration(s.guess) * time.Second
	}
	var units []unit
	for _, s := range all {
		switch {
		case !o.wants(s.name), s.name == "link" && o.wants("from"):
		case s.name == "from" && o.wants("link"):
			i := slices.IndexFunc(all, func(l section) bool { return l.name == "link" })
			units = append(units, unit{weight(s) + weight(all[i]), []string{"from", "link"}})
		default:
			units = append(units, unit{weight(s), []string{s.name}})
		}
	}
	sort.SliceStable(units, func(i, j int) bool { return units[i].secs > units[j].secs })
	load := make([]time.Duration, o.slots)
	mine := make([][]string, o.slots)
	for _, u := range units {
		best := 0
		for k := 1; k < o.slots; k++ {
			if load[k] < load[best] {
				best = k
			}
		}
		load[best] += u.secs
		mine[best] = append(mine[best], u.names...)
	}
	out := map[int][]string{}
	for k, names := range mine {
		for _, s := range all {
			if slices.Contains(names, s.name) {
				out[k] = append(out[k], s.name)
			}
		}
	}
	return out
}

// runSlots is the --slots run: every slot's lock taken while each
// other slot gets what slot 0 has and it lacks (seed; with --clean,
// each slot deletes its own instead), then each slot a run of this test
// binary of its own, all at once. Their lines show as they come, each led
// by its slot ("[1] "), and stay in the slot's e2e.log; their results are
// added up at the end. A signal stops every slot with TERM.
func runSlots(ctx context.Context, out *output, o options) (bool, error) {
	started := time.Now()
	p := plan(o, filepath.Join(slotWork(0), "timings", o.tier()), sections())
	if len(p) == 0 {
		return false, errors.New("no section chosen")
	}
	for k := range o.slots {
		if err := os.MkdirAll(slotWork(k), 0o700); err != nil {
			return false, err
		}
		if err := takeLock(slotWork(k) + ".lock"); err != nil {
			return false, err
		}
		defer dropLock(slotWork(k) + ".lock")
	}
	if !o.clean {
		for k := 1; k < o.slots; k++ {
			if len(p[k]) > 0 {
				seed(ctx, out, k)
			}
		}
	}
	for k := range o.slots {
		dropLock(slotWork(k) + ".lock")
	}
	slots := make([]*slotRun, 0, len(p))
	for k := range o.slots {
		if len(p[k]) == 0 {
			continue
		}
		sr, err := startSlot(ctx, k, p[k], o)
		if err != nil {
			return false, err
		}
		out.line("e2e: slot %d runs %s", k, strings.Join(p[k], ", "))
		slots = append(slots, sr)
	}
	follow(out, slots)
	return summarize(out, slots, started), nil
}

// slotRun is one slot's run: its sections, its process and log.
type slotRun struct {
	k     int
	names []string
	cmd   *exec.Cmd
	log   string
	seen  int
	done  chan struct{}
	err   error
}

// startSlot starts slot k's run of names: this test binary again, in its
// own process group (so Ctrl+C reaches only this run, which stops it),
// its output into the slot's e2e.log.
func startSlot(ctx context.Context, k int, names []string, o options) (*slotRun, error) {
	args := []string{"-test.run=^TestE2E$", "-test.timeout=0", "-" + flagSlot + "=" + strconv.Itoa(k), "-" + flagOnly + "=" + strings.Join(names, ",")}
	for _, f := range []struct {
		on   bool
		name string
	}{{o.clean, flagClean}, {o.every, flagEvery}, {o.golden, flagGolden}, {o.reuse, flagReuse}} {
		if f.on {
			args = append(args, "-"+f.name)
		}
	}
	sr := &slotRun{k: k, names: names, log: filepath.Join(slotWork(k), "e2e.log"), done: make(chan struct{})}
	_ = os.Remove(filepath.Join(slotWork(k), "summary"))
	f, err := os.OpenFile(sr.log, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	// A signal (ctx) sends it TERM, once, and its own cleanup runs.
	sr.cmd = exec.CommandContext(ctx, os.Args[0], args...)
	sr.cmd.Cancel = func() error { return sr.cmd.Process.Signal(syscall.SIGTERM) }
	sr.cmd.Stdout, sr.cmd.Stderr, sr.cmd.Env = f, f, goOnPath(ctx, os.Environ())
	sr.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := sr.cmd.Start(); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	go func() {
		sr.err = errors.Join(sr.cmd.Wait(), f.Close())
		close(sr.done)
	}()
	return sr, nil
}

// goOnPath is env with Go's folder first on PATH when go isn't on it but
// mise has it: slots that each ran mise exec at once would each have mise
// install the repo's tools, over one another.
func goOnPath(ctx context.Context, env []string) []string {
	if _, err := exec.LookPath("go"); err == nil {
		return env
	}
	b, err := exec.CommandContext(ctx, "mise", "which", "go").Output()
	if err != nil {
		return env
	}
	return append(env, "PATH="+filepath.Dir(strings.TrimSpace(string(b)))+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// follow shows each slot's new whole lines every second until every slot
// has ended, then the rest.
func follow(out *output, slots []*slotRun) {
	for {
		alive := false
		for _, sr := range slots {
			select {
			case <-sr.done:
			default:
				alive = true
			}
			sr.show(out, false)
		}
		if !alive {
			break
		}
		time.Sleep(time.Second)
	}
	for _, sr := range slots {
		sr.show(out, true)
	}
}

// show prints the slot's lines it hasn't yet, whole ones (all: the rest
// too, once it has ended), each led by its slot.
func (sr *slotRun) show(out *output, all bool) {
	b, err := os.ReadFile(sr.log)
	if err != nil {
		return
	}
	text := string(b[min(sr.seen, len(b)):])
	if !all {
		i := strings.LastIndexByte(text, '\n')
		if i < 0 {
			return
		}
		text = text[:i+1]
	}
	sr.seen += len(text)
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		out.line("[%d] %s", sr.k, sc.Text())
	}
}

// summarize adds up the slots' results: each slot's line, then the sums,
// the failed checks by name and the time the whole run took. It's
// whether every slot passed.
func summarize(out *output, slots []*slotRun, started time.Time) bool {
	ok := true
	var pass, fail, skipped int
	var failed, retried, short []string
	out.line("")
	for _, sr := range slots {
		f := filepath.Join(slotWork(sr.k), "summary")
		b, err := os.ReadFile(f)
		if err != nil {
			ok = false
			out.line("e2e: slot %d (%s) ended without its results (%v): look at its lines above, or %s", sr.k, strings.Join(sr.names, ", "), sr.err, sr.log)
			continue
		}
		var p, r, sk, n int
		for line := range strings.Lines(string(b)) {
			key, val, _ := strings.Cut(strings.TrimRight(line, "\n"), "\t")
			i, _ := strconv.Atoi(val)
			switch key {
			case "pass":
				p = i
			case "fail":
				r = i
			case "skipped":
				sk = i
			case "elapsed":
				n = i
			case "failed":
				failed = append(failed, val)
			case "retried":
				retried = append(retried, val)
			case "short":
				short = append(short, fmt.Sprintf("slot %d: %s", sr.k, val))
			}
		}
		pass, fail, skipped = pass+p, fail+r, skipped+sk
		ok = ok && sr.err == nil
		out.line("e2e: slot %d (%s): %s in %s", sr.k, strings.Join(sr.names, ", "), counts(p, r, sk, 0), mins(time.Duration(n)*time.Second))
	}
	out.line("e2e: %s in %s on %d slots%s", counts(pass, fail, skipped, len(retried)), mins(time.Since(started)), len(slots), failedList(failed))
	for _, l := range retried {
		out.line("e2e: retried once: %s", l)
	}
	for _, l := range short {
		out.line("e2e: FAILED: %s", l)
	}
	return ok && fail == 0 && len(short) == 0
}

// seed gives slot k what slot 0 has and it lacks, cloned (cloneTree),
// never copied whole: each image's Lima instance (then its record, small,
// copied), each tool image (then its record), devvm's caches (the --from
// digests, small, copied) and base's package cache. What can't be cloned
// is said, and slot k builds its own.
func seed(ctx context.Context, out *output, k int) {
	l0, lk := slotLima(0), slotLima(k)
	d0, dk := filepath.Join(slotWork(0), "data", "devvm"), filepath.Join(slotWork(k), "data", "devvm")
	_ = os.MkdirAll(lk, 0o700)
	for _, d := range []string{dk, filepath.Join(dk, "images"), filepath.Join(dk, "tools"), filepath.Join(dk, "cache")} {
		_ = os.MkdirAll(d, 0o700)
	}
	var cold []string
	images, tools := 0, 0
	recs, _ := filepath.Glob(filepath.Join(d0, "images", "*", "record.json"))
	for _, r := range recs {
		key := filepath.Base(filepath.Dir(r))
		inst := "devvm-img-" + key
		if !Exists(filepath.Join(l0, inst)) || Exists(filepath.Join(lk, inst)) || Exists(filepath.Join(dk, "images", key)) {
			continue
		}
		if cloneTree(ctx, filepath.Join(l0, inst), filepath.Join(lk, inst)) != nil {
			cold = append(cold, "images")
			break
		}
		if copyTree(ctx, filepath.Join(d0, "images", key), filepath.Join(dk, "images", key)) == nil {
			images++
		}
	}
	ts, _ := filepath.Glob(filepath.Join(d0, "tools", "*"))
	for _, f := range ts {
		dst := filepath.Join(dk, "tools", filepath.Base(f))
		if st, err := os.Stat(f); err != nil || !st.Mode().IsRegular() || !Exists(f+".json") || Exists(dst) {
			continue
		}
		if cloneTree(ctx, f, dst) != nil {
			cold = append(cold, "tool-images")
			break
		}
		if copyTree(ctx, f+".json", dst+".json") == nil {
			tools++
		}
	}
	if f := filepath.Join(d0, "cache", "from-digests.json"); Exists(f) && !Exists(filepath.Join(dk, "cache", "from-digests.json")) {
		_ = copyTree(ctx, f, filepath.Join(dk, "cache", "from-digests.json"))
	}
	caches, _ := filepath.Glob(filepath.Join(d0, "cache", "*"))
	for _, c := range caches {
		dst := filepath.Join(dk, "cache", filepath.Base(c))
		if st, err := os.Stat(c); err != nil || !st.IsDir() || Exists(dst) {
			continue
		}
		if cloneTree(ctx, c, dst) != nil {
			cold = append(cold, "caches")
			break
		}
	}
	if seedPackages(ctx, filepath.Join(d0, "packages"), filepath.Join(dk, "packages")) != nil {
		cold = append(cold, "packages")
	}
	out.line("e2e: slot %d: %d images and %d tool images cloned from slot 0", k, images, tools)
	if len(cold) > 0 {
		out.line("e2e: slot %d: these can't be cloned here, so it builds its own: %s", k, strings.Join(cold, " "))
	}
}

// seedPackages gives dst each <sha256>.deb of base's package cache src
// has and it lacks, cloned, and copies its sets/ (small files: the
// packages each base key recorded). It fails at the first .deb that can't
// be cloned.
func seedPackages(ctx context.Context, src, dst string) error {
	if !Exists(src) {
		return nil
	}
	for _, d := range []string{dst, filepath.Join(dst, "sets")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	debs, _ := filepath.Glob(filepath.Join(src, "*.deb"))
	for _, f := range debs {
		if to := filepath.Join(dst, filepath.Base(f)); !Exists(to) {
			if err := cloneTree(ctx, f, to); err != nil {
				return err
			}
		}
	}
	sets, _ := filepath.Glob(filepath.Join(src, "sets", "*.json"))
	for _, f := range sets {
		if to := filepath.Join(dst, "sets", filepath.Base(f)); !Exists(to) {
			_ = copyTree(ctx, f, to)
		}
	}
	return nil
}

// copyTree copies a small file or folder whole, its modes and times kept.
func copyTree(ctx context.Context, src, dst string) error {
	return exec.CommandContext(ctx, "cp", "-R", "-p", src, dst).Run()
}

// takeLock takes the lock f, a symlink to the pid of the run holding it,
// one run at a time in a slot. A lock whose process
// has ended is taken over.
func takeLock(f string) error {
	me := strconv.Itoa(os.Getpid())
	if os.Symlink(me, f) == nil {
		return nil
	}
	if h, err := os.Readlink(f); err == nil {
		if pid, _ := strconv.Atoi(h); pid > 0 && syscall.Kill(pid, 0) == nil {
			return fmt.Errorf("%s is held by process %d, another run in that slot", f, pid)
		}
	}
	_ = os.Remove(f)
	if err := os.Symlink(me, f); err != nil {
		return fmt.Errorf("couldn't take %s: %w", f, err)
	}
	return nil
}

// dropLock lets the lock f go, when it's this run's.
func dropLock(f string) {
	if h, _ := os.Readlink(f); h == strconv.Itoa(os.Getpid()) {
		_ = os.Remove(f)
	}
}
