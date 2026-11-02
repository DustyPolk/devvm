package docker

// Network policy (FeatureEgressProxy) and Docker Desktop's build
// confinement (FeatureBuildConfinement), both emulated.
//
// A VM with network policy is on an internal network of its own
// (netName): no route out, no address of the host's on it
// (inhibit_ipv4), no names resolved but its members'. Its only other
// member is its relay (relayName): a container of devvm's from an empty
// image devvm imports itself (proxyImage), running devvm's own Linux
// build, read-only, as devvm _proxy -relay, with no capabilities. The
// relay passes each connection the VM makes to it, unread, to the VM's
// proxy on the host, through a unix socket the proxy also listens on
// (ProxySocket), mounted into the relay alone: the proxy, devvm's CA and
// the VM's tokens stay on the host, as on other backends, and the proxy
// checks the VM's password and policy on each connection. The VM reaches
// the relay by its name on the network (devvm-proxy), which its proxy
// settings name; they go in its /etc/environment at each start. Network
// policy turned on or off moves the stopped container between that
// network and Docker's default one (docker network connect and
// disconnect) at its next start: what the container was made with stays.
//
// The proxy follows the VM's start (Lifeline): docker container wait on
// it, a host process started once it runs, which ends when it stops.
//
// A build on Docker Desktop, whose default network reaches the host's
// loopback, gets the same: a network and relay of its own, and a proxy
// of devvm's on the host for it alone, allowing any host on ports 80 and
// 443 but the host's own and internal addresses (the proxy's floor), which
// its commands are told of in an env file (execEnv), never in their
// arguments. So does a build on any engine behind a company proxy: the
// build's proxy goes through it, and the build gets only its own proxy's
// address and password, never the company proxy's.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/shell"
	"github.com/w3cj/devvm/internal/state"
)

const (
	// kindProxy is a VM's or build's own network, its relay, and the
	// relay's empty image.
	kindProxy = "proxy"
	// proxyAlias is the relay's name on its network, which the proxy
	// settings name.
	proxyAlias = "devvm-proxy"
	// relayBin is where the relay has devvm's Linux build; relayDir the
	// folder of the proxy's socket, relaySock.
	relayBin  = "/devvm"
	relayDir  = "/run/devvm"
	relaySock = relayDir + "/proxy.sock"
	// buildPort is a build's relay's port.
	buildPort = 3128
	// inhibitIPv4 is the bridge driver's option that gives the host no
	// address on the network.
	inhibitIPv4 = "com.docker.network.bridge.inhibit_ipv4"
	// proxyUser is the user in the proxy's address, as netpolicy names it.
	proxyUser = "devvm"
)

// proxyVars are the variables that name the proxy.
func proxyVars() []string { return []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} }

// buildEnv is what a build's commands get besides the proxy's address:
// no proxy for the container itself, and Node told to use the proxy
// variables.
func buildEnv() map[string]string {
	return map[string]string{
		"NO_PROXY": "localhost,127.0.0.1,::1", "no_proxy": "localhost,127.0.0.1,::1", "NODE_USE_ENV_PROXY": "1",
	}
}

// sudoKeep is the sudoers line that keeps the proxy settings for sudo in a
// build's container while its scripts run.
const sudoKeep = `Defaults env_keep += "http_proxy https_proxy no_proxy HTTP_PROXY HTTPS_PROXY NO_PROXY NODE_USE_ENV_PROXY"`

// netName is a VM's or build's own network, relayName its relay.
func netName(name string) string   { return name + "-net" }
func relayName(name string) string { return name + "-proxy" }

// proxySocket is the socket of the proxy for a VM or build (name): in a
// folder of its own, named by a hash, so the path stays within the limit
// on sockets' paths.
func proxySocket(d paths.Dirs, name string) string {
	sum := sha256.Sum256([]byte(name))
	// Named as the relay has it, whose folder a Linux engine mounts.
	return filepath.Join(d.Docker(), "sock", hex.EncodeToString(sum[:8]), filepath.Base(relaySock))
}

// proxyURL is the proxy's address as the relay serves it on port, with
// its user and password.
func proxyURL(port int, password string) string {
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(proxyAlias, strconv.Itoa(port)), User: url.UserPassword(proxyUser, password)}
	return u.String()
}

// execEnvKey is the context key of a build's env file.
type execEnvKey struct{}

// withExecEnv is ctx whose docker exec commands get the variables in the
// env file at path (a build's proxy settings).
func withExecEnv(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, execEnvKey{}, path)
}

