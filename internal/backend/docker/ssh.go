package docker

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/shell"
	"github.com/w3cj/devvm/internal/state"
)

// ssh for editors is emulated, and reviewed: a container runs no sshd,
// and publishes nothing, so devvm's ssh reaches a VM's sshd through Docker
// itself (Confinement.SSH asks for it at each start):
//
//   - base has openssh-server, its host keys deleted (sshd.sh); after each
//     start services.sh makes the VM new ones (ed25519) and writes devvm's
//     sshd config (the VM's user alone, with the VM's own key alone, no
//     passwords; local forwarding only), and starts no daemon
//     (DEVVM_SSHD=inetd): nothing in the container listens for ssh;
//   - each ssh connection is one sshd of its own in inetd mode (sshd -i),
//     run as root in the container by docker exec, its standard input and
//     output the connection: devvm _docker-connect (Connect), ssh's
//     ProxyCommand in devvm's ssh config and its own connections. It
//     checks the container is this devvm's, a VM's, and running, then
//     runs docker exec on its ID;
//   - the host key is read through docker exec at each start and pinned,
//     as on the other backends;
//   - SSH answers only once this start's sshd config is written
//     (record.SSHUp) and the container still runs that start
//     (record.Started), so nothing reads host keys a start is about to
//     replace, and a container started outside devvm has no entry. A VM
//     whose image has no sshd starts without ssh, warned.

// What the connection runs in the container: the image's sshd, on devvm's
// config, logging to a file of its own.
const (
	sshdPath   = "/usr/sbin/sshd"
	sshdConfig = "/etc/devvm/ssh/sshd_config"
	sshdLog    = "/var/log/devvm-sshd.log"
)

// ConnectCmd is the hidden command that connects ssh to a VM's sshd
// (Connect).
const ConnectCmd = "_docker-connect"

// sshdProbe answers "sshd" when the container's image has the sshd the
// connection runs, else "none".
const sshdProbe = `if [ -x ` + sshdPath + ` ]; then echo sshd; else echo none; fi`

// sshKey is the key devvm's ssh logs in to a VM with, made at its first
// start with ssh (vmssh.EnsureKey); knownHosts is its sshd's host key, as
// its last start read it, which devvm's own sessions pin. Both are in its
// backend folder.
func (d *Docker) sshKey(vm string) string     { return d.Dirs.VM(vm).Key() }
func (d *Docker) knownHosts(vm string) string { return d.Dirs.VM(vm).KnownHosts() }

// connectEnv are the variables of the user's that name Docker's engine,
// which a connection is given as devvm had them: an editor may start
// without them.
var connectEnv = []string{"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG"}

// proxyCommand is ssh's ProxyCommand for a VM: devvm _docker-connect, with
// docker's path and the engine devvm reaches, its words quoted for the
// shell (sh, or the user's: ssh runs it with SHELL).
func (d *Docker) proxyCommand(vm string) (string, error) {
	docker, err := d.LookPath("docker")
	if err != nil {
		return "", err
	}
	if docker, err = filepath.Abs(docker); err != nil {
		return "", err
	}
	args := []string{d.Self, ConnectCmd, "-docker", docker, "-data", d.Data}
	for _, k := range connectEnv {
		if v := d.Getenv(k); v != "" {
			args = append(args, "-env", k+"="+v)
		}
	}
	return shellWords(append(args, vm))
}

// shellWords quotes each word in single quotes for a shell. A word with a
// quote, a backslash, a percent sign (ssh's tokens) or a control character
// is refused: fish reads backslashes in single quotes too.
func shellWords(words []string) (string, error) {
	quoted := make([]string, len(words))
	for i, w := range words {
		if w == "" || strings.ContainsAny(w, `'\%`) || strings.ContainsFunc(w, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return "", msg.DockerSSHPath(w)
		}
		quoted[i] = "'" + w + "'"
	}
	return strings.Join(quoted, " "), nil
}

// lacksSSHD reports whether a running container's image has no sshd (made
// before devvm installed one), asked before services.sh runs, so no other
// failure reads as it; one that can't say stops it.
func (d *Docker) lacksSSHD(ctx context.Context, vm string) (bool, error) {
	out, err := d.vmOutput(ctx, vm, "sh", "-c", sshdProbe)
	switch strings.TrimSpace(string(out)) {
	case "none":
		return true, nil
	case "sshd":
		if err == nil {
			return false, nil
		}
	}
	if ctx.Err() == nil {
		_ = d.stop(ctx, vm)
	}
	return false, backend.Logged(msg.StartFailed(vm))
}

