//go:build e2e

package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// What the images, tools and clone sections read of the sandbox and its
// VMs.

// RunLogs is devvm's run logs by name, which starts with the second each
// run began (logs/runs/20261103-062206-PID.log): oldest first, but for
// runs that began in the same second (NewestLog goes by their times).
func (p Paths) RunLogs() []string {
	m, _ := filepath.Glob(filepath.Join(p.Devvm, "logs", "runs", "*.log"))
	return m
}

// RunLogHas is whether one of devvm's run logs has line, a whole line
// (grep -qx in logs/runs/*.log): what a build's script printed.
func (p Paths) RunLogHas(line string) bool {
	for _, f := range p.RunLogs() {
		if b, err := os.ReadFile(f); err == nil && HasLine(string(b), line) {
			return true
		}
	}
	return false
}

// MountLine is the host folders the VM's Lima config mounts, each with a
// space after it, as limactl list's template prints them: nothing for a
// VM that mounts none, or none at all.
func MountLine(s *S, vm string) string {
	r, err := s.Limactl("list", "--format", "{{range .Config.Mounts}}{{.Location}} {{end}}", vm)
	if err != nil {
		return ""
	}
	return strings.TrimRight(r.Stdout, "\n")
}

// MountsOf is MountLine's folders, one a line.
func MountsOf(s *S, vm string) string { return strings.Join(strings.Fields(MountLine(s, vm)), "\n") }

// HasLine is whether line is one of text's lines (grep -qx).
func HasLine(text, line string) bool { return slices.Contains(strings.Split(text, "\n"), line) }

// CaptureLine is the first group of the first of text's lines that
// matches re (sed -n 's/re/\1/p' | head -1): nothing when none does.
func CaptureLine(text, re string) string {
	if m := regexp.MustCompile("(?m)" + re).FindStringSubmatch(text); len(m) > 1 {
		return m[1]
	}
	return ""
}

// gitTool and sshTool are host git and ssh, with what installs them.
func gitTool() tool { return tool{"git", "git", "git", "git"} }
func sshTool() tool { return tool{"ssh", "openssh", "openssh-client", "openssh-clients"} }

// ExitStatus is the exit status a command's error says (Run's), 0 for
// none and -1 for one that never ran.
func ExitStatus(err error) int {
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		return ee.ExitCode()
	}
	return -1
}