// execEnvArgs are docker exec's options that give a command ctx's env
// file; none without one.
func execEnvArgs(ctx context.Context) []string {
	if f, ok := ctx.Value(execEnvKey{}).(string); ok && f != "" {
		return []string{"--env-file", f}
	}
	return nil
}

// network is a network as docker network inspect has it: the fields devvm
// reads.
type network struct {
	ID         string            `json:"Id"`
	Driver     string            `json:"Driver"`
	Internal   bool              `json:"Internal"`
	EnableIPv6 bool              `json:"EnableIPv6"`
	Options    map[string]string `json:"Options"`
	Labels     map[string]string `json:"Labels"`
	// Containers are its members, running containers, by ID.
	Containers map[string]struct{} `json:"Containers"`
}

// confining reports whether the network has no route out and no address
// of the host's.
func (n network) confining() bool { return n.Internal && n.Options[inhibitIPv4] == "true" }

// plain reports whether the network is one as devvm makes a docker layer
// VM's: a bridge, not internal, without IPv6 or options (another gateway
// mode, no isolation).
func (n network) plain() bool {
	return n.Driver == "bridge" && !n.Internal && !n.EnableIPv6 && len(n.Options) == 0
}

// egress is the Docker backend's network policy and Desktop's build
// confinement.
type egress struct{ d *Docker }

// network returns devvm's network by that name: an error matching
// backend.ErrNotFound when Docker has none, or errForeign when it isn't
// this devvm's.
func (e egress) network(ctx context.Context, name string) (network, error) {
	out, err := e.d.Runner.Output(e.d.docker(ctx, "network", "inspect", name))
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok && bytes.Contains(ee.Stderr, []byte(" not found")) {
			return network{}, fmt.Errorf("%s: %w", name, backend.ErrNotFound)
		}
		return network{}, err
	}
	var ns []network
	if err := json.Unmarshal(out, &ns); err != nil || len(ns) != 1 {
		return network{}, fmt.Errorf("docker network inspect %s: %d networks: %w", name, len(ns), err)
	}
	if ns[0].Labels[LabelKey] != e.d.Data {
		return network{}, fmt.Errorf("%s: %w", name, errForeign)
	}
	return ns[0], nil
}

// ensureNetwork makes devvm's network by that name unless one as asked for
// is there (another is made again), and returns its ID: internal, one that
// confines (network policy's, a build's); else a plain one (a docker layer
// VM's, dockerlayer.go). Someone else's by that name is refused.
func (e egress) ensureNetwork(ctx context.Context, name string, internal bool) (string, error) {
	n, err := e.network(ctx, name)
	switch {
	case errors.Is(err, errForeign):
		return "", msg.DockerNetworkForeign(name)
	case err == nil && internal && n.confining(), err == nil && !internal && n.plain():
		return n.ID, nil
	case err == nil:
		if err := e.removeNetwork(ctx, name); err != nil {
			return "", err
		}
	case !errors.Is(err, backend.ErrNotFound):
		return "", err
	}
	args, kind := []string{"network", "create"}, kindVM
	if internal {
		args, kind = append(args, "--internal", "-o", inhibitIPv4+"=true"), kindProxy
	}
	args = append(args, "--label", LabelKey+"="+e.d.Data, "--label", kindLabel+"="+kind, name)
	if err := e.d.Runner.Run(ctx, msg.M{}, e.d.docker(ctx, args...)); err != nil {
		return "", backend.Logged(msg.DockerNetworkFailed(name))
	}
	if n, err = e.network(ctx, name); err != nil {
		return "", err
	}
	return n.ID, nil
}

// removeNetwork deletes devvm's network by that name, if Docker has it,
// once what's on it is off: one a container devvm didn't put there still
// runs on is refused, naming it.
func (e egress) removeNetwork(ctx context.Context, name string) error {
	n, err := e.network(ctx, name)
	switch {
	case errors.Is(err, backend.ErrNotFound):
		return nil
	case errors.Is(err, errForeign):
		return msg.DockerNetworkForeign(name)
	case err != nil:
		return err
	case len(n.Containers) > 0:
		return msg.DockerNetworkShared(name)
	}
	if err := e.d.Runner.Run(ctx, msg.M{}, e.d.docker(ctx, "network", "rm", name)); err != nil {
		return backend.Logged(msg.DockerNetworkFailed(name))
	}
	return nil
}