// What devvm reads of what the VM prints, and how long it waits for it:
// root in the VM decides both.
const (
	vmOutputMax     = 64 << 10
	vmOutputTimeout = 30 * time.Second
)

// vmOutput runs docker with args, as root in the VM, a command whose
// output the VM makes: it returns at most vmOutputMax bytes of it and
// waits vmOutputTimeout at most.
func (d *Docker) vmOutput(ctx context.Context, vm string, argv ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, vmOutputTimeout)
	defer cancel()
	out := &vmssh.Capped{Max: vmOutputMax}
	cmd := d.docker(ctx, append([]string{"exec", "-u", "0", vm}, argv...)...)
	cmd.Stdout, cmd.WaitDelay = out, time.Second
	err := d.Runner.Attach(cmd)
	return out.Bytes(), err
}

// sshdArgs gets a stopped VM ready for this start's sshd when c asks for
// ssh: services.sh's arguments (the user and the VM's own key), or nil
// for none. A container whose
// image has no sshd gets none, warned.
func (d *Docker) sshdArgs(ctx context.Context, vm string, r *record, c backend.Confinement) ([]string, error) {
	if !c.SSH {
		return nil, nil
	}
	noSSHD, err := d.lacksSSHD(ctx, vm)
	if err != nil {
		return nil, err
	}
	if r.NoSSHD = noSSHD; noSSHD {
		d.Warn(msg.VMNoSshd(vm))
		return nil, nil
	}
	pub, err := vmssh.EnsureKey(d.sshKey(vm))
	if err != nil {
		return nil, err
	}
	name, _, _, err := d.user()
	if err != nil {
		return nil, err
	}
	// Its sessions' environment is /etc/environment's, where Start puts c's
	// (egress.go): no variables of services.sh's own.
	return vmssh.ServicesArgs(name, pub, nil, nil), nil
}

// pinHostKey reads the VM's sshd's host key, which services.sh just made,
// through docker exec, into its known hosts file, returning it (TYPE
// BASE64); a failure stops the VM.
func (d *Docker) pinHostKey(ctx context.Context, vm string) (string, error) {
	out, err := d.vmOutput(ctx, vm, "cat", vmssh.HostKeyPath)
	key, ok := vmssh.HostKey(out)
	if err == nil && ok {
		err = state.WriteFile(d.knownHosts(vm), []byte(vm+" "+key+"\n"))
	}
	if err != nil || !ok {
		if ctx.Err() == nil {
			_ = d.stop(ctx, vm)
		}
		return "", backend.Logged(msg.StartFailed(vm))
	}
	return key, nil
}

// sshReady returns a running VM whose start ran its sshd, and its record:
// vm_no_ssh for one started without it (or outside devvm since).
func (d *Docker) sshReady(ctx context.Context, vm string) (container, record, error) {
	c, err := d.vm(ctx, vm)
	if err != nil {
		return container{}, record{}, err
	}
	if c.status() != backend.StatusRunning {
		return container{}, record{}, msg.VMNotRunning(vm)
	}
	r, err := d.readRecord(vm)
	switch {
	case err != nil:
		return container{}, record{}, err
	case r.NoSSHD:
		return container{}, record{}, msg.VMNoSshd(vm)
	case !r.SSHUp || r.Started == "" || r.Started != c.State.StartedAt:
		return container{}, record{}, msg.VMNoSSH(vm)
	}
	return c, r, nil
}

// SSH returns how ssh reaches a running VM whose start ran its sshd:
// through devvm _docker-connect, as the VM's user, with devvm's key alone.
func (d *Docker) SSH(ctx context.Context, vm string) (backend.SSHTarget, error) {
	if _, _, err := d.sshReady(ctx, vm); err != nil {
		return backend.SSHTarget{}, err
	}
	proxy, err := d.proxyCommand(vm)
	if err != nil {
		return backend.SSHTarget{}, err
	}
	name, _, _, err := d.user()
	if err != nil {
		return backend.SSHTarget{}, err
	}
	return backend.SSHTarget{
		Host: vm, User: name, ProxyCommand: proxy,
		Options: []backend.SSHOption{
			{Key: "IdentityFile", Value: `"` + d.sshKey(vm) + `"`},
			{Key: "IdentitiesOnly", Value: "yes"},
			{Key: "PreferredAuthentications", Value: "publickey"},
		},
	}, nil
}

