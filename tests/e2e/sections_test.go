//go:build e2e

package e2e

// section is one of e2e's sections: its name, its time when it has never
// run (for -e2e.slots to place it), the host commands it uses beyond
// baseTools, and its checks. One whose run is nil is still in
// tests/e2e.sh, which runs it; the rest are here, each in a file of its
// own (gc_test.go), and tests/e2e.sh hands them to TestE2E.
type section struct {
	name  string
	guess int // seconds
	tools []tool
	run   func(*S)
}

// sections is every section, in the order a run runs them: tests/e2e.sh's.
func sections() []section {
	return []section{
		{name: "images", guess: 600},
		{name: "project", guess: 900},
		{name: "clone", guess: 120},
		{name: "gc", guess: 120, run: gcSection},
		{name: "tools", guess: 600},
		{name: "network", guess: 600},
		{name: "desktop", guess: 300},
		{name: "nix", guess: 900},
		{name: "from", guess: 1800},
		{name: "link", guess: 900},
	}
}