// useNetwork leaves a stopped container on the network want alone.
func (e egress) useNetwork(ctx context.Context, name, want string) error {
	c, err := e.d.inspect(ctx, name)
	if err != nil {
		return err
	}
	for _, n := range slices.Sorted(maps.Keys(c.NetworkSettings.Networks)) {
		if n == want {
			continue
		}
		if err := e.d.Runner.Run(ctx, msg.Configuring(name), e.d.docker(ctx, "network", "disconnect", n, name)); err != nil {
			return backend.Logged(msg.DockerNetworkFailed(n))
		}
	}
	if _, ok := c.NetworkSettings.Networks[want]; ok {
		return nil
	}
	if err := e.d.Runner.Run(ctx, msg.Configuring(name), e.d.docker(ctx, "network", "connect", want, name)); err != nil {
		return backend.Logged(msg.DockerNetworkFailed(want))
	}
	return nil
}

// vmNetwork puts a stopped VM on the network c asks for: with NetProxy,
// its own network alone, with its relay made again for c's port (the
// record keeps the network's and the relay's IDs); else Docker's default
// network alone (a docker layer VM's own, layerNetwork), its relay and own
// network gone.
func (e egress) vmNetwork(ctx context.Context, vm string, r *record, c backend.Confinement) error {
	if c.Net != backend.NetProxy {
		if err := e.d.removeIfThere(ctx, relayName(vm)); err != nil {
			return err
		}
		if err := e.useNetwork(ctx, vm, directNet(vm, *r)); err != nil {
			return err
		}
		r.NetID, r.RelayID = "", ""
		return e.removeNetwork(ctx, netName(vm))
	}
	id, err := e.ownNetwork(ctx, vm)
	if err != nil {
		return err
	}
	if err := e.useNetwork(ctx, vm, netName(vm)); err != nil {
		return err
	}
	relay, err := e.startRelay(ctx, vm, c.ProxyPort)
	if err != nil {
		return err
	}
	r.NetID, r.RelayID = id, relay
	return nil
}

// ownNetwork readies the network of a VM or build (name) before its
// container starts (ensureNetwork), its old relay deleted, and returns
// its ID: one with anything else on it is refused, naming it.
func (e egress) ownNetwork(ctx context.Context, name string) (string, error) {
	id, err := e.ensureNetwork(ctx, netName(name), true)
	if err != nil {
		return "", err
	}
	if err := e.d.removeIfThere(ctx, relayName(name)); err != nil {
		return "", err
	}
	n, err := e.network(ctx, netName(name))
	if err != nil {
		return "", err
	}
	if len(n.Containers) > 0 {
		return "", msg.DockerNetworkShared(netName(name))
	}
	return id, nil
}

// startRelay makes the relay of a VM or build (name) again and starts it,
// on its network, listening on port: devvm's Linux build, read-only, in
// the empty image, read-only too, with no capabilities, and the proxy's
// socket (socketMount). It returns the relay's container's ID.
func (e egress) startRelay(ctx context.Context, name string, port int) (string, error) {
	bin, err := e.linuxBuild(ctx)
	if err != nil {
		return "", err
	}
	binMount, err := bindAt(bin, relayBin, false)
	if err != nil {
		return "", err
	}
	img, err := e.proxyImage(ctx)
	if err != nil {
		return "", err
	}
	sock, user, err := e.socketMount(proxySocket(e.d.Dirs, name))
	if err != nil {
		return "", err
	}
	relay := relayName(name)
	if err := e.d.removeIfThere(ctx, relay); err != nil {
		return "", err
	}
	args := []string{
		"container", "create", "--name", relay,
		"--label", LabelKey + "=" + e.d.Data, "--label", kindLabel + "=" + kindProxy,
		"--network", netName(name), "--network-alias", proxyAlias,
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "64", "--memory", "128m", "--memory-swap", "128m", "--user", user,
		"--mount", binMount,
	}
	args = append(append(append(args, noClientProxy()...), sock...), "--entrypoint", relayBin, img,
		"_proxy", "-relay", "0.0.0.0:"+strconv.Itoa(port), "-relay-to", relaySock)
	if err := e.d.Runner.Run(ctx, msg.M{}, e.d.docker(ctx, args...)); err != nil {
		return "", backend.Logged(msg.StartFailed(relay))
	}
	c, err := e.d.inspect(ctx, relay)
	if err != nil {
		return "", err
	}
	if err := e.d.Runner.Run(ctx, msg.M{}, e.d.docker(ctx, "container", "start", c.ID)); err != nil {
		return "", backend.Logged(msg.StartFailed(relay))
	}
	return c.ID, nil
}

