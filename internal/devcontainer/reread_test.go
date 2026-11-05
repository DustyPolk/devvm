package devcontainer

import (
	"slices"
	"testing"
)

// Reread replaces what the file gave and nothing else: the user's parts
// stay, what the user took out of the file's doesn't come back, and only
// what the file calls for anew is added.
func TestReread(t *testing.T) {
	for _, c := range []struct {
		name                string
		was, gave, has, now []string
		add, drop, gaveNow  []string
	}{
		{
			name: "unchanged", was: []string{"docker", "node"}, gave: []string{"docker", "node"},
			has: []string{"docker", "node"}, now: []string{"docker", "node"},
			gaveNow: []string{"docker", "node"},
		},
		{
			name: "the file's replaced", was: []string{"docker", "node"}, gave: []string{"docker", "node"},
			has: []string{"docker", "node", "claude"}, now: []string{"go"},
			add: []string{"go"}, drop: []string{"docker", "node"}, gaveNow: []string{"go"},
		},
		// docker was the user's (--layers), so the file dropping it keeps it.
		{name: "the user's kept", was: []string{"docker"}, has: []string{"docker", "extra"}, now: nil},
		// node was unchecked when the VM was made: still called for, it
		// stays out; rust is new.
		{
			name: "an unchecked one stays out", was: []string{"docker", "node"}, gave: []string{"docker"},
			has: []string{"docker"}, now: []string{"docker", "node", "rust"},
			add: []string{"rust"}, gaveNow: []string{"docker", "rust"},
		},
		// The VM has go already, the user's: it stays the user's.
		{
			name: "new and had", has: []string{"go"}, now: []string{"go", "docker"},
			add: []string{"docker"}, gaveNow: []string{"docker"},
		},
		// python was the file's, then taken out with devvm tools rm.
		{name: "taken out since", was: []string{"python"}, gave: []string{"python"}, now: nil},
		{name: "a new file", now: []string{"docker", "docker"}, add: []string{"docker"}, gaveNow: []string{"docker"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			add, drop, gaveNow := Reread(c.was, c.gave, c.has, c.now)
			if !slices.Equal(add, c.add) || !slices.Equal(drop, c.drop) || !slices.Equal(gaveNow, c.gaveNow) {
				t.Errorf("Reread = add %q, drop %q, gave %q; want %q, %q, %q", add, drop, gaveNow, c.add, c.drop, c.gaveNow)
			}
		})
	}
}
