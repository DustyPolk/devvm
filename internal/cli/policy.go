package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newPolicyCmd is devvm policy, and allow, deny and rm RULE... [-g].
func newPolicyCmd(u *ui.UI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Show the network policy allowlist of this folder's VM",
		Long: "Show the allowlist of this folder's VM, which its network policy proxy reads: each policy " +
			"file that has rules, in order (its image's, yours for every VM, the VM's own, its tokens' " +
			"hosts), and the rules in it. devvm policy allow, deny and rm change the VM's own rules, or " +
			"with -g yours for every VM; a change applies at once, to the next connection.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return env.Policy(cmd.Context(), u.Stdout()) }, msg.PolicyFailed)
			return nil
		},
	}
	sub := func(use, short, long string, deny, remove bool) *cobra.Command {
		var global bool
		c := &cobra.Command{
			Use:   use + " RULE...",
			Short: short,
			Long:  long,
			Args:  cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				withEnv(u, func(env *vm.Env) error {
					return env.PolicyChange(cmd.Context(), args, deny, remove, global)
				}, msg.PolicyFailed)
				return nil
			},
		}
		c.Flags().BoolVarP(&global, "global", "g", false, "change your rules for every VM instead")
		return c
	}
	rules := " RULE is a host (example.com), any subdomain of one (*.example.com), any host (**), an " +
		"address or CIDR (10.0.0.5, fd00::/8), each with an optional port (example.com:8443, " +
		"[fd00::1]:22); without a port, 443 and 80."
	cmd.AddCommand(
		sub("allow", "Allow hosts for this folder's VM", "Add RULEs to the allowlist of this folder's VM, "+
			"or with -g of every VM. They apply at once, to the next connection."+rules, false, false),
		sub("deny", "Deny hosts for this folder's VM", "Add RULEs as denies to this folder's VM's rules, "+
			"or with -g every VM's: a deny beats any allow. They apply at once, to the next connection."+rules,
			true, false),
		sub("rm", "Remove rules from this folder's VM", "Remove RULEs, as written (a deny with its !), from "+
			"this folder's VM's own rules, or with -g yours for every VM. They apply at once.", false, true),
	)
	return cmd
}
