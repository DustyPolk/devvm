package vm

import (
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/settings"
)

// partLines are the names of a summary's lines for its toolchains, agents
// and layers, in order.
func partLines(names []string) []string {
	var out []string
	for _, n := range names {
		for _, p := range []string{"new_vm_toolchain", "new_vm_agent", "new_vm_layer"} {
			if strings.HasPrefix(n, p) {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

// A summary names the VM, then each toolchain, agent and layer with why
// it gets it, or none of a part, then its size, tier and ports, and the
// devcontainer.json it's made as, when there's one.
func TestSummaryLines(t *testing.T) {
	d := partsDefault{
		why:         map[string]string{"node": "package.json", "python": ".devcontainer/devcontainer.json", "docker": ".devcontainer/devcontainer.json"},
		setupLayers: []string{"nix"},
	}
	full := Size{CPUs: "4", Memory: "8", Disk: "100"}
	for _, tc := range []struct {
		name string
		s    summary
		want []string
	}{
		{
			name: "defaults",
			s: summary{
				vm: "v", d: d, p: Parts{Toolchains: []string{"node", "python"}, Agents: []string{"claude", "codex"}, Layers: []string{"nix", "docker"}},
				agentsFrom: "devvm setup", layersFrom: "config.toml", size: full, tier: "allowlist", ports: "all", dc: ".devcontainer/devcontainer.json",
			},
			want: []string{
				"new_vm_summary vm=v",
				"new_vm_toolchain_for file=package.json tool=node",
				"new_vm_toolchain_for file=.devcontainer/devcontainer.json tool=python",
				"new_vm_agent_from agent=claude from=devvm setup",
				"new_vm_agent_from agent=codex from=devvm setup",
				"new_vm_layer_from from=config.toml layer=nix",
				"new_vm_layer_for file=.devcontainer/devcontainer.json layer=docker",
				"new_vm_size cpus=4 disk=100 memory=8",
				"new_vm_network tier=allowlist",
				"new_vm_ports ports=all",
				"new_vm_devcontainer file=.devcontainer/devcontainer.json",
			},
		},
		{
			name: "given",
			s: summary{
				vm: "v", d: d, o: Options{Toolchains: &[]string{"go"}, Agents: &[]string{"opencode"}, Layers: &[]string{"nix"}},
				p:          Parts{Toolchains: []string{"go"}, Agents: []string{"opencode"}, Layers: []string{"nix"}},
				agentsFrom: "devvm setup", layersFrom: "devvm setup", size: full, tier: "none", ports: "3000,8000-8010",
			},
			want: []string{
				"new_vm_summary vm=v",
				"new_vm_toolchain_from from=--tools tool=go",
				"new_vm_agent_from agent=opencode from=--agents",
				"new_vm_layer_from from=--layers layer=nix",
				"new_vm_size cpus=4 disk=100 memory=8",
				"new_vm_network tier=none",
				"new_vm_ports ports=3000,8000-8010",
			},
		},
		{
			name: "none",
			s:    summary{vm: "v", d: d, size: full, tier: "open", ports: "none"},
			want: []string{
				"new_vm_summary vm=v", "new_vm_toolchains_none", "new_vm_agents_none", "new_vm_layers_none",
				"new_vm_size cpus=4 disk=100 memory=8", "new_vm_network tier=open", "new_vm_ports ports=none",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, m := range tc.s.lines() {
				got = append(got, m.Names())
			}
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}

// What a summary shows of a size it leaves unset: base's CPUs and memory
// fitted to the host, and base's disk; what's set stays.
func TestShownSize(t *testing.T) {
	for _, tc := range []struct {
		s    Size
		host Host
		want Size
	}{
		{Size{}, Host{CPUs: 8, MemoryGiB: 32}, Size{CPUs: "4", Memory: "8", Disk: "100"}},
		{Size{}, Host{CPUs: 2, MemoryGiB: 6}, Size{CPUs: "2", Memory: "3", Disk: "100"}},
		{Size{CPUs: "6", Disk: "200"}, Host{CPUs: 8, MemoryGiB: 32}, Size{CPUs: "6", Memory: "8", Disk: "200"}},
	} {
		if got := shownSize(tc.s, tc.host); got != tc.want {
			t.Errorf("shownSize(%+v, %+v) = %+v, want %+v", tc.s, tc.host, got, tc.want)
		}
	}
}

// What chose a default list: config.toml, else devvm setup.
func TestChosenBy(t *testing.T) {
	for source, want := range map[settings.Source]string{
		settings.FromConfig: "config.toml", settings.FromSettings: "devvm setup", settings.FromBuiltin: "devvm setup",
	} {
		if got := chosenBy(source); got != want {
			t.Errorf("chosenBy(%s) = %q, want %q", source, got, want)
		}
	}
}

// Something is left to choose unless the flags give every part, the
// tier can't be chosen, and the CPUs, the memory and the disk are given.
func TestLeftToChoose(t *testing.T) {
	none := &[]string{}
	all := Options{Toolchains: none, Agents: none, Layers: none, Size: Size{CPUs: "2", Memory: "4", Disk: "100"}}
	for _, tc := range []struct {
		name     string
		o        Options
		tierOpen bool
		want     bool
	}{
		{"nothing given", Options{}, true, true},
		{"everything given", all, false, false},
		{"the tier open", all, true, true},
		{"a part left", Options{Toolchains: none, Agents: none, Size: all.Size}, false, true},
		{"the disk left", Options{Toolchains: none, Agents: none, Layers: none, Size: Size{CPUs: "2", Memory: "4"}}, false, true},
		{"the memory left", Options{Toolchains: none, Agents: none, Layers: none, Size: Size{CPUs: "2"}}, false, true},
	} {
		if got := leftToChoose(tc.o, tc.tierOpen); got != tc.want {
			t.Errorf("%s: leftToChoose = %v, want %v", tc.name, got, tc.want)
		}
	}
}
