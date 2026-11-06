//go:build e2e

package e2e

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// options are the run's flags, tests/e2e.sh's options by the same names,
// and those tests/e2e.sh passes when it hands its sections over.
type options struct {
	sel     []string // the chosen sections, in sections' order
	chosen  bool     // -e2e.only or -e2e.from chose them
	clean   bool     // delete the sandbox's cached images and tool images first
	every   bool     // base with every feature (the features tier)
	newest  bool     // base from Ubuntu's newest cloud image, in a sandbox of its own
	golden  bool     // also capture the golden fixtures
	reuse   bool     // keep the VMs Reuse launches between runs
	slots   int      // the sections spread over this many slots
	slot    int      // this run's slot
	handoff handoff  // tests/e2e.sh's run, when it handed these sections over
}

// handoff is what tests/e2e.sh passes when it runs these sections after
// its own: its pid, which holds the slot's lock, how many checks it ran
// and expects in all, when it started, and the file for the results it
// adds to its own.
type handoff struct {
	holder  int
	count   int
	total   string
	started int64
	summary string
}

// The flags' names: tests/e2e.sh's options with e2e. before them.
const (
	flagOnly    = "e2e.only"
	flagFrom    = "e2e.from"
	flagClean   = "e2e.clean"
	flagEvery   = "e2e.every-feature"
	flagNewest  = "e2e.newest-base"
	flagGolden  = "e2e.golden"
	flagReuse   = "e2e.reuse"
	flagSlots   = "e2e.slots"
	flagSlot    = "e2e.slot"
	flagHolder  = "e2e.lock-holder"
	flagCount   = "e2e.count"
	flagTotal   = "e2e.total"
	flagStarted = "e2e.started"
	flagSummary = "e2e.summary"
)

// registerFlags puts the run's flags on fs; readOptions reads them back.
func registerFlags(fs *flag.FlagSet) {
	fs.String(flagOnly, "", "just these sections, comma-separated")
	fs.Bool(flagFrom, false, "every section, with from, and base with every feature")
	fs.Bool(flagClean, false, "delete the sandbox's cached images and tool images first")
	fs.Bool(flagEvery, false, "base with every feature, as a run with from builds it")
	fs.Bool(flagNewest, false, "base from Ubuntu's newest cloud image and packages, in a sandbox of its own")
	fs.Bool(flagGolden, false, "also capture the golden fixtures the test stubs are checked against")
	fs.Bool(flagReuse, false, "keep the VMs a section reuses between runs, and reuse them")
	fs.Int(flagSlots, 1, "the chosen sections spread over this many slots (1-9), run at once")
	fs.Int(flagSlot, 0, "run in this slot's sandbox (0-9)")
	fs.Int(flagHolder, 0, "tests/e2e.sh's pid, which holds the slot's lock (tests/e2e.sh passes it)")
	fs.Int(flagCount, 0, "the checks tests/e2e.sh ran before these (tests/e2e.sh passes it)")
	fs.String(flagTotal, "", "the checks tests/e2e.sh expects in all (tests/e2e.sh passes it)")
	fs.Int64(flagStarted, 0, "when tests/e2e.sh's run started, in Unix seconds (tests/e2e.sh passes it)")
	fs.String(flagSummary, "", "where to write the results for tests/e2e.sh (tests/e2e.sh passes it)")
}

// readOptions reads the flags registerFlags put on fs and checks them as
// tests/e2e.sh checks its options. secs are every section, ported or not:
// one still in tests/e2e.sh can't be chosen here.
func readOptions(fs *flag.FlagSet, secs []section) (options, error) {
	get := func(name string) string { return fs.Lookup(name).Value.String() }
	on := func(name string) bool { return get(name) == "true" }
	num := func(name string) int { n, _ := strconv.Atoi(get(name)); return n }
	o := options{
		clean: on(flagClean), every: on(flagEvery), newest: on(flagNewest), golden: on(flagGolden),
		reuse: on(flagReuse), slots: num(flagSlots), slot: num(flagSlot),
		handoff: handoff{holder: num(flagHolder), count: num(flagCount), total: get(flagTotal), summary: get(flagSummary)},
	}
	o.handoff.started, _ = strconv.ParseInt(get(flagStarted), 10, 64)
	if o.slots < 1 || o.slots > 9 || o.slot < 0 || o.slot > 9 || (o.slots > 1 && o.slot != 0) {
		return o, errors.New("-e2e.slots is 1 to 9, -e2e.slot 0 to 9, and -e2e.slots runs from slot 0")
	}
	var err error
	o.sel, o.chosen, err = choose(get(flagOnly), on(flagFrom), o.newest, secs)
	if err != nil {
		return o, err
	}
	if o.newest {
		if o.slots != 1 || o.slot != 0 {
			return o, errors.New("-e2e.newest-base runs in a sandbox of its own, never a slot's")
		}
		o.clean = true
	}
	// A run with from builds base with every feature, which from checks, so
	// its other sections run on that base too.
	if on(flagFrom) || o.wants("from") {
		o.every = true
	}
	return o, nil
}

// choose is the sections a run's flags choose, in secs' order: -e2e.only's,
// every one with -e2e.from, images, project and tools with
// -e2e.newest-base, else every one but desktop and from. Of those, only
// the ones ported here run: one chosen by name that's still in
// tests/e2e.sh is refused.
func choose(only string, from, newest bool, secs []section) ([]string, bool, error) {
	var names, ported []string
	for _, s := range secs {
		names = append(names, s.name)
		if s.run != nil {
			ported = append(ported, s.name)
		}
	}
	var want []string
	chosen := true
	switch {
	case only != "":
		want = strings.Split(only, ",")
		for _, w := range want {
			if !slices.Contains(names, w) {
				return nil, false, fmt.Errorf("-e2e.only: no section %q (sections: %s)", w, strings.Join(names, " "))
			}
			if !slices.Contains(ported, w) {
				return nil, false, fmt.Errorf("-e2e.only: %s is still tests/e2e.sh's (tests/e2e.sh --only %s), and these are here: %s", w, w, strings.Join(ported, " "))
			}
		}
	case from:
		want = names
	case newest:
		want = []string{"images", "project", "tools"}
		chosen = false
	default:
		want = slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == "desktop" || n == "from" })
		chosen = false
	}
	var sel []string
	for _, n := range ported {
		if slices.Contains(want, n) {
			sel = append(sel, n)
		}
	}
	return sel, chosen, nil
}

// wants is whether the run chose the section.
func (o options) wants(name string) bool { return slices.Contains(o.sel, name) }

// tier is the timings' tier: features when base has every feature, and
// -newest for a newest base's run, which builds everything.
func (o options) tier() string {
	t := "plain"
	if o.every {
		t = "features"
	}
	if o.newest {
		t += "-newest"
	}
	return t
}

// handedOver is whether tests/e2e.sh runs these sections after its own.
func (o options) handedOver() bool { return o.handoff.holder != 0 }
