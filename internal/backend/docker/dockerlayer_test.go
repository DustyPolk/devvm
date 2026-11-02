package docker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// withSysbox makes the sandbox's engine one with Sysbox's runtime.
func (s *sandbox) withSysbox(t *testing.T) {
	t.Helper()
	info := `{"ServerVersion":"29.1.3","OSType":"linux","Architecture":"aarch64","OperatingSystem":"Ubuntu 26.04 LTS","Name":"stub-host",` +
		`"MemTotal":68719476736,"Runtimes":{"runc":{},"sysbox-runc":{"path":"/usr/bin/sysbox-runc"}}}`
	if err := os.WriteFile(filepath.Join(s.stub, "docker-info"), []byte(info), 0o600); err != nil {
		t.Fatal(err)
	}
	if ps := s.Problems(t.Context()); len(ps) > 0 {
		t.Fatalf("Problems: %v", ps)
	}
	s.ProcSys = filepath.Join(t.TempDir(), "sys")
	s.routeLocalnet(t, "0")
}

// routeLocalnet sets route_localnet for the bridge of vm's own network
// (the stub's network IDs are id-net-NAME).
func (s *sandbox) routeLocalnet(t *testing.T, v string) {
	t.Helper()
	conf := filepath.Join(s.ProcSys, "net", "ipv4", "conf", "br-id-net-devvm")
	if err := os.MkdirAll(conf, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conf, "route_localnet"), []byte(v+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A docker layer VM's bridge routing to the loopback (Docker's userland
// proxy off) is refused at creation and at each start, and so is one whose
// setting can't be read; a Linux engine of another machine's with Sysbox
// is refused, being one devvm can't read.
func TestRouteLocalnet(t *testing.T) {
	s := newSandbox(t)
	s.withSysbox(t)
	s.routeLocalnet(t, "1")
	s.image(t, baseKey)
	err := s.CreateVM(t.Context(), baseKey, backend.VMSpec{Name: vm, Project: "/p/web", DockerLayer: true})
	if msg.IDOf(err) != "docker_route_localnet" || s.exists("containers/"+vm) {
		t.Errorf("create: %v", err)
	}
	s.routeLocalnet(t, "0")
	if err := s.CreateVM(t.Context(), baseKey, backend.VMSpec{Name: vm, Project: "/p/web", DockerLayer: true}); err != nil {
		t.Fatal(err)
	}
	s.routeLocalnet(t, "1")
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetDirect}); msg.IDOf(err) != "docker_route_localnet" {
		t.Errorf("start: %v", err)
	}
	if err := os.RemoveAll(s.ProcSys); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetDirect}); msg.IDOf(err) != "docker_bridge_unreadable" {
		t.Errorf("unreadable: %v", err)
	}
	s.Kind = kindOther
	s.Problems(t.Context())
	if _, err := s.DockerLayer(vm); msg.IDOf(err) != "docker_layer_needs_vm" || s.Caps()[backend.FeatureDockerLayer] != backend.None {
		t.Errorf("another machine's: %v", err)
	}
}

// Sysbox's runtime on the engine: the docker layer emulated under it,
// reviewed, and said by Report.
func TestCapsSysbox(t *testing.T) {
	s := newSandbox(t)
	s.withSysbox(t)
	if got := s.Caps()[backend.FeatureDockerLayer]; got != backend.Emulated(true) {
		t.Errorf("docker-layer = %s", got)
	}
	if rt, err := s.DockerLayer(vm); rt != "sysbox" || err != nil {
		t.Errorf("DockerLayer = %q, %v", rt, err)
	}
	got := s.Report(t.Context())
	if len(got) != 2 || got[1].ID() != "docker_sysbox" {
		t.Errorf("Report: %v", got)
	}
}

