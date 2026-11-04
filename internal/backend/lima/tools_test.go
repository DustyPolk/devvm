package lima

import (
	"errors"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
)

func toolsSpec(t *testing.T, out string) backend.ToolsSpec {
	t.Helper()
	return backend.ToolsSpec{
		Base: baseKey, Out: out, OutAt: "/mnt/devvm-tools", Net: backend.NetInternet,
		Script: backend.Script{Name: "build.sh", Data: []byte("#!/bin/bash\n")},
		Args:   []string{"node", "24.21.0", "node-24.21.0-0123456789ab.erofs"},
	}
}

// The tools build: base stopped first, a leftover build VM deleted, then a
// clone of base whose only mount is the build's own folder, writable,
// started confined, the script run with its arguments, and the VM deleted.
func TestBuildTools(t *testing.T) {
	l, f := buildLima(t, "linux", "Running")
	f.out["limactl list --json"] += `{"name":"` + ToolsVM + `","status":"Stopped"}` + "\n"
	out := t.TempDir()
	if err := l.BuildTools(t.Context(), toolsSpec(t, out)); err != nil {
		t.Fatal(err)
	}
	checkLines(t, f.lines(), []string{
		"limactl stop devvm-img-" + baseKey,
		"limactl delete -f " + ToolsVM,
		`limactl clone --tty=false --mount-none --set .mounts = [{"location":"` + out +
			`","mountPoint":"/mnt/devvm-tools","writable":true}] devvm-img-` + baseKey + " " + ToolsVM,
		"limactl edit --tty=false " + ToolsVM + " --set .propagateProxyEnv=false --set .portForwards = ",
		"limactl start --tty=false " + ToolsVM,
		"limactl shell --workdir / " + ToolsVM + " bash -s -- node 24.21.0 node-24.21.0-0123456789ab.erofs",
		"limactl delete -f " + ToolsVM,
	})
}

// A failed script is a logged tools_build_failed; the VM is left to the
// run's cleanup, so the run log ends with the script's output.
func TestBuildToolsFails(t *testing.T) {
	l, f := buildLima(t, "darwin", "Stopped")
	f.fail = []string{"limactl shell --workdir / " + ToolsVM + " bash -s"}
	err := l.BuildTools(t.Context(), toolsSpec(t, t.TempDir()))
	if e, ok := errors.AsType[*backend.LoggedError](err); !ok || e.Msg.ID() != "tools_build_failed" {
		t.Fatalf("BuildTools = %v", err)
	}
	lines := f.lines()
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "limactl shell --workdir / "+ToolsVM) {
		t.Errorf("last call %q, want the script", last)
	}
}

func TestBuildToolsRefuses(t *testing.T) {
	for name, change := range map[string]func(*backend.ToolsSpec){
		"a missing base":     func(s *backend.ToolsSpec) { s.Base = layerKey },
		"a comma in its out": func(s *backend.ToolsSpec) { s.Out = "/tmp/a,b" },
	} {
		t.Run(name, func(t *testing.T) {
			l, f := buildLima(t, "darwin", "Stopped")
			spec := toolsSpec(t, t.TempDir())
			change(&spec)
			if err := l.BuildTools(t.Context(), spec); err == nil {
				t.Error("BuildTools succeeded")
			}
			for _, line := range f.lines() {
				if strings.Contains(line, "clone") || strings.Contains(line, "start") {
					t.Errorf("made a VM: %s", line)
				}
			}
		})
	}
}