// socketMount is how the relay gets the proxy's socket, and the user it
// runs as: on Docker Desktop the socket alone (-v: it follows the path at
// each connection, as root's), on a Linux engine its folder (a socket
// mounted alone stays the one that was there), as the host's user.
func (e egress) socketMount(sock string) ([]string, string, error) {
	switch e.d.engineKind() {
	case kindDesktop:
		if strings.ContainsAny(sock, ":,\"\n") {
			return nil, "", msg.DockerMountPath(sock)
		}
		if err := e.d.checkShared(sock); err != nil {
			return nil, "", err
		}
		return []string{"-v", sock + ":" + relaySock}, "0:0", nil
	case kindLinux:
		m, err := bindAt(filepath.Dir(sock), relayDir, false)
		if err != nil {
			return nil, "", err
		}
		_, uid, gid, err := e.d.user()
		if err != nil {
			return nil, "", err
		}
		return []string{"--mount", m}, uid + ":" + gid, nil
	}
	return nil, "", errors.New("docker: no relay for devvm's proxy on this engine")
}

// proxyImage is the relay's image, imported from an empty archive unless
// Docker has it: nothing in it, nothing downloaded.
func (e egress) proxyImage(ctx context.Context) (string, error) {
	ref := e.d.imageRef(kindProxy)
	if _, err := e.d.inspectImage(ctx, ref); err == nil {
		return ref, nil
	} else if !errors.Is(err, backend.ErrNotFound) {
		return "", err
	}
	cmd := e.d.docker(ctx, "image", "import",
		"--change", "LABEL "+LabelKey+"="+strconv.Quote(e.d.Data), "--change", "LABEL "+kindLabel+"="+kindProxy, "-", ref)
	cmd.Stdin = strings.NewReader(strings.Repeat("\x00", 1024)) // an empty tar archive
	if err := e.d.Runner.Run(ctx, msg.M{}, cmd); err != nil {
		return "", backend.Logged(msg.DockerRelayImageFailed())
	}
	return ref, nil
}

// linuxBuild is devvm's Linux build (Docker's Linux, linux.go: fetched
// there when it's the release's), copied into devvm's data folder, where
// Docker Desktop can mount it, when the copy there differs.
func (e egress) linuxBuild(ctx context.Context) (string, error) {
	src, err := e.d.Linux(ctx, true)
	if err != nil {
		return "", err
	}
	dst := linuxCopy(e.d.Dirs, e.d.Arch)
	data, err := os.ReadFile(src) //nolint:gosec // devvm's own build
	if err != nil {
		return "", err
	}
	want := sha256.Sum256(data)
	if have, err := fileSum(dst); err != nil || have != hex.EncodeToString(want[:]) {
		if err := state.WriteFileMode(dst, data, 0o700); err != nil {
			return "", err
		}
	}
	if err := e.d.checkShared(dst); err != nil {
		return "", err
	}
	return dst, nil
}

var envValue = regexp.MustCompile(`^[A-Za-z0-9_./:@,=+-]*$`)

// envScript replaces devvm's block in /etc/environment with its input.
const envScript = `set -eu; f=/etc/environment; t=$(mktemp); ` +
	`if [ -f "$f" ]; then sed '/^# devvm-proxy begin$/,/^# devvm-proxy end$/d' "$f" > "$t"; fi; ` +
	`cat >> "$t"; cat "$t" > "$f"; rm -f "$t"`

// setEnv puts env in a running VM's /etc/environment, which devvm's
// commands, its ssh sessions and its shell read, in place of what devvm
// put there before; none takes it out. Fed on the input, so its values
// (the proxy's password) are in no process's arguments.
func (e egress) setEnv(ctx context.Context, vm string, env map[string]string) error {
	var b strings.Builder
	if len(env) > 0 {
		b.WriteString("# devvm-proxy begin\n")
		for _, k := range slices.Sorted(maps.Keys(env)) {
			if !shell.EnvName(k) || !envValue.MatchString(env[k]) {
				return fmt.Errorf("docker: %s can't go in %s's /etc/environment", k, vm)
			}
			b.WriteString(k + "=" + env[k] + "\n")
		}
		b.WriteString("# devvm-proxy end\n")
	}
	cmd := e.d.docker(ctx, "exec", "-i", "-u", "0", vm, "sh", "-c", envScript)
	cmd.Stdin = strings.NewReader(b.String())
	if err := e.d.Runner.Run(ctx, msg.M{}, cmd); err != nil {
		return backend.Logged(msg.StartFailed(vm))
	}
	return nil
}

