//go:build e2e

package e2e

import (
	"errors"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// ExecQuoting runs TestE2EExecQuoting on the running VM: Exec's
// arguments, folder and input reach the guest as given.
func ExecQuoting(s *S, vm string) error {
	return s.GoE2E("TestE2EExecQuoting", "backend/registry", "DEVVM_E2E_VM="+vm)
}

// Desktop is a devvm desktop --no-open running in the background, in
// names mode, until it says where the VM's desktop is: its address
// (127.0.0.1:PORT), port and password.
type Desktop struct {
	Addr, Port, Password string
	proc                 *Proc
}

// ServeDesktop starts devvm desktop --no-open args in dir and waits up to
// 4 minutes for it to say where the desktop is. Its fields are empty when
// it didn't.
func ServeDesktop(s *S, dir string, args ...string) (*Desktop, error) {
	d := &Desktop{}
	p, err := s.Start(Cmd{Args: append([]string{s.sb.devvm, "desktop", "--no-open"}, args...), Dir: dir, Env: []string{"DEVVM_MESSAGES=names"}})
	if err != nil {
		return d, err
	}
	d.proc = p
	for range 240 {
		if strings.Contains("\n"+p.Out(), "\ndesktop_ready ") || !p.Running() {
			break
		}
		time.Sleep(time.Second)
	}
	out := p.Out()
	if m := regexp.MustCompile(`(?m)^desktop_ready address=([^ ]*) `).FindStringSubmatch(out); m != nil {
		d.Addr = m[1]
		d.Port = d.Addr[strings.LastIndex(d.Addr, ":")+1:]
	}
	if m := regexp.MustCompile(`(?m)^desktop_ready .*password=([a-z0-9]*) `).FindStringSubmatch(out); m != nil {
		d.Password = m[1]
	}
	if d.Addr == "" || d.Password == "" {
		return d, errors.New("devvm desktop didn't say where the desktop is:\n" + out)
	}
	return d, nil
}

// View runs TestE2EDesktop on the desktop it serves: a wrong password
// refused, the password let in (with DEVVM_E2E_CLIPBOARD in vars, a
// clipboard and a key event sent).
func (d *Desktop) View(s *S, vars ...string) error {
	return s.GoE2E("TestE2EDesktop", "desktop", append([]string{"DEVVM_E2E_DESKTOP=" + d.Addr, "DEVVM_E2E_PASSWORD=" + d.Password}, vars...)...)
}

// End ends it with Ctrl+C: SIGINT to devvm alone, as a terminal's Ctrl+C
// reaches a command run with exec, then up to killAfter for it to end,
// then Stop.
func (d *Desktop) End() {
	if d == nil || d.proc == nil || !d.proc.Running() {
		return
	}
	_ = d.proc.cmd.Process.Signal(syscall.SIGINT)
	select {
	case <-d.proc.done:
	case <-time.After(killAfter):
		d.proc.Stop()
	}
}
