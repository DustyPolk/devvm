package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newCodeCmd is devvm code.
func newCodeCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "code",
		Short: "Open this folder's VM in VS Code over Remote-SSH",
		Long: "Get this folder's VM ready as devvm ssh does, then open its folder in VS Code over Remote-SSH, " +
			"with VS Code's code command. The VM's VS Code settings turn off forwarding its ports to this " +
			"machine and answering git's sign-in prompts with your GitHub sign-in.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return env.Code(cmd.Context()) }, msg.CodeFailed)
			return nil
		},
	}
}
