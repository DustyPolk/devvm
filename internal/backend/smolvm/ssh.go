package smolvm

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"io/fs"
	"maps"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// ssh for editors is emulated, and reviewed: smolvm runs no sshd, so devvm
// runs one of its own in each project VM, for which the caller asks it
// (Confinement.SSH):
//
//   - base has openssh-server, its host keys deleted (sshd.sh), and
//     services.sh makes the VM new ones (ed25519) at each start, before
//     sshd starts: no two VMs made from one pack share them, and devvm
//     reads them through smolvm's exec, as it does on Lima;
//   - sshd runs on devvm's own config (services.sh): the VM's user alone,
//     with the VM's own key (sshKey) alone, no passwords; local forwarding
//     only, no remote forwarding, tunnels or X11. Root in the VM can change
//     that: what keeps the VM's connections from going out through the
//     host is the host's ssh, which asks for none of those (devvm's entry
//     pins its options);
//   - its guest port, 22, is forwarded from a host loopback port
//     (sshForward): Confinement.SSHPort, or one of the backend's, kept while
//     it stays free, never one of the VM's own forwards. It's one of the 64
//     forwards smolvm makes;
//   - SSH answers only once this start's sshd runs (record.SSHUp), so
//     nothing reads the host keys a start is about to replace. A VM whose
//     image has no sshd starts without ssh, warned.
const (
	sshGuestPort = 22
	// userPortsMax is the most ports a project VM's user forwards: one of
	// smolvm's is its ssh.
	userPortsMax = maxPorts - 1
)

// sshComment is the comment of devvm's keys for its smolvm VMs.
const sshComment = "devvm"

// sshKey is the key devvm's ssh logs in to one VM with, beside its record,
// made at its first start with ssh: its public half is that VM's only
// authorized key, so it logs in to no other.
func (s *Smolvm) sshKey(vm string) string {
	return filepath.Join(filepath.Dir(s.Dirs.SmolvmVM(vm)), vm+".key")
}

// ensureSSHKey makes the VM's key when it's missing (or isn't one of
// devvm's), and returns its public half as an authorized_keys line, read
// from the private key itself: one file, written whole.
func (s *Smolvm) ensureSSHKey(vm string) (string, error) {
	path := s.sshKey(vm)
	b, err := os.ReadFile(path) //nolint:gosec // devvm's own file
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if pub, ok := sshPublic(b); ok {
		return pub, nil
	}
	privPEM, pub, err := newSSHKey(sshComment)
	if err != nil {
		return "", err
	}
	return pub, state.WriteFile(path, privPEM)
}

// sshPublic returns the public half of an ed25519 key in OpenSSH's format,
// unencrypted (as newSSHKey writes it), as an authorized_keys line.
func sshPublic(privPEM []byte) (string, bool) {
	blk, _ := pem.Decode(privPEM)
	if blk == nil || blk.Type != "OPENSSH PRIVATE KEY" {
		return "", false
	}
	rest, ok := bytes.CutPrefix(blk.Bytes, []byte("openssh-key-v1\x00"))
	if !ok {
		return "", false
	}
	var fields [4][]byte
	for i := range fields {
		if fields[i], rest, ok = sshField(rest); !ok {
			return "", false
		}
		if i == 2 { // the key count
			if len(rest) < 4 || binary.BigEndian.Uint32(rest) != 1 {
				return "", false
			}
			rest = rest[4:]
		}
	}
	// cipher none, kdf none, no kdf options, then the public key's blob.
	blob := fields[3]
	if string(fields[0]) != "none" || string(fields[1]) != "none" || len(fields[2]) != 0 {
		return "", false
	}
	kt, keyRest, ok := sshField(blob)
	if pk, _, pok := sshField(keyRest); !ok || !pok || string(kt) != "ssh-ed25519" || len(pk) != ed25519.PublicKeySize {
		return "", false
	}
	if inner, _, iok := sshField(rest); !iok || len(inner) == 0 {
		return "", false
	}
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " " + sshComment, true
}

// sshField reads one ssh wire format string from b.
func sshField(b []byte) (field, rest []byte, ok bool) {
	if len(b) < 4 {
		return nil, nil, false
	}
	n := int64(binary.BigEndian.Uint32(b))
	if int64(len(b)-4) < n {
		return nil, nil, false
	}
	return b[4 : 4+n], b[4+n:], true
}

// sshPubLine is an ed25519 public key line, as newSSHKey writes it.
var sshPubLine = regexp.MustCompile(`^ssh-ed25519 [A-Za-z0-9+/]+=* [A-Za-z0-9.-]+\n?$`)

// newSSHKey makes an ed25519 key: the private one in OpenSSH's format,
// unencrypted, and the public one as an authorized_keys line.
func newSSHKey(comment string) (privPEM []byte, pubLine string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	blob := sshString(nil, []byte("ssh-ed25519"))
	blob = sshString(blob, pub)
	var check [4]byte
	if _, err := rand.Read(check[:]); err != nil {
		return nil, "", err
	}
	inner := append(append([]byte{}, check[:]...), check[:]...)
	inner = sshString(inner, []byte("ssh-ed25519"))
	inner = sshString(inner, pub)
	inner = sshString(inner, priv)
	inner = sshString(inner, []byte(comment))
	for i := byte(1); len(inner)%8 != 0; i++ {
		inner = append(inner, i)
	}
	body := sshString([]byte("openssh-key-v1\x00"), []byte("none"))
	body = sshString(body, []byte("none"))
	body = sshString(body, nil)
	body = binary.BigEndian.AppendUint32(body, 1)
	body = sshString(body, blob)
	body = sshString(body, inner)
	privPEM = pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: body})
	return privPEM, "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " " + comment, nil
}

