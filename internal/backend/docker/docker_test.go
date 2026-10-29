package docker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// execRunner runs the commands as they are: here, the docker stub.
type execRunner struct{}

func (execRunner) Run(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }

func (execRunner) Output(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

func (execRunner) Attach(cmd *exec.Cmd) error { return cmd.Run() }

// sandbox is a Docker backend on the stub (testdata/stubs/docker), first
// on PATH, with devvm's folders in a new temporary folder, on Linux, its
// engine this machine's; its user is u, uid 501, gid 1000.
type sandbox struct {
	*Docker
	stub  string
	infos []msg.M
	// detached are the processes Detach and Spawn started, each its path
	// and arguments.
	detached [][]string
	// spawned is how each process Spawn started ended, once it has.
	spawned []chan error
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	root := t.TempDir()
	stub := filepath.Join(root, "stub")
	for _, d := range []string{"data", "config", "stub"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stubs, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "stubs"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STUB_DIR", stub)
	t.Setenv("STUB_LOG", filepath.Join(stub, "calls"))
	t.Setenv("PATH", stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	d := paths.Dirs{Data: filepath.Join(root, "data", "devvm"), Config: filepath.Join(root, "config", "devvm"), Lima: filepath.Join(root, "lima")}
	s := &sandbox{stub: stub}
	// devvm's Linux build, as findLinux would find it.
	linux := filepath.Join(root, "devvm-linux")
	if err := os.WriteFile(linux, []byte("devvm for linux"), 0o700); err != nil { //nolint:gosec // a stand-in for a binary
		t.Fatal(err)
	}
	s.Docker = &Docker{
		client: client{Runner: execRunner{}, names: names{Data: d.Data}}, Dirs: d, OS: "linux", Arch: "arm64", Assets: assets.FS,
		User:     func() (*user.User, error) { return &user.User{Username: "u", Uid: "501", Gid: "1000"}, nil },
		Hostname: func() (string, error) { return "stub-host", nil },
		Terminal: func() bool { return false },
		Self:     "/devvm", Getenv: func(string) string { return "" },
		Linux: func(context.Context, bool) (string, error) { return linux, nil },
		// A sleep stands in for each, a proxy saying it listens.
		Detach: func(path string, args []string, out *os.File) (int, error) {
			p, err := s.sleep(t, path, args, out)
			if err != nil {
				return 0, err
			}
			return p.Pid, nil
		},
		Spawn: func(path string, args []string, out *os.File) (*os.Process, error) {
			return s.sleep(t, path, args, out)
		},
		Tick:     time.Millisecond,
		LookPath: func(c string) (string, error) { return "/bin/" + c, nil },
		Forwards: func() vmssh.Forwards {
			f := newForwards(d, "/devvm")
			f.Tick = time.Millisecond
			f.Detach = func(string, []string, *os.File) (int, error) { return 0, errors.New("no forwarder here") }
			f.Args = func(int) ([]string, error) { return nil, errors.New("no process") }
			return f
		}(),
		Info: func(m msg.M) { s.infos = append(s.infos, m) }, Warn: func(msg.M) {},
		WithSteps:  func(ctx context.Context, _ int) context.Context { return ctx },
		AddCleanup: func(func(context.Context)) func() { return func() {} }, Interrupted: func() bool { return false },
	}
	if err := os.WriteFile(filepath.Join(stub, "docker-info"), []byte(`{"ServerVersion":"29.1.3","OSType":"linux","Architecture":"aarch64","OperatingSystem":"Ubuntu 26.04 LTS","Name":"stub-host","MemTotal":68719476736}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ps := s.Problems(t.Context()); len(ps) > 0 {
		t.Fatalf("Problems: %v", ps)
	}
	return s
}

// calls are the stub's calls, without the reads.
func (s *sandbox) calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.stub, "calls"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	var out []string
	for l := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		if l == "" || strings.HasPrefix(l, "docker container inspect") || strings.HasPrefix(l, "docker image inspect") ||
			strings.HasPrefix(l, "docker container ls") || strings.HasPrefix(l, "docker volume inspect") || strings.HasPrefix(l, "docker info") ||
			strings.HasPrefix(l, "docker network inspect") {
			continue
		}
		out = append(out, l)
	}
	return out
}

// sleep starts a sleep in place of path with args, recorded (detached,
// spawned); one for _proxy writes to out that it listens.
func (s *sandbox) sleep(t *testing.T, path string, args []string, out *os.File) (*os.Process, error) {
	s.detached = append(s.detached, append([]string{path}, args...))
	sleep := exec.CommandContext(t.Context(), "sleep", "60")
	if err := sleep.Start(); err != nil {
		return nil, err
	}
	ended := make(chan error, 1)
	go func() { ended <- sleep.Wait() }()
	t.Cleanup(func() { _ = sleep.Process.Kill() })
	s.spawned = append(s.spawned, ended)
	if slices.Contains(args, "_proxy") {
		_, _ = out.WriteString("devvm proxy: listening on 127.0.0.1:40000 for x\n")
	}
	return sleep.Process, nil
}

func (s *sandbox) reset(t *testing.T) {
	t.Helper()
	if err := os.Remove(filepath.Join(s.stub, "calls")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func (s *sandbox) touch(t *testing.T, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.stub, name), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// id is the ID of the message err carries; "" for none.
func id(err error) string {
	var m msg.M
	if errors.As(err, &m) {
		return m.ID()
	}
	return ""
}

// shown is the message err carries, as DEVVM_MESSAGES=names shows it; ""
// for none.
func shown(err error) string {
	var m msg.M
	if errors.As(err, &m) {
		return m.Names()
	}
	return ""
}

// field reads a stub object's field: containers/NAME/status.
func (s *sandbox) field(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.stub, "docker", path)) //nolint:gosec // the test's own stub
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (s *sandbox) exists(path string) bool {
	_, err := os.Stat(filepath.Join(s.stub, "docker", path))
	return err == nil
}

// image makes an image of this devvm's in the stub, by key.
func (s *sandbox) image(t *testing.T, key string) {
	t.Helper()
	dir := filepath.Join(s.stub, "docker", "images", s.imageRef(key))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	labels := LabelKey + "=" + s.Dirs.Data + "\n" + kindLabel + "=" + kindImage + "\n" + keyLabel + "=" + key + "\n"
	if err := os.WriteFile(filepath.Join(dir, "labels"), []byte(labels), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "layers"), []byte("3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// has reports whether a call starting with prefix was made, and returns
// it.
func has(calls []string, prefix string) (string, bool) {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return c, true
		}
	}
	return "", false
}

const (
	baseKey = "aaaaaaaaaaaa"
	nodeKey = "bbbbbbbbbbbb"
	vm      = "devvm-web-0123456789abcd"
)

// The engine's kind decides build confinement: native on a Linux engine
// of this machine's, rootful; emulated, and reviewed, on Docker Desktop,
// as network policy is on both; neither elsewhere.
func TestCapsByEngine(t *testing.T) {
	reviewed := backend.Emulated(true)
	cases := []struct {
		name, info, os, host string
		confinement, egress  backend.Support
		kind                 string
	}{
		{"this machine's", `"OperatingSystem":"Ubuntu","Name":"h"`, "linux", "h", backend.Native, reviewed, kindLinux},
		{"Docker Desktop", `"OperatingSystem":"Docker Desktop","Name":"docker-desktop"`, "darwin", "mac", reviewed, reviewed, kindDesktop},
		{"Docker Desktop on Linux", `"OperatingSystem":"Docker Desktop","Name":"h"`, "linux", "h", reviewed, reviewed, kindDesktop},
		{"another machine's", `"OperatingSystem":"Ubuntu","Name":"elsewhere"`, "linux", "h", backend.None, backend.None, kindOther},
		{"rootless", `"OperatingSystem":"Ubuntu","Name":"h","SecurityOptions":["name=rootless"]`, "linux", "h", backend.None, backend.None, kindOther},
		{"colima on a Mac", `"OperatingSystem":"Ubuntu","Name":"mac"`, "darwin", "mac", backend.None, backend.None, kindOther},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSandbox(t)
			s.OS = c.os
			s.Hostname = func() (string, error) { return c.host, nil }
			if err := os.WriteFile(filepath.Join(s.stub, "docker-info"), []byte(`{"ServerVersion":"29.8.2","OSType":"linux","Architecture":"aarch64",`+c.info+`}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if ps := s.Problems(t.Context()); len(ps) > 0 {
				t.Fatalf("Problems: %v", ps)
			}
			caps := s.Caps()
			if caps[backend.FeatureBuildConfinement] != c.confinement || caps[backend.FeatureEgressProxy] != c.egress || s.engineKind() != c.kind {
				t.Errorf("build confinement %s, egress proxy %s, kind %s; want %s, %s, %s", caps[backend.FeatureBuildConfinement],
					caps[backend.FeatureEgressProxy], s.engineKind(), c.confinement, c.egress, c.kind)
			}
			for _, f := range []backend.Feature{backend.FeatureDisk, backend.FeatureDockerLayer} {
				if s.Caps()[f] != backend.None {
					t.Errorf("%s = %s, want none", f, s.Caps()[f])
				}
			}
			// Without Sysbox, the docker layer is refused: on this machine's
			// Linux engine, saying to install it; elsewhere, to use a VM
			// backend.
			want := "docker_layer_needs_vm"
			if c.kind == kindLinux {
				want = "docker_layer_no_sysbox"
			}
			if rt, err := s.DockerLayer("devvm-x"); rt != "" || id(err) != want {
				t.Errorf("DockerLayer = %q, %v; want %s", rt, err, want)
			}
			// Over docker exec, on every engine, reviewed.
			for _, f := range []backend.Feature{
				backend.FeatureSSH, backend.FeaturePorts, backend.FeaturePortsAll, backend.FeatureAgentForwarding, backend.FeatureAgentToggle,
			} {
				if s.Caps()[f] != backend.Emulated(true) {
					t.Errorf("%s = %s, want emulated, reviewed", f, s.Caps()[f])
				}
			}
		})
	}
	// Before anything asked the engine: not confined.
	if (&Docker{}).Caps()[backend.FeatureBuildConfinement].Usable() {
		t.Error("build confinement without an engine")
	}
}

// Problems: no docker, no engine, an engine the user can't reach (with
// the fix), one too old, of another system or processor.
func TestProblems(t *testing.T) {
	s := newSandbox(t)
	key := func() []string {
		var out []string
		for _, p := range s.Problems(t.Context()) {
			out = append(out, p.Key+":"+p.Msg.ID()+":"+p.Fix)
		}
		return out
	}
	if got := key(); len(got) != 0 {
		t.Errorf("an engine of this machine's: %v", got)
	}
	s.touch(t, "docker-down")
	if got := key(); !slices.Equal(got, []string{"docker-engine:docker_down:"}) {
		t.Errorf("no engine: %v", got)
	}
	_ = os.Remove(filepath.Join(s.stub, "docker-down"))
	s.touch(t, "docker-denied")
	if got := key(); !slices.Equal(got, []string{"docker-access:docker_access:sudo usermod -aG docker u"}) {
		t.Errorf("no access: %v", got)
	}
	_ = os.Remove(filepath.Join(s.stub, "docker-denied"))
	if err := os.WriteFile(filepath.Join(s.stub, "docker-info"), []byte(`{"ServerVersion":"28.5.1","OSType":"windows","Architecture":"x86_64"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := key(); !slices.Equal(got, []string{"docker-version:docker_old:", "docker-os:docker_not_linux:", "docker-arch:docker_arch:"}) {
		t.Errorf("an old engine of another kind: %v", got)
	}
	s.OS = "windows"
	if got := key(); !slices.Equal(got, []string{"os:docker_host_unsupported:"}) {
		t.Errorf("Windows: %v", got)
	}
	s.OS = "linux"
	t.Setenv("PATH", t.TempDir())
	if got := key(); !slices.Equal(got, []string{"docker:docker_not_installed:"}) {
		t.Errorf("no docker: %v", got)
	}
}

func TestRecent(t *testing.T) {
	for v, want := range map[string]bool{
		"29.1.3": true, "29.8.2": true, "30.0.0": true, "29.10.0-rc1": true,
		"29.0.4": false, "28.5.1": false, "": false, "x.y": false,
	} {
		if recent(v) != want {
			t.Errorf("recent(%q) = %v", v, !want)
		}
	}
}

// doctor's report: which engine, and on Docker Desktop which folders it
// shares (its settings', or its defaults when they don't say).
func TestReport(t *testing.T) {
	s := newSandbox(t)
	if got := s.Report(t.Context()); len(got) != 1 || got[0].ID() != "docker_engine_linux" {
		t.Errorf("Linux: %v", got)
	}
	s.OS = "darwin"
	if err := os.WriteFile(filepath.Join(s.stub, "docker-info"), []byte(`{"ServerVersion":"29.8.2","OSType":"linux","Architecture":"aarch64","OperatingSystem":"Docker Desktop"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s.SharingFile = filepath.Join(t.TempDir(), "settings-store.json")
	if err := os.WriteFile(s.SharingFile, []byte(`{"FilesharingDirectories":["/Users","/tmp"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ps := s.Problems(t.Context()); len(ps) > 0 {
		t.Fatal(ps)
	}
	got := s.Report(t.Context())
	if len(got) != 2 || got[0].ID() != "docker_engine_desktop" || got[1].String() != msg.DockerFileSharing("/Users /tmp").String() {
		t.Errorf("Desktop: %v", got)
	}
	// Without devvm's Linux build, which network policy and its builds run.
	s.Linux = func(context.Context, bool) (string, error) {
		return "", msg.DockerLinuxBuildMissing("/x/devvm-linux-arm64")
	}
	if got := s.Report(t.Context()); len(got) != 3 || got[2].ID() != "docker_linux_build_missing" {
		t.Errorf("no Linux build: %v", got)
	}
	if err := s.checkShared("/Users/me/p"); err != nil {
		t.Errorf("a shared folder: %v", err)
	}
	if err := s.checkShared("/opt/p"); id(err) != "docker_not_shared" {
		t.Errorf("a folder it doesn't share: %v", err)
	}
	if err := os.WriteFile(s.SharingFile, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if dirs, ok := s.sharedFolders(); !ok || !slices.Equal(dirs, desktopDefault) {
		t.Errorf("defaults: %v %v", dirs, ok)
	}
}

// Base: Ubuntu's image pulled by digest, a labelled container from it,
// prepared (the host's gid too, on Linux), its scripts run between the
// stand-ins, sshd, then committed and deleted.
func TestBuildBase(t *testing.T) {
	s := newSandbox(t)
	built, err := s.BuildImage(t.Context(), backend.ImageSpec{
		Image: "base", Key: baseKey, Net: backend.NetInternet,
		Scripts: []backend.Script{{Name: "base.sh", Data: []byte("#!/bin/bash\n# base\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pinned := "docker.io/library/ubuntu@" + ubuntu.ContainerDigest
	if built.Origin != "docker.io/library/ubuntu:26.04@"+ubuntu.ContainerDigest || built.Libc != "gnu" {
		t.Errorf("Built = %+v", built)
	}
	inst := s.buildName(baseKey)
	want := []string{
		"docker image pull " + pinned,
		"docker container create --name " + inst + " --label devvm.data=" + s.Dirs.Data + " --label devvm.key=" + baseKey +
			" --label devvm.kind=build --init --user 0:0 --entrypoint sleep --cpus 4 --memory 8192m --memory-swap 8192m" +
			" --hostname devvm-build --network bridge " + pinned + " infinity",
		"docker container start " + inst,
		"docker exec -i -u 0 " + inst + " sh -s -- u 501 1000",
		"docker exec " + inst + " sh -c if command -v apk >/dev/null; then echo musl; else echo gnu; fi",
		"docker exec -i -u 0 " + inst + " sh -s -- on",
	}
	calls := s.calls(t)
	if !slices.Equal(calls[:len(want)], want) {
		t.Errorf("calls:\n%s\nwant first:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
	if c, ok := has(calls, "docker exec -i -u u -w / "); !ok || !strings.HasSuffix(c, " "+inst+" sh -c "+loginEnv+" sh bash -s") {
		t.Errorf("base.sh as the user: %q", c)
	}
	for _, p := range []string{
		"docker exec -i -u 0 " + inst + " sh -s -- off",
		"docker container stop -t 10 " + inst,
		"docker container commit --change LABEL devvm.kind=image --change LABEL devvm.key=" + baseKey + " " + inst + " " + s.imageRef(baseKey),
		"docker container rm -f -v " + inst,
	} {
		if _, ok := has(calls, p); !ok {
			t.Errorf("no %q in\n%s", p, strings.Join(calls, "\n"))
		}
	}
	if keys, err := s.Images(t.Context()); err != nil || !slices.Equal(keys, []string{baseKey}) {
		t.Errorf("Images = %v, %v", keys, err)
	}
	if n, err := s.ImageSize(t.Context(), baseKey); err != nil || n != 500000000 {
		t.Errorf("ImageSize = %d, %v", n, err)
	}
}

// A layer is a container from its parent's image; a failed script keeps
// it, stopped, and its key is an unfinished image until it's deleted.
func TestBuildLayerFails(t *testing.T) {
	s := newSandbox(t)
	s.image(t, baseKey)
	s.touch(t, "script-fails")
	_, err := s.BuildImage(t.Context(), backend.ImageSpec{
		Image: "base+node", Key: nodeKey, Parent: baseKey, Net: backend.NetInternet,
		Scripts: []backend.Script{{Name: "node.sh", Data: []byte("#!/bin/bash\n# node\n")}},
	})
	inst := s.buildName(nodeKey)
	var le *backend.LoggedError
	if !errors.As(err, &le) || le.Msg.String() != msg.DockerScriptFailed("node.sh", inst).String() {
		t.Fatalf("err = %v", err)
	}
	if st := s.field(t, "containers/"+inst+"/status"); st != "exited" {
		t.Errorf("kept %q, want stopped", st)
	}
	if img := s.field(t, "containers/"+inst+"/image"); img != s.imageRef(baseKey) {
		t.Errorf("made from %q", img)
	}
	keys, err := s.Images(t.Context())
	if err != nil || !slices.Equal(keys, []string{baseKey, nodeKey}) {
		t.Errorf("Images = %v, %v", keys, err)
	}
	if err := s.DeleteImage(t.Context(), nodeKey); err != nil {
		t.Fatal(err)
	}
	if s.exists("containers/" + inst) {
		t.Error("the build container is still there")
	}
}

// Builds are refused where devvm can't confine them: an engine it doesn't
// know, which can't reach its proxy.
func TestBuildRefusedElsewhere(t *testing.T) {
	s := newSandbox(t)
	s.Kind = kindOther
	if ps := s.Problems(t.Context()); len(ps) > 0 {
		t.Fatal(ps)
	}
	_, err := s.BuildImage(t.Context(), backend.ImageSpec{Image: "base", Key: baseKey, Net: backend.NetInternet})
	if shown(err) != msg.BackendLacks(Name, "build-confinement", "base").Names() {
		t.Errorf("err = %v", err)
	}
	if calls := s.calls(t); len(calls) != 0 {
		t.Errorf("calls: %v", calls)
	}
}

// --from an image only Docker has runs it by Docker's ID, without a pull;
// one a registry has is pulled by its digest.
func TestBuildFrom(t *testing.T) {
	s := newSandbox(t)
	id := "sha256:" + strings.Repeat("d", 64)
	if err := os.WriteFile(filepath.Join(s.stub, "docker-image"), []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	built, err := s.BuildImage(t.Context(), backend.ImageSpec{
		Image: "base", Key: baseKey, Net: backend.NetInternet, From: &backend.From{Ref: "mine:1", Digest: id},
	})
	if err != nil || built.Origin != id {
		t.Fatalf("Built %+v, %v", built, err)
	}
	if _, ok := has(s.calls(t), "docker image pull"); ok {
		t.Error("pulled an image only Docker has")
	}
	s.reset(t)
	digest := "docker.io/library/node@sha256:" + strings.Repeat("e", 64)
	if _, err := s.BuildImage(t.Context(), backend.ImageSpec{
		Image: "base", Key: nodeKey, Net: backend.NetInternet, From: &backend.From{Ref: "node:24", Digest: digest},
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := has(s.calls(t), "docker image pull "+digest); !ok {
		t.Errorf("no pull: %v", s.calls(t))
	}
}

// createVM makes vm from base with a project folder, an agent's folder and
// the tools.
func (s *sandbox) createVM(t *testing.T, spec backend.VMSpec) {
	t.Helper()
	s.image(t, baseKey)
	if spec.Name == "" {
		spec.Name = vm
	}
	if err := s.CreateVM(t.Context(), baseKey, spec); err != nil {
		t.Fatal(err)
	}
}

// A VM's container: labelled, its config's hash on it, its project folder
// read-write, agents' folders and the tools volume read-only, its home a
// volume of its own, its hostname the folder's part of its name, base's
// size unless set.
func TestCreateVM(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{
		Project: "/p/web", AgentDirs: []string{"/h/.claude"}, Tools: "/share", ToolsAt: "/mnt/devvm-tools",
		Size: backend.Size{CPUs: 2},
	})
	c, ok := has(s.calls(t), "docker container create --name "+vm)
	if !ok {
		t.Fatalf("calls: %v", s.calls(t))
	}
	cfg, err := s.vmConfig(vm, record{Project: "/p/web", AgentDirs: []string{"/h/.claude"}, Tools: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "docker container create --name " + vm + " --label devvm.data=" + s.Dirs.Data + " --label devvm.config=" + cfg.hash() +
		" --label devvm.kind=vm --init --user 0:0 --entrypoint sleep --cpus 2 --memory 8192m --memory-swap 8192m" +
		" --hostname web --network bridge --mount type=bind,src=/p/web,dst=/p/web --mount type=bind,src=/h/.claude,dst=/h/.claude,readonly" +
		" --mount type=volume,src=" + s.toolsVolume() + ",dst=/mnt/devvm-tools,readonly --mount type=volume,src=" + vm + "-home,dst=/home/u.guest --cap-drop NET_RAW " +
		s.imageRef(baseKey) + " infinity"
	if c != want {
		t.Errorf("create:\n%s\nwant\n%s", c, want)
	}
	for _, v := range []string{vm + "-home", s.toolsVolume()} {
		if !strings.Contains(s.field(t, "volumes/"+v+"/labels"), LabelKey+"="+s.Dirs.Data) {
			t.Errorf("volume %s isn't labelled", v)
		}
	}
	in, err := s.Inspect(t.Context(), vm)
	if err != nil || in.Status != backend.StatusStopped || in.Size.CPUs != 2 || in.Size.MemoryGiB != 8 || !slices.Equal(in.AgentDirs, []string{"/h/.claude"}) {
		t.Errorf("Inspect = %+v, %v", in, err)
	}
	if ins, err := s.List(t.Context()); err != nil || len(ins) != 1 || ins[0].Name != vm {
		t.Errorf("List = %+v, %v", ins, err)
	}
	if users, err := s.imageUsers(t.Context(), baseKey); err != nil || !slices.Equal(users, []string{vm}) {
		t.Errorf("imageUsers = %v, %v", users, err)
	}
	if err := s.DeleteImage(t.Context(), baseKey); id(err) != "image_in_use" {
		t.Errorf("DeleteImage of an image in use: %v", err)
	}
}

// What the backend lacks is refused, and so is a container by the VM's
// name that isn't this devvm's, or a path Docker's --mount can't take.
func TestCreateVMRefused(t *testing.T) {
	s := newSandbox(t)
	s.image(t, baseKey)
	err := s.CreateVM(t.Context(), baseKey, backend.VMSpec{Name: vm, Size: backend.Size{DiskGiB: 200}})
	if shown(err) != msg.BackendLacks(Name, "disk", vm).Names() {
		t.Errorf("disk: %v", err)
	}
	if err := s.CreateVM(t.Context(), baseKey, backend.VMSpec{Name: vm, Project: "/p/a,b"}); id(err) != "docker_mount_path" {
		t.Errorf("a comma: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(s.stub, "docker", "containers", vm), 0o700); err != nil {
		t.Fatal(err)
	}
	s.touch(t, "docker-foreign-"+vm)
	if err := s.CreateVM(t.Context(), baseKey, backend.VMSpec{Name: vm, Project: "/p/web"}); id(err) != "docker_container_foreign" {
		t.Errorf("someone else's: %v", err)
	}
	if _, err := s.Inspect(t.Context(), vm); !errors.Is(err, backend.ErrNotFound) {
		t.Errorf("Inspect someone else's: %v", err)
	}
}

// Start: the services without Docker's daemon, and a --clone VM's clone
// folder given to its user. A container whose config is what the VM wants
// isn't made again.
func TestStart(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{CloneAt: "/p/web"})
	s.reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetDirect}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docker container start " + vm,
		"docker exec -i -u 0 " + vm + " sh -c " + envScript,
		"docker exec -i -u 0 -e DEVVM_DOCKERD=off -e DEVVM_LOOP=off -e DEVVM_SSHD=inetd " + vm + " sh -s --",
		"docker exec -u 0 " + vm + " chown u /p/web",
	}
	if calls := s.calls(t); !slices.Equal(calls, want) {
		t.Errorf("calls:\n%s\nwant\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(s.field(t, "containers/"+vm+"/mounts"), "type=volume,src="+vm+"-clone,dst=/p/web") {
		t.Errorf("mounts: %s", s.field(t, "containers/"+vm+"/mounts"))
	}
	// Without ssh: none to reach.
	if _, err := s.SSH(t.Context(), vm); id(err) != "vm_no_ssh" {
		t.Errorf("SSH without ssh: %v", err)
	}
	// Running: left as it is.
	s.reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil || len(s.calls(t)) != 0 {
		t.Errorf("a running VM: %v %v", s.calls(t), err)
	}
}

// New agent folders take effect at the next start: the container is
// committed and made again from that, with its CPUs, memory and volumes,
// under its name; the commit before goes once the next one is made.
func TestStartRemakes(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web", Size: backend.Size{CPUs: 3}})
	if err := s.SetAgentDirs(t.Context(), vm, []string{"/h/.claude"}); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	commit := s.commitRef(vm, 1)
	calls := s.calls(t)
	for i, p := range []string{
		"docker container commit --change LABEL devvm.kind=disk " + vm + " " + commit,
		"docker container create --name " + vm + "-next ",
		"docker container rm -f -v " + vm,
		"docker container rename " + vm + "-next " + vm,
		"docker container start " + vm,
	} {
		if i >= len(calls) || !strings.HasPrefix(calls[i], p) {
			t.Fatalf("calls:\n%s\nwant %d: %s", strings.Join(calls, "\n"), i, p)
		}
	}
	if !strings.Contains(calls[1], " --cpus 3 ") || !strings.Contains(calls[1], "type=bind,src=/h/.claude,dst=/h/.claude,readonly") || !strings.HasSuffix(calls[1], " "+commit+" infinity") {
		t.Errorf("made again: %s", calls[1])
	}
	r, err := s.readRecord(vm)
	if err != nil || r.Commit != commit || r.Gen != 1 {
		t.Errorf("record = %+v, %v", r, err)
	}
	// Again: the first commit goes.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAgentDirs(t.Context(), vm, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	if s.exists("images/"+commit) || !s.exists("images/"+s.commitRef(vm, 2)) {
		t.Error("the first commit wasn't replaced by the second")
	}
	// Delete takes the container, its volumes, its commit and its record.
	if err := s.Delete(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if s.exists("containers/"+vm) || s.exists("volumes/"+vm+"-home") || s.exists("images/"+s.commitRef(vm, 2)) {
		t.Error("Delete left something")
	}
	if _, err := os.Stat(s.Dirs.DockerVM(vm)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("record: %v", err)
	}
}

// A run stopped between deleting the old container and renaming the next
// one: the next one is renamed when the VM is next looked at.
func TestNextRenamed(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Name: vm + "-next", Project: "/p/web"})
	in, err := s.Inspect(t.Context(), vm)
	if err != nil || in.Name != vm {
		t.Fatalf("Inspect = %+v, %v", in, err)
	}
	if s.exists("containers/"+vm+"-next") || !s.exists("containers/"+vm) {
		t.Error("not renamed")
	}
}

// A commit with too many layers is flattened: exported and imported with
// the image's environment and devvm's labels.
func TestRemakeFlattens(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	if err := os.WriteFile(filepath.Join(s.stub, "docker", "containers", vm, "layers"), []byte("120\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAgentDirs(t.Context(), vm, []string{"/h/.codex"}); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	c, ok := has(s.calls(t), "docker image import ")
	if !ok || !strings.Contains(c, "--change LABEL devvm.data=\""+s.Dirs.Data+"\"") || !strings.Contains(c, `--change ENV PATH="/usr/bin"`) ||
		!strings.HasSuffix(c, " - "+s.commitRef(vm, 1)) {
		t.Errorf("import: %q", c)
	}
	if s.field(t, "images/"+s.commitRef(vm, 1)+"/layers") != "1" {
		t.Error("not one layer")
	}
}

// SetSize: CPUs and memory where they differ, on a stopped VM; a disk
// size is refused.
func TestSetSize(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	s.reset(t)
	if err := s.SetSize(t.Context(), vm, backend.Size{CPUs: 6, MemoryGiB: 8}); err != nil {
		t.Fatal(err)
	}
	if calls := s.calls(t); !slices.Equal(calls, []string{"docker update --cpus 6 " + vm}) {
		t.Errorf("calls: %v", calls)
	}
	if err := s.SetSize(t.Context(), vm, backend.Size{DiskGiB: 200}); id(err) != "backend_lacks" {
		t.Errorf("disk: %v", err)
	}
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSize(t.Context(), vm, backend.Size{CPUs: 2}); id(err) != "vm_not_stopped" {
		t.Errorf("running: %v", err)
	}
}

// The shared tools: built in a container from base's image with only the
// output folder mounted, then unpacked into the tools volume by another,
// with no network, from the output folder; the manifest lists them. A
// start drops what the share lost, and unpacks the share again into a
// volume that's gone.
func TestTools(t *testing.T) {
	s := newSandbox(t)
	s.image(t, baseKey)
	out := t.TempDir()
	file := "node-24.1.0-cccccccccccc.erofs"
	err := s.BuildTools(t.Context(), backend.ToolsSpec{
		Base: baseKey, Out: out, OutAt: "/mnt/devvm-tools", Args: []string{"node", "24.1.0", file},
		Format: s.ToolFormat(), Net: backend.NetInternet, Script: backend.Script{Name: "build.sh", Data: []byte("#!/bin/bash\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := s.calls(t)
	if c, ok := has(calls, "docker container create --name "+s.toolsName()); !ok ||
		!strings.Contains(c, " --network bridge --mount type=bind,src="+out+",dst=/mnt/devvm-tools ") {
		t.Errorf("build container: %q", c)
	}
	if c, ok := has(calls, "docker container run --rm -i --name "+s.unpackName()); !ok ||
		!strings.Contains(c, " --network none ") || !strings.Contains(c, "--mount type=bind,src="+out+",dst=/mnt/devvm-share,readonly") ||
		!strings.HasSuffix(c, " "+s.imageRef(baseKey)+" -s -- /mnt/devvm-share /mnt/devvm-store +"+file) {
		t.Errorf("unpacking: %q", c)
	}
	if files, err := s.readManifest(); err != nil || !slices.Equal(files, []string{file}) {
		t.Errorf("manifest = %v, %v", files, err)
	}
	if !strings.HasPrefix(s.ToolFormat(), "dir-") {
		t.Errorf("ToolFormat = %q", s.ToolFormat())
	}
	// The share has it: the volume is kept as it is.
	if err := os.MkdirAll(s.Dirs.Tools(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dirs.Tools(), file), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	if err := s.syncTools(t.Context(), s.imageRef(baseKey)); err != nil || len(s.calls(t)) != 0 {
		t.Errorf("in sync: %v %v", s.calls(t), err)
	}
	// The volume gone: made again, from the share.
	if err := os.RemoveAll(filepath.Join(s.stub, "docker", "volumes", s.toolsVolume())); err != nil {
		t.Fatal(err)
	}
	if err := s.syncTools(t.Context(), s.imageRef(baseKey)); err != nil {
		t.Fatal(err)
	}
	if c, ok := has(s.calls(t), "docker container run "); !ok || !strings.Contains(c, "src="+s.Dirs.Tools()+",dst=/mnt/devvm-share,readonly") || !strings.HasSuffix(c, " +"+file) {
		t.Errorf("again: %v", s.calls(t))
	}
	// The share lost it (devvm gc): it goes from the volume.
	if err := os.Remove(filepath.Join(s.Dirs.Tools(), file)); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	if err := s.syncTools(t.Context(), s.imageRef(baseKey)); err != nil {
		t.Fatal(err)
	}
	if c, ok := has(s.calls(t), "docker container run "); !ok || !strings.HasSuffix(c, " -"+file) {
		t.Errorf("dropped: %v", s.calls(t))
	}
	if files, err := s.readManifest(); err != nil || len(files) != 0 {
		t.Errorf("manifest = %v, %v", files, err)
	}
}

// What a VM holds: its container's writes and its volumes, against
// base's disk.
func TestDiskUsed(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	used, limit, err := s.DiskUsed(t.Context(), vm)
	if err != nil || used != 1048576+1500000000 || limit != 100<<30 {
		t.Errorf("DiskUsed = %d, %d, %v", used, limit, err)
	}
	for in, want := range map[string]int64{"3.238kB": 3238, "1.2GB": 1200000000, "0B": 0, "12MB": 12000000, "x": 0, "1TB": 1e12} {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %d", in, got)
		}
	}
}

// Commands in a VM run as its user with a login's environment; a shell
// gets a terminal when devvm has one; without ssh, through docker exec.
func TestExecAndShell(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	s.Terminal = func() bool { return true }
	c, err := s.Shell(t.Context(), vm, "/p/web")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docker", "exec", "-i", "-t", "-u", "u", "-w", "/p/web", "-e", "USER=u", "-e", "LOGNAME=u",
		"-e", "HOME=/home/u.guest", "-e", "SHELL=/bin/bash", vm, "sh", "-c", loginEnv, "sh", "bash", "-l",
	}
	if !slices.Equal(c.Args, want) {
		t.Errorf("Shell = %q", c.Args)
	}
}

func TestHostname(t *testing.T) {
	for in, want := range map[string]string{
		"devvm-web-0123456789abcd": "web", "devvm-my-app-0123456789abcd": "my-app", "devvm-img-x": "img", "other": "other",
	} {
		if got := hostname(in); got != want {
			t.Errorf("hostname(%q) = %q", in, got)
		}
	}
}
