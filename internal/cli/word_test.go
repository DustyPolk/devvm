package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/image"
)

// Every command's name, hidden or not, and help's, is a word devvm keeps:
// no layer can take one.
func TestEveryCommandReserved(t *testing.T) {
	t.Parallel()
	u, _, _ := testUI()
	names := []string{"help"}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			names = append(names, sub.Name())
			walk(sub)
		}
	}
	walk(NewRoot(u))
	for _, n := range names {
		if !image.IsReserved(n) {
			t.Errorf("command %s isn't in image.Reserved", n)
		}
	}
}

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

// Base, the shared tools and the built-in layers are images, so none is
// a word devvm keeps for its commands.
func TestImagesNotReserved(t *testing.T) {
	t.Parallel()
	for _, n := range slices.Concat([]string{image.Base}, image.Toolchains, image.Agents, image.Builtins) {
		if image.IsReserved(n) {
			t.Errorf("%s is an image's name and a reserved word", n)
		}
	}
}

func TestLevenshtein(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"stop", "stop", 0}, {"sotp", "stop", 2}, {"nodee", "node", 1}, {"", "ab", 2}, {"kitten", "sitting", 3},
	} {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
	if got := closest("Pyhton", []string{"node", "python"}); got != "python" {
		t.Errorf("closest(Pyhton) = %q, want python", got)
	}
	if got := closest("zzz", []string{"node", "python"}); got != "" {
		t.Errorf("closest(zzz) = %q, want none", got)
	}
}

// devvm WORD, devvm WORD --help and devvm help WORD, for a word that's
// neither a command nor an image, fail alike: a usage error suggesting
// the closest command or image, naming devvm --help, and nothing runs.
// An image's name, joined with + or not, goes on to up.
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
		want string // the first line; "" for up running
	}{
		{[]string{"stoop"}, "no_command_or_image_command command=stop word=stoop"},
		{[]string{"stoop", "--help"}, "no_command_or_image_command command=stop word=stoop"},
		{[]string{"help", "stoop"}, "no_command_or_image_command command=stop word=stoop"},
		{[]string{"imagse"}, "no_command_or_image_command command=images word=imagse"},
		{[]string{"pyton"}, "no_command_or_image_image image=python word=pyton"},
		{[]string{"pq"}, "no_command_or_image_both command=gc image=pg word=pq"},
		{[]string{"xyzzy"}, "no_command_or_image word=xyzzy"},
		{[]string{"join"}, "layer_is_command file=" + filepath.Join(cfg, "devvm", "layers", "join.sh") + " layer=join"},
		{[]string{"help", "tools", "nope"}, "no_subcommand command=devvm tools commands=add, rm, update given=nope"},
		{[]string{"pg"}, ""},
		{[]string{"node+sotp"}, ""},
		{[]string{"claude"}, ""},
	} {
		s := stubbed(nil)
		code := s.run(t, tc.args...)
		got, _, _ := strings.Cut(s.errOut.String(), "\n")
		if tc.want == "" {
			if code != 0 || !slices.Equal(s.ran, []string{"devvm"}) {
				t.Errorf("devvm %q: exit %d, ran %q, stderr %q; want up run", tc.args, code, s.ran, s.errOut)
			}
			continue
		}
		if code != usageCode || len(s.ran) > 0 || got != "devvm: "+tc.want {
			t.Errorf("devvm %q: exit %d, ran %q, stderr %q; want %d and %q", tc.args, code, s.ran, s.errOut, usageCode, tc.want)
		}
	}
}
