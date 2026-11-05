package cli

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newHistoryCmd is devvm history: the exports of agents' history in
// devvm's exports folder (vm.Exports), and history export, which writes
// one.
func newHistoryCmd(a *app) *cobra.Command {
	u := a.u
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List the saved exports of agents' history",
		Long: "List the exports of agents' history that devvm history export and devvm rm saved in " +
			"~/.local/share/devvm/exports, oldest first. It changes nothing and starts no VM.",
		Example: examples(
			"devvm history", "list the saved exports",
			"devvm history export", "save this folder's VM's agent history",
			"devvm history import", "put the newest export back into this folder's VM",
		),
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error {
			d, err := a.dirs()
			var es []vm.Export
			if err == nil {
				es, err = vm.Exports{Dirs: d}.List()
			}
			fail(u, err, msg.HistoryFailed)
			if asJSON {
				return printJSON(u, historyJSON(es))
			}
			return writeHistory(u.Table(), es)
		},
	}
	jsonFlag(cmd, &asJSON)
	cmd.AddCommand(newHistoryExportCmd(a), newHistoryImportCmd(a))
	return cmd
}

// historyOutput is devvm history --json: the exports, oldest first, each
// with when it was written in RFC 3339 (this machine's zone) and its size
// in bytes. A folder devvm has no record of is "".
type historyOutput struct {
	Exports []exportJSON `json:"exports"`
}

type exportJSON struct {
	File   string `json:"file"`
	VM     string `json:"vm"`
	Folder string `json:"folder"`
	Date   string `json:"date"`
	Size   int64  `json:"size"`
}

func historyJSON(es []vm.Export) historyOutput {
	out := historyOutput{Exports: make([]exportJSON, 0, len(es))}
	for _, e := range es {
		out.Exports = append(out.Exports, exportJSON{File: e.Path, VM: e.VM, Folder: e.Folder, Date: e.At.Format(time.RFC3339), Size: e.Size})
	}
	return out
}

// writeHistory writes the exports as a table, a header first; a folder
// devvm has no record of is -.
func writeHistory(tab *ui.Table, es []vm.Export) error {
	tab.Header("FILE", "VM", "FOLDER", "DATE", "SIZE")
	for _, e := range es {
		tab.Row(cell(e.Path), cell(e.VM), cell(e.Folder), cell(exportDate(e.At)), cell(humanSize(e.Size)))
	}
	return tab.Flush()
}

// newHistoryExportCmd is devvm history export [FILE].
func newHistoryExportCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export [FILE]",
		Short: "Save this folder's VM's agent history to a .tar.gz",
		Long: "Save the history of this folder's VM's agents, their transcripts and sessions without their logins, " +
			"to FILE, a .tar.gz, or to a new file in ~/.local/share/devvm/exports. " +
			"devvm starts a stopped VM for it.",
		Example: examples(
			"devvm history export", "save it in ~/.local/share/devvm/exports",
			"devvm history export ~/claude.tar.gz", "save it to a file of your own",
		),
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.env(func(env *vm.Env) error {
				file := ""
				if len(args) == 1 {
					abs, err := filepath.Abs(args[0])
					if err != nil {
						return err
					}
					file = abs
				}
				return vm.Lifecycle{Env: env}.Export(cmd.Context(), file)
			}, msg.ExportFailed)
			return nil
		},
	}
	addFolderFlags(cmd, a)
	return cmd
}

// newHistoryImportCmd is devvm history import [FILE].
func newHistoryImportCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [FILE]",
		Short: "Put an export of agent history into this folder's VM",
		Long: "Put the agent history in FILE, a .tar.gz that devvm history export or devvm rm saved, into this folder's VM, " +
			"or the newest export devvm saved for this folder. " +
			"It leaves out the agents' logins and the files devvm copies in, and keeps every file the VM already has.",
		Example: examples(
			"devvm history import", "import the newest export saved for this folder",
			"devvm history import ~/claude.tar.gz", "import a file of your own",
		),
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.env(func(env *vm.Env) error {
				file := ""
				if len(args) == 1 {
					abs, err := filepath.Abs(args[0])
					if err != nil {
						return err
					}
					file = abs
				}
				return vm.Lifecycle{Env: env}.Import(cmd.Context(), file)
			}, msg.ImportFailed)
			return nil
		},
	}
	addFolderFlags(cmd, a)
	return cmd
}

// exportDate is how an export's list shows its date.
func exportDate(t time.Time) string { return strings.TrimSpace(t.Format("2006-01-02 15:04")) }
