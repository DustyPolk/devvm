package cli

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newToolsCmd is devvm tools: this folder's VM's shared tools, and add,
// rm and update, which change them.
func newToolsCmd(u *ui.UI, g *globals) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Show this folder's VM's shared tools",
		Long: "Show the shared tools of this folder's VM: each tool's version, and the newer version devvm's " +
			"tools lock has, if any, with whether it's built. devvm tools update moves the VM to the newer ones. " +
			"It changes nothing, starts no VM and doesn't go online.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var ts []tools.VMTool
			withEnv(u, func(env *vm.Env) error {
				var err error
				ts, err = vm.Tools{Env: env}.List(cmd.Context())
				return err
			}, msg.ToolsFailed)
			if asJSON {
				return printJSON(u, toolsJSON(ts))
			}
			return writeTools(u.Table(), ts)
		},
	}
	jsonFlag(cmd, &asJSON)
	cmd.AddCommand(newToolsAddCmd(u, g), newToolsRmCmd(u, g), newToolsUpdateCmd(u, g))
	return cmd
}

// toolsOutput is devvm tools --json: the VM's shared tools, each with the
// newer version the tools lock has, when it has one, and whether that one
// is built.
type toolsOutput struct {
	Tools []vmToolJSON `json:"tools"`
}

type vmToolJSON struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
	Newer   string `json:"newer,omitempty"`
	Built   *bool  `json:"newer_built,omitempty"`
}

func toolsJSON(ts []tools.VMTool) toolsOutput {
	out := toolsOutput{Tools: make([]vmToolJSON, 0, len(ts))}
	for _, t := range ts {
		j := vmToolJSON{Tool: t.Tool, Version: t.Version, Newer: t.Newer}
		if t.Newer != "" {
			j.Built = &t.Built
		}
		out.Tools = append(out.Tools, j)
	}
	return out
}

// writeTools writes a VM's tools as a table, a header first.
func writeTools(tab *ui.Table, ts []tools.VMTool) error {
	tab.Header("TOOL", "VERSION", "NEWER", "BUILT")
	for _, t := range ts {
		built := ""
		if t.Newer != "" {
			built = "no"
			if t.Built {
				built = "yes"
			}
		}
		tab.Row(cell(t.Tool), cell(t.Version), cell(t.Newer), cell(built))
	}
	return tab.Flush()
}

func newToolsAddCmd(u *ui.UI, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "add TOOL...",
		Short: "Add shared tools to this folder's VM, then open a shell in it",
		Long: "Add shared tools to this folder's VM, each TOOL a separate word (devvm tools add python claude), " +
			"then open a shell in it: node, python, rust, go, claude, codex or opencode. devvm builds the ones " +
			"the share lacks, and nothing in the VM's disk changes, because its tools are mounted. A new agent's " +
			"config folders are mounted too, which needs the VM to start again: devvm asks before restarting a " +
			"running VM. Layers can't be added: only a new VM gets one.",
		Args: toolNames("add"),
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Tools{Env: env}.Add(cmd.Context(), g.options(), args) }, msg.UpFailed)
			return nil
		},
		ValidArgsFunction: completeToolNames,
	}
}

func newToolsRmCmd(u *ui.UI, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "rm TOOL...",
		Short: "Take shared tools out of this folder's VM, then open a shell in it",
		Long: "Take shared tools out of this folder's VM, each TOOL a separate word (devvm tools rm python claude), " +
			"then open a shell in it. They're not mounted in it any more; what the VM installed itself stays. Your " +
			"layers are in the VM's disk, so they can't be taken out: devvm rm, and make the VM again.",
		Args: toolNames("rm"),
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Tools{Env: env}.Remove(cmd.Context(), g.options(), args) }, msg.UpFailed)
			return nil
		},
		ValidArgsFunction: completeToolNames,
	}
}

func newToolsUpdateCmd(u *ui.UI, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Move this folder's VM to the newest shared tools, then open a shell in it",
		Long: "Check for newer versions of the shared tools (node, python, rust, go, the agents), build " +
			"them, and move this folder's VM to them, then open a shell in it. Nothing in the VM's disk " +
			"changes: its tools are mounted from the share.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Tools{Env: env}.Update(cmd.Context(), g.options()) }, msg.UpFailed)
			return nil
		},
	}
}

// toolNames takes one or more TOOLs, each a word of its own: one joined
// with + is refused, naming sub, the devvm tools command (add, rm). add
// takes only shared tools' names.
func toolNames(sub string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := minArgs(1)(cmd, args); err != nil {
			return err
		}
		names := slices.Concat(image.Toolchains, image.Agents)
		for _, a := range args {
			if strings.Contains(a, "+") {
				return msg.ToolsJoined(sub, a)
			}
			if sub == "add" && !slices.Contains(names, a) {
				return msg.UnknownTool(a, strings.Join(names, " "))
			}
		}
		return nil
	}
}
