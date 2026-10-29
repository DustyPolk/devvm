package docker

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// The oldest Docker devvm runs: the oldest it was tested with.
const (
	minMajor = 29
	minMinor = 1
)

// Engine kinds.
const (
	// kindDesktop is Docker Desktop: its containers run in a VM of its
	// own, reach the host's loopback, and mount only folders it shares.
	kindDesktop = "desktop"
	// kindLinux is a rootful engine on this Linux machine: its containers
	// share its kernel, and don't reach its loopback.
	kindLinux = "linux"
	// kindOther is any other engine: another machine's, a rootless one,
	// or one in a VM of its own on macOS (colima, OrbStack), whose
	// containers devvm doesn't know to be confined.
	kindOther = "other"
)

// engine is Docker's engine as docker info has it: the fields devvm reads.
type engine struct {
	ServerVersion   string   `json:"ServerVersion"`
	OSType          string   `json:"OSType"`
	Architecture    string   `json:"Architecture"`
	OperatingSystem string   `json:"OperatingSystem"`
	Name            string   `json:"Name"`
	MemTotal        int64    `json:"MemTotal"`
	SecurityOptions []string `json:"SecurityOptions"`
	// Runtimes are the container runtimes it has, by name: sysbox-runc
	// where Sysbox is installed.
	Runtimes map[string]json.RawMessage `json:"Runtimes"`
	// kind is engineKind's, set when it's read.
	kind string
}

// sysboxRuntime is Sysbox's runtime, as Docker names it once Sysbox's
// package is installed.
const sysboxRuntime = "sysbox-runc"

// Check reports whether Docker can run VMs here: the first of Problems.
func (d *Docker) Check(ctx context.Context) error {
	if ps := d.Problems(ctx); len(ps) > 0 {
		return ps[0].Msg
	}
	return nil
}

// Problems lists what the backend needs that this host lacks: macOS or
// Linux, docker on PATH, an engine it reaches running Linux containers of
// the host's architecture, Docker 29.1 or newer. What it finds of the
// engine is kept, for Caps.
func (d *Docker) Problems(ctx context.Context) []backend.Problem {
	if d.OS != "darwin" && d.OS != "linux" {
		return []backend.Problem{{Key: "os", Msg: msg.DockerHostUnsupported(d.OS + "/" + d.Arch)}}
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return []backend.Problem{{Key: "docker", Msg: msg.DockerNotInstalled()}}
	}
	e, p, ok := d.readEngine(ctx)
	if !ok {
		return []backend.Problem{p}
	}
	var ps []backend.Problem
	if !recent(e.ServerVersion) {
		ps = append(ps, backend.Problem{Key: "docker-version", Msg: msg.DockerOld(e.ServerVersion)})
	}
	if e.OSType != "linux" {
		ps = append(ps, backend.Problem{Key: "docker-os", Msg: msg.DockerNotLinux(e.OSType)})
	}
	if want := unameArch(d.Arch); e.Architecture != want {
		ps = append(ps, backend.Problem{Key: "docker-arch", Msg: msg.DockerArch(e.Architecture, want)})
	}
	return ps
}

// readEngine asks docker info about the engine and keeps what it says; a
// problem when it can't: not running, or not the user's to reach.
func (d *Docker) readEngine(ctx context.Context) (engine, backend.Problem, bool) {
	out, err := d.Runner.Output(d.docker(ctx, "info", "--format", "{{json .}}"))
	var e engine
	if err == nil {
		err = json.Unmarshal(out, &e)
	}
	if err != nil || e.ServerVersion == "" {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok && bytes.Contains(bytes.ToLower(ee.Stderr), []byte("permission denied")) {
			fix := "sudo usermod -aG docker " + userName(d.User)
			return e, backend.Problem{Key: "docker-access", Fix: fix, Msg: msg.DockerAccess(fix)}, false
		}
		return e, backend.Problem{Key: "docker-engine", Msg: msg.DockerDown()}, false
	}
	e.kind = d.kindOf(e)
	d.mu.Lock()
	d.eng = &e
	d.mu.Unlock()
	return e, backend.Problem{}, true
}

// userName is the host user's name (current's), for a fix.
func userName(current func() (*user.User, error)) string {
	if u, err := current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "$USER"
}

