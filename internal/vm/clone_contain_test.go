package vm

import "testing"

// worktreePath takes the VM's path only clean, relative and below the
// project folder, never the folder itself.
func TestWorktreePathPins(t *testing.T) {
	for _, tc := range []struct {
		p    string
		want string
	}{
		{"a", "/w/c/a"},
		{"a/b", "/w/c/a/b"},
		{".worktrees/x/api", "/w/c/.worktrees/x/api"},
		{".", ""},
		{"", ""},
		{"a/", ""},
		{"a/../b", ""},
		{"../x", ""},
		{"..", ""},
		{"/w/c/a", ""},
		{"./a", ""},
	} {
		got, ok := worktreePath("/w/c", tc.p)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("worktreePath(%q) = %q, %v", tc.p, got, ok)
		}
	}
}