// proxyEnv is what a VM started with c gets in its environment: c's, and
// with NetProxy the proxy's address.
func proxyEnv(c backend.Confinement) map[string]string {
	env := maps.Clone(c.Env)
	if c.Net != backend.NetProxy {
		return env
	}
	if env == nil {
		env = map[string]string{}
	}
	for _, k := range proxyVars() {
		env[k] = proxyURL(c.ProxyPort, c.ProxyPassword)
	}
	return env
}

// follow starts a host process that ends when the container (its ID)
// stops, docker container wait, and writes its pid to pidFile: the
// lifeline its proxy follows.
func (e egress) follow(ctx context.Context, id, pidFile string) error {
	docker, err := e.d.LookPath("docker")
	if err != nil {
		return err
	}
	out, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	pid, err := e.d.Detach(daemon.Cmd{Path: docker, Args: []string{"container", "wait", id}, Out: out})
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return state.WriteFile(pidFile, []byte(strconv.Itoa(pid)+"\n"))
}

// confined reports whether a running VM runs as its last start through
// devvm's proxy left it: the same start of its container, on its own
// network alone, which still confines, with nothing on it but the VM and
// the relay that start made, both by ID.
func (e egress) confined(ctx context.Context, vm string) (bool, error) {
	r, err := e.d.readRecord(vm)
	if err != nil || r.Confined == "" {
		return false, err
	}
	c, err := e.d.vm(ctx, vm)
	if err != nil {
		return false, err
	}
	if c.status() != backend.StatusRunning || c.ID+" "+c.State.StartedAt != r.Confined {
		return false, nil
	}
	if nets := c.NetworkSettings.Networks; len(nets) != 1 || nets[netName(vm)].NetworkID != r.NetID {
		return false, nil
	}
	n, err := e.network(ctx, netName(vm))
	if errors.Is(err, backend.ErrNotFound) || errors.Is(err, errForeign) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if n.ID != r.NetID || !n.confining() {
		return false, nil
	}
	for id := range n.Containers {
		if id != c.ID && (r.RelayID == "" || id != r.RelayID) {
			return false, nil
		}
	}
	return true, nil
}

// stopRelay stops the relay of a VM or build (name), if it runs.
func (e egress) stopRelay(ctx context.Context, name string) error {
	c, err := e.d.inspect(ctx, relayName(name))
	if errors.Is(err, backend.ErrNotFound) || errors.Is(err, errForeign) {
		return nil
	} else if err != nil {
		return err
	}
	if c.status() != backend.StatusRunning {
		return nil
	}
	return e.d.stop(ctx, relayName(name))
}

// checkAlone refuses to delete a VM (its container's ID) while a network
// of its own has a container on it but the VM and its relay, naming the
// network, before anything is deleted.
func (e egress) checkAlone(ctx context.Context, vm, id string) error {
	ours := map[string]bool{id: true}
	if r, err := e.d.inspect(ctx, relayName(vm)); err == nil {
		ours[r.ID] = true
	}
	for _, name := range []string{netName(vm), layerNetName(vm)} {
		n, err := e.network(ctx, name)
		if errors.Is(err, backend.ErrNotFound) || errors.Is(err, errForeign) {
			continue // removeNetwork says so
		} else if err != nil {
			return err
		}
		for m := range n.Containers {
			if !ours[m] {
				return msg.DockerNetworkShared(name)
			}
		}
	}
	return nil
}

