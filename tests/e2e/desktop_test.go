//go:build e2e

package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// desktopInVM is what's checked in a VM whose desktop runs, as its user:
// its Xvnc has its socket in a folder of the user's own (0700), the
// socket 0600, none of its sockets listening on TCP, and the clipboard
// off.
const desktopInVM = `set -e
d=/run/devvm-desktop pid=$(cat /run/devvm-desktop/xvnc.pid)
[ "$(cat "/proc/$pid/comm")" = Xvnc ]
[ "$(stat -c %a "$d")" = 700 ] && [ "$(stat -c %a "$d/vnc.sock")" = 600 ] && [ "$(stat -c %U "$d/vnc.sock")" = "$(id -un)" ]
for f in "/proc/$pid/fd/"*; do
  l=$(readlink "$f" 2>/dev/null) || continue
  case $l in socket:*) i=${l#socket:[}; i=${i%]}
    if awk -v i="$i" "\$4 == \"0A\" && \$10 == i { f = 1 } END { exit !f }" /proc/net/tcp /proc/net/tcp6 2>/dev/null; then
      echo "Xvnc listens on TCP"; exit 1; fi ;;
  esac
done
for p in AcceptCutText SendCutText; do [ "$(DISPLAY=:1 XAUTHORITY="$HOME/.Xauthority" vncconfig -get "$p")" = off ]; done`

// desktopSecret: the password, read from the input, is in no file of the
// VM's (run as root there), its fixed folders searched. Never on a
// command line: sudo would log it in the VM's journal.
const desktopSecret = `read -r pw && [ -n "$pw" ] && ! grep -rlsF -D skip -- "$pw" /home /root /etc /run /tmp /var/log /usr/local` //nolint:gosec // a script that looks for the password, not one

// fakeRFB is root's own server in the desktop's place: no password, its
// socket anyone's, everything that reaches it recorded in
// /tmp/devvm-e2e-rfb.bin, each connection counted.
const fakeRFB = `import os, socket
p = "/run/devvm-desktop/vnc.sock"
if os.path.exists(p):
    os.unlink(p)
s = socket.socket(socket.AF_UNIX)
s.bind(p)
os.chmod(p, 0o666)
s.listen(4)
n = 0
while True:
    c, _ = s.accept()
    n += 1
    open("/tmp/devvm-e2e-rfb.count", "w").write(str(n))
    c.sendall(b"RFB 003.008\n"); c.recv(12); c.sendall(b"\x01\x01"); c.recv(1); c.sendall(b"\0\0\0\0")
    c.recv(1)
    c.sendall(b"\0\x01\0\x01" + b"\0" * 16 + b"\0\0\0\0")
    data = b""
    while True:
        b = c.recv(4096)
        if not b:
            break
        data += b
    open("/tmp/devvm-e2e-rfb.bin", "wb").write(data)
    c.close()
`

// desktopSection: base has no desktop, and devvm desktop refuses it; a
// VM with the desktop layer and network policy serves its desktop here,
// a viewer checked against its password, even when root in the VM
// replaces the server.
func desktopSection(s *S) {
	s.Section("desktop")
	// base has no desktop: a VM of base has none of its programs, and devvm desktop refuses it, naming the
	// layer.
	b, _ := s.NewProject("nodesk")
	vm, _, _ := s.Launch(b, "exit", "--network", "open", "--no-git-protect", "--layers", "none")
	s.Check("a VM of base has none of the desktop's programs", func() error {
		if vm == "" || s.Paths.State(vm)["image"] != "base" {
			return errors.New("no VM of base")
		}
		_, err := s.Limactl("shell", "--workdir", "/", vm, "bash", "-c", `! command -v Xvnc && ! command -v startxfce4 && [[ ! -e /usr/local/bin/devvm-desktop ]]`)
		return err
	})
	s.Check("... and devvm desktop refuses it, naming the layer", func() error {
		r, err := s.DevvmNames(b, "desktop", "--no-open")
		if err == nil || !HasLine(r.Out, "devvm: desktop_no_layer vm="+vm) {
			return errors.New("not refused:\n" + r.Out)
		}
		return nil
	})
	_, _ = s.Devvm(b, "rm", "--force", "--no-export")
	desktopLayer(s)
}

