package cli

import "testing"

func TestExecuteExitCodes(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want int
	}{
		{nil, 0},
		{[]string{"--help"}, 0},
		{[]string{"nope"}, 1},
		{[]string{"--nope"}, 1},
	} {
		if got := Execute(tc.args); got != tc.want {
			t.Errorf("Execute(%q) = %d, want %d", tc.args, got, tc.want)
		}
	}
}
