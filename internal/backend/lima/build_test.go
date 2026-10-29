package lima

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ubuntu"
)

const (
	baseKey  = "0123456789ab"
	layerKey = "ba9876543210"
)

// buildLima is testLima with devvm's files in a temporary folder, a fixed
// Ubuntu image and a 16 GiB host. Each create or clone adds its instance,
// stopped, to what limactl list reports; parent is listed from the start
// with parentStatus, unless that's "".
func buildLima(t *testing.T, os string, parentStatus string) (*Lima, *fakeRunner) {
	t.Helper()
	l, f, _ := testLima(t, os)
	l.Assets = fstest.MapFS{"base.yaml": {Data: []byte("cpus: 4\nmemory: 8GiB\n")}}
	l.Ubuntu = func(context.Context) (ubuntu.Image, error) {
		return ubuntu.Image{URL: "https://example.com/r/ubuntu.img", Digest: "sha256:1111"}, nil
	}
	l.Memory = func() (uint64, error) { return 16 * gib, nil }
	l.Info = func(msg.M) {}
	list := ""
	if parentStatus != "" {
		list = `{"name":"devvm-img-` + baseKey + `","status":"` + parentStatus + `"}` + "\n"
	}
	f.out["limactl list --json"] = list
	f.do = func(c call) {
		line := c.line()
		switch {
		case strings.HasPrefix(line, "limactl create"), strings.HasPrefix(line, "limactl clone"):
			name := c.argv[len(c.argv)-1]
			if strings.HasPrefix(line, "limactl create") {
				name = strings.TrimPrefix(c.argv[4], "--name=")
			}
			f.out["limactl list --json"] += `{"name":"` + name + `","status":"Stopped"}` + "\n"
		case strings.HasPrefix(line, "limactl stop devvm-img-"+baseKey):
			f.out["limactl list --json"] = strings.Replace(f.out["limactl list --json"], parentStatus, "Stopped", 1)
		}
	}
	return l, f
}

func writeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// assetsDir is where l's files are written out.
func assetsDir(t *testing.T, l *Lima) string {
	t.Helper()
	dir, err := state.WriteAssets(l.Dirs, l.Assets)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func checkLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("call %d: %q, want it to start %q", i, got[i], want[i])
		}
	}
}

