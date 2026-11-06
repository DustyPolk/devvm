//go:build e2e

package e2e

import "runtime"

// section is one of e2e's sections: its name, its time when it has never
// run (for -e2e.slots to place it), the host commands it uses beyond
// baseTools, and its checks. One whose run is nil is still in
// tests/e2e.sh, which runs it; the rest are here, each in a file of its
// own (gc_test.go), and tests/e2e.sh hands them to TestE2E.
type section struct {
	name  string
	guess int // seconds
	tools []tool
	run   func(*S)
}

// sections is every section, in the order a run runs them: tests/e2e.sh's.
func sections() []section {
	return []section{
		{name: "images", guess: 600, run: imagesSection},
		{name: "project", guess: 900, tools: projectTools(), run: projectSection},
		{name: "clone", guess: 120, tools: []tool{gitTool(), sshTool()}, run: cloneSection},
		{name: "gc", guess: 120, run: gcSection},
		{name: "tools", guess: 600, tools: []tool{sshTool()}, run: toolsSection},
		{name: "network", guess: 600, tools: []tool{python3Tool(), sshTool()}, run: networkSection},
		{name: "desktop", guess: 300, tools: []tool{listenTool()}, run: desktopSection},
		{name: "nix", guess: 900, tools: []tool{sshTool()}, run: nixSection},
		{name: "from", guess: 1800, run: fromSection},
		{name: "link", guess: 900, tools: linkTools(), run: linkSection},
	}
}

// listenTool is what lists this machine's listeners: lsof on macOS, ss
// on Linux.
func listenTool() tool {
	if runtime.GOOS == "darwin" {
		return tool{"lsof", "lsof", "-", "-"}
	}
	return tool{"ss", "-", "iproute2", "iproute"}
}

// projectTools is what the project section runs on this machine: git,
// OpenSSL's s_server, OpenSSH's commands, tar, ps, the listeners' lister,
// and a terminal for a question (script on macOS, Python's pty on Linux).
func projectTools() []tool {
	tty := tool{"script", "-", "-", "-"}
	if runtime.GOOS != "darwin" {
		tty = tool{"python3", "-", "python3", "python3"}
	}
	return []tool{
		gitTool(),
		{"openssl", "openssl@3", "openssl", "openssl"},
		sshTool(),
		{"ssh-agent", "openssh", "openssh-client", "openssh-clients"},
		{"ssh-add", "openssh", "openssh-client", "openssh-clients"},
		{"ssh-keygen", "openssh", "openssh-client", "openssh-clients"},
		{"tar", "gnu-tar", "tar", "tar"},
		{"ps", "-", "procps", "procps-ng"},
		listenTool(), tty,
	}
}
