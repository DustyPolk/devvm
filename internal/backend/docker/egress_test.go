package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// proxied is a start through devvm's proxy.
func proxied() backend.Confinement {
	return backend.Confinement{
		Net: backend.NetProxy, ProxyPort: 41000, ProxyUser: "devvm", ProxyPassword: "k3y",
		Env: map[string]string{"NO_PROXY": "localhost"},
	}
}

// stub runs the docker stub as someone on the host would.
func (s *sandbox) stubDocker(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.CommandContext(t.Context(), "docker", args...).CombinedOutput(); err != nil { //nolint:gosec // the stub, with the test's arguments

		t.Fatalf("docker %v: %v %s", args, err, out)
	}
}

// inOrder checks calls has each prefix, in order, and returns the call of
// each.
func inOrder(t *testing.T, calls []string, prefixes ...string) []string {
	t.Helper()
	var got []string
	i := 0
	for _, p := range prefixes {
		for i < len(calls) && !strings.HasPrefix(calls[i], p) {
			i++
		}
		if i == len(calls) {
			t.Fatalf("calls:\n%s\nwant, in order: %s", strings.Join(calls, "\n"), p)
		}
		got = append(got, calls[i])
		i++
	}
	return got
}

// A start through devvm's proxy: the VM on an internal network of its own
// alone, beside its relay (devvm's Linux build, in the empty image, read
// only, no capabilities, the proxy's socket's folder mounted on Linux), the
// proxy's address in its /etc/environment and never in an argument, and
// the lifeline its proxy follows. It's confined until something else is on
// its network, or it's on another; Stop stops its relay too.
func TestStartProxy(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	s.reset(t)
	if err := s.Start(t.Context(), vm, proxied()); err != nil {
		t.Fatal(err)
	}
	netw, relay := vm+"-net", vm+"-proxy"
	calls := s.calls(t)
	got := inOrder(t, calls,
		"docker network create --internal -o com.docker.network.bridge.inhibit_ipv4=true --label devvm.data="+s.Data+" --label devvm.kind=proxy "+netw,
		"docker network disconnect bridge "+vm,
		"docker network connect "+netw+" "+vm,
		"docker image import ",
		"docker container create --name "+relay+" ",
		"docker container start id-"+relay+"-", // by its ID
		"docker container start "+vm,
		"docker exec -i -u 0 "+vm+" sh -c "+envScript,
	)
	bin := filepath.Join(s.Dirs.Docker(), "bin", "devvm-linux-arm64")
	for _, want := range []string{
		" --label devvm.kind=proxy --network " + netw + " --network-alias devvm-proxy --read-only --cap-drop ALL --security-opt no-new-privileges ",
		" --user 501:1000 --mount type=bind,src=" + bin + ",dst=/devvm,readonly",
		" --mount type=bind,src=" + filepath.Dir(s.ProxySocket(vm)) + ",dst=/run/devvm,readonly --entrypoint /devvm " + s.imageRef("proxy") +
			" _proxy -relay 0.0.0.0:41000 -relay-to /run/devvm/proxy.sock",
	} {
		if !strings.Contains(got[4], want) {
			t.Errorf("relay: %s\nwant %s", got[4], want)
		}
	}
	// The socket the relay dials is the one in the folder it mounts.
	if filepath.Base(s.ProxySocket(vm)) != filepath.Base(relaySock) {
		t.Errorf("socket %s, relay dials %s", s.ProxySocket(vm), relaySock)
	}
	if !strings.Contains(s.field(t, "images/"+s.imageRef("proxy")+"/labels"), "devvm.kind=proxy") {
		t.Error("the relay's image isn't labelled")
	}
	if b, _ := os.ReadFile(bin); string(b) != "devvm for linux" { //nolint:gosec // the test's own
		t.Errorf("the Linux build's copy: %q", b)
	}
	for _, c := range calls {
		if strings.Contains(c, "k3y") {
			t.Errorf("the password in an argument: %s", c)
		}
	}
	u := "http://devvm:k3y@devvm-proxy:41000" //nolint:gosec // the test's own password
	wantEnv := "# devvm-proxy begin\nHTTPS_PROXY=" + u + "\nHTTP_PROXY=" + u + "\nNO_PROXY=localhost\nhttp_proxy=" + u + "\nhttps_proxy=" + u + "\n# devvm-proxy end\n"
	if b, _ := os.ReadFile(filepath.Join(s.stub, "environment")); string(b) != wantEnv {
		t.Errorf("/etc/environment's block:\n%s\nwant\n%s", b, wantEnv)
	}
	if s.ProxyURL(proxied()) != u || s.ProxyURL(backend.Confinement{}) != "" {
		t.Errorf("ProxyURL = %q", s.ProxyURL(proxied()))
	}
	id := s.field(t, "containers/"+vm+"/id")
	if len(s.detached) != 1 || !slices.Equal(s.detached[0], []string{"/bin/docker", "container", "wait", id}) {
		t.Errorf("detached: %v", s.detached)
	}
	if b, err := os.ReadFile(s.Lifeline(t.Context(), vm)); err != nil || strings.TrimSpace(string(b)) == "" {
		t.Errorf("lifeline: %q %v", b, err)
	} else if _, err := strconv.Atoi(strings.TrimSpace(string(b))); err != nil {
		t.Errorf("lifeline: %q", b)
	}
	confined := func(want bool, what string) {
		t.Helper()
		if ok, err := s.Confined(t.Context(), vm); err != nil || ok != want {
			t.Errorf("%s: Confined = %v, %v; want %v", what, ok, err, want)
		}
	}
	confined(true, "started through the proxy")
	s.stubDocker(t, "network", "connect", "bridge", vm)
	confined(false, "on Docker's default network too")
	s.stubDocker(t, "network", "disconnect", "bridge", vm)
	confined(true, "off it again")
	s.stubDocker(t, "container", "create", "--name", "devvm-other", "--network", netw, "alpine")
	s.stubDocker(t, "container", "start", "devvm-other")
	confined(false, "another container on its network")
	s.stubDocker(t, "container", "rm", "devvm-other")
	confined(true, "gone again")
	// The relay is told by its ID: another container by its name isn't it.
	s.stubDocker(t, "container", "rm", "-f", relay)
	s.stubDocker(t, "container", "create", "--name", relay, "--label", LabelKey+"="+s.Data, "--network", netw, "alpine")
	s.stubDocker(t, "container", "start", relay)
	confined(false, "another relay by its name")
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if st := s.field(t, "containers/"+relay+"/status"); st != "exited" {
		t.Errorf("relay after Stop: %s", st)
	}
	confined(false, "stopped")
	// Something else on its network: the next start is refused, naming it,
	// before the VM starts; started again once it's gone.
	s.stubDocker(t, "container", "create", "--name", "devvm-other", "--network", netw, "alpine")
	s.stubDocker(t, "container", "start", "devvm-other")
	s.reset(t)
	if err := s.Start(t.Context(), vm, proxied()); shown(err) != msg.DockerNetworkShared(netw).Names() {
		t.Errorf("another container on its network: %v", err)
	}
	if _, ok := has(s.calls(t), "docker container start "+vm); ok {
		t.Error("started with another container on its network")
	}
	s.stubDocker(t, "container", "rm", "-f", "devvm-other")
	if err := s.Start(t.Context(), vm, proxied()); err != nil {
		t.Fatal(err)
	}
	confined(true, "started again")
}