// A VM with the desktop layer and network policy: devvm desktop serves its desktop on this machine's
// loopback alone, a viewer checked against its password here; the VM's server listens on a socket of
// its user's alone, never a port, and gets no way out. Then root in the VM replaces the server with
// one that asks for no password and records what reaches it: a viewer still needs the password, and
// its clipboard still never reaches the VM.
func desktopLayer(s *S) {
	p, _ := s.NewProject("desk")
	vm, _, _ := s.Launch(p, "exit", "--network", "allowlist", "--no-git-protect", "--layers", "desktop")
	st := func() map[string]string { return s.Paths.State(vm) }
	s.Check("--layers desktop makes a VM of the desktop image", func() error {
		if vm == "" || st()["image"] != "desktop" {
			return errors.New("no VM of the desktop image")
		}
		return nil
	})
	ps, psErr := WatchProcs(s)
	d, _ := ServeDesktop(s, p)
	var psSaw bool
	if psErr == nil {
		psSaw, psErr = ps.Saw(d.Password)
	}
	s.Check("devvm desktop serves its desktop on this machine's 127.0.0.1 alone, its port and password kept", func() error {
		port, _ := strconv.Atoi(d.Port)
		if vm == "" || port == 0 {
			return errNo
		}
		if err := loopbackOnly(s, port); err != nil {
			return err
		}
		if st()["ports.host.desktop"] != d.Port || st()["desktop.made.password"] != d.Password {
			return errors.New("its state doesn't keep its port and password")
		}
		return nil
	})
	s.Check("a viewer with a wrong password is refused, one with the password gets the VM's desktop", func() error { return d.View(s) })
	s.Check("the VM's server listens on its user's own socket, on no TCP port, the clipboard off", func() error {
		_, err := s.InVM(vm, desktopInVM)
		return err
	})
	s.Check("the password is in no file of the VM's", func() error {
		_, err := s.Run(Cmd{Args: []string{"limactl", "shell", "--workdir", "/", vm, "sudo", "sh", "-c", desktopSecret}, Stdin: d.Password + "\n"})
		return err
	})
	s.Check("... nor any run log, nor any host process's arguments while devvm desktop started", func() error {
		switch {
		case d.Password == "":
			return errNo
		case psErr != nil:
			return psErr
		case FilesHave(d.Password, filepath.Join(s.Paths.Devvm, "logs"), filepath.Join(s.Paths.VMDir(vm), "logs")):
			return errors.New("a run log has the password")
		case psSaw:
			return errors.New("a host process had the password in its arguments")
		}
		return nil
	})
	s.Check("with network policy, the VM still reaches only its proxy, and not the desktop's port here", func() error {
		if st()["net.made.check"] != "ok" {
			return errors.New("net.made.check=" + st()["net.made.check"])
		}
		if _, err := s.Limactl("shell", "--workdir", "/", vm, "curl", "--noproxy", "*", "-sS", "-m", "5", "-o", "/dev/null", "https://1.1.1.1"); err == nil {
			return errors.New("the VM reached 1.1.1.1")
		}
		if _, err := s.Limactl("shell", "--workdir", "/", vm, "timeout", "5", "bash", "-c", "</dev/tcp/host.lima.internal/"+d.Port); err == nil {
			return errors.New("the VM reached the desktop's port here")
		}
		return nil
	})
	// The VM's local proxy, which browsers use without a password: the desktop's session points at it,
	// it listens on the VM's loopback alone and adds the VM's password, a host the tier doesn't allow
	// still gets the proxy's 403, and the port forwarder never puts it on this machine.
	s.Check("the desktop's browsers use the VM's local proxy without a password, a blocked host still 403", func() error {
		_, err := s.InVM(vm, desktopLocalProxy)
		return err
	})
	s.Check("... and the local proxy isn't forwarded to this machine", func() error {
		time.Sleep(Settle)
		if Reaches(s, Local(3128)) {
			return errors.New("something here takes connections on 3128")
		}
		return nil
	})
	desktopFake(s, p, vm, d)
}

