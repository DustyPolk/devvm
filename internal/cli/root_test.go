package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// testUI returns a UI with no input, printing into out and errOut.
// testUI is a UI on buffers, plain, its messages shown as their names
// (DEVVM_MESSAGES=names), whatever the test's environment says.
func testUI() (u *ui.UI, out, errOut *bytes.Buffer) {
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	return ui.New(strings.NewReader(""), out, errOut, namesEnv), out, errOut
}

func namesEnv(k string) string {
	if k == "DEVVM_MESSAGES" {
		return "names"
	}
	return ""
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
		{[]string{"--network", "on"}, usageCode},
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

// execute shows what a command's RunE returned, each kind of error as
// its own, and exits 1.
func TestExecuteShowsEachFailure(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir()) // the run log's folder
	m := msg.MsgUnknown("m")
	corrupt := &state.CorruptError{Path: "/x/vm.json", Reason: "bad: ", Msg: msg.MsgUnknown("v")}
	for _, tc := range []struct {
		name string
		err  func(u *ui.UI) error
		want string
	}{
		{"message", func(*ui.UI) error { return m }, "devvm: msg_unknown name=m\n"},
		{"failed message", func(*ui.UI) error { return failed(fmt.Errorf("x: %w", m), msg.LsFailed) }, "devvm: msg_unknown name=m\n"},
		{"failed error", func(*ui.UI) error { return failed(errors.New("boom"), msg.LsFailed) }, "devvm: ls_failed reason=boom\n"},
		{
			"failed corrupt file", func(*ui.UI) error { return failed(corrupt, msg.LsFailed) },
			"devvm: ls_failed reason=" + corrupt.Shown(msg.M.Names) + "\n",
		}, // its message as names mode shows it
		{"other error", func(*ui.UI) error { return errors.New("short write") }, "devvm: output_failed reason=short write\n"},
		{"logged", func(u *ui.UI) error {
			_ = u.LoggedCmd(t.Context(), msg.M{}, exec.CommandContext(t.Context(), "sh", "-c", "echo from-the-log; exit 1"))
			return failed(backend.Logged(m), msg.LsFailed)
		}, "from-the-log\n---\ndevvm: msg_unknown name=m\n"},
	} {
		u, out, errOut := testUI()
		root := &cobra.Command{Use: "devvm", SilenceErrors: true, SilenceUsage: true}
		root.AddCommand(&cobra.Command{Use: "x", Args: noArgs, RunE: func(*cobra.Command, []string) error { return tc.err(u) }})
		checkCommands(root)
		if code := execute(t.Context(), u, root, []string{"x"}); code != 1 {
			t.Errorf("%s: exit code %d, want 1", tc.name, code)
		}
		if got := errOut.String(); !strings.HasSuffix(got, tc.want) || out.Len() != 0 {
			t.Errorf("%s: stderr %q, stdout %q; want it to end %q", tc.name, got, out, tc.want)
		}
		if tc.name == "logged" && !strings.HasPrefix(errOut.String(), "--- log_end file=") {
			t.Errorf("logged: stderr %q, want the log's end first", errOut)
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
