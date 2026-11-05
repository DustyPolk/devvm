package devcontainer

import "slices"

// Reread is what a VM made with a devcontainer.json gets of its
// features' toolchains and layers when the file is read again (devvm
// update): was is what the features mapped to when the file was last
// read (a Plan's Layers), gave those of them the VM got from the file
// alone, has the toolchains and layers the VM has, and now what the
// features map to now. It drops what the file gave that it no longer
// calls for (drop), adds what it calls for anew that the VM lacks (add),
// and leaves the rest as it is: what the user chose, added or took out,
// and what the file called for before. gaveNow is what the VM then has
// from the file alone; what it gave that the VM no longer has, taken out
// since, is neither dropped nor kept.
func Reread(was, gave, has, now []string) (add, drop, gaveNow []string) {
	for _, p := range gave {
		switch {
		case !slices.Contains(has, p):
		case !slices.Contains(now, p):
			drop = append(drop, p)
		default:
			gaveNow = append(gaveNow, p)
		}
	}
	for _, p := range now {
		if slices.Contains(was, p) || slices.Contains(has, p) || slices.Contains(add, p) {
			continue
		}
		add = append(add, p)
		gaveNow = append(gaveNow, p)
	}
	return add, drop, gaveNow
}