// forget deletes what network policy left of a VM by that name (its relay,
// its network, its lifeline and its socket's folder) and a docker layer
// VM's own network.
func (e egress) forget(ctx context.Context, vm string) error {
	if err := e.d.removeIfThere(ctx, relayName(vm)); err != nil {
		return err
	}
	for _, n := range []string{netName(vm), layerNetName(vm)} {
		if err := e.removeNetwork(ctx, n); err != nil {
			return err
		}
	}
	if err := os.Remove(e.d.Lifeline(ctx, vm)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.RemoveAll(filepath.Dir(proxySocket(e.d.Dirs, vm)))
}

// buildNet readies the network of a build's container (name) with n: with
// NetInternet on Docker Desktop, or behind a company proxy (the host's
// HTTPS_PROXY or HTTP_PROXY) on any engine, a network of its own, its
// relay and a proxy of devvm's for it (backend.ProxyStarter), through the
// company proxy, whose address with the build's own password the returned
// context's commands get; else Docker's own (netMode), and no proxy.
// lifeline is where the proxy's lifeline goes once the container runs
// (follow), "" for none; done undoes the rest, the container left, off
// the network: a failed build's, kept, keeps its proxy's logs (proxy.log,
// events.jsonl) in its folder.
func (e egress) buildNet(ctx context.Context, name string, n backend.Net) (context.Context, string, string, func(context.Context), error) {
	dir := filepath.Join(e.d.Dirs.Docker(), "builds", name)
	host := backend.ReadHostProxy(e.d.Getenv)
	if n != backend.NetInternet || e.d.engineKind() != kindDesktop && host.None() {
		return ctx, netMode(n), "", func(context.Context) {}, nil
	}
	var proxy *backend.BuildProxy
	done := func(ctx context.Context) {
		proxy.Stop()
		_ = e.d.removeIfThere(ctx, relayName(name))
		// A failed build's container, kept, keeps its proxy's logs too.
		_, err := e.d.inspect(ctx, name)
		kept := err == nil
		if kept {
			_ = e.d.Runner.Run(ctx, msg.M{}, e.d.docker(ctx, "network", "disconnect", "-f", netName(name), name))
		}
		_ = e.removeNetwork(ctx, netName(name))
		for _, f := range []string{"auth", "env", "policy", "lifeline", "upstream"} {
			_ = os.Remove(filepath.Join(dir, f))
		}
		if !kept {
			_ = os.RemoveAll(dir)
		}
		_ = os.RemoveAll(filepath.Dir(proxySocket(e.d.Dirs, name)))
	}
	fail := func(err error) (context.Context, string, string, func(context.Context), error) {
		done(ctx)
		return ctx, "", "", func(context.Context) {}, err
	}
	// The host's proxy, which devvm's goes through.
	upstream, err := host.Upstream()
	if err != nil {
		return fail(err)
	}
	sock := proxySocket(e.d.Dirs, name)
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		return fail(err)
	}
	starter := backend.ProxyStarter{Self: e.d.Self, Spawn: e.d.Spawn, Tick: e.d.Tick}
	extra := []string{"-listen-unix", sock}
	if e.d.engineKind() != kindDesktop {
		// Without a company proxy, a Linux engine's build is on Docker's own
		// network, reaching internal addresses directly, so names this
		// machine can't resolve go on to the company proxy; Desktop's builds
		// are kept off this machine's own.
		extra = append(extra, "-upstream-unresolved")
	}
	if proxy, err = starter.Start(name, dir, filepath.Join(dir, "lifeline"), upstream, extra...); err != nil {
		return fail(err)
	}
	env := buildEnv()
	for _, k := range proxyVars() {
		env[k] = proxyURL(buildPort, proxy.Password)
	}
	var lines []byte
	for _, k := range slices.Sorted(maps.Keys(env)) {
		lines = append(lines, k+"="+env[k]+"\n"...)
	}
	envFile := filepath.Join(dir, "env")
	if err := state.WriteFile(envFile, lines); err != nil {
		return fail(err)
	}
	if _, err := e.ownNetwork(ctx, name); err != nil {
		return fail(err)
	}
	if _, err := e.startRelay(ctx, name, buildPort); err != nil {
		return fail(err)
	}
	return withExecEnv(ctx, envFile), netName(name), filepath.Join(dir, "lifeline"), done, nil
}

// buildCaps are the capabilities the container of a build (name) on the
// network netw is made without: on a network of its own (buildNet's), NET_RAW,
// as a VM's, so no packet socket reaches its bridge's IPv6 address.
func buildCaps(name, netw string) []string {
	if netw == netName(name) {
		return []string{"NET_RAW"}
	}
	return nil
}

// buildSudo keeps a build's proxy settings for sudo in its container
// while its scripts run (on), and takes that out again before it's
// committed (off); nothing without them.
func (e egress) buildSudo(ctx context.Context, name string, on bool) error {
	if execEnvArgs(ctx) == nil {
		return nil
	}
	script := "rm -f /etc/sudoers.d/devvm-build-proxy"
	if on {
		script = "printf '%s\\n' '" + sudoKeep + "' > /etc/sudoers.d/devvm-build-proxy && chmod 440 /etc/sudoers.d/devvm-build-proxy"
	}
	if err := e.d.Runner.Run(ctx, msg.M{}, e.d.docker(ctx, "exec", "-u", "0", name, "sh", "-c", script)); err != nil {
		return backend.Logged(msg.DockerScriptFailed("sudoers", name))
	}
	return nil
}