// transport is how devvm's own ssh reaches a VM whose start ran its sshd.
func (d *Docker) transport(vm string) (vmssh.SSH, error) {
	proxy, err := d.proxyCommand(vm)
	if err != nil {
		return vmssh.SSH{}, err
	}
	name, _, _, err := d.user()
	if err != nil {
		return vmssh.SSH{}, err
	}
	return vmssh.SSH{VM: vm, User: name, Key: d.sshKey(vm), KnownHosts: d.knownHosts(vm), Proxy: proxy}, nil
}

// sshShell returns ssh's arguments for a login shell in dir, over the
// VM's sshd, forwarding the agent its start chose (record.Agent); false
// for a VM without ssh this start.
func (d *Docker) sshShell(ctx context.Context, vm, dir string) ([]string, bool, error) {
	if _, _, err := d.sshReady(ctx, vm); err != nil {
		var m msg.M
		if errors.As(err, &m) {
			return nil, false, nil
		}
		return nil, false, err
	}
	r, err := d.readRecord(vm)
	if err != nil {
		return nil, false, err
	}
	t, err := d.transport(vm)
	if err != nil {
		return nil, false, err
	}
	if dir == "" {
		dir = "/"
	}
	return t.SessionArgs(r.Agent, d.Terminal(), "cd -- "+shell.Quote(dir)+" && exec bash -l"), true, nil
}

// Connect connects its standard input and output to a new sshd in a VM's
// container (devvm _docker-connect, ssh's ProxyCommand), with its
// arguments: -docker, docker's path; -data, devvm's data folder, which the
// container's label must name; -env KEY=VALUE, Docker's own variables
// (connectEnv) as devvm had them; the VM. It checks the container is this
// devvm's, a VM's and running, then runs, in its place (run), docker exec
// on the container's ID: sshd in inetd mode as root, on devvm's config.
func Connect(ctx context.Context, args []string, run func(path string, argv, env []string) error) error {
	fs := flag.NewFlagSet(ConnectCmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var docker, data string
	var envs []string
	fs.StringVar(&docker, "docker", "", "docker's path")
	fs.StringVar(&data, "data", "", "devvm's data folder")
	fs.Func("env", "Docker's variables, KEY=VALUE", func(v string) error {
		if k, _, ok := strings.Cut(v, "="); !ok || !slices.Contains(connectEnv, k) {
			return errors.New("-env " + v + ": not one of Docker's variables")
		}
		envs = append(envs, v)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return msg.DockerConnectFailed(err.Error(), "")
	}
	vm := fs.Arg(0)
	if fs.NArg() != 1 || docker == "" || data == "" {
		return msg.DockerConnectFailed("-docker, -data and the VM are needed", vm)
	}
	env := slices.DeleteFunc(os.Environ(), func(e string) bool {
		k, _, _ := strings.Cut(e, "=")
		return slices.Contains(connectEnv, k)
	})
	env = append(env, envs...)
	cmd := exec.CommandContext(ctx, docker, "container", "inspect", vm) //nolint:gosec // docker, as devvm found it
	cmd.Env = env
	out, err := cmd.Output()
	var cs []container
	if err == nil {
		err = json.Unmarshal(out, &cs)
	}
	switch {
	case errMissing(err) || err == nil && len(cs) == 0:
		return msg.DockerConnectFailed("Docker has no container by that name", vm)
	case err != nil:
		return msg.DockerConnectFailed(err.Error(), vm).Because(err)
	case len(cs) != 1 || !cs[0].ours(data) || cs[0].Config.Labels[kindLabel] != kindVM || cs[0].name() != vm || cs[0].ID == "":
		return msg.DockerContainerForeign(vm)
	case cs[0].status() != backend.StatusRunning:
		return msg.VMNotRunning(vm)
	}
	return run(docker, []string{docker, "exec", "-i", "-u", "0", cs[0].ID, sshdPath, "-i", "-f", sshdConfig, "-E", sshdLog}, env)
}
