package smolvm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
)

// Runner runs the commands the backend makes. Tests swap in a fake.
type Runner interface {
	// Run runs a command into the run log, with step on the status line
	// (ui.LoggedCmd).
	Run(ctx context.Context, step msg.M, cmd *exec.Cmd) error
	// Output runs a command and returns its standard output.
	Output(cmd *exec.Cmd) ([]byte, error)
	// Attach runs a command connected to the streams already set on it.
	Attach(cmd *exec.Cmd) error
}

type uiRunner struct{}

func (uiRunner) Run(ctx context.Context, step msg.M, cmd *exec.Cmd) error {
	return ui.LoggedCmd(ctx, step, cmd)
}

func (uiRunner) Output(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

func (uiRunner) Attach(cmd *exec.Cmd) error { return cmd.Run() }

// smolvm returns a smolvm command: its usage journal off, never the
// machine SMOLVM_MACHINE_NAME names (devvm always names one), in devvm's
// data folder. On Linux it keeps its state in devvm's own folder; on
// macOS, where it can't, devvm's machines share the user's smolvm and are
// told apart by their label.
func (s *Smolvm) smolvm(ctx context.Context, args ...string) *exec.Cmd {
	if s.OS == "linux" && len(args) >= 2 {
		args = slices.Concat(args[:2], []string{"--state-dir", s.stateDir()}, args[2:])
	}
	cmd := exec.CommandContext(ctx, "smolvm", args...) //nolint:gosec // devvm's own arguments
	cmd.Env = setEnv(os.Environ(), "SMOLVM_USAGE=0", "SMOLVM_MACHINE_NAME")
	cmd.Dir = s.Dirs.Data
	if err := os.MkdirAll(s.Dirs.Data, 0o700); err != nil && cmd.Err == nil {
		cmd.Err = err // the command then fails with it
	}
	return cmd
}

// setEnv returns env with each KEY=value in kvs replacing any value KEY
// had. A bare KEY removes it.
func setEnv(env []string, kvs ...string) []string {
	out := slices.Clone(env)
	for _, kv := range kvs {
		key, _, hasValue := strings.Cut(kv, "=")
		out = slices.DeleteFunc(out, func(e string) bool { return strings.HasPrefix(e, key+"=") })
		if hasValue {
			out = append(out, kv)
		}
	}
	return out
}

// machine is a machine as smolvm machine ls --json has it: the fields
// devvm reads.
type machine struct {
	Name      string            `json:"name"`
	State     string            `json:"state"`
	CPUs      int               `json:"cpus"`
	MemoryMiB int               `json:"memory_mib"`
	StorageGB *float64          `json:"storage_gb"`
	Network   bool              `json:"network"`
	Labels    map[string]string `json:"labels"`
	// Its allow lists: none for smolvm's whole network.
	AllowedCIDRs []string `json:"allowed_cidrs"`
	AllowedHosts []string `json:"allowed_hosts"`
}

// defaultStorage is smolvm's disk size for a machine that doesn't say.
const defaultStorage = 20

// status is the machine's state as devvm's: created is stopped too.
func (m machine) status() backend.Status {
	switch m.State {
	case "running":
		return backend.StatusRunning
	case "stopped", "created":
		return backend.StatusStopped
	}
	return backend.Status(m.State)
}

func (m machine) size() backend.Size {
	disk := float64(defaultStorage)
	if m.StorageGB != nil {
		disk = *m.StorageGB
	}
	return backend.Size{CPUs: m.CPUs, MemoryGiB: float64(m.MemoryMiB) / 1024, DiskGiB: disk}
}

// list returns every machine smolvm has: devvm's, and on macOS the user's
// own.
func (s *Smolvm) list(ctx context.Context) ([]machine, error) {
	out, err := s.Runner.Output(s.smolvm(ctx, "machine", "ls", "--json"))
	if err != nil {
		return nil, fmt.Errorf("smolvm machine ls: %w", err)
	}
	var ms []machine
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(out, &ms); err != nil {
		return nil, fmt.Errorf("smolvm machine ls: %w", err)
	}
	return ms, nil
}

// ours returns this devvm's machines: those with its label.
func (s *Smolvm) ours(ctx context.Context) ([]machine, error) {
	ms, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(ms, func(m machine) bool { return m.Labels[LabelKey] != s.Dirs.Data }), nil
}

// errForeign is a machine by a name devvm uses that isn't its own.
var errForeign = errors.New("not this devvm's machine")

// inspect returns one of devvm's machines, an error matching
// backend.ErrNotFound when smolvm has none by that name, or one matching
// errForeign when the machine by that name isn't this devvm's: devvm never
// touches it.
func (s *Smolvm) inspect(ctx context.Context, name string) (machine, error) {
	ms, err := s.list(ctx)
	if err != nil {
		return machine{}, err
	}
	for _, m := range ms {
		if m.Name != name {
			continue
		}
		if m.Labels[LabelKey] != s.Dirs.Data {
			return machine{}, fmt.Errorf("%s: %w", name, errForeign)
		}
		return m, nil
	}
	return machine{}, fmt.Errorf("%s: %w", name, backend.ErrNotFound)
}

// absent checks smolvm has no machine by that name: one of devvm's is an
// error, and so is anyone else's (smolvm_machine_foreign).
func (s *Smolvm) absent(ctx context.Context, name string) error {
	_, err := s.inspect(ctx, name)
	switch {
	case errors.Is(err, backend.ErrNotFound):
		return nil
	case errors.Is(err, errForeign):
		return msg.SmolvmMachineForeign(name)
	case err != nil:
		return err
	}
	return fmt.Errorf("smolvm: %s exists already", name)
}

// update changes a stopped machine's settings, with machine update's
// options.
func (s *Smolvm) update(ctx context.Context, vm string, opts ...string) error {
	if len(opts) == 0 {
		return nil
	}
	args := append([]string{"machine", "update", "--name", vm}, opts...)
	if err := s.Runner.Run(ctx, msg.Configuring(vm), s.smolvm(ctx, args...)); err != nil {
		return backend.Logged(msg.SmolvmUpdateFailed(vm))
	}
	return nil
}

// stop stops a machine.
func (s *Smolvm) stop(ctx context.Context, vm string) error {
	if err := s.Runner.Run(ctx, msg.Stopping(vm), s.smolvm(ctx, "machine", "stop", "--name", vm)); err != nil {
		return backend.Logged(msg.StopFailed(vm))
	}
	return nil
}

// remove deletes a machine and what devvm keeps for it.
func (s *Smolvm) remove(ctx context.Context, vm string) error {
	if err := s.Runner.Run(ctx, msg.Deleting(vm), s.smolvm(ctx, "machine", "delete", "--name", vm, "-f")); err != nil {
		return backend.Logged(msg.DeleteFailed(vm))
	}
	for _, f := range []string{s.Dirs.SmolvmVM(vm), s.proxyFile(vm)} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// removeIfThere deletes one of devvm's machines if smolvm has it.
func (s *Smolvm) removeIfThere(ctx context.Context, vm string) error {
	if _, err := s.inspect(ctx, vm); errors.Is(err, backend.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	return s.remove(ctx, vm)
}

// asRoot is a command run as root in a running machine, its input fed to
// sh: devvm's own scripts.
func (s *Smolvm) asRoot(ctx context.Context, vm string, args ...string) *exec.Cmd {
	a := append(append([]string{"machine", "exec", "--name", vm}, s.proxyArgs(vm)...), "-i", "--", "sh", "-s", "--")
	return s.smolvm(ctx, append(a, args...)...)
}
