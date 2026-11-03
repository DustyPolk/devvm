// Package desktop is devvm desktop's side on this machine: the VNC
// connections a viewer makes to this machine's loopback, each checked
// against the VM's password here, before anything reaches the VM, then
// carried to the VM's VNC server (its desktop layer's, on a socket in the
// VM, never a port) over the VM's ssh, with the viewer's clipboard kept from
// the VM unless it's asked for.
//
// The check is the relay's, not the VM's server's: root in the VM can
// change the server (no password, the clipboard on), but every connection
// starts on this machine and passes the relay first.
package desktop

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// Socket is where the desktop layer's VNC server listens in the VM: a
// socket in a folder of the VM's user's own (0700), the socket 0600.
const Socket = "/run/devvm-desktop/vnc.sock"

// Command is the desktop layer's command in the VM: devvm-desktop start
// on|off starts its desktop unless it runs, the clipboard as asked.
const Command = "/usr/local/bin/devvm-desktop"

// PasswordLen is a password's length: VNC's own authentication uses 8
// characters at most, which macOS's Screen Sharing asks for.
const PasswordLen = 8

// alphabet is what passwords are made of: lowercase letters and digits,
// without those read as others (0 o 1 l i).
const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// NewPassword returns a new random password: PasswordLen characters of
// alphabet, about 40 bits, which the relay lets a viewer guess about once a
// second at most.
func NewPassword() (string, error) {
	b := make([]byte, PasswordLen)
	n := big.NewInt(int64(len(alphabet)))
	for i := range b {
		k, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", err
		}
		b[i] = alphabet[k.Int64()]
	}
	return string(b), nil
}

// ValidPassword reports whether p is a password NewPassword could make.
func ValidPassword(p string) bool {
	if len(p) != PasswordLen {
		return false
	}
	for i := range len(p) {
		if strings.IndexByte(alphabet, p[i]) < 0 {
			return false
		}
	}
	return true
}
