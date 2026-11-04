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
	derived = "a fixed table's view, worked out once from it, only read"
)

// devvm is what the repo is held to.
func devvm() rules {
	return rules{
		// It keeps a type to one job, an interface too: one that grows past
		// it splits by what its methods do, as vm's Launch, Lifecycle,
		// Settings and Net, and backend.Backend's roles, which Lima's and the
		// fake's own types each implement.
		maxMethods: 24,
		largeTypes: map[string]large{
			"internal/paths.Dirs": {54, "one method per folder or file devvm uses: a list (a VM's own files are paths.VM's)"},
			"internal/ui.UI":      {47, "the one terminal: messages and where they go, as text or names, errors with the caller's exit code, prompts and whether one can show, status line, tables, run log and how many are kept, cleanups, signals"},
		},
		allowedVars: devvmVars(),
		// hostgit runs git without the repo's fsmonitor, hooks or filters,
		// over ssh only, without the environment's GIT_* and prompts.
		commands:  map[string]string{"git": "internal/hostgit"},
		execSites: devvmExecSites(),
		// The registry's backends are behind its guard: only backend.As
		// finds their other interfaces.
		guardOwner: "internal/backend/guard.go",
		literals:   devvmLiterals(),
		// paths names every file and folder devvm keeps, so the layout,
		// its golden listing and a VM's teardown miss none.
		pathsOwner: "internal/paths",
		// readOverrides reads a test build's stand-ins, and a release
		// none.
		envOwner:    "internal/cli/overrides.go",
		envSettings: devvmEnvSettings(),
		// paths.Dirs.Home is the home folder, read once: tests set HOME,
		// and e2e gives a run a HOME of its own.
		envOwners: map[string]string{"HOME": "internal/paths/dirs.go"},
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
				// vmssh's port forwarder and link sessions are devvm's,
				// on every backend: none runs its own.
				name: "backends are below what uses them",
				from: []string{"internal/backend/..."},
				to: []string{
					"internal/vm", "internal/build", "internal/tools", "internal/netpolicy", "internal/secrets",
					"internal/project", "internal/gitprotect", "internal/hostsafety", "internal/sshconfig", "internal/sshagent", "internal/trust",
					"internal/vmssh",
				},
			},
			{
				name: "only the registry and internal/cli name a backend",
				from: []string{"..."}, exceptFrom: []string{
					"internal/cli/...", "internal/backend/registry", "internal/backend/lima",
				},
				to: []string{"internal/backend/lima"},
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
				name: "internal/proxy is standard library only, with internal/daemon",
				from: []string{"internal/proxy/..."}, exceptTo: []string{"internal/proxy/...", "internal/daemon"},
				to: []string{"..."},
			},
			{
				name: "leaves import nothing of devvm's",
				from: []string{
					"assets", "locales", "internal/platform", "internal/msg/gen", "internal/testbuild", "internal/hostgit",
					"internal/shell", "internal/vmname", "internal/daemon", "internal/fetch", "internal/jsonc",
					"internal/redact",
				},
				to: []string{"..."},
			},
			{
				// Agents' logins hold their tokens: the package that reads,
				// refreshes and writes them knows nothing of VMs or the UI.
				name: "internal/agentlogin imports only internal/fetch, internal/jsonc, internal/proxy and internal/state",
				from: []string{"internal/agentlogin"}, exceptTo: []string{"internal/fetch", "internal/jsonc", "internal/proxy", "internal/state"},
				to: []string{"..."},
			},
			{
				name: "internal/msg imports only locales",
				from: []string{"internal/msg"}, exceptTo: []string{"locales"},
				to: []string{"..."},
			},
			{
				name: "internal/paths imports only internal/msg and internal/vmname",
				from: []string{"internal/paths"}, exceptTo: []string{"internal/msg", "internal/vmname"},
				to: []string{"..."},
			},
			{
				name: "internal/state imports only internal/msg, internal/paths, internal/platform and internal/vmname",
				from: []string{"internal/state"}, exceptTo: []string{"internal/msg", "internal/paths", "internal/platform", "internal/vmname"},
				to: []string{"..."},
			},
			{
				name: "internal/image imports only assets, internal/msg and internal/paths",
				from: []string{"internal/image"}, exceptTo: []string{"assets", "internal/msg", "internal/paths"},
				to: []string{"..."},
			},
			{
				name: "internal/devcontainer imports only internal/image, internal/jsonc, internal/msg and internal/shell",
				from: []string{"internal/devcontainer"}, exceptTo: []string{"internal/image", "internal/jsonc", "internal/msg", "internal/shell"},
				to: []string{"..."},
			},
		},
	}
}