// sshString appends b to buf as the ssh wire format's string: its length,
// then its bytes.
func sshString(buf, b []byte) []byte {
	return append(binary.BigEndian.AppendUint32(buf, uint32(len(b))), b...) //nolint:gosec // devvm's own short fields
}

// sshForward is the host loopback port a VM's ssh is forwarded from at a
// start that runs it: c.SSHPort when the caller chose one (network
// policy's), else the one it had while that's free, else a new one; never
// one of the VM's own forwards.
func (s *Smolvm) sshForward(vm string, c backend.Confinement, r record) (int, error) {
	if c.SSHPort > 0 {
		if forwards(r.Ports, c.SSHPort) {
			return 0, msg.SmolvmSSHPortForwarded(strconv.Itoa(c.SSHPort), vm)
		}
		return c.SSHPort, nil
	}
	if r.SSHPort > 0 && !forwards(r.Ports, r.SSHPort) && s.PortFree(r.SSHPort) {
		return r.SSHPort, nil
	}
	for range 20 {
		p, err := s.FreePort()
		if err != nil {
			return 0, err
		}
		if !forwards(r.Ports, p) {
			return p, nil
		}
	}
	return 0, errors.New("smolvm: no free port on the host's loopback")
}

// forwards reports whether one of -p's values (5173:5173,
// 8000-8002:8000-8002) forwards a host port.
func forwards(ports []string, port int) bool {
	for _, p := range ports {
		host, _, _ := strings.Cut(p, ":")
		from, to, ok := strings.Cut(host, "-")
		if !ok {
			to = from
		}
		a, err1 := strconv.Atoi(from)
		b, err2 := strconv.Atoi(to)
		if err1 == nil && err2 == nil && a <= port && port <= b {
			return true
		}
	}
	return false
}

// sshForwardSpec is -p's value for a VM's ssh forward.
func sshForwardSpec(port int) string { return strconv.Itoa(port) + ":" + strconv.Itoa(sshGuestPort) }

// freePort is a port free on the host's loopback, IPv4 and IPv6, as
// smolvm forwards on both.
func freePort() (int, error) {
	for range 20 {
		l, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx // a moment's listen
		if err != nil {
			return 0, err
		}
		port := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		if portFree(port) {
			return port, nil
		}
	}
	return 0, errors.New("smolvm: no free port on the host's loopback")
}

// portFree reports whether nothing listens on a host loopback port, IPv4
// or IPv6 (a host without IPv6 counts as free there).
func portFree(port int) bool {
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))) //nolint:noctx // a moment's listen
	if err != nil {
		return false
	}
	_ = l.Close()
	l6, err := net.Listen("tcp", net.JoinHostPort("::1", strconv.Itoa(port))) //nolint:noctx // a moment's listen
	if err != nil {
		return !errors.Is(err, syscall.EADDRINUSE)
	}
	_ = l6.Close()
	return true
}

// envName is a variable's name, as services.sh is given those its ssh
// sessions get.
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// servicesArgs are services.sh's arguments for a start that runs the VM's
// sshd: the user, devvm's public key, and the names of the variables its
// sessions get from the start's environment (env's, and the proxy's under
// network policy), values never in arguments.
func servicesArgs(user, pub string, env map[string]string, proxy bool) []string {
	var names []string
	if proxy {
		names = slices.Clone(proxyVars)
	}
	for _, k := range slices.Sorted(maps.Keys(env)) {
		if envName.MatchString(k) {
			names = append(names, k)
		}
	}
	return append([]string{"ssh", user, pub}, names...)
}

// SSH returns how ssh reaches a running VM whose start ran its sshd: its
// forward on the host's loopback, the VM's user, and devvm's key alone.
// A VM started without it gets smolvm_no_ssh.
func (s *Smolvm) SSH(ctx context.Context, vm string) (backend.SSHTarget, error) {
	m, err := s.inspect(ctx, vm)
	if err != nil {
		return backend.SSHTarget{}, notFound(vm, err)
	}
	if m.status() != backend.StatusRunning {
		return backend.SSHTarget{}, msg.VMNotRunning(vm)
	}
	r, err := s.readRecord(vm)
	if err != nil {
		return backend.SSHTarget{}, err
	}
	switch {
	case r.NoSSHD:
		return backend.SSHTarget{}, msg.SmolvmNoSshd(vm)
	case r.SSHPort == 0 || !r.SSHUp:
		return backend.SSHTarget{}, msg.SmolvmNoSSH(vm)
	}
	name, _, err := s.user()
	if err != nil {
		return backend.SSHTarget{}, err
	}
	return backend.SSHTarget{
		Host: "127.0.0.1", Port: r.SSHPort, User: name,
		Options: []backend.SSHOption{
			{Key: "IdentityFile", Value: `"` + s.sshKey(vm) + `"`},
			{Key: "IdentitiesOnly", Value: "yes"},
			{Key: "PreferredAuthentications", Value: "publickey"},
		},
	}, nil
}
