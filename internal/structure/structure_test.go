// Package structure checks the code's shape: how many methods a type has,
// that no func init runs, which package-level vars there are, and which
// packages import which. It has only tests.
package structure

import (
	"os"
	"testing"
)

// root is the repo's root, from this package's folder.
const root = "../.."

// Why a var in allowedVars is constants' kin.
const (
	ldflags = "set by -ldflags at build time"
	table   = "a fixed table: set here, only read"
	fixture = "a test's fixed input: set here, only read"
)

// devvm is what the repo is held to.
func devvm() rules {
	return rules{
		// It keeps a type to one job: one that grows past it splits by what
		// its methods do, as vm's Launch, Lifecycle, Settings and Net.
		maxMethods: 24,
		largeTypes: map[string]large{
			"internal/backend/lima.Lima":        {75, "a backend: backend.Backend's 28 methods, KeyFiles, DiskDirs, Floors, Usage, Location, Forget, Strays, DevvmCache, DeleteCache and Lima's own"},
			"internal/backend/smolvm.Smolvm":    {79, "a backend: backend.Backend's 28 methods, DiskDirs, Floors, Usage, Location, Forget, Strays, DevvmCache, DeleteCache and smolvm's own"},
			"internal/backend/docker.Docker":    {86, "a backend: backend.Backend's 28 methods, the optional ones it implements and Docker's own, its ssh, ports, agent and docker layer emulations among them; its docker commands are its client's"},
			"internal/backend/backendtest.Fake": {35, "backend.Backend's 28 methods and what tests ask of it"},
			"internal/paths.Dirs":               {41, "one method per folder or file devvm uses: a list"},
			"internal/ui.UI":                    {45, "the one terminal: messages and where they go, errors with the caller's exit code, prompts, status line, tables, run log and how many are kept, cleanups, signals"},
		},
		allowedVars: map[string]string{
			"internal/cli.version":                   ldflags,
			"internal/testbuild.testBuild":           ldflags + ", by test builds only",
			"internal/backend/registry.linuxURL":     ldflags + ", by releases' macOS builds only",
			"internal/backend/registry.linuxSHA256":  ldflags + ", by releases' macOS builds only",
			"internal/backend.Features":              table,
			"internal/backend.Native":                "a Support value, as a const would be",
			"internal/backend.None":                  "a Support value, as a const would be",
			"internal/backend/lima.sshLogin":         table,
			"internal/backend/smolvm.proxyVars":      table,
			"internal/backend/docker.desktopDefault": table,
			"internal/backend/docker.connectEnv":     table,
			"internal/cli.shells":                    table,
			"internal/gitprotect.Modes":              table,
			"internal/image.Toolchains":              table,
			"internal/image.Agents":                  table,
			"internal/image.Builtins":                table,
			"internal/image.Reserved":                table,
			"internal/msg.docs":                      table,
			"internal/msg.commandDocs":               table,
			"internal/msg/gen.taken":                 table,
			"internal/msg/gen.initialisms":           table,
			"internal/netpolicy.Env":                 table,
			"internal/netpolicy.directArgv":          table,
			"internal/project.markers":               table,
			"internal/secrets.Known":                 table,
			"internal/secrets.loginKinds":            table,
			"internal/sshagent.Modes":                table,
			"internal/state.sizeKeys":                table,
			"internal/state.imageKeys":               table,
			"internal/tools.miseSHA256":              table,
			"internal/trust.Env":                     table,
			"internal/vm.agentDirsOf":                table,
			"internal/vm.agentFilesOf":               table,
			"internal/vm.historyPaths":               table,
			"internal/vm.historyExcludes":            table,
			"internal/vm.defaultKeys":                table,
			"internal/backend/lima.nasty":            fixture,
			"internal/image.images":                  fixture,
			"internal/tools.gnuT":                    fixture,
			"internal/tools.muslT":                   fixture,
			"internal/ubuntu.sumArm64":               fixture,
			"internal/ubuntu.sumAmd64":               fixture,
			"internal/ubuntu.testAssets":             fixture,
		},
		// hostgit runs git without the repo's fsmonitor, hooks or filters,
		// over ssh only, without the environment's GIT_* and prompts.
		commands: map[string]string{"git": "internal/hostgit"},
		// readOverrides reads a test build's stand-ins, and a release
		// none.
		envOwner: "internal/cli/overrides.go",
		envSettings: map[string]envSetting{
			"DEVVM_NO_TOOL_CHECK":  {[]string{"internal/tools/tools.go"}, "a user setting: no daily check for newer shared tools"},
			"DEVVM_LIMA_HOME":      {[]string{"internal/paths/dirs.go"}, "a user setting: devvm's Lima home"},
			"DEVVM_SECRET_SERVICE": {[]string{"internal/secrets/secrets.go"}, "a user setting: the Keychain service secrets are saved under"},
			"DEVVM_FROM":           {[]string{"internal/cli/flags.go"}, "a user setting: --from's default"},
			"DEVVM_MESSAGES":       {[]string{"internal/msg/msg.go", "internal/ui/style.go"}, "names mode, which tests and e2e read messages in: any build has it"},
			"DEVVM_LOGIN_SALT":     {[]string{"internal/proxy/proxy.go"}, "what devvm gives its own proxy, devvm _proxy"},
			"DEVVM_GUESTFWD":       {[]string{"internal/backend/lima/qemu.go"}, "what devvm gives its own QEMU wrapper, devvm _qemu"},
		},
		layers: []layer{
			{
				name: "only cmd/devvm and internal/cli hold the UI",
				from: []string{"..."}, exceptFrom: []string{"cmd/devvm", "internal/cli/...", "internal/ui"},
				to: []string{"internal/ui"},
			},
			{
				name: "only internal/cli runs the commands on VMs",
				from: []string{"..."}, exceptFrom: []string{"internal/cli/...", "internal/vm"},
				to: []string{"internal/vm"},
			},
			{
				name: "only cmd/devvm runs the commands",
				from: []string{"..."}, exceptFrom: []string{"cmd/devvm", "internal/cli/..."},
				to: []string{"internal/cli/..."},
			},
			{
				name: "backends are below what uses them",
				from: []string{"internal/backend/..."},
				to: []string{
					"internal/vm", "internal/build", "internal/tools", "internal/netpolicy", "internal/secrets",
					"internal/project", "internal/gitprotect", "internal/sshconfig", "internal/sshagent", "internal/trust",
				},
			},
			{
				name: "only the registry and internal/cli name a backend",
				from: []string{"..."}, exceptFrom: []string{
					"internal/cli/...", "internal/backend/registry", "internal/backend/lima", "internal/backend/smolvm", "internal/backend/docker",
				},
				to: []string{"internal/backend/lima", "internal/backend/smolvm", "internal/backend/docker"},
			},
			{
				name: "backendtest is for tests",
				from: []string{"..."}, exceptFrom: []string{"internal/backend/backendtest"},
				to: []string{"internal/backend/backendtest"},
			},
			{
				name: "stubtest is for tests",
				from: []string{"..."}, exceptFrom: []string{"internal/stubtest"},
				to: []string{"internal/stubtest"},
			},
			{
				name: "internal/stubtest imports only internal/msg",
				from: []string{"internal/stubtest"}, exceptTo: []string{"internal/msg"},
				to: []string{"..."},
			},
			{
				name: "internal/proxy is standard library only",
				from: []string{"internal/proxy/..."}, exceptTo: []string{"internal/proxy/..."},
				to: []string{"..."},
			},
			{
				name: "leaves import nothing of devvm's",
				from: []string{"assets", "locales", "internal/platform", "internal/msg/gen", "internal/testbuild", "internal/hostgit"},
				to:   []string{"..."},
			},
			{
				name: "internal/msg imports only locales",
				from: []string{"internal/msg"}, exceptTo: []string{"locales"},
				to: []string{"..."},
			},
			{
				name: "internal/paths imports only internal/msg",
				from: []string{"internal/paths"}, exceptTo: []string{"internal/msg"},
				to: []string{"..."},
			},
			{
				name: "internal/state imports only internal/msg and internal/paths",
				from: []string{"internal/state"}, exceptTo: []string{"internal/msg", "internal/paths"},
				to: []string{"..."},
			},
			{
				name: "internal/image imports only assets, internal/msg and internal/paths",
				from: []string{"internal/image"}, exceptTo: []string{"assets", "internal/msg", "internal/paths"},
				to: []string{"..."},
			},
			{
				name: "internal/devcontainer imports only internal/image and internal/msg",
				from: []string{"internal/devcontainer"}, exceptTo: []string{"internal/image", "internal/msg"},
				to: []string{"..."},
			},
		},
	}
}

// The repo keeps to its rules.
func TestStructure(t *testing.T) {
	m, err := load(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.files) == 0 {
		t.Fatal("no Go files found")
	}
	for _, v := range check(m, devvm()) {
		t.Error(v)
	}
}