// devvmVars are the package-level vars that are constants' kin, each with
// why.
func devvmVars() map[string]string {
	return map[string]string{
		"internal/agentlogin.opencodeAPI":         table,
		"internal/agentlogin.opencodeOAuth":       table,
		"internal/agentlogin.codexKnown":          table,
		"internal/agentlogin.claudeTopKeys":       table,
		"internal/agentlogin.claudeServerKeys":    table,
		"internal/agentlogin.openaiAuth":          fixture,
		"internal/cli.version":                    ldflags,
		"internal/testbuild.testBuild":            ldflags + ", by test builds only",
		"internal/backend.Features":               table,
		"internal/backend.Native":                 "a Support value, as a const would be",
		"internal/backend.None":                   "a Support value, as a const would be",
		"internal/backend/lima.sshLogin":          table,
		"internal/cli.shells":                     table,
		"internal/gitprotect.Modes":               table,
		"internal/image.SharedTools":              table,
		"internal/image.Toolchains":               derived,
		"internal/image.Agents":                   derived,
		"internal/image.Builtins":                 table,
		"internal/backend/lima.conversionScripts": table,
		"internal/msg.docs":                       table,
		"internal/msg.commandDocs":                table,
		"internal/msg/gen.taken":                  table,
		"internal/msg/gen.initialisms":            table,
		"internal/netpolicy.Env":                  table,
		"internal/netpolicy.directArgv":           table,
		"internal/vm.nixMarkers":                  table,
		"internal/proxy.notPublic":                table,
		"internal/proxy.nat64":                    table,
		"internal/proxy.githubMinting":            table,
		"internal/secrets.Known":                  table,
		"internal/secrets.loginKinds":             table,
		"internal/sshagent.Modes":                 table,
		"internal/tools.miseSHA256":               table,
		"internal/trust.Env":                      table,
		"internal/vm.agentDirsOf":                 table,
		"internal/vm.agentFilesOf":                table,
		"internal/vm.historyPaths":                table,
		"internal/vm.historyExcludes":             table,
		"internal/vm.defaultKeys":                 table,
		"internal/shell.nasty":                    fixture,
		"internal/build.keyImages":                fixture,
		"internal/tools.gnuT":                     fixture,
		"internal/tools.muslT":                    fixture,
	}
}

// devvmEnvSettings are the DEVVM_ variables read outside the overrides,
// each with the files that read it and why.
func devvmEnvSettings() map[string]envSetting {
	return map[string]envSetting{
		"DEVVM_NO_TOOL_CHECK":  {[]string{"internal/tools/tools.go"}, "a user setting: no daily check for newer shared tools"},
		"DEVVM_LIMA_HOME":      {[]string{"internal/paths/dirs.go"}, "a user setting: devvm's Lima home"},
		"DEVVM_SECRET_SERVICE": {[]string{"internal/secrets/secrets.go"}, "a user setting: the Keychain service secrets are saved under"},
		"DEVVM_FROM":           {[]string{"internal/cli/flags.go"}, "a user setting: --from's default"},
		"DEVVM_MESSAGES":       {[]string{"internal/ui/style.go"}, "names mode, which tests and e2e read messages in: any build has it"},
		"DEVVM_GUESTFWD":       {[]string{"internal/backend/lima/qemu.go"}, "what devvm gives its own QEMU wrapper, devvm _qemu"},
		"DEVVM_DAEMON_STATUS":  {[]string{"internal/daemon/status.go"}, "what devvm gives its own background processes: where each says it's ready"},
	}
}

// devvmExecSites are the files that make a host command (exec.Command or
// exec.CommandContext), each with why. Elsewhere a command goes through
// one of their runners: the UI's LoggedCmd, which a package takes as a
// field, a backend's Runner, or hostgit.
func devvmExecSites() map[string]string {
	return map[string]string{
		"internal/hostgit/hostgit.go":          "the host git runner: git without the repo's fsmonitor, hooks or filters",
		"internal/backend/lima/limactl.go":     "limactl's command, which Lima's Runner runs into the run log",
		"internal/backend/lima/lima.go":        "qemu-img's command, which Lima's Runner runs",
		"internal/backend/lima/oci.go":         "docker save's command, which Lima's Runner runs",
		"internal/tools/mise.go":               "mise's command, in its own folders, which the UI's LoggedCmd runs, or whose output is read",
		"internal/from/from.go":                "docker's answers (an image's ID, whether it runs): output or exit code, nothing to log",
		"internal/sshagent/sshagent.go":        "OpenSSH's tools on devvm's agent socket, their output read or on the terminal",
		"internal/vm/sshagent.go":              "ssh-add on the terminal, which may ask for a passphrase",
		"internal/vm/desktop.go":               "ssh -W to the VM's desktop socket: a stream for the viewer's relay",
		"internal/vm/env.go":                   "macOS's open, for the desktop's viewer",
		"internal/vmssh/forward.go":            "the port forwarder's ssh master and -W relays: streams that last as long as the forwarder",
		"internal/vmssh/links.go":              "links' ssh sessions, which last as long as the link, their output to its log",
		"internal/platform/keychain_darwin.go": "security and tmutil: secrets on stdin and stdout, never in a log",
		"internal/platform/folder_darwin.go":   "chflags, for .git protection",
		"internal/daemon/daemon.go":            "devvm's background processes, which outlive the run and its context",
	}
}

// devvmLiterals are devvm's literals: a second shell quoter can let a value
// run as a command in a VM or on this machine, and a second copy of the
// VM-name grammar can read a project VM as an image, which gc deletes.
func devvmLiterals() map[string]literalOwner {
	return map[string]literalOwner{
		`'\''`:         {"internal/shell", "shell.Quote"},
		`[0-9a-f]{14}`: {"internal/vmname", "vmname.IsVM"},
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
