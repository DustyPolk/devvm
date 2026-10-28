package docker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// Runner runs the commands the backend makes. Tests swap in a fake.
type Runner interface {
	// Run runs a command into the run log, with step on the status line.
	Run(ctx context.Context, step msg.M, cmd *exec.Cmd) error
	// Output runs a command and returns its standard output; a failed
	// one's error is an *exec.ExitError with its standard error.
	Output(cmd *exec.Cmd) ([]byte, error)
	// Attach runs a command connected to the streams already set on it.
	Attach(cmd *exec.Cmd) error
}

// logRunner runs commands into the run log through logged (the UI's
// LoggedCmd).
type logRunner struct {
	logged func(ctx context.Context, step msg.M, cmd *exec.Cmd) error
}

func (r logRunner) Run(ctx context.Context, step msg.M, cmd *exec.Cmd) error {
	return r.logged(ctx, step, cmd)
}

func (logRunner) Output(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

func (logRunner) Attach(cmd *exec.Cmd) error { return cmd.Run() }

// client is the docker command on what devvm makes in Docker's engine:
// its containers, images and volumes. It acts only on those with its
// label (Data).
type client struct {
	// Runner runs the docker commands.
	Runner Runner
	names
}

// names are the names of what devvm makes in Docker, from its data
// folder.
type names struct {
	// Data is devvm's data folder: the value of the label on all devvm
	// makes, and where docker runs.
	Data string
}

// docker returns a docker command, run in devvm's data folder: on the
// engine the user's docker reaches (DOCKER_HOST, its context).
func (cl client) docker(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // devvm's own arguments
	cmd.Dir = cl.Data
	cmd.Env = os.Environ()
	if err := os.MkdirAll(cl.Data, 0o700); err != nil && cmd.Err == nil {
		cmd.Err = err // the command then fails with it
	}
	return cmd
}

// ns is a hash of devvm's data folder, in the names Docker keeps for
// every user: two devvms (yours and a test's) never share one.
func (n names) ns() string {
	sum := sha256.Sum256([]byte(n.Data))
	return hex.EncodeToString(sum[:])[:8]
}

// imageRef is an image's tag, in the repository of devvm's images
// (devvm-<ns>), tagged with their keys.
func (n names) imageRef(key string) string { return "devvm-" + n.ns() + ":" + key }

// commitRef is a VM's container's nth commit, which its next container is
// made from: in the same repository, tagged <vm>-<n>.
func (n names) commitRef(vm string, gen int) string {
	return fmt.Sprintf("devvm-%s:%s-%d", n.ns(), vm, gen)
}

// buildName is an image's build container.
func (n names) buildName(key string) string { return "devvm-img-" + key + "-" + n.ns() }

// toolsName is the tools build's container, and unpackName the tools
// volume's helper.
func (n names) toolsName() string  { return "devvm-tools-build-" + n.ns() }
func (n names) unpackName() string { return "devvm-tools-unpack-" + n.ns() }

// toolsVolume is the volume of the unpacked shared tools.
func (n names) toolsVolume() string { return "devvm-tools-" + n.ns() }

// container is a container as docker container inspect has it: the fields
// devvm reads.
type container struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"` // /name
	State struct {
		Status    string `json:"Status"`
		StartedAt string `json:"StartedAt"` // its last start, which a stop keeps
	} `json:"State"`
	Config struct {
		Labels map[string]string `json:"Labels"`
		Env    []string          `json:"Env"`
	} `json:"Config"`
	HostConfig struct {
		NanoCpus int64 `json:"NanoCpus"`
		Memory   int64 `json:"Memory"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		// Networks are the networks it's on, by name.
		Networks map[string]struct {
			NetworkID string `json:"NetworkID"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
	Image string `json:"Image"` // its image's ID
}

func (c container) name() string { return strings.TrimPrefix(c.Name, "/") }

// ours reports whether devvm, with its data folder data, made it.
func (c container) ours(data string) bool { return c.Config.Labels[LabelKey] == data }

// status is the container's state as devvm's: created and exited are
// stopped.
func (c container) status() backend.Status {
	switch c.State.Status {
	case "running":
		return backend.StatusRunning
	case "created", "exited":
		return backend.StatusStopped
	}
	return backend.Status(c.State.Status)
}

// gib is a GiB in bytes.
const gib = 1 << 30

func (c container) size() backend.Size {
	return backend.Size{CPUs: int(c.HostConfig.NanoCpus / 1e9), MemoryGiB: float64(c.HostConfig.Memory) / gib}
}

// errForeign is a container, image or volume by a name devvm uses that
// isn't its own.
var errForeign = errors.New("not this devvm's")

// errMissing is what docker says of a container, image or volume it
// doesn't have.
func errMissing(err error) bool {
	ee, ok := errors.AsType[*exec.ExitError](err)
	return ok && bytes.Contains(bytes.ToLower(ee.Stderr), []byte("no such"))
}

// inspect returns the container by that name: an error matching
// backend.ErrNotFound when Docker has none, or one matching errForeign
// when it isn't this devvm's (devvm never touches it).
func (cl client) inspect(ctx context.Context, name string) (container, error) {
	cs, err := cl.inspectAll(ctx, name)
	switch {
	case errMissing(err):
		return container{}, fmt.Errorf("%s: %w", name, backend.ErrNotFound)
	case err != nil:
		return container{}, err
	case len(cs) != 1:
		return container{}, fmt.Errorf("docker container inspect %s: %d containers", name, len(cs))
	case !cs[0].ours(cl.Data):
		return container{}, fmt.Errorf("%s: %w", name, errForeign)
	}
	return cs[0], nil
}

// inspectAll inspects containers by name or ID.
func (cl client) inspectAll(ctx context.Context, names ...string) ([]container, error) {
	out, err := cl.Runner.Output(cl.docker(ctx, append([]string{"container", "inspect"}, names...)...))
	if err != nil {
		return nil, err
	}
	var cs []container
	if err := json.Unmarshal(out, &cs); err != nil {
		return nil, fmt.Errorf("docker container inspect: %w", err)
	}
	return cs, nil
}

// containers returns devvm's containers of a kind, with its label.
func (cl client) containers(ctx context.Context, kind string) ([]container, error) {
	out, err := cl.Runner.Output(cl.docker(ctx, "container", "ls", "-a", "-q", "--no-trunc",
		"--filter", "label="+LabelKey+"="+cl.Data, "--filter", "label="+kindLabel+"="+kind))
	if err != nil {
		return nil, fmt.Errorf("docker container ls: %w", err)
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil, nil
	}
	cs, err := cl.inspectAll(ctx, ids...)
	if err != nil {
		return nil, fmt.Errorf("docker container inspect: %w", err)
	}
	var mine []container
	for _, c := range cs {
		if c.ours(cl.Data) {
			mine = append(mine, c)
		}
	}
	return mine, nil
}

// vm returns one of devvm's VMs' containers. One whose container was
// being made again when a run stopped (its old one gone, its next one not
// yet renamed) is renamed first.
func (cl client) vm(ctx context.Context, vm string) (container, error) {
	c, err := cl.inspect(ctx, vm)
	if errors.Is(err, backend.ErrNotFound) {
		next, nerr := cl.inspect(ctx, nextName(vm))
		if nerr != nil || next.Config.Labels[kindLabel] != kindVM {
			return container{}, err
		}
		if rerr := cl.Runner.Run(ctx, msg.Configuring(vm), cl.docker(ctx, "container", "rename", nextName(vm), vm)); rerr != nil {
			return container{}, backend.Logged(msg.DockerUpdateFailed(vm))
		}
		c, err = cl.inspect(ctx, vm)
	}
	if errors.Is(err, errForeign) {
		return container{}, fmt.Errorf("%s: %w", vm, backend.ErrNotFound)
	}
	return c, err
}

// absent checks Docker has no container by that name: one of devvm's is an
// error, and so is anyone else's (docker_container_foreign).
func (cl client) absent(ctx context.Context, name string) error {
	_, err := cl.inspect(ctx, name)
	switch {
	case errors.Is(err, backend.ErrNotFound):
		return nil
	case errors.Is(err, errForeign):
		return msg.DockerContainerForeign(name)
	case err != nil:
		return err
	}
	return fmt.Errorf("docker: %s exists already", name)
}

// stop stops a container.
func (cl client) stop(ctx context.Context, name string) error {
	if err := cl.Runner.Run(ctx, msg.Stopping(name), cl.docker(ctx, "container", "stop", "-t", "10", name)); err != nil {
		return backend.Logged(msg.StopFailed(name))
	}
	return nil
}

// removeContainer deletes one of devvm's containers, stopping it first.
func (cl client) removeContainer(ctx context.Context, step msg.M, name string) error {
	if err := cl.Runner.Run(ctx, step, cl.docker(ctx, "container", "rm", "-f", "-v", name)); err != nil {
		return backend.Logged(msg.DeleteFailed(name))
	}
	return nil
}

// removeIfThere deletes one of devvm's containers if Docker has it.
func (cl client) removeIfThere(ctx context.Context, name string) error {
	if _, err := cl.inspect(ctx, name); errors.Is(err, backend.ErrNotFound) {
		return nil
	} else if errors.Is(err, errForeign) {
		return msg.DockerContainerForeign(name)
	} else if err != nil {
		return err
	}
	return cl.removeContainer(ctx, msg.Deleting(name), name)
}

// volumeExists reports whether devvm's volume by that name is there; one
// of someone else's is refused (docker_volume_foreign).
func (cl client) volumeExists(ctx context.Context, name string) (bool, error) {
	out, err := cl.Runner.Output(cl.docker(ctx, "volume", "inspect", "--format", "{{json .Labels}}", name))
	if errMissing(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	labels := map[string]string{}
	if t := bytes.TrimSpace(out); len(t) > 0 && string(t) != "null" {
		if err := json.Unmarshal(t, &labels); err != nil {
			return false, fmt.Errorf("docker volume inspect: %w", err)
		}
	}
	if labels[LabelKey] != cl.Data {
		return false, msg.DockerVolumeForeign(name)
	}
	return true, nil
}

// createVolume makes a volume of devvm's, labelled.
func (cl client) createVolume(ctx context.Context, name string) error {
	if err := cl.Runner.Run(ctx, msg.M{}, cl.docker(ctx, "volume", "create", "--label", LabelKey+"="+cl.Data, name)); err != nil {
		return backend.Logged(msg.DockerVolumeFailed(name))
	}
	return nil
}

// ensureVolume makes a volume of devvm's unless it's there: Docker would
// make one a container names, unlabelled.
func (cl client) ensureVolume(ctx context.Context, name string) error {
	ok, err := cl.volumeExists(ctx, name)
	if err != nil || ok {
		return err
	}
	return cl.createVolume(ctx, name)
}

// removeVolume deletes one of devvm's volumes, if Docker has it.
func (cl client) removeVolume(ctx context.Context, name string) error {
	ok, err := cl.volumeExists(ctx, name)
	if err != nil || !ok {
		return err
	}
	if err := cl.Runner.Run(ctx, msg.M{}, cl.docker(ctx, "volume", "rm", name)); err != nil {
		return backend.Logged(msg.DockerVolumeFailed(name))
	}
	return nil
}

// dockerImage is an image as docker image inspect has it: the fields
// devvm reads.
type dockerImage struct {
	Size   int64 `json:"Size"`
	Config struct {
		Labels map[string]string `json:"Labels"`
		Env    []string          `json:"Env"`
	} `json:"Config"`
	RootFS struct {
		Layers []string `json:"Layers"`
	} `json:"RootFS"`
}

// inspectImage returns one of devvm's images by its tag: an error matching
// backend.ErrNotFound when Docker has none, or one matching errForeign
// when it isn't this devvm's.
func (cl client) inspectImage(ctx context.Context, ref string) (dockerImage, error) {
	out, err := cl.Runner.Output(cl.docker(ctx, "image", "inspect", ref))
	if errMissing(err) {
		return dockerImage{}, fmt.Errorf("%s: %w", ref, backend.ErrNotFound)
	} else if err != nil {
		return dockerImage{}, err
	}
	var is []dockerImage
	if err := json.Unmarshal(out, &is); err != nil || len(is) != 1 {
		return dockerImage{}, fmt.Errorf("docker image inspect %s: %d images: %w", ref, len(is), err)
	}
	if is[0].Config.Labels[LabelKey] != cl.Data {
		return dockerImage{}, fmt.Errorf("%s: %w", ref, errForeign)
	}
	return is[0], nil
}

// removeImage untags one of devvm's images, if Docker has it: Docker
// deletes what no other image or container uses.
func (cl client) removeImage(ctx context.Context, ref string) error {
	if _, err := cl.inspectImage(ctx, ref); errors.Is(err, backend.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if err := cl.Runner.Run(ctx, msg.M{}, cl.docker(ctx, "image", "rm", ref)); err != nil {
		return backend.Logged(msg.DockerImageRmFailed(ref))
	}
	return nil
}

// pull pulls an image by its digest: Docker keeps it, as any image it
// pulls, and devvm never deletes it.
func (cl client) pull(ctx context.Context, ref string) error {
	if err := cl.Runner.Run(ctx, msg.PullingImage(ref), cl.docker(ctx, "image", "pull", ref)); err != nil {
		return backend.Logged(msg.DockerPullFailed(ref))
	}
	return nil
}

// commit commits a stopped container as ref, labelled as devvm's of kind
// (and key, for an image): the container's own labels, devvm's data
// folder's among them, go with it.
func (cl client) commit(ctx context.Context, name, ref, kind, key string) error {
	args := []string{"container", "commit", "--change", "LABEL " + kindLabel + "=" + kind}
	if key != "" {
		args = append(args, "--change", "LABEL "+keyLabel+"="+key)
	}
	if err := cl.Runner.Run(ctx, msg.CommittingImage(name), cl.docker(ctx, append(args, name, ref)...)); err != nil {
		return backend.Logged(msg.DockerCommitFailed(name))
	}
	return nil
}

// asRoot is a command run as root in a running container, its input fed
// to sh: devvm's own scripts; with a build's proxy settings (execEnv).
func (cl client) asRoot(ctx context.Context, name string, env []string, args ...string) *exec.Cmd {
	a := append([]string{"exec", "-i", "-u", "0"}, execEnvArgs(ctx)...)
	for _, e := range env {
		a = append(a, "-e", e)
	}
	a = append(a, name, "sh", "-s", "--")
	return cl.docker(ctx, append(a, args...)...)
}
