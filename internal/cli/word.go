package cli

import (
	"errors"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
)

// suggestDistance is how far, in Levenshtein distance, a word can be from
// a command's or an image's name for devvm to suggest it.
const suggestDistance = 2

// checkWord refuses devvm WORD when WORD is neither a command (which the
// parser would have run) nor an image: no +, and not base, a shared tool
// or a layer. It suggests the closest command and the closest image name
// when one is close. A layer of the user's named with a word devvm keeps
// for its commands is refused, naming its file. Image names with + are
// left to the VM's launch, which checks each part.
func checkWord(root *cobra.Command, word string) error {
	if strings.Contains(word, "+") {
		return nil
	}
	dirs, err := paths.FromEnv()
	if err != nil {
		return nil //nolint:nilerr // without its folders, devvm says why once it runs
	}
	b := &build.Env{Dirs: dirs}
	images := slices.Concat(b.Environments(), image.Agents)
	if slices.Contains(images, word) {
		return nil
	}
	if image.IsReserved(word) {
		if m, ok := errors.AsType[msg.M](b.CheckLayers(word)); ok && m.ID() == "layer_is_command" {
			return m
		}
	}
	// Cobra's suggestions, by distance or as a prefix: the closest first.
	command := ""
	if s := root.SuggestionsFor(word); len(s) > 0 {
		command = s[0]
		if c := closest(word, s); c != "" {
			command = c
		}
	}
	img := closest(word, images)
	switch {
	case command != "" && img != "":
		return msg.NoCommandOrImageBoth(command, img, word)
	case command != "":
		return msg.NoCommandOrImageCommand(command, word)
	case img != "":
		return msg.NoCommandOrImageImage(img, word)
	}
	return msg.NoCommandOrImage(word)
}

// closest is the name in names closest to word, ignoring case, within
// suggestDistance; "" when none is.
func closest(word string, names []string) string {
	best, bestD := "", suggestDistance+1
	for _, n := range names {
		if d := levenshtein(strings.ToLower(word), strings.ToLower(n)); d < bestD {
			best, bestD = n, d
		}
	}
	return best
}

// levenshtein is the edit distance between a and b, by runes.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// wordPanic is what the root's help panics with for devvm WORD --help
// when WORD is neither a command nor an image: execute recovers it and
// fails as for devvm WORD, since the help can't return an error.
type wordPanic struct{ err usageError }

// addHelp gives root devvm's own help command, devvm help [COMMAND], and
// a help that refuses a word that's neither a command nor an image, as
// devvm WORD does: devvm help WORD and devvm WORD --help fail the same
// way instead of printing the root's help.
func addHelp(root *cobra.Command) {
	help := &cobra.Command{
		Use:   "help [COMMAND]",
		Short: "Print the help for devvm or a command",
		Long:  "Print the help of devvm, or of COMMAND, as devvm COMMAND --help does.",
		Args: func(c *cobra.Command, args []string) error {
			_, err := helpTarget(c.Root(), args)
			return err
		},
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			target, _, err := c.Root().Find(args)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var out []cobra.Completion
			for _, sub := range target.Commands() {
				if sub.IsAvailableCommand() && strings.HasPrefix(sub.Name(), toComplete) {
					out = append(out, sub.Name())
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(c *cobra.Command, args []string) error {
			target, err := helpTarget(c.Root(), args)
			if err != nil {
				return err
			}
			target.InitDefaultHelpFlag()
			return target.Help()
		},
	}
	checkCommand(help)
	root.SetHelpCommand(help)
	shown := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		if c == root {
			if words := c.Flags().Args(); len(words) > 0 {
				if err := checkWord(root, words[0]); err != nil {
					panic(wordPanic{usage(err)})
				}
			}
		}
		shown(c, args)
	})
}

// helpTarget is the command devvm help ARGS shows: a word that's neither
// a command nor an image is refused, as devvm WORD refuses it, and one
// under a command with commands of its own as that command refuses it.
func helpTarget(root *cobra.Command, args []string) (*cobra.Command, error) {
	target, rest, err := root.Find(args)
	switch {
	case err != nil:
		return nil, err
	case len(rest) == 0:
		return target, nil
	case target == root:
		return root, checkWord(root, rest[0])
	case target.HasAvailableSubCommands():
		return nil, noArgs(target, rest[:1])
	}
	return target, nil
}

// printUsage prints a usage error: its message, then the line naming
// cmd's own --help.
func printUsage(u *ui.UI, cmd *cobra.Command, e usageError) {
	u.Error(e.m)
	u.Note(msg.UsageHelp(cmd.CommandPath()))
}
