package vm

import "testing"

// insideClone takes the VM's path only clean, absolute and below the
// clone, never the clone itself.
func TestInsideClonePins(t *testing.T) {
	for _, tc := range []struct {
		p    string
		want bool
	}{
		{"/w/c/a", true},
		{"/w/c/a/b", true},
		{"/w/c", false},
		{"/w/c/", false},
		{"/w/c/a/", false},
		{"/w/c/a/../b", false},
		{"/w/c/../x", false},
		{"/w/c-sibling/a", false},
		{"w/c/a", false},
		{"", false},
	} {
		if got := insideClone("/w/c", tc.p); got != tc.want {
			t.Errorf("insideClone(%q) = %v", tc.p, got)
		}
	}
}
