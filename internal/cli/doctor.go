package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/trust"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newDoctorCmd is devvm doctor.
func newDoctorCmd(u *ui.UI, g *globals) *cobra.Command {
	var fixes bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check what devvm needs on this machine",
		Long: "Check what the backend new VMs get (--backend for another) needs on this machine, and say how to " +
			"get what's missing: devvm never installs it. The docker backend also says which engine it uses, and on " +
			"Docker Desktop which folders it shares. Then list each backend's features (native, emulated, " +
			"unreviewed: emulated but refused until reviewed, or none) and whether Docker is installed, which " +
			"only --from with an image only Docker has needs. When HTTPS_PROXY is set and ~/.config/devvm/ca has no CA " +
			"certificates, it says that a proxy that inspects TLS needs its CA there. It warns when another devvm " +
			"comes first on PATH, when the disk the backend's images and VMs go on has less free space than building " +
			"base needs, and when this machine has less than 8 GiB of memory or fewer than 4 CPUs. Fails when " +
			"something is missing, but not for any of these warnings.",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, b, err := doctorBackend(u, g.backend)
			fail(u, err, msg.DoctorFailed)
			d := doctor{
				Path: onPath{LookPath: exec.LookPath, Executable: os.Executable},
				Free: platform.FreeSpace,
				Host: vm.ThisHost,
			}
			err = d.run(cmd.Context(), u, reg.All(), b, fixes)
			if m, ok := errors.AsType[msg.M](err); ok {
				u.Die(m)
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&fixes, "fixes", false, "print only what's missing, for scripts: a KEY<tab>FIX<tab>MESSAGE line each, FIX being a command or empty; succeeds either way")
	return cmd
}

// The host devvm recommends: base asks for 4 CPUs and 8 GiB.
const (
	recommendedCPUs      = 4
	recommendedMemoryGiB = 8
)

// doctor is devvm doctor's checks of this machine; tests set its fields.
type doctor struct {
	Path onPath
	Free func(path string) (uint64, error) // free bytes on path's volume
	Host func() (vm.Host, error)
}

// run checks b, the backend new VMs get, and lists all's features. It
// returns the last problem's message when b has problems, having warned
// the others. With fixes, it writes only the problems' lines, and
// succeeds. Warnings (another devvm on PATH, low free space, a small
// host) never change what it returns.
func (d doctor) run(ctx context.Context, u *ui.UI, all []backend.Backend, b backend.Backend, fixes bool) error {
	problems := b.Problems(ctx)
	if fixes {
		return writeFixes(u.Stdout(), problems)
	}
	if err := writeFeatures(u.Stdout(), all); err != nil {
		return err
	}
	if m, ok := d.Path.other(); ok {
		u.Warn(m)
	}
	if _, err := exec.LookPath("docker"); err == nil {
		u.Say(msg.DockerFound())
	} else {
		u.Say(msg.DockerMissing())
	}
	sayProxyCA(u)
	if m, low := (backend.Space{Free: d.Free}).Warning(b); low {
		u.Warn(m)
	}
	for _, m := range smallHost(d.Host) {
		u.Warn(m)
	}
	if len(problems) == 0 {
		if r, ok := b.(backend.Reporter); ok {
			for _, m := range r.Report(ctx) {
				u.Say(m)
			}
		}
		u.Say(msg.DoctorOK(b.Name(), devvmVersion()))
		return nil
	}
	for _, p := range problems[:len(problems)-1] {
		u.Warn(p.Msg)
	}
	return problems[len(problems)-1].Msg
}

// smallHost returns a warning for each way this machine is below the host
// devvm recommends: less memory, fewer CPUs. None when host can't say.
func smallHost(host func() (vm.Host, error)) []msg.M {
	h, err := host()
	if err != nil {
		return nil
	}
	var ms []msg.M
	if h.MemoryGiB > 0 && h.MemoryGiB < recommendedMemoryGiB {
		ms = append(ms, msg.HostMemoryLow(strconv.Itoa(h.MemoryGiB), strconv.Itoa(recommendedMemoryGiB)))
	}
	if h.CPUs > 0 && h.CPUs < recommendedCPUs {
		ms = append(ms, msg.HostCpusFew(strconv.Itoa(h.CPUs), strconv.Itoa(recommendedCPUs)))
	}
	return ms
}

// sayProxyCA says, when HTTPS_PROXY is set and devvm's CA folder has no
// CAs, that a proxy that inspects TLS needs its CA there: not a problem,
// as most proxies don't. A folder devvm would refuse is warned about.
func sayProxyCA(u *ui.UI) {
	if os.Getenv("HTTPS_PROXY") == "" && os.Getenv("https_proxy") == "" {
		return
	}
	dirs, err := paths.FromEnv()
	if err != nil {
		return
	}
	cas, err := trust.Read(dirs.TrustedCAs())
	var m msg.M
	switch {
	case errors.As(err, &m):
		u.Warn(m)
	case err == nil && len(cas) == 0:
		u.Say(msg.DoctorProxyCA(dirs.TrustedCAs()))
	}
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

// onPath finds the devvm first on PATH; tests set its fields.
type onPath struct {
	LookPath   func(file string) (string, error)
	Executable func() (string, error)
}

// other returns a warning when the devvm first on PATH isn't this binary,
// both with their links resolved; none when devvm isn't on PATH or either
// can't be found.
func (p onPath) other() (msg.M, bool) {
	first, err := p.LookPath("devvm")
	if err != nil {
		return msg.M{}, false
	}
	if first, err = filepath.EvalSymlinks(first); err != nil {
		return msg.M{}, false
	}
	self, err := p.Executable()
	if err != nil {
		return msg.M{}, false
	}
	if self, err = filepath.EvalSymlinks(self); err != nil || self == first {
		return msg.M{}, false
	}
	return msg.DevvmOtherFirst(first, self), true
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
