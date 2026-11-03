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

// The layout check runs before every command that reads devvm's folders,
// and retention after it; what reads none, and devvm's own hidden
// commands, skip both.
func TestSkipsLayout(t *testing.T) {
	u, _, _ := testUI()
	root := NewRoot(u)
	root.InitDefaultHelpCmd() // as Execute does
	for _, tc := range []struct {
		args              []string
		layout, retention bool // skipped
	}{
		{nil, false, false},
		{[]string{"ls"}, false, false},
		{[]string{"doctor"}, false, false},
		{[]string{"network", "allow"}, false, false},
		{[]string{"gc"}, false, true},
		{[]string{"space"}, false, true},
		{[]string{"retention"}, false, true},
		{[]string{"version"}, true, true},
		{[]string{"completion", "bash"}, true, true},
		{[]string{"help"}, true, true},
		{[]string{"_proxy"}, true, true},
		{[]string{"_msg"}, true, true},
	} {
		cmd, _, err := root.Find(tc.args)
		if err != nil {
			t.Fatalf("%q: %v", tc.args, err)
		}
		if got := skipsLayout(cmd); got != tc.layout {
			t.Errorf("skipsLayout(%q) = %v", tc.args, got)
		}
		if got := skipsRetention(cmd); got != tc.retention {
			t.Errorf("skipsRetention(%q) = %v", tc.args, got)
		}
	}
}