// base: a VM from Ubuntu's image with base.yaml, nothing mounted, started
// in its build sandbox, its script run, flushed and stopped.
func TestBuildImageBase(t *testing.T) {
	l, f := buildLima(t, "darwin", "")
	built, err := l.BuildImage(t.Context(), backend.ImageSpec{
		Image: "base", Key: baseKey, Net: backend.NetInternet,
		Scripts: []backend.Script{{Name: "base.sh", Data: []byte("#!/bin/bash\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := (backend.Built{Origin: "https://example.com/r/ubuntu.img sha256:1111", Libc: "gnu"}); built != want {
		t.Errorf("Built = %+v, want %+v", built, want)
	}
	inst := "devvm-img-" + baseKey
	checkLines(t, f.lines(), []string{
		"limactl create --tty=false --mount-none --name=" + inst +
			` --set .images = [{"location":"https://example.com/r/ubuntu.img","arch":"aarch64","digest":"sha256:1111"}]` +
			" --memory=8 " + filepath.Join(assetsDir(t, l), "base.yaml"),
		"limactl edit --tty=false " + inst + " --set .ssh.localPort=",
		"sandbox-exec -f " + l.profileFile(inst) + " ",
		"limactl shell " + inst + " bash -s",
		"limactl shell --workdir / " + inst + " sync",
		"limactl stop " + inst,
	})
	if p, err := os.ReadFile(l.profileFile(inst)); err != nil || !strings.Contains(string(p), `(deny network-outbound (remote ip "localhost:*"))`) {
		t.Errorf("the build's profile: %s, %v", p, err)
	}
}

// A layer: its parent stopped first, then a clone of it. On Linux hosts
// there's no sandbox for a build with the internet, and the image's disk is
// read-only once built.
func TestBuildImageLayer(t *testing.T) {
	l, f := buildLima(t, "linux", "Running")
	inst := "devvm-img-" + layerKey
	writeTestFile(t, l.disk(inst), "qcow2")
	built, err := l.BuildImage(t.Context(), backend.ImageSpec{
		Image: "foo", Key: layerKey, Parent: baseKey, Net: backend.NetInternet,
		Scripts: []backend.Script{{Name: "foo.sh", Data: []byte("#!/bin/bash\necho foo\n")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if built != (backend.Built{}) {
		t.Errorf("Built = %+v, want nothing above base", built)
	}
	checkLines(t, f.lines(), []string{
		"limactl stop devvm-img-" + baseKey,
		"limactl clone --tty=false --mount-none devvm-img-" + baseKey + " " + inst,
		"limactl edit --tty=false " + inst + " --set .portForwards = ",
		"limactl start --tty=false " + inst,
		"limactl shell " + inst + " bash -s",
		"limactl shell --workdir / " + inst + " sync",
		"limactl stop " + inst,
	})
	if fi, err := os.Stat(l.disk(inst)); err != nil || fi.Mode().Perm()&0o222 != 0 {
		t.Errorf("the image's disk: %v, %v", fi.Mode(), err)
	}
}

// A failed script is a logged failure naming it, with devvm's Lima home
// to look inside; the VM is stopped and kept, and nothing else runs.
func TestBuildImageScriptFails(t *testing.T) {
	l, f := buildLima(t, "darwin", "")
	inst := "devvm-img-" + baseKey
	f.fail = []string{"limactl shell " + inst + " bash -s"}
	_, err := l.BuildImage(t.Context(), backend.ImageSpec{
		Image: "base", Key: baseKey, Net: backend.NetInternet,
		Scripts: []backend.Script{{Name: "base.sh", Data: []byte("#!/bin/bash\n")}},
	})
	e, ok := errors.AsType[*backend.LoggedError](err)
	if !ok || e.Msg.ID() != "script_failed" {
		t.Fatalf("BuildImage = %v, want a logged script_failed", err)
	}
	if want := msg.ScriptFailed("base.sh", inst).Names(); e.Msg.Names() != want {
		t.Errorf("BuildImage = %s, want %s", e.Msg.Names(), want)
	}
	lines := f.lines()
	if last := lines[len(lines)-1]; last != "limactl stop "+inst {
		t.Errorf("last call %q, want the VM stopped", last)
	}
	for _, line := range lines {
		if strings.Contains(line, "sync") || strings.Contains(line, "delete") {
			t.Errorf("carried on after the script failed: %s", line)
		}
	}
}

func TestBuildImageRefuses(t *testing.T) {
	base := backend.ImageSpec{Image: "base", Key: baseKey, Net: backend.NetInternet}
	for name, change := range map[string]func(*backend.ImageSpec){
		"--from":             func(s *backend.ImageSpec) { s.From = &backend.From{Ref: "node:24"} },
		"an unconfined net":  func(s *backend.ImageSpec) { s.Net = backend.NetDirect },
		"the proxy's net":    func(s *backend.ImageSpec) { s.Net = backend.NetProxy },
		"a bad key":          func(s *backend.ImageSpec) { s.Key = "../x" },
		"a bad parent key":   func(s *backend.ImageSpec) { s.Parent = "x y" },
		"a missing parent":   func(s *backend.ImageSpec) { s.Parent = layerKey },
		"an existing image":  func(s *backend.ImageSpec) { s.Key = baseKey; s.Parent = "" },
		"no Ubuntu image":    func(*backend.ImageSpec) {},
		"no memory in GiB":   func(*backend.ImageSpec) {},
		"no host memory":     func(*backend.ImageSpec) {},
		"a missing base.yml": func(*backend.ImageSpec) {},
	} {
		t.Run(name, func(t *testing.T) {
			status := ""
			if name == "an existing image" {
				status = "Stopped"
			}
			l, f := buildLima(t, "darwin", status)
			switch name {
			case "no Ubuntu image":
				l.Ubuntu = func(context.Context) (ubuntu.Image, error) {
					return ubuntu.Image{}, msg.NoUbuntuImage("release-20260101", "x", "26.04")
				}
			case "no memory in GiB":
				l.Assets = fstest.MapFS{"base.yaml": {Data: []byte("memory: 8G\n")}}
			case "no host memory":
				l.Memory = func() (uint64, error) { return 0, errors.ErrUnsupported }
			case "a missing base.yml":
				l.Assets = fstest.MapFS{}
			}
			spec := base
			change(&spec)
			if _, err := l.BuildImage(t.Context(), spec); err == nil {
				t.Error("BuildImage succeeded")
			}
			for _, line := range f.lines() {
				if strings.HasPrefix(line, "limactl start") || strings.Contains(line, "limactl start") {
					t.Errorf("started a VM: %s", line)
				}
			}
		})
	}
}

// base's memory: base.yaml's, or half the host's when that leaves it less
// than 2 GiB.
func TestFitMemory(t *testing.T) {
	for host, want := range map[uint64]float64{16: 8, 10: 8, 8: 4, 7: 3.5} {
		if got := fitMemory(8, host*gib+gib/2); got != want {
			t.Errorf("fitMemory(8, %d GiB) = %v, want %v", host, got, want)
		}
	}
}

func TestLimaArch(t *testing.T) {
	for goarch, want := range map[string]string{"arm64": "aarch64", "amd64": "x86_64", "riscv64": "riscv64"} {
		if got := limaArch(goarch); got != want {
			t.Errorf("limaArch(%q) = %q, want %q", goarch, got, want)
		}
	}
}
