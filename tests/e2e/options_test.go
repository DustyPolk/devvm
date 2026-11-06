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

// options are the run's flags.
type options struct {
	sel    []string // the chosen sections, in sections' order
	chosen bool     // --only or --from chose them
	clean  bool     // delete the sandbox's cached images and tool images first
	every  bool     // base with every feature (the features tier)
	newest bool     // base from Ubuntu's newest cloud image, in a sandbox of its own
	golden bool     // also capture the golden fixtures
	reuse  bool     // keep the VMs Reuse launches between runs
	slots  int      // the sections spread over this many slots
	slot   int      // this run's slot
}

// The flags' names, given after go test's -args as --only images and so on.
const (
	flagOnly   = "only"
	flagFrom   = "from"
	flagClean  = "clean"
	flagEvery  = "every-feature"
	flagNewest = "newest-base"
	flagGolden = "golden"
	flagReuse  = "reuse"
	flagSlots  = "slots"
	flagSlot   = "slot"
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
}

// readOptions reads the flags registerFlags put on fs and checks them.
// secs are every section.
func readOptions(fs *flag.FlagSet, secs []section) (options, error) {
	get := func(name string) string { return fs.Lookup(name).Value.String() }
	on := func(name string) bool { return get(name) == "true" }
	num := func(name string) int { n, _ := strconv.Atoi(get(name)); return n }
	o := options{
		clean: on(flagClean), every: on(flagEvery), newest: on(flagNewest), golden: on(flagGolden),
		reuse: on(flagReuse), slots: num(flagSlots), slot: num(flagSlot),
	}
	if o.slots < 1 || o.slots > 9 || o.slot < 0 || o.slot > 9 || (o.slots > 1 && o.slot != 0) {
		return o, errors.New("--slots is 1 to 9, --slot 0 to 9, and --slots runs from slot 0")
	}
	var err error
	o.sel, o.chosen, err = choose(get(flagOnly), on(flagFrom), o.newest, secs)
	if err != nil {
		return o, err
	}
	if o.newest {
		if o.slots != 1 || o.slot != 0 {
			return o, errors.New("--newest-base runs in a sandbox of its own, never a slot's")
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

// choose is the sections a run's flags choose, in secs' order: --only's,
// every one with --from, images, project and tools with --newest-base,
// else every one but desktop and from.
func choose(only string, from, newest bool, secs []section) ([]string, bool, error) {
	var names []string
	for _, s := range secs {
		names = append(names, s.name)
	}
	var want []string
	chosen := true
	switch {
	case only != "":
		want = strings.Split(only, ",")
		for _, w := range want {
			if !slices.Contains(names, w) {
				return nil, false, fmt.Errorf("--only: no section %q (sections: %s)", w, strings.Join(names, " "))
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
	for _, n := range names {
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
