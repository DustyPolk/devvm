package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
)

func TestExamples(t *testing.T) {
	got := examples("devvm", "open the VM", "devvm ls --json", "list them")
	want := "  devvm            # open the VM\n  devvm ls --json  # list them"
	if got != want {
		t.Errorf("examples:\n%s\nwant:\n%s", got, want)
	}
}

// The root's help lists its commands in commandGroups' sections, in
// order, and help in Cobra's own; each help ends with its docs page, and
// the root's with the issue tracker after it.
func TestHelpGroupsAndDocs(t *testing.T) {
	t.Parallel()
	u, out, _ := testUI()
	if code := Execute(t.Context(), u, []string{"--help"}); code != 0 {
		t.Fatalf("devvm --help = %d", code)
	}
	help := out.String()
	at := 0
	for _, title := range []string{"Your VM:", "Change your VM:", "Images:", "Disk space:", "This machine:", "Additional Commands:\n  help "} {
		i := strings.Index(help[at:], "\n"+title)
		if i < 0 {
			t.Fatalf("devvm --help has no %q after byte %d:\n%s", title, at, help)
		}
		at += i + 1
	}
	if strings.Contains(help, "Links between VMs:") {
		t.Error("devvm --help shows a group none of whose commands exist")
	}
	wantEnd := "Docs:\n  " + msg.CommandDocURL("devvm") + "\n\nIssues:\n  " + msg.IssuesURL + "\n"
	if !strings.HasSuffix(help, wantEnd) {
		t.Errorf("devvm --help ends:\n%s\nwant it to end with:\n%s", help[max(0, len(help)-200):], wantEnd)
	}

	u, out, _ = testUI()
	if code := Execute(t.Context(), u, []string{"network", "allow", "--help"}); code != 0 {
		t.Fatalf("devvm network allow --help = %d", code)
	}
	wantEnd = "Docs:\n  " + msg.CommandDocURL("devvm network allow") + "\n"
	if got := out.String(); !strings.HasSuffix(got, wantEnd) || strings.Contains(got, "Issues:") {
		t.Errorf("devvm network allow --help:\n%s\nwant it to end with:\n%s", got, wantEnd)
	}
}

func TestAddGroups(t *testing.T) {
	root := &cobra.Command{Use: "devvm"}
	stop, zap := &cobra.Command{Use: "stop"}, &cobra.Command{Use: "zap"}
	root.AddCommand(stop, zap)
	addGroups(root)
	if stop.GroupID != "vm" || zap.GroupID != "" {
		t.Errorf("GroupIDs = %q, %q, want vm and none", stop.GroupID, zap.GroupID)
	}
	var ids []string
	for _, g := range root.Groups() {
		ids = append(ids, g.ID)
	}
	if strings.Join(ids, " ") != "vm" {
		t.Errorf("groups = %v, want only vm, the one with commands", ids)
	}
}
