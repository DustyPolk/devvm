package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
)

// checkWord refuses devvm WORD: WORD isn't a command, which the parser
// would have run, and devvm takes no other word. It suggests the closest
// command when one is close.
func checkWord(root *cobra.Command, word string) error {
	// Cobra's suggestions, by distance or as a prefix: the closest first.
	if s := root.SuggestionsFor(word); len(s) > 0 {
		command := s[0]
		if c := msg.Closest(word, s); c != "" {
			command = c
		}
		return msg.NoCommandLike(command, word)
	}
	return msg.NoCommand(word)
}

// wordPanic is what the root's help panics with for devvm WORD --help
// when WORD isn't a command: execute recovers it and fails as for devvm
// WORD, since the help can't return an error.
type wordPanic struct{ err usageError }

// addHelp gives root devvm's own help command, devvm help [COMMAND], and
// a help that refuses a word that isn't a command, as devvm WORD does:
// devvm help WORD and devvm WORD --help fail the same way instead of
// printing the root's help.
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

// helpTarget is the command devvm help ARGS shows: a word that isn't a
// command is refused, as devvm WORD refuses it, and one under a command
// with commands of its own as that command refuses it.
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
