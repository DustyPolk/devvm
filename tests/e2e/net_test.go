//go:build e2e

package e2e

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Local is 127.0.0.1:port, and Local6 [::1]:port.
func Local(port int) string  { return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) }
func Local6(port int) string { return net.JoinHostPort("::1", strconv.Itoa(port)) }

// Reaches is whether something takes a TCP connection on addr (nc -z).
func Reaches(s *S, addr string) bool {
	c, err := dial(s, addr)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// dial connects to addr over TCP, for at most 2 s, in the window open now.
func dial(s *S, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 2 * time.Second}
	return d.DialContext(s.r.window().ctx, "tcp", addr)
}

// ReachesWithin waits up to timeout for addr to take a connection.
func ReachesWithin(s *S, timeout time.Duration, addr string) bool {
	return s.Until(timeout, func() error {
		if Reaches(s, addr) {
			return nil
		}
		return errors.New(addr + " takes no connection")
	}) == nil
}

// Answer is what a connection to addr reads before it's closed, within 2
// seconds, its trailing newlines gone (nc -w 2 ADDR </dev/null).
func Answer(s *S, addr string) string {
	c, err := dial(s, addr)
	if err != nil {
		return ""
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	b, _ := io.ReadAll(c)
	return Trim(string(b))
}

// hostProgram is a program on this machine that has a port: a listener
// of the run's own, with SO_REUSEADDR (Go's, as most servers set it).
type hostProgram struct {
	l    net.Listener
	done chan struct{}
}

// HostProgram listens on addr (":port" is every IPv4 address); with
// answer, it answers each connection with host-program and a newline,
// and without, it accepts none.
func HostProgram(s *S, addr string, answer bool) (*hostProgram, error) {
	var lc net.ListenConfig
	l, err := lc.Listen(s.r.ctx, "tcp4", addr)
	if err != nil {
		return nil, err
	}
	h := &hostProgram{l: l, done: make(chan struct{})}
	if !answer {
		close(h.done)
		return h, nil
	}
	go func() {
		defer close(h.done)
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("host-program\n"))
			_ = c.Close()
		}
	}()
	return h, nil
}

// Close lets its port go.
func (h *hostProgram) Close() {
	if h == nil {
		return
	}
	_ = h.l.Close()
	<-h.done
}

// LimaServe starts a server in the VM on each port, every address, left
// running.
func LimaServe(s *S, vm string, ports ...int) {
	for _, p := range ports {
		_, _ = s.InVM(vm, fmt.Sprintf("setsid -f python3 -m http.server -b 0.0.0.0 %d </dev/null >/dev/null 2>&1", p))
	}
}

// VMSay starts a server in the VM on addr:port alone that answers each
// connection with word and a newline (pkill -f '[d]evvm-e2e-say' ends
// it).
func VMSay(s *S, vm, addr string, port int, word string) {
	_, _ = s.InVM(vm, fmt.Sprintf("setsid -f python3 -c 'import socket,sys; s=socket.socket(); s.bind((sys.argv[1],int(sys.argv[2]))); s.listen(5); [(lambda c: (c.sendall(sys.argv[3].encode()+bytes([10])), c.close()))(s.accept()[0]) for _ in iter(int,1)]' %s %d %s devvm-e2e-say </dev/null >/dev/null 2>&1", addr, port, word))
}

// LimaBoot is the VM's boot's ID.
func LimaBoot(s *S, vm string) string {
	r, _ := s.InVM(vm, "cat /proc/sys/kernel/random/boot_id")
	return Trim(r.Stdout)
}

// PortsLive checks that devvm ports add applies to a running VM at once:
// the VM lists ports without port, with a server in it on port (serve),
// which doesn't reach this machine; devvm ports add says the VM forwards
// its new ports and the server reaches this machine at once; devvm ports
// rm closes it again. The VM is never restarted: boot (its boot or
// start) is the same after.
func PortsLive(s *S, dir, vm string, port int, serve func(*S, string, ...int), boot func(*S, string) string) error {
	b0 := boot(s, vm)
	serve(s, vm, port)
	time.Sleep(Settle)
	if Reaches(s, Local(port)) {
		return fmt.Errorf("%d reached before it's listed", port)
	}
	r, err := s.Devvm(dir, "ports", "add", strconv.Itoa(port))
	if err != nil {
		return err
	}
	now, _ := s.Devvm(dir, "ports")
	if !grepF(r.Out, s.Msg("ports_forwarding", "ports="+Trim(now.Stdout), "vm="+vm)) {
		return errors.New("devvm ports add didn't say the VM forwards its ports:\n" + r.Out)
	}
	if !ReachesWithin(s, 20*time.Second, Local(port)) {
		return fmt.Errorf("%d not forwarded once added", port)
	}
	if _, err := s.Devvm(dir, "ports", "rm", strconv.Itoa(port)); err != nil {
		return err
	}
	if s.Until(15*time.Second, func() error {
		if Reaches(s, Local(port)) {
			return errors.New("still forwarded")
		}
		return nil
	}) != nil {
		return fmt.Errorf("%d still forwarded once removed", port)
	}
	b1 := boot(s, vm)
	if b0 == "" || b0 != b1 {
		return fmt.Errorf("boot or start: %q before, %q after", b0, b1)
	}
	return nil
}

// HostPortKept checks a port a program on this machine has, which the VM
// lists, on every address: the VM's next start leaves it out, warned,
// and with servers in the VM on port and control (serve), port's
// localhost connections still reach the program and control's the VM.
// Once the program is gone, the next start forwards port again.
func HostPortKept(s *S, dir, vm string, port, control int, serve func(*S, string, ...int)) error {
	if _, err := s.Devvm(dir, "stop"); err != nil {
		return err
	}
	h, err := HostProgram(s, ":"+strconv.Itoa(port), true)
	if err != nil {
		return err
	}
	r, _ := s.Run(Cmd{Args: []string{s.sb.devvm}, Dir: dir, Stdin: "exit\n", Env: []string{"DEVVM_MESSAGES=names"}})
	if !HasLine(r.Out, fmt.Sprintf("devvm: ports_taken count=1 ports=%d vm=%s", port, vm)) {
		h.Close()
		return errors.New("the start didn't warn that the port is taken:\n" + r.Out)
	}
	serve(s, vm, port, control)
	ReachesWithin(s, 20*time.Second, Local(control))
	time.Sleep(Settle)
	out := Answer(s, Local(port))
	ok := Reaches(s, Local(control)) && out == "host-program"
	h.Close()
	if !ok {
		return fmt.Errorf("control %d forwarded: %v; %d answered by: %q", control, Reaches(s, Local(control)), port, out)
	}
	_, _ = s.Devvm(dir, "stop")
	r, _ = s.Run(Cmd{Args: []string{s.sb.devvm}, Dir: dir, Stdin: "exit\n", Env: []string{"DEVVM_MESSAGES=names"}})
	if strings.Contains(r.Out, "ports_taken") {
		return errors.New("the port is still taken:\n" + r.Out)
	}
	serve(s, vm, port)
	if !ReachesWithin(s, 20*time.Second, Local(port)) {
		return fmt.Errorf("%d not forwarded again", port)
	}
	return nil
}