// A VM's container is made without NET_RAW, which its config hash
// includes.
func TestVMCapDrop(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	if c, ok := has(s.calls(t), "docker container create --name "+vm+" "); !ok || !strings.Contains(c, " --cap-drop NET_RAW ") {
		t.Errorf("create: %s", c)
	}
	c, err := s.vmConfig(vm, record{Project: "/p/web"})
	if err != nil {
		t.Fatal(err)
	}
	without := c
	without.CapDrop = nil
	if c.hash() == without.hash() {
		t.Error("the config hash leaves out the capabilities dropped")
	}
}

// A build's proxy is stopped through the process devvm started: one still
// running ends; one that has ended and been waited for is left alone (no
// signal reaches a process given its pid since); none is nothing.
func TestStopProxy(t *testing.T) {
	p := exec.CommandContext(t.Context(), "sleep", "60")
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- p.Wait() }()
	stopProxy(p.Process)
	select {
	case err := <-exited:
		if ee, ok := errors.AsType[*exec.ExitError](err); !ok || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGTERM {
			t.Errorf("ended: %v, want by SIGTERM", err)
		}
	case <-time.After(2 * time.Second):
		_ = p.Process.Kill()
		t.Fatal("not stopped")
	}
	if err := p.Process.Signal(syscall.SIGTERM); !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("a signal to the ended process: %v, want it refused as done", err)
	}
	stopProxy(p.Process)
	stopProxy(nil)
}