// A VM with the docker layer, under Sysbox: its runtime, no capability
// dropped (Sysbox ignores it), its engine's volumes, made first, labelled;
// Docker's daemon started after each start; no network policy; its
// volumes deleted with it. Without Sysbox, refused, nothing made.
func TestCreateVMSysbox(t *testing.T) {
	s := newSandbox(t)
	s.image(t, baseKey)
	err := s.CreateVM(t.Context(), baseKey, backend.VMSpec{Name: vm, Project: "/p/web", DockerLayer: true})
	if msg.NamesOf(err) != msg.DockerLayerNoSysbox(vm).Names() || s.exists("containers/"+vm) {
		t.Errorf("without Sysbox: %v", err)
	}
	s.withSysbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web", DockerLayer: true})
	c, ok := has(s.calls(t), "docker container create --name "+vm)
	if !ok || !strings.HasSuffix(c, " --mount type=volume,src="+vm+"-docker,dst=/var/lib/docker --mount type=volume,src="+vm+"-containerd,dst=/var/lib/containerd"+
		" --runtime sysbox-runc "+s.imageRef(baseKey)+" infinity") || strings.Contains(c, "--cap-drop") || strings.Contains(c, "--privileged") {
		t.Errorf("create: %s", c)
	}
	for _, v := range engineVolumes(vm) {
		if !strings.Contains(s.field(t, "volumes/"+v+"/labels"), LabelKey+"="+s.Dirs.Data) {
			t.Errorf("volume %s isn't labelled", v)
		}
	}
	r, err := s.readRecord(vm)
	if err != nil || r.Runtime != sysboxRuntime {
		t.Fatalf("record: %+v, %v", r, err)
	}
	cfg, err := s.vmConfig(vm, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.field(t, "containers/"+vm+"/labels"), configLabel+"="+cfg.hash()) {
		t.Errorf("config hash isn't %s", cfg.hash())
	}
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetProxy, ProxyPort: 3128, ProxyPassword: "p"}); msg.NamesOf(err) != msg.NetDockerLayer(Name, vm).Names() {
		t.Errorf("network policy: %v", err)
	}
	s.stubs.Reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetDirect}); err != nil {
		t.Fatal(err)
	}
	if _, ok := has(s.calls(t), "docker exec -i -u 0 -e DEVVM_LOOP=off -e DEVVM_SSHD=inetd "+vm+" sh -s --"); !ok {
		t.Errorf("services with Docker's daemon: %v", s.calls(t))
	}
	if err := s.Delete(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	for _, v := range engineVolumes(vm) {
		if s.exists("volumes/" + v) {
			t.Errorf("volume %s kept", v)
		}
	}
}

// A docker layer VM's own network: made before its container, labelled,
// not internal, the VM on it alone; one with another container on it is
// refused at the next start, and so is deleting the VM, before anything's
// deleted; deleted with the VM.
func TestLayerNetwork(t *testing.T) {
	s := newSandbox(t)
	s.withSysbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web", DockerLayer: true})
	net := vm + "-docker-net"
	calls := s.calls(t)
	nc, ok := has(calls, "docker network create ")
	cc, _ := has(calls, "docker container create --name "+vm)
	if !ok || nc != "docker network create --label "+LabelKey+"="+s.Dirs.Data+" --label "+kindLabel+"="+kindVM+" "+net ||
		slices.Index(calls, nc) > slices.Index(calls, cc) {
		t.Errorf("calls: %v", calls)
	}
	if !strings.Contains(cc, " --network "+net+" ") || s.field(t, "networks/"+net+"/internal") != "false" {
		t.Errorf("create: %s", cc)
	}
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetDirect}); err != nil {
		t.Fatal(err)
	}
	if got := s.field(t, "containers/"+vm+"/networks"); strings.TrimSpace(got) != net {
		t.Errorf("networks: %q", got)
	}
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	// One of devvm's by its name that isn't as devvm makes it (IPv6 on) is
	// made again.
	if err := os.WriteFile(filepath.Join(s.stub, "docker", "networks", net, "ipv6"), []byte("true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.stubs.Reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetDirect}); err != nil {
		t.Fatal(err)
	}
	calls = s.calls(t)
	rm, _ := has(calls, "docker network rm "+net)
	mk, _ := has(calls, "docker network create ")
	if rm == "" || mk == "" || slices.Index(calls, rm) > slices.Index(calls, mk) || s.exists("networks/"+net+"/ipv6") {
		t.Errorf("not plain: %v", calls)
	}
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	// Another container on it, running.
	other := filepath.Join(s.stub, "docker", "containers", "other")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	for f, v := range map[string]string{"labels": "", "status": "running", "networks": net, "id": "id-other"} {
		if err := os.WriteFile(filepath.Join(other, f), []byte(v+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetDirect}); msg.IDOf(err) != "docker_network_shared" {
		t.Errorf("shared: %v", err)
	}
	if err := s.Delete(t.Context(), vm); msg.IDOf(err) != "docker_network_shared" || !s.exists("containers/"+vm) {
		t.Errorf("delete with it shared: %v", err)
	}
	if err := os.RemoveAll(other); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if s.exists("networks/" + net) {
		t.Error("network kept")
	}
}
