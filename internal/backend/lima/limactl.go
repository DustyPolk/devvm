package lima

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
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

// limactl returns a limactl command on devvm's own Lima home. It runs in
// devvm's data folder: Lima writes lima.REJECTED.yaml into the folder it
// runs in when a config is invalid, and a project folder is the VM's to
// write, so the VM could have made that name a link to anything.
func (l *Lima) limactl(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "limactl", args...) //nolint:gosec // devvm's own arguments
	cmd.Env = setEnv(os.Environ(), "LIMA_HOME="+l.Dirs.Lima)
	cmd.Dir = l.Dirs.Data
	if err := os.MkdirAll(l.Dirs.Data, 0o700); err != nil && cmd.Err == nil {
		cmd.Err = err // the command then fails with it
	}
	return cmd
}

// Limactl is devvm limactl: limactl with args, on devvm's own Lima home,
// in devvm's place, so its input, output, terminal and exit code are the
// user's. It runs in devvm's data folder, as devvm's own limactl commands
// do. It returns only when it can't run limactl.
func Limactl(d paths.Dirs, args []string) error {
	path, err := exec.LookPath("limactl")
	if err != nil {
		return msg.LimaMissing()
	}
	if err := os.MkdirAll(d.Data, 0o700); err != nil {
		return err
	}
	if err := os.Chdir(d.Data); err != nil {
		return err
	}
	argv := append([]string{"limactl"}, args...)
	return platform.Exec(path, argv, setEnv(os.Environ(), "LIMA_HOME="+d.Lima))
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

// edit changes a stopped VM's Lima config, with limactl edit's options.
func (l *Lima) edit(ctx context.Context, vm string, opts ...string) error {
	args := append([]string{"edit", "--tty=false", vm}, opts...)
	if err := l.Runner.Run(ctx, msg.Configuring(vm), l.limactl(ctx, args...)); err != nil {
		return backend.Logged(msg.LimaEditFailed(vm))
	}
	return nil
}

// instance is a VM as limactl list --json has it: the fields devvm reads.
type instance struct {
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	CPUs          int     `json:"cpus"`
	Memory        int64   `json:"memory"` // bytes
	Disk          int64   `json:"disk"`   // bytes
	SSHLocalPort  int     `json:"sshLocalPort"`
	SSHConfigFile string  `json:"sshConfigFile"`
	Config        *config `json:"config"`
}

// config is the parts of a VM's Lima config devvm reads.
type config struct {
	Mounts            []mount           `json:"mounts"`
	Networks          []json.RawMessage `json:"networks"`
	Env               map[string]string `json:"env"`
	PropagateProxyEnv *bool             `json:"propagateProxyEnv"`
	DNS               []string          `json:"dns"`
	HostResolver      struct {
		Enabled *bool `json:"enabled"`
	} `json:"hostResolver"`
	SSH struct {
		LocalPort    int   `json:"localPort"`
		ForwardAgent *bool `json:"forwardAgent"`
	} `json:"ssh"`
}

// mount is one of a VM's mounts, as Lima's config and --set have it.
type mount struct {
	Location   string  `json:"location"`
	MountPoint *string `json:"mountPoint,omitempty"`
	Writable   *bool   `json:"writable,omitempty"`
}

func (m mount) writable() bool { return m.Writable != nil && *m.Writable }

// agentDir reports whether the mount is an agent folder: read-only, at the
// same path as on the host (the tools share is elsewhere).
func (m mount) agentDir() bool {
	return !m.writable() && (m.MountPoint == nil || *m.MountPoint == "" || *m.MountPoint == m.Location)
}

// list returns every instance in devvm's Lima home: VMs and images.
func (l *Lima) list(ctx context.Context) ([]instance, error) {
	out, err := l.Runner.Output(l.limactl(ctx, "list", "--json"))
	if err != nil {
		return nil, fmt.Errorf("limactl list: %w", err)
	}
	var ins []instance
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var in instance
		if err := dec.Decode(&in); errors.Is(err, io.EOF) {
			return ins, nil
		} else if err != nil {
			return nil, fmt.Errorf("limactl list: %w", err)
		}
		if in.Config == nil {
			in.Config = &config{}
		}
		ins = append(ins, in)
	}
}

// inspect returns one instance, or an error matching backend.ErrNotFound.
func (l *Lima) inspect(ctx context.Context, name string) (instance, error) {
	ins, err := l.list(ctx)
	if err != nil {
		return instance{}, err
	}
	for _, in := range ins {
		if in.Name == name {
			return in, nil
		}
	}
	return instance{}, fmt.Errorf("%s: %w", name, backend.ErrNotFound)
}

// checkGlobalConfigs refuses Lima's global configs in devvm's Lima home:
// Lima merges them into every VM, so a setting meant for other VMs (a
// network, a mount) would reach devvm's.
func (l *Lima) checkGlobalConfigs() error {
	if m, ok := l.globalConfigs(); ok {
		return m
	}
	return nil
}

// globalConfigs returns lima_global_config when devvm's Lima home has
// any of Lima's global configs.
func (l *Lima) globalConfigs() (msg.M, bool) {
	var found []string
	for _, f := range []string{"default.yaml", "override.yaml"} {
		p := filepath.Join(l.Dirs.Lima, "_config", f)
		if _, err := os.Lstat(p); err == nil {
			found = append(found, p)
		}
	}
	if len(found) > 0 {
		return msg.LimaGlobalConfig(strings.Join(found, " ")), true
	}
	return msg.M{}, false
}

// yqJSON is v as a yq value for limactl's --set: JSON, which yq reads,
// with every string quoted.
func yqJSON(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err) // devvm's own values: always encodable
	}
	return strings.TrimSuffix(b.String(), "\n")
}
