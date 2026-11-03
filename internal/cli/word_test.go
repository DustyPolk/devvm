package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var (
	// commandName is a command's name: lower-case words joined by -.
	commandName = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)
	// useArg is an argument as a Use shows it: a placeholder in capitals
	// or a word to type, alternatives joined by |, [optional], ...
	// repeated.
	useArg = regexp.MustCompile(`^\[?(?:[a-z][a-z-]*|[A-Z][A-Z_]*(?:=[A-Z][A-Z_]*)?)(?:\|(?:[a-z][a-z-]*|[A-Z][A-Z_]*))*(?:\.\.\.)?\]?$`)
)

// The commands users see follow the tree's naming rules: lower-case
// names, no aliases; a noun shows or lists bare, so no command is named
// for showing or listing but devvm ls, the one list of VMs; rm deletes;
// an unX undoes its sibling X; and a Use's arguments are placeholders or
// words, apart, never joined with + or a comma.
func TestCommandNames(t *testing.T) {
	t.Parallel()
	// names for what a bare noun or rm does
	banned := []string{"ls", "list", "show", "get", "delete", "remove"}
	u, _, _ := testUI()
	root := NewRoot(u)
	eachCommand(root, func(c *cobra.Command) {
		if c.Hidden || c == root {
			return
		}
		name, path := c.Name(), c.CommandPath()
		if !commandName.MatchString(name) || len(c.Aliases) > 0 {
			t.Errorf("%s: want a lower-case name without aliases, %q", path, c.Aliases)
		}
		if path != "devvm ls" && slices.Contains(banned, name) {
			t.Errorf("%s: a noun shows or lists bare, and rm deletes", path)
		}
		if undone, ok := strings.CutPrefix(name, "un"); ok {
			if sib, _, err := c.Parent().Find([]string{undone}); err != nil || sib.Name() != undone {
				t.Errorf("%s undoes %s, which isn't beside it", path, undone)
			}
		}
		for word := range strings.FieldsSeq(argsOf(c)) {
			if !useArg.MatchString(word) {
				t.Errorf("%s: Use's argument %q, want a PLACEHOLDER or word, [optional], ... repeated", path, word)
			}
		}
	})
}

// devvm WORD, devvm WORD --help and devvm help WORD, for a word that isn't
// a command, fail alike: a usage error suggesting the closest command,
// naming devvm --help, and nothing runs. An image's or a shared tool's
// name is no command either, nor is a layer of the user's: devvm takes
// no word, and a layer may have a command's name.
func TestUnknownWord(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(cfg, "devvm", "layers"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{"pg", "join"} {
		if err := os.WriteFile(filepath.Join(cfg, "devvm", "layers", l+".sh"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		args []string
		want string // the first line
	}{
		{[]string{"stoop"}, "no_command_like command=stop word=stoop"},
		{[]string{"stoop", "--help"}, "no_command_like command=stop word=stoop"},
		{[]string{"help", "stoop"}, "no_command_like command=stop word=stoop"},
		{[]string{"imagse"}, "no_command_like command=images word=imagse"},
		{[]string{"xyzzy"}, "no_command word=xyzzy"},
		{[]string{"node"}, "no_command_like command=code word=node"},
		{[]string{"claude"}, "no_command word=claude"},
		{[]string{"base"}, "no_command word=base"},
		{[]string{"docker"}, "no_command_like command=doctor word=docker"},
		{[]string{"pg"}, "no_command_like command=gc word=pg"},
		{[]string{"node+claude"}, "no_command word=node+claude"},
		{[]string{"help", "tools", "nope"}, "no_subcommand command=devvm tools commands=add, rm, update given=nope"},
	} {
		s := stubbed(nil)
		code := s.run(t, tc.args...)
		got, _, _ := strings.Cut(s.errOut.String(), "\n")
		if code != usageCode || len(s.ran) > 0 || got != "devvm: "+tc.want {
			t.Errorf("devvm %q: exit %d, ran %q, stderr %q; want %d and %q", tc.args, code, s.ran, s.errOut, usageCode, tc.want)
		}
	}
	// A layer named like a command is the command's word, and the layer's
	// name for --layers.
	s := stubbed(nil)
	if code := s.run(t, "--layers", "join"); code != 0 || !slices.Equal(s.ran, []string{"devvm"}) {
		t.Errorf("devvm --layers join: exit %d, ran %q, stderr %q", code, s.ran, s.errOut)
	}
}
