package cli

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/vm"
)

// shape is a JSON value's fields, one "path: type" line each, sorted: an
// array's elements are path[], and only its first is read.
func shape(t *testing.T, data []byte) []string {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%v:\n%s", err, data)
	}
	var out []string
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				walk(strings.TrimPrefix(path+"."+k, "."), e)
			}
		case []any:
			if len(x) == 0 {
				out = append(out, path+": empty")
				return
			}
			walk(path+"[]", x[0])
		case string:
			out = append(out, path+": string")
		case float64:
			out = append(out, path+": number")
		case bool:
			out = append(out, path+": bool")
		case nil:
			out = append(out, path+": null")
		}
	}
	walk("", v)
	sort.Strings(out)
	return out
}

// Each command's --json shape: its field names and types, from a sample
// with every field set. A field is only ever added: a change to one that
// fails here is a new jsonVersion.
func TestJSONShapes(t *testing.T) {
	yes := true
	used := 1.5
	fix := "brew install lima"
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		command string
		v       any
		want    []string
	}{
		{
			"ls", lsJSON([]vm.Listed{{Name: "devvm-a", Status: "Running", Image: "base", From: "node:24", Git: "on", Net: "on", Backend: "docker", DockerLayer: "sysbox", Folder: "/a"}}),
			[]string{
				"vms[].backend: string", "vms[].docker_layer: string", "vms[].folder: string", "vms[].from: string", "vms[].git: string", "vms[].image: string",
				"vms[].name: string", "vms[].net: string", "vms[].status: string",
			},
		},
		{
			"images", imagesJSON(
				[]listedImage{{Listed: build.Listed{Image: build.Image{Key: "k", Name: "base"}, SHA256: "ab", State: "current", Origin: "ubuntu 26.04", Size: 1, Sized: true}, Backend: "lima"}},
				[]tools.Listed{{Name: "node", Tool: "node", Version: "24", Built: true, Size: 2, VMs: 1, State: "current"}},
				func(tools.Listed) (string, string) { return "lima", "gnu" }, nixCacheJSON{Files: 1, Size: 2, Cap: 3}),
			[]string{
				"images[].backend: string", "images[].digest: string", "images[].from: string", "images[].key: string", "images[].name: string",
				"images[].size: number", "images[].sized: bool", "images[].state: string",
				"nix_cache.cap: number", "nix_cache.files: number", "nix_cache.size: number",
				"tools[].backend: string", "tools[].built: bool", "tools[].libc: string", "tools[].name: string", "tools[].size: number",
				"tools[].state: string", "tools[].tool: string", "tools[].version: string", "tools[].vms: number",
				"total: number", "unused_tools: number",
			},
		},
		{
			"tools", toolsJSON([]tools.VMTool{{Tool: "node", Version: "24.1", Newer: "24.2", Built: true}}),
			[]string{"tools[].newer: string", "tools[].newer_built: bool", "tools[].tool: string", "tools[].version: string"},
		},
		{
			"size", sizeJSON(vm.SizeShown{Size: backend.Size{CPUs: 4, MemoryGiB: 8, DiskGiB: 100}, DiskUsedGiB: used, DiskMeasured: true, Pending: backend.Size{CPUs: 2, MemoryGiB: 4, DiskGiB: 120}}),
			[]string{
				"cpus: number", "disk_gib: number", "disk_used_gib: number", "memory_gib: number",
				"pending.cpus: number", "pending.disk_gib: number", "pending.memory_gib: number",
			},
		},
		{"ports", map[string]string{"ports": "all"}, []string{"ports: string"}},
		{
			"network",
			networkOutput{Policy: "on", Allowlist: []policyFileJSON{{File: "/p", Rules: []string{"a.test"}}}},
			[]string{"allowlist[].file: string", "allowlist[].rules[]: string", "policy: string"},
		},
		{"network -g", map[string]string{"policy": "ask"}, []string{"policy: string"}},
		{
			"network log",
			logOutput{Events: []logEventJSON{{
				Time: "t", Event: "forward", Host: "h", Port: 443, Method: "GET", Path: "/",
				Rule: "h", Reason: "r", Agent: "claude", Secrets: []string{"github"}, Refused: true, Upstream: "proxy:3128",
			}}},
			[]string{
				"events[].agent: string", "events[].event: string", "events[].host: string", "events[].method: string", "events[].path: string",
				"events[].port: number", "events[].reason: string", "events[].refused: bool", "events[].rule: string", "events[].secrets[]: string",
				"events[].time: string", "events[].upstream: string",
			},
		},
		{
			"network log --blocked",
			blockedOutput{Blocked: []blockJSON{blockJSON(netpolicy.Block{Last: "t", Count: 2, Host: "h", Port: 443, Allow: "h"})}},
			[]string{"blocked[].allow: string", "blocked[].count: number", "blocked[].host: string", "blocked[].last: string", "blocked[].port: number"},
		},
		{
			"ssh-agent",
			sshAgentOutput{Mode: "devvm", Keys: []agentKeyJSON{agentKeyJSON(sshagent.ListedKey{File: "/k", Fingerprint: "SHA256:x", Confirm: true, Lifetime: "12h", Loaded: true})}},
			[]string{"keys[].confirm: bool", "keys[].file: string", "keys[].fingerprint: string", "keys[].lifetime: string", "keys[].loaded: bool", "mode: string"},
		},
		{"git-protect", map[string]string{"mode": "on"}, []string{"mode: string"}},
		{
			"secret",
			secretOutput{Secrets: []secretJSON{{Name: "github", Saved: true, UsedBy: []string{"devvm-a"}}}},
			[]string{"secrets[].name: string", "secrets[].saved: bool", "secrets[].used_by[]: string"},
		},
		{
			"history", historyJSON([]savedExport{{Path: "/e.tar.gz", VM: "devvm-a", Folder: "/a", At: at, Size: 3}}),
			[]string{"exports[].date: string", "exports[].file: string", "exports[].folder: string", "exports[].size: number", "exports[].vm: string"},
		},
		{"space", spaceReport{
			Rows:  []spaceRow{{What: "VMs", Backend: "lima", Count: 1, Size: 1, Sized: true, Kept: keptOnlyCopy, GC: 1, Rest: "devvm rm", Retain: "30d", RetainFrees: 1}},
			Total: 1, GC: 1,
			Caches:  []cacheRow{{Backend: "lima", Name: "images", Path: "/c", Count: 1, Size: 1, Sized: true}},
			Unknown: []string{"docker"}, Strays: []strayVM{{Backend: "lima", Name: "devvm-b", Data: "/d"}},
		}, []string{
			"backends[].backend: string", "backends[].count: number", "backends[].name: string", "backends[].path: string",
			"backends[].size: number", "backends[].sized: bool",
			"devvm[].backend: string", "devvm[].count: number", "devvm[].gc: number", "devvm[].kept: string", "devvm[].rest: string",
			"devvm[].retain: string", "devvm[].retain_frees: number", "devvm[].size: number", "devvm[].sized: bool", "devvm[].what: string",
			"gc: number", "strays[].backend: string", "strays[].data: string", "strays[].name: string", "total: number", "unknown[]: string",
		}},
		{"retention", retentionJSON(state.Retention{}), retentionShape()},
		{
			"doctor",
			doctorOutput{
				Backend: "lima", Problems: []problemJSON{{Key: "lima", Fix: &fix, Message: "m"}, {Key: "kvm", Message: "m"}},
				Warnings: []string{"w"}, Docker: yes, Features: map[string]map[string]string{"lima": {"ssh": "native"}},
			},
			[]string{
				"backend: string", "docker: bool", "features.lima.ssh: string", "problems[].fix: string", "problems[].key: string",
				"problems[].message: string", "warnings[]: string",
			},
		},
	} {
		t.Run(tc.command, func(t *testing.T) {
			u, out, _ := testUI()
			if err := printJSON(u, tc.v); err != nil {
				t.Fatal(err)
			}
			want := append([]string{"version: number"}, tc.want...)
			sort.Strings(want)
			if got := shape(t, out.Bytes()); !slices.Equal(got, want) {
				t.Errorf("devvm %s --json:\n%s\nwant:\n%s", tc.command, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// retentionShape is every retention setting's key, a string each.
func retentionShape() []string {
	var out []string
	for _, k := range state.RetentionKeys() {
		out = append(out, "settings."+k+": string")
	}
	return out
}

// printJSON writes one indented object, "version" first, HTML characters
// as they are; nothing but an object.
func TestPrintJSON(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want string
	}{
		{struct{}{}, "{\n  \"version\": 1\n}\n"},
		{map[string]string{"a": "<b>&"}, "{\n  \"version\": 1,\n  \"a\": \"<b>&\"\n}\n"},
	} {
		u, out, _ := testUI()
		if err := printJSON(u, tc.v); err != nil || out.String() != tc.want {
			t.Errorf("printJSON(%v) = %q, %v; want %q", tc.v, out, err, tc.want)
		}
	}
	u, out, _ := testUI()
	if err := printJSON(u, []string{"a"}); err == nil || out.Len() != 0 {
		t.Errorf("an array: %q, %v", out, err)
	}
}
