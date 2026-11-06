//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// SSHAgent starts a throwaway ssh-agent on the socket sock, stopped at
// the run's end, and is its pid.
func SSHAgent(s *S, sock string) (int, error) {
	r, err := s.Run(Cmd{Args: []string{"ssh-agent", "-a", sock}})
	if err != nil {
		return 0, err
	}
	m := regexp.MustCompile(`SSH_AGENT_PID=([0-9]+);`).FindStringSubmatch(r.Stdout)
	if m == nil {
		return 0, errors.New("ssh-agent printed no pid")
	}
	pid, _ := strconv.Atoi(m[1])
	s.Defer(func(context.Context) { StopPid(pid) })
	return pid, nil
}

// StopPid sends the process TERM, once: a process the run started
// outside a command (an ssh-agent, which forks).
func StopPid(pid int) {
	if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}

// KeyComments is the third field of each of ssh-add -l's lines, the
// keys' comments, one a line ($(ssh-add -l | awk '{ print $3 }')).
func KeyComments(out string) string {
	var cs []string
	for l := range strings.SplitSeq(out, "\n") {
		if f := strings.Fields(l); len(f) >= 3 {
			cs = append(cs, f[2])
		}
	}
	return strings.Join(cs, "\n")
}

// RfwdLinks checks that the VM's sshd, reached as host by the command
// ssh, binds a remote forward only on 127.77.0.0/16, the addresses of
// links: ssh's default (localhost), 127.0.0.1, 0.0.0.0, ::1, localhost
// and another range are refused, as is a remote socket forward (of
// sock, a socket here), and 127.77.0.250 binds (the control). A
// safeguard for an honest VM: the boundary is the host's ssh.
func RfwdLinks(s *S, host, sock string, ssh ...string) error {
	p := strconv.Itoa(s.Port(47613))
	for _, a := range []string{
		"0:127.0.0.1:9", p + ":127.0.0.1:9", "127.0.0.1:" + p + ":127.0.0.1:9", "0.0.0.0:" + p + ":127.0.0.1:9",
		"[::1]:" + p + ":127.0.0.1:9", "localhost:" + p + ":127.0.0.1:9", "127.78.0.2:" + p + ":127.0.0.1:9",
		fmt.Sprintf("/tmp/devvm-e2e-%d.sock:%s", os.Getpid(), sock),
	} {
		if _, err := s.Run(Cmd{Args: append(append(ssh[:len(ssh):len(ssh)], "-o", "ExitOnForwardFailure=yes", "-R", a), host, "true")}); err == nil {
			return errors.New("-R " + a + " bound")
		}
	}
	if _, err := s.Run(Cmd{Args: append(append(ssh[:len(ssh):len(ssh)], "-o", "ExitOnForwardFailure=yes", "-R", "127.77.0.250:"+p+":127.0.0.1:9"), host, "true")}); err != nil {
		return errors.New("-R 127.77.0.250 refused")
	}
	return nil
}
