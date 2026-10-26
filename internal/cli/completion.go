package cli

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
)

// shells are the shells devvm completion writes a script for.
var shells = []string{"bash", "zsh", "fish"}

// newCompletionCmd is devvm completion SHELL.
func newCompletionCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "completion SHELL",
		Short: "Print the shell completion script for bash, zsh or fish",
		Long: "Print the script that completes devvm's commands, flags and image names in SHELL: bash, " +
			"zsh or fish. Load it from your shell's startup file: source <(devvm completion bash) in " +
			"~/.bashrc (it needs the bash-completion package), source <(devvm completion zsh) in ~/.zshrc " +
			"(after compinit), devvm completion fish | source in ~/.config/fish/config.fish.",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: shells,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, out := cmd.Root(), u.Stdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			default:
				return root.GenFishCompletion(out, true)
			}
		},
	}
}

// addCompletions sets how the shell completes root's commands' arguments
// and flags: image names for IMAGE, tool names for TOOLS, backend names
// for --backend, and no file names, which none of them take.
func addCompletions(root *cobra.Command, u *ui.UI) {
	backendNames := completeBackend(u)
	root.CompletionOptions.DisableDefaultCmd = true
	root.CompletionOptions.SetDefaultShellCompDirective(cobra.ShellCompDirectiveNoFileComp)
	for _, c := range root.Commands() {
		switch c.Name() {
		case "build", "rebuild":
			c.ValidArgsFunction = completeImage
		case "remove":
			c.ValidArgsFunction = completeTools
		case "setup":
			_ = c.RegisterFlagCompletionFunc("backend", backendNames)
			_ = c.RegisterFlagCompletionFunc("features", completeFeatures)
		}
	}
	root.ValidArgsFunction = completeImage
	_ = root.RegisterFlagCompletionFunc("backend", backendNames)
	_ = root.RegisterFlagCompletionFunc("ports", cobra.FixedCompletions([]string{"all", "none"}, cobra.ShellCompDirectiveNoFileComp))
}

// completeImage completes IMAGE: base, the toolchains, your layers and
// the agents, after the parts already given.
func completeImage(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	dirs, err := paths.FromEnv()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	b := &build.Builder{Dirs: dirs}
	return joined(toComplete, slices.Concat(b.Environments(), image.Agents)), cobra.ShellCompDirectiveNoFileComp
}

// completeTools completes TOOLS: the toolchains and the agents, after the
// parts already given.
func completeTools(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return joined(toComplete, slices.Concat(image.Toolchains, image.Agents)), cobra.ShellCompDirectiveNoFileComp
}

// completeFeatures completes setup's --features: base's features, after
// the ones already given.
func completeFeatures(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	dirs, err := paths.FromEnv()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	features, _ := (&build.Builder{Dirs: dirs, Assets: assets.FS}).Features()
	return joinedBy(",", toComplete, features), cobra.ShellCompDirectiveNoFileComp
}

// completeBackend returns what completes --backend with the backends'
// names.
func completeBackend(u *ui.UI) cobra.CompletionFunc {
	return func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		dirs, err := paths.FromEnv()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		reg, err := registry.New(dirs, assets.FS, u)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return reg.Names(), cobra.ShellCompDirectiveNoFileComp
	}
}

// joined completes the last of the +-joined parts in toComplete with the
// names it starts, leaving out the parts already given: node+c gives
// node+claude and node+codex.
func joined(toComplete string, names []string) []cobra.Completion {
	return joinedBy("+", toComplete, names)
}

// joinedBy is joined for parts joined by sep.
func joinedBy(sep, toComplete string, names []string) []cobra.Completion {
	i := strings.LastIndex(toComplete, sep) + 1
	given := strings.Split(toComplete[:i], sep)
	var out []cobra.Completion
	for _, n := range names {
		if strings.HasPrefix(n, toComplete[i:]) && !slices.Contains(given, n) {
			out = append(out, toComplete[:i]+n)
		}
	}
	return out
}