// kindOf says which kind of engine e is: Docker Desktop; a rootful one on
// this Linux machine (it has the host's name); or another.
func (d *Docker) kindOf(e engine) string {
	if d.Kind != "" {
		return d.Kind
	}
	if strings.Contains(e.OperatingSystem, "Docker Desktop") {
		return kindDesktop
	}
	rootless := slices.ContainsFunc(e.SecurityOptions, func(o string) bool { return strings.Contains(o, "rootless") })
	host, err := d.Hostname()
	if d.OS == "linux" && !rootless && err == nil && host != "" && e.Name == host {
		return kindLinux
	}
	return kindOther
}

// engineKind is the kind of engine Problems last found; other until it
// looked.
func (d *Docker) engineKind() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.eng == nil {
		return kindOther
	}
	return d.eng.kind
}

// engineMemory is the memory the engine's machine has, in bytes; 0 until
// Problems looked.
func (d *Docker) engineMemory() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.eng == nil {
		return 0
	}
	return d.eng.MemTotal
}

// recent reports whether a version is 29.1 or newer.
func recent(v string) bool {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(strings.TrimRightFunc(parts[1], func(r rune) bool { return r < '0' || r > '9' }))
	if err1 != nil || err2 != nil {
		return false
	}
	return major > minMajor || major == minMajor && minor >= minMinor
}

// unameArch is a Go architecture as docker info has it.
func unameArch(goarch string) string {
	switch goarch {
	case "arm64":
		return "aarch64"
	case "amd64":
		return "x86_64"
	}
	return goarch
}

// Report says which engine devvm uses, whether it has Sysbox, and on
// Docker Desktop which folders it shares with containers.
func (d *Docker) Report(ctx context.Context) []msg.M {
	d.mu.Lock()
	e := d.eng
	d.mu.Unlock()
	if e == nil {
		ne, _, ok := d.readEngine(ctx)
		if !ok {
			return nil
		}
		e = &ne
	}
	var out []msg.M
	switch e.kind {
	case kindDesktop:
		out = append(out, msg.DockerEngineDesktop(e.ServerVersion))
		if dirs, ok := d.sharedFolders(); ok {
			out = append(out, msg.DockerFileSharing(strings.Join(dirs, " ")))
		}
	case kindLinux:
		out = append(out, msg.DockerEngineLinux(e.ServerVersion))
	default:
		out = append(out, msg.DockerEngineOther(e.ServerVersion, cmp.Or(e.OperatingSystem, e.Name)))
	}
	if _, ok := e.Runtimes[sysboxRuntime]; ok {
		out = append(out, msg.DockerSysbox())
	}
	// What network policy's relays run, and Docker Desktop's builds' too.
	var m msg.M
	if _, err := d.Linux(ctx, false); e.kind != kindOther && errors.As(err, &m) {
		out = append(out, m)
	}
	return out
}

// desktopDefault is what Docker Desktop shares when its settings don't
// say.
var desktopDefault = []string{"/Users", "/Volumes", "/private", "/tmp", "/var/folders"}

// sharedFolders are the host folders Docker Desktop shares with
// containers, as its settings file says; false when there's none to read.
func (d *Docker) sharedFolders() ([]string, bool) {
	if d.SharingFile == "" {
		return nil, false
	}
	b, err := os.ReadFile(d.SharingFile)
	if err != nil {
		return nil, false
	}
	var s struct {
		FilesharingDirectories *[]string `json:"FilesharingDirectories"`
	}
	if json.Unmarshal(b, &s) != nil {
		return nil, false
	}
	if s.FilesharingDirectories == nil {
		return slices.Clone(desktopDefault), true
	}
	return *s.FilesharingDirectories, true
}

// checkShared refuses a host folder Docker Desktop doesn't share with
// containers, where its settings say so: Docker would fail to start the
// container. Other engines mount what's there.
func (d *Docker) checkShared(path string) error {
	if d.engineKind() != kindDesktop {
		return nil
	}
	dirs, ok := d.sharedFolders()
	if !ok {
		return nil
	}
	for _, dir := range dirs {
		for _, p := range []string{path, resolved(path)} {
			if rel, err := filepath.Rel(dir, p); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
				return nil
			}
		}
	}
	return msg.DockerNotShared(path)
}

// resolved is a path with its links resolved, as far as it exists.
func resolved(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