// desktopLocalProxy is what's checked of the VM's local proxy, as its
// user: the Xfce session's proxy variables point at it, it listens on
// 127.0.77.1 alone, curl reaches github.com through it without a
// password, example.com gets the proxy's 403 over CONNECT and plain HTTP,
// and an origin-form request gets 400.
const desktopLocalProxy = `set -e
lp=http://127.0.77.1:3128
s=$(pgrep -u "$(id -u)" -x xfce4-session | head -1)
tr "\0" "\n" <"/proc/$s/environ" | grep -qx "http_proxy=$lp"
tr "\0" "\n" <"/proc/$s/environ" | grep -qx "HTTPS_PROXY=$lp"
[ "$(awk "\$4 == \"0A\" && \$2 ~ /:0C38\$/ { print \$2 }" /proc/net/tcp /proc/net/tcp6)" = 014D007F:0C38 ]
curl -fsS -m 20 --noproxy "" -x "$lp" -o /dev/null https://github.com
if curl -sS -m 20 --noproxy "" -x "$lp" -o /dev/null https://example.com 2>"$HOME/lp.err"; then exit 1; fi
grep -q 403 "$HOME/lp.err"
[ "$(curl -s -m 20 --noproxy "" -x "$lp" -o "$HOME/lp.out" -w "%{http_code}" http://example.com/)" = 403 ]
grep -q "devvm: example.com:80 is blocked" "$HOME/lp.out"
[ "$(curl -s -m 20 --noproxy "*" -o /dev/null -w "%{http_code}" "$lp/")" = 400 ]
rm -f "$HOME/lp.err" "$HOME/lp.out"`

// desktopFake: root's own server in the desktop's place, then devvm
// desktop --clipboard.
func desktopFake(s *S, p, vm string, d *Desktop) {
	_ = os.WriteFile(filepath.Join(p, "fake-rfb.py"), []byte(fakeRFB), 0o644) //nolint:gosec // the VM's root reads it
	_, _ = s.InVM(vm, "sudo pkill -x Xvnc; sleep 1; sudo rm -f /tmp/devvm-e2e-rfb.*; sudo setsid -f python3 '"+p+"/fake-rfb.py' </dev/null >/dev/null 2>&1; sleep 1")
	s.Check("root's own server, without a password: a viewer with a wrong one is still refused, the password let in", func() error {
		return d.View(s, "DEVVM_E2E_CLIPBOARD=devvm-e2e-clip")
	})
	s.Check("... the refused viewer never reached the VM, nor the viewer's clipboard: only its key event did", func() error {
		time.Sleep(time.Second)
		_, err := s.Limactl("shell", "--workdir", "/", vm, "sh", "-c", `[ "$(cat /tmp/devvm-e2e-rfb.count)" = 1 ] &&
    od -An -tx1 /tmp/devvm-e2e-rfb.bin | tr -d " \n" | grep -q 040100000000006b &&
    ! grep -q devvm-e2e-clip /tmp/devvm-e2e-rfb.bin`)
		return err
	})
	d.End()
	s.Check("Ctrl+C ends devvm desktop, its port closed", func() error {
		port, _ := strconv.Atoi(d.Port)
		if port == 0 || Reaches(s, Local(port)) {
			return errors.New("its port is open: " + d.Port)
		}
		return nil
	})
	_, _ = s.InVM(vm, "sudo pkill -f '[f]ake-rfb.py'; sudo rm -f /run/devvm-desktop/vnc.sock /tmp/devvm-e2e-rfb.*")
	d2, _ := ServeDesktop(s, p, "--clipboard")
	s.Check("devvm desktop --clipboard starts its server again, on the same port, the clipboard on", func() error {
		if d2.Port == "" || d2.Port != s.Paths.State(vm)["ports.host.desktop"] {
			return errors.New("not on the same port: " + d2.Port)
		}
		_, err := s.Limactl("shell", "--workdir", "/", vm, "bash", "-lc", `DISPLAY=:1 XAUTHORITY=$HOME/.Xauthority vncconfig -get AcceptCutText | grep -qx on`)
		return err
	})
	s.Check("... and a viewer with the password gets it", func() error { return d2.View(s) })
	d2.End()
}