// Started without it again: Docker's default network alone, its relay and
// network gone, nothing of the proxy in /etc/environment, not confined;
// Delete takes what a start through it left.
func TestStartProxyOff(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	if err := s.Start(t.Context(), vm, proxied()); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	// Something else on its network: its network isn't deleted, and the
	// start is refused, naming it; started once it's gone.
	netw := netName(vm)
	s.stubDocker(t, "container", "create", "--name", "devvm-other", "--network", netw, "alpine")
	s.stubDocker(t, "container", "start", "devvm-other")
	s.reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{}); shown(err) != msg.DockerNetworkShared(netw).Names() {
		t.Errorf("another container on its network: %v", err)
	}
	if _, ok := has(s.calls(t), "docker container start "+vm); ok || !s.exists("networks/"+netw) {
		t.Error("started, or its network deleted, with another container on it")
	}
	s.stubDocker(t, "container", "rm", "-f", "devvm-other")
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	if n := s.field(t, "containers/"+vm+"/networks"); n != "bridge" || s.exists("containers/"+vm+"-proxy") || s.exists("networks/"+vm+"-net") {
		t.Errorf("networks %q, relay %v, network %v", n, s.exists("containers/"+vm+"-proxy"), s.exists("networks/"+vm+"-net"))
	}
	if b, _ := os.ReadFile(filepath.Join(s.stub, "environment")); len(b) != 0 {
		t.Errorf("/etc/environment's block: %q", b)
	}
	if ok, err := s.Confined(t.Context(), vm); ok || err != nil {
		t.Errorf("Confined = %v, %v", ok, err)
	}
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context(), vm, proxied()); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if s.exists("containers/"+vm+"-proxy") || s.exists("networks/"+vm+"-net") {
		t.Error("Delete left the relay or the network")
	}
	for _, f := range []string{s.Lifeline(t.Context(), vm), filepath.Dir(s.ProxySocket(vm))} {
		if _, err := os.Stat(f); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Delete left %s: %v", f, err)
		}
	}
}

// On Docker Desktop the relay gets the proxy's socket alone, as root:
// Desktop shows it as root's.
func TestStartProxyDesktop(t *testing.T) {
	s := newSandbox(t)
	s.Kind = kindDesktop
	if ps := s.Problems(t.Context()); len(ps) > 0 {
		t.Fatal(ps)
	}
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	s.reset(t)
	if err := s.Start(t.Context(), vm, proxied()); err != nil {
		t.Fatal(err)
	}
	c, ok := has(s.calls(t), "docker container create --name "+vm+"-proxy ")
	if !ok || !strings.Contains(c, " --user 0:0 ") || !strings.Contains(c, " -v "+s.ProxySocket(vm)+":/run/devvm/proxy.sock ") {
		t.Errorf("relay: %s", c)
	}
}

// Refused: on an engine that can't reach devvm's proxy, and without the
// proxy's port or password; a network by its network's name that isn't
// this devvm's.
func TestStartProxyRefused(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetProxy}); err == nil || id(err) != "" {
		t.Errorf("no proxy: %v", err)
	}
	s.touch(t, "docker-foreign-"+vm+"-net")
	s.stubDocker(t, "network", "create", vm+"-net")
	if err := s.Start(t.Context(), vm, proxied()); shown(err) != msg.DockerNetworkForeign(vm+"-net").Names() {
		t.Errorf("someone else's network: %v", err)
	}
	s.Kind = kindOther
	if ps := s.Problems(t.Context()); len(ps) > 0 {
		t.Fatal(ps)
	}
	s.reset(t)
	if err := s.Start(t.Context(), vm, proxied()); shown(err) != msg.BackendLacks(Name, "egress-proxy", vm).Names() {
		t.Errorf("another engine: %v", err)
	}
	if calls := s.calls(t); len(calls) != 0 {
		t.Errorf("calls: %v", calls)
	}
}

