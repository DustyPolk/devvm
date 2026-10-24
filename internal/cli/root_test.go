package cli

import "testing"

// Only arguments that never reach up (a shell in this folder's VM): the
// CLI tests run up in their sandbox.
func TestExecuteExitCodes(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"--help"}, 0},
		{[]string{"--nope"}, 1},
		{[]string{"a", "b"}, 1},
		{[]string{"build"}, 1},
		{[]string{"--network-policy", "--no-network-policy"}, 1},
		{[]string{"version", "x"}, 1},
		{[]string{"completion"}, 1},
		{[]string{"completion", "powershell"}, 1},
	} {
		if got := Execute(t.Context(), tc.args); got != tc.want {
			t.Errorf("Execute(%q) = %d, want %d", tc.args, got, tc.want)
		}
	}
}
