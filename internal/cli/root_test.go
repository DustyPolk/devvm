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
		{[]string{"--nope"}, usageCode},
		{[]string{"a", "b"}, usageCode},
		{[]string{"build"}, usageCode},
		{[]string{"--network-policy", "--no-network-policy"}, usageCode},
		{[]string{"version", "x"}, usageCode},
		{[]string{"completion"}, usageCode},
		{[]string{"completion", "powershell"}, usageCode},
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
