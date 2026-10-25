package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/ui"
)

// testUI returns a UI with no input, printing into out and errOut.
func testUI() (u *ui.UI, out, errOut *bytes.Buffer) {
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	return ui.New(strings.NewReader(""), out, errOut), out, errOut
}

// Only arguments that never reach up (a shell in this folder's VM): the
// CLI tests run up in their sandbox.
func TestExecuteExitCodes(t *testing.T) {
	t.Parallel()
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
		u, _, errOut := testUI()
		if got := Execute(t.Context(), u, tc.args); got != tc.want {
			t.Errorf("Execute(%q) = %d, want %d", tc.args, got, tc.want)
		}
		if failed := errOut.Len() > 0; failed != (tc.want != 0) {
			t.Errorf("Execute(%q) printed %q", tc.args, errOut)
		}
	}
}