// A build on Docker Desktop: on a network of its own, beside its relay
// (the proxy's socket alone, as root), a proxy of devvm's on the host for
// it (any host, on its socket), whose settings its commands get from an
// env file and sudo keeps while its scripts run; all gone at the end, the
// image committed as on Linux.
func TestBuildOnDesktop(t *testing.T) {
	s := newSandbox(t)
	s.Kind = kindDesktop
	if ps := s.Problems(t.Context()); len(ps) > 0 {
		t.Fatal(ps)
	}
	_, err := s.BuildImage(t.Context(), backend.ImageSpec{
		Image: "base", Key: baseKey, Net: backend.NetInternet,
		Scripts: []backend.Script{{Name: "base.sh", Data: []byte("#!/bin/bash\n# base\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	inst := s.buildName(baseKey)
	netw := inst + "-net"
	envFile := filepath.Join(s.Dirs.Docker(), "builds", inst, "env")
	calls := s.calls(t)
	got := inOrder(t, calls,
		"docker network create --internal -o com.docker.network.bridge.inhibit_ipv4=true ",
		"docker container create --name "+inst+"-proxy ",
		"docker container start id-"+inst+"-proxy-",
		"docker container create --name "+inst+" ",
		"docker container start "+inst,
		"docker exec -i -u 0 --env-file "+envFile+" "+inst+" sh -s -- u 501",
		"docker exec -u 0 "+inst+" sh -c printf '%s\\n' '"+sudoKeep+"' > /etc/sudoers.d/devvm-build-proxy",
		"docker exec --env-file "+envFile+" -i -u u ",
		"docker exec -u 0 "+inst+" sh -c rm -f /etc/sudoers.d/devvm-build-proxy",
		"docker container commit ",
		"docker container rm -f -v "+inst,
		"docker container rm -f -v "+inst+"-proxy",
		"docker network rm "+netw,
	)
	if !strings.Contains(got[1], " --user 0:0 ") || !strings.Contains(got[1], " -v "+proxySocket(s.Dirs, inst)+":/run/devvm/proxy.sock ") ||
		!strings.HasSuffix(got[1], " _proxy -relay 0.0.0.0:3128 -relay-to /run/devvm/proxy.sock") {
		t.Errorf("relay: %s", got[1])
	}
	if !strings.Contains(got[3], " --network "+netw+" ") || !strings.Contains(got[3], " --cap-drop NET_RAW ") {
		t.Errorf("build container: %s", got[3])
	}
	if len(s.detached) != 2 {
		t.Fatalf("detached: %v", s.detached)
	}
	p := s.detached[0]
	dir := filepath.Join(s.Dirs.Docker(), "builds", inst)
	wantProxy := []string{
		"/devvm", "_proxy", "-listen", "127.0.0.1:0", "-listen-unix", proxySocket(s.Dirs, inst), "-vm", inst,
		"-policy", filepath.Join(dir, "policy"), "-log", filepath.Join(dir, "events.jsonl"),
		"-auth-file", filepath.Join(dir, "auth"), "-lifeline", filepath.Join(dir, "lifeline"),
	}
	if !slices.Equal(p, wantProxy) {
		t.Errorf("proxy: %v\nwant %v", p, wantProxy)
	}
	if w := s.detached[1]; len(w) != 4 || w[1] != "container" || w[2] != "wait" {
		t.Errorf("lifeline: %v", w)
	}
	select {
	case err := <-s.spawned[0]:
		t.Logf("the build's proxy stopped: %v", err)
	case <-time.After(2 * time.Second):
		t.Error("the build's proxy wasn't stopped")
	}
	for _, c := range calls {
		if strings.Contains(c, "devvm:") && strings.Contains(c, "@devvm-proxy") {
			t.Errorf("the proxy's address in an argument: %s", c)
		}
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the build's proxy folder stayed: %v", err)
	}
	if s.exists("networks/"+netw) || s.exists("containers/"+inst+"-proxy") || !s.exists("images/"+s.imageRef(baseKey)) {
		t.Error("the build left its network or relay, or made no image")
	}
	// A build with no network needs no proxy.
	s.reset(t)
	s.detached = nil
	if _, err := s.BuildImage(t.Context(), backend.ImageSpec{Image: "node", Key: nodeKey, Parent: baseKey, Net: backend.NetNone}); err != nil {
		t.Fatal(err)
	}
	if c, ok := has(s.calls(t), "docker container create --name "+s.buildName(nodeKey)+" "); !ok || !strings.Contains(c, " --network none ") || strings.Contains(c, "--cap-drop") || len(s.detached) != 0 {
		t.Errorf("no network: %s, detached %v", c, s.detached)
	}
}

// A build whose proxy doesn't say it listens stops, naming its log, and
// leaves nothing.
func TestBuildProxyFails(t *testing.T) {
	s := newSandbox(t)
	s.Kind = kindDesktop
	if ps := s.Problems(t.Context()); len(ps) > 0 {
		t.Fatal(ps)
	}
	s.Spawn = func(string, []string, *os.File) (*os.Process, error) { return nil, errors.New("no") }
	_, err := s.BuildImage(t.Context(), backend.ImageSpec{Image: "base", Key: baseKey, Net: backend.NetInternet})
	if err == nil {
		t.Fatal("no error")
	}
	if calls := s.calls(t); slices.ContainsFunc(calls, func(c string) bool { return strings.Contains(c, " create ") }) {
		t.Errorf("calls: %v", calls)
	}
}

// devvm's Linux build: on Linux, devvm itself; elsewhere beside it, or in
// ../libexec, refused when it's missing or isn't this build.
func TestFindLinux(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "bin", "devvm")
	if err := os.MkdirAll(filepath.Dir(self), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(self, []byte("devvm"), 0o700); err != nil { //nolint:gosec // a stand-in
		t.Fatal(err)
	}
	link := filepath.Join(dir, "devvm-link")
	if err := os.Symlink(self, link); err != nil {
		t.Fatal(err)
	}
	find := func(goos string) (string, error) {
		return linuxFinder{Self: link, GOOS: goos, GOARCH: "arm64", Info: func(msg.M) {}}.find(t.Context(), true)
	}
	if p, err := find("linux"); err != nil || p != resolved(self) {
		t.Errorf("Linux: %q %v", p, err)
	}
	if _, err := find("darwin"); shown(err) != msg.DockerLinuxBuildMissing(filepath.Join(resolved(filepath.Dir(self)), "devvm-linux-arm64")).Names() {
		t.Errorf("missing, nothing to fetch: %v", err)
	}
	other := filepath.Join(dir, "libexec", "devvm-linux-arm64")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("not a Go binary"), 0o700); err != nil { //nolint:gosec // a stand-in
		t.Fatal(err)
	}
	if _, err := find("darwin"); id(err) != "docker_linux_build_other" {
		t.Errorf("not this build: %v", err)
	}
}

// linuxArchive is a release's Linux archive as GoReleaser makes it, its
// devvm holding body.
func linuxArchive(t *testing.T, body []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		data []byte
	}{{"devvm-1.0.0-linux-arm64/README.md", []byte("readme")}, {"devvm-1.0.0-linux-arm64/devvm", body}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A release's own Linux build: not fetched until a start asks; fetched,
// with a status step, then refused unless the devvm in it has the SHA-256
// this devvm has, nothing kept; one that fails to come is refused too.
// (A real build that matches is the CLI tests'.)
func TestFetchLinux(t *testing.T) {
	body := []byte("devvm for linux, a stand-in")
	sum := sha256.Sum256(body)
	pin := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/devvm-1.0.0-linux-arm64.tar.gz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(linuxArchive(t, body))
	}))
	defer srv.Close()
	dir := t.TempDir()
	self := filepath.Join(dir, "devvm")
	if err := os.WriteFile(self, []byte("devvm"), 0o700); err != nil { //nolint:gosec // a stand-in
		t.Fatal(err)
	}
	var infos []string
	f := linuxFinder{
		Self: self, GOOS: "darwin", GOARCH: "arm64", URL: srv.URL + "/devvm-1.0.0-linux-arm64.tar.gz", SHA256: pin,
		Copy: filepath.Join(dir, "data", "devvm-linux-arm64"), Info: func(m msg.M) { infos = append(infos, m.ID()) },
	}
	if _, err := f.find(t.Context(), false); !errors.Is(err, errUnfetched) || len(infos) != 0 {
		t.Errorf("without fetch: %v, %v", err, infos)
	}
	// The SHA-256 matches; the stand-in isn't a Go build, so the same-build
	// check refuses it, and it goes.
	if _, err := f.find(t.Context(), true); id(err) != "docker_linux_build_other" || !slices.Equal(infos, []string{"docker_linux_build_fetching"}) {
		t.Errorf("fetched: %v, %v", err, infos)
	}
	if _, err := os.Stat(f.Copy); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("kept: %v", err)
	}
	// Another SHA-256: refused before anything is kept.
	f.SHA256 = strings.Repeat("0", 64)
	if _, err := f.find(t.Context(), true); id(err) != "docker_linux_build_mismatch" {
		t.Errorf("mismatch: %v", err)
	}
	if ents, _ := os.ReadDir(filepath.Dir(f.Copy)); len(ents) != 0 {
		t.Errorf("left %v", ents)
	}
	f.URL = srv.URL + "/missing.tar.gz"
	if _, err := f.find(t.Context(), true); id(err) != "docker_linux_build_fetch_failed" {
		t.Errorf("not there: %v", err)
	}
	// -ldflags compare without the pin's flags.
	if withoutPin(`-s -w -X github.com/w3cj/devvm/internal/cli.version=1 -X `+pinFlags+`URL=u -X `+pinFlags+`SHA256=s`) != "-s -w -X github.com/w3cj/devvm/internal/cli.version=1" {
		t.Error("withoutPin")
	}
}
