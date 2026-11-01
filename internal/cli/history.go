package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newHistoryCmd is devvm history: the exports of agents' history in
// devvm's exports folder, and history export, which writes one.
func newHistoryCmd(u *ui.UI) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List the saved exports of agents' history",
		Long: "List the exports of agents' history in ~/.local/share/devvm/exports, which devvm history export " +
			"and devvm rm write there, oldest first: each one's file, the VM it's of, that VM's folder, when " +
			"it was written and its size. Exports written to a file of your own aren't listed. It changes " +
			"nothing and starts no VM.",
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error {
			d, err := paths.FromEnv()
			var es []savedExport
			if err == nil {
				es, err = exports(d)
			}
			fail(u, err, msg.HistoryFailed)
			if asJSON {
				return printJSON(u, historyJSON(es))
			}
			return writeHistory(u.Table(), es)
		},
	}
	jsonFlag(cmd, &asJSON)
	cmd.AddCommand(newHistoryExportCmd(u))
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

func historyJSON(es []savedExport) historyOutput {
	out := historyOutput{Exports: make([]exportJSON, 0, len(es))}
	for _, e := range es {
		out.Exports = append(out.Exports, exportJSON{File: e.Path, VM: e.VM, Folder: e.Folder, Date: e.At.Format(time.RFC3339), Size: e.Size})
	}
	return out
}

// writeHistory writes the exports as a table, a header first; a folder
// devvm has no record of is -.
func writeHistory(tab *ui.Table, es []savedExport) error {
	tab.Header("FILE", "VM", "FOLDER", "DATE", "SIZE")
	for _, e := range es {
		tab.Row(cell(e.Path), cell(e.VM), cell(e.Folder), cell(exportDate(e.At)), cell(humanSize(e.Size)))
	}
	return tab.Flush()
}

// newHistoryExportCmd is devvm history export [FILE].
func newHistoryExportCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "export [FILE]",
		Short: "Save this folder's VM's agent history to a .tar.gz",
		Long: "Save this folder's VM's agent history (transcripts and sessions, without the agents' " +
			"logins) to FILE, a .tar.gz, or to a new file in ~/.local/share/devvm/exports. A stopped " +
			"VM is started for it. FILE is replaced only once the export is complete.",
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error {
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
}

// exportName is an export's name in the exports folder, as devvm history
// export and devvm rm write it: <vm>-<date>-<time>.tar.gz. Nothing else
// there is one: an export being written (.devvm-export-…), the file
// beside one that names its folder (paths.ExportFolder) or the user's
// own.
var exportName = regexp.MustCompile(`^(devvm-.+-[0-9a-f]{14})-(\d{8}-\d{6})\.tar\.gz$`)

// savedExport is one of the exports in devvm's exports folder.
type savedExport struct {
	Path   string
	VM     string    // the VM it's of
	Folder string    // its project folder, from the file beside it; "" without one
	At     time.Time // when it was written, from its name
	Size   int64
}

// exportFolder reads the project folder the file beside an export names;
// "" without one.
func exportFolder(export string) string {
	data, err := os.ReadFile(paths.ExportFolder(export))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// exports lists the exports in devvm's exports folder, oldest first: only
// plain files named as devvm names them.
func exports(d paths.Dirs) ([]savedExport, error) {
	entries, err := os.ReadDir(d.Exports())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []savedExport
	for _, e := range entries {
		m := exportName.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		// The name has no zone: it's the host's wall clock when the export
		// was written (devvm history export names it in local time), so it's read
		// in local time too and shown as written. An age is elapsed time
		// from that instant (30d is 720 hours), so across a DST change an
		// export reaches it an hour before or after the same wall-clock
		// time 30 days on, and on a host moved to another zone it's off by
		// the zones' difference.
		at, err := time.ParseInLocation("20060102-150405", m[2], time.Local)
		if err != nil {
			continue
		}
		p := filepath.Join(d.Exports(), e.Name())
		out = append(out, savedExport{Path: p, VM: m[1], Folder: exportFolder(p), At: at, Size: du(p)})
	}
	slices.SortFunc(out, func(a, b savedExport) int { return a.At.Compare(b.At) })
	return out, nil
}

// olderThan returns the exports written at least age before now; all of
// them when age is 0.
func olderThan(es []savedExport, age time.Duration, now time.Time) []savedExport {
	if age == 0 {
		return es
	}
	var out []savedExport
	for _, e := range es {
		if now.Sub(e.At) >= age {
			out = append(out, e)
		}
	}
	return out
}

// exportPaths are these exports' files, each with the file beside it
// that names its folder.
func exportPaths(es []savedExport) []string {
	out := make([]string, 0, 2*len(es))
	for _, e := range es {
		out = append(out, e.Path, paths.ExportFolder(e.Path))
	}
	return out
}

// exportsSize is the disk these exports take.
func exportsSize(es []savedExport) int64 {
	var n int64
	for _, e := range es {
		n += e.Size
	}
	return n
}

// exportDate is how an export's list shows its date.
func exportDate(t time.Time) string { return strings.TrimSpace(t.Format("2006-01-02 15:04")) }
