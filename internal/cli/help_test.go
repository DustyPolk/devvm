package cli

import (
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/msg"
)

// sentenceEnd ends a sentence: a full stop, question or exclamation mark
// before a space or the end, as in prose, so devvm.dev or 1.5 isn't one.
var sentenceEnd = regexp.MustCompile(`[.?!](\s|$)`)

// Every command users see has help of the same shape: a Short in sentence
// case without a full stop, a Long of one to three sentences, Examples and
// its docs page, and each of the root's is in a group.
func TestEveryCommandsHelp(t *testing.T) {
	t.Parallel()
	// devvm help is every Cobra command line's help command: Cobra lists
	// it apart, in Additional Commands, and the CLI reference leaves it
	// out, so it has no examples, docs page or group.
	const excepted = "devvm help"
	u, _, _ := testUI()
	root := NewRoot(u)
	root.InitDefaultHelpCmd()
	eachCommand(root, func(c *cobra.Command) {
		path := c.CommandPath()
		if c.Hidden {
			return
		}
		if s := c.Short; s == "" || !unicode.IsUpper([]rune(s)[0]) || strings.HasSuffix(s, ".") {
			t.Errorf("%s: Short %q, want an action in sentence case without a full stop", path, s)
		}
		if n := len(sentenceEnd.FindAllString(c.Long, -1)); n < 1 || n > 3 || !strings.HasSuffix(c.Long, ".") {
			t.Errorf("%s: Long has %d sentences, want one to three, the last ending with a full stop: %q", path, n, c.Long)
		}
		if path == excepted {
			return
		}
		if c.Example == "" {
			t.Errorf("%s: no Example: add one with examples()", path)
		}
		if c.Annotations["docs"] == "" {
			t.Errorf("%s: no docs page: add its path to commandDocs in internal/msg/docs.go", path)
		}
		if c.Parent() == root && c.GroupID == "" {
			t.Errorf("%s: in no group: add it to commandGroups", path)
		}
	})
}

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
	for _, title := range []string{"Your VM:", "Change your VM:", "Links between VMs:", "Images:", "Disk space:", "This machine:", "Additional Commands:\n  help "} {
		i := strings.Index(help[at:], "\n"+title)
		if i < 0 {
			t.Fatalf("devvm --help has no %q after byte %d:\n%s", title, at, help)
		}
		at += i + 1
	}
	// layer add and layer rm change the VM, while layer stays in Images.
	change := help[strings.Index(help, "\nChange your VM:"):strings.Index(help, "\nLinks between VMs:")]
	images := help[strings.Index(help, "\nImages:"):strings.Index(help, "\nDisk space:")]
	if !strings.Contains(change, "\n  layer add ") || !strings.Contains(change, "\n  layer rm ") ||
		!strings.Contains(images, "\n  layer ") || strings.Contains(images, "layer new") {
		t.Errorf("devvm --help's Change your VM:%s\nand Images:%s", change, images)
	}
	wantEnd := "Docs:\n  " + msg.CommandDocURL("devvm") + "\n\nIssues:\n  " + msg.IssuesURL + "\n"
	if !strings.HasSuffix(help, wantEnd) {
		t.Errorf("devvm --help ends:\n%s\nwant it to end with:\n%s", help[max(0, len(help)-200):], wantEnd)
	}

	u, out, _ = testUI()
	if code := Execute(t.Context(), u, []string{"layer", "--help"}); code != 0 {
		t.Fatalf("devvm layer --help = %d", code)
	}
	if got := out.String(); !regexp.MustCompile(`\nChange your VM:\n  add +\S.*\n  rm +\S.*\n\nImages:\n  new +\S`).MatchString(got) {
		t.Errorf("devvm layer --help:\n%s\nwant add and rm in Change your VM, new in Images", got)
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
