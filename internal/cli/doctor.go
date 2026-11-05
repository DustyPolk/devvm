package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/trust"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newDoctorCmd is devvm doctor.
func newDoctorCmd(a *app) *cobra.Command {
	u := a.u
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check what devvm needs on this machine",
		Long: "Check what Lima needs on this machine, and say how to get what's missing, " +
			"because devvm never installs it itself. Then list the features Lima gives VMs. " +
			"doctor fails when something is missing, but never for a warning, such as one about low free space.",
		Example: examples(
			"devvm doctor", "check what devvm needs", "devvm doctor --json", "the same as JSON, for scripts",
		),
		Args:              noArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, b, err := doctorBackend(a)
			if err != nil {
				return failed(err, msg.DoctorFailed)
			}
			d := doctor{
				Path: onPath{LookPath: exec.LookPath, Executable: os.Executable},
				Free: reg.Host.FreeSpace,
				Host: func() (vm.Host, error) { return vm.ThisHost(reg.Host) },
				Dirs: a.dirs,
				Policies: func() []msg.M {
					dirs, err := a.dirs()
					if err != nil {
						return nil
					}
					return netpolicy.NotRules(dirs)
				},
				Folders: func() []msg.M {
					dirs, err := a.dirs()
					if err != nil {
						return nil
					}
					return state.VMFolderProblems(dirs)
				},
			}
			if asJSON {
				return printJSON(u, d.report(cmd.Context(), u.Text, reg.All(), b))
			}
			return d.run(cmd.Context(), u, b)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print what it found as JSON, and succeed whatever is missing")
	return cmd
}

// doctorOutput is devvm doctor --json: the backend new VMs get and what
// it's missing, doctor's warnings, whether Docker is installed, and every
// backend's features.
type doctorOutput struct {
	Backend  string        `json:"backend"`
	Problems []problemJSON `json:"problems"`
	Warnings []string      `json:"warnings"`
	Docker   bool          `json:"docker"`
	// Features is, for each backend, how it has each feature: native,
	// emulated, unreviewed or none.
	Features map[string]map[string]string `json:"features"`
}

// problemJSON is something the backend is missing: its key, the command
// that fixes it (null when there's none to run) and the message.
type problemJSON struct {
	Key     string  `json:"key"`
	Fix     *string `json:"fix"`
	Message string  `json:"message"`
}

// report is what doctor finds, as doctor --json shows it, each message
// as text shows it (the UI's Text). It prints nothing.
func (d doctor) report(ctx context.Context, text func(msg.M) string, all []backend.Backend, b backend.Backend) doctorOutput {
	out := doctorOutput{
		Backend:  b.Name(),
		Problems: []problemJSON{}, Warnings: []string{}, Features: map[string]map[string]string{},
	}
	for _, p := range b.Problems(ctx) {
		j := problemJSON{Key: p.Key, Message: text(p.Msg)}
		if p.Fix != "" {
			j.Fix = &p.Fix
		}
		out.Problems = append(out.Problems, j)
	}
	for _, m := range d.warnings(b) {
		out.Warnings = append(out.Warnings, text(m))
	}
	_, err := exec.LookPath("docker")
	out.Docker = err == nil
	for _, be := range all {
		fs := map[string]string{}
		for _, f := range backend.Features {
			fs[string(f)] = be.Caps()[f].String()
		}
		out.Features[be.Name()] = fs
	}
	return out
}

// warnings are doctor's warnings, which never make it fail: another devvm
// first on PATH, low free space, a small host, policy files' lines that
// aren't rules, and what in the VMs' folder isn't a VM's.
func (d doctor) warnings(b backend.Backend) []msg.M {
	var ms []msg.M
	if m, ok := d.Path.other(); ok {
		ms = append(ms, m)
	}
	if m, low := (backend.Space{Free: d.Free}).Warning(b); low {
		ms = append(ms, m)
	}
	ms = append(ms, smallHost(d.Host)...)
	if d.Policies != nil {
		ms = append(ms, d.Policies()...)
	}
	if d.Folders != nil {
		ms = append(ms, d.Folders()...)
	}
	return ms
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
	// Dirs is devvm's folders, whose CA folder the company proxy's check
	// reads (sayProxyCA).
	Dirs func() (paths.Dirs, error)
	// Policies are the warnings for your policy files' lines that aren't
	// rules (netpolicy.NotRules); nil for none.
	Policies func() []msg.M
	// Folders are the warnings for what in the VMs' folder isn't a VM's
	// (state.VMFolderProblems); nil for none.
	Folders func() []msg.M
}

// run checks b, the backend new VMs get, and lists its features. It
// returns the last problem's message when b has problems, having warned
// the others. Warnings (another devvm on PATH, low free space, a small
// host) never change what it returns.
func (d doctor) run(ctx context.Context, u *ui.UI, b backend.Backend) error {
	problems := b.Problems(ctx)
	if err := writeFeatures(u.Table(), b); err != nil {
		return err
	}
	if _, err := exec.LookPath("docker"); err == nil {
		u.Say(msg.DockerFound())
	} else {
		u.Say(msg.DockerMissing())
	}
	sayProxyCA(u, d.Dirs)
	for _, m := range d.warnings(b) {
		u.Warn(m)
	}
	if len(problems) == 0 {
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

// sayProxyCA says, when HTTPS_PROXY or config.toml's proxy.https is set
// (settings.HostProxy) and devvm's CA folder has no CAs, that a proxy that
// inspects TLS needs its CA there: not a problem, as most proxies don't. A
// folder devvm would refuse is warned about.
func sayProxyCA(u *ui.UI, folders func() (paths.Dirs, error)) {
	dirs, err := folders()
	if err != nil {
		return
	}
	if host, err := settings.HostProxy(dirs, os.Getenv); err != nil || host.HTTPS == "" {
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

// doctorBackend returns devvm's backends and the one new VMs get.
func doctorBackend(a *app) (*backend.Registry, backend.Backend, error) {
	reg, err := a.registry()
	if err != nil {
		return nil, nil, err
	}
	c, err := reg.For(new(state.VM))
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

// writeFeatures writes a table of every feature and how b has it: native
// green, emulated yellow, unreviewed red and none dim.
func writeFeatures(tab *ui.Table, b backend.Backend) error {
	tab.Header("FEATURE", "SUPPORT")
	for _, f := range backend.Features {
		tab.Row(cell(string(f)), supportCell(b.Caps()[f]))
	}
	return tab.Flush()
}

// supportCell is how a backend has a feature, colored by what it means:
// none is dim, as a backend lacking a feature is no fault; unreviewed, an
// emulation devvm refuses, stands out in red.
func supportCell(s backend.Support) ui.Cell {
	role := ui.Secondary
	switch {
	case s.IsNative():
		role = ui.Good
	case s.Usable():
		role = ui.Attention
	case s.IsEmulated():
		role = ui.Bad
	}
	return ui.Cell{Text: s.String(), Role: role}
}
