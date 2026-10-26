package cli

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// newDoctorCmd is devvm doctor.
func newDoctorCmd(u *ui.UI, g *globals) *cobra.Command {
	var fixes bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check what devvm needs on this machine",
		Long: "Check what the backend new VMs get (--backend for another) needs on this machine, and say how to " +
			"get what's missing: devvm never installs it. Then list each backend's features (native, emulated, " +
			"unreviewed: emulated but refused until reviewed, or none) and whether Docker is installed, which " +
			"only --from with an image only Docker has needs. Fails when something is missing.",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, b, err := doctorBackend(u, g.backend)
			fail(u, err, msg.DoctorFailed)
			problems := b.Problems(cmd.Context())
			if fixes {
				return writeFixes(u.Stdout(), problems)
			}
			if err := writeFeatures(u.Stdout(), reg.All()); err != nil {
				return err
			}
			if _, err := exec.LookPath("docker"); err == nil {
				u.Say(msg.DockerFound())
			} else {
				u.Say(msg.DockerMissing())
			}
			if len(problems) == 0 {
				u.Say(msg.DoctorOK(b.Name(), devvmVersion()))
				return nil
			}
			for _, p := range problems[:len(problems)-1] {
				u.Warn(p.Msg)
			}
			u.Die(problems[len(problems)-1].Msg)
			return nil
		},
	}
	cmd.Flags().BoolVar(&fixes, "fixes", false, "print only what's missing, for scripts: a KEY<tab>FIX<tab>MESSAGE line each, FIX being a command or empty; succeeds either way")
	return cmd
}

// doctorBackend returns devvm's backends and the one new VMs get: flag's
// (--backend), else the default.
func doctorBackend(u *ui.UI, flag string) (*backend.Registry, backend.Backend, error) {
	dirs, err := paths.FromEnv()
	if err != nil {
		return nil, nil, err
	}
	reg, err := registry.New(dirs, assets.FS, u)
	if err != nil {
		return nil, nil, err
	}
	c, err := reg.For(dirs, new(state.VM), flag)
	if err != nil {
		return nil, nil, err
	}
	return reg, c.Backend, nil
}

// writeFixes writes a KEY<tab>FIX<tab>MESSAGE line for each problem, the
// message on one line.
func writeFixes(w io.Writer, problems []backend.Problem) error {
	for _, p := range problems {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", p.Key, p.Fix, p.Msg.Line()); err != nil {
			return err
		}
	}
	return nil
}

// writeFeatures writes a table of every feature, with a column for each
// backend saying how it has it.
func writeFeatures(w io.Writer, bs []backend.Backend) error {
	t := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	row := []string{"FEATURE"}
	for _, b := range bs {
		row = append(row, strings.ToUpper(b.Name()))
	}
	_, _ = fmt.Fprintln(t, strings.Join(row, "\t"))
	for _, f := range backend.Features {
		row = []string{string(f)}
		for _, b := range bs {
			row = append(row, b.Caps()[f].String())
		}
		_, _ = fmt.Fprintln(t, strings.Join(row, "\t"))
	}
	return t.Flush()
}
