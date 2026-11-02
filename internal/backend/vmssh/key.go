// Package vmssh is devvm's own ssh into VMs whose backend has none for
// editors (smolvm's machines, Docker's containers): each VM's key, its
// sshd's host key as devvm reads it, what services.sh is told to run its
// sshd with, and the port forwarder that reaches the VM over that ssh, or
// a Lima VM over Lima's (Forwarder, run detached by Forwards).
package vmssh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/shell"
	"github.com/w3cj/devvm/internal/state"
)

// Comment is the comment of devvm's keys for its VMs.
const Comment = "devvm"

// EnsureKey makes a VM's key at path when it's missing (or isn't one of
// devvm's), and returns its public half as an authorized_keys line, read
// from the private key itself: one file, written whole, mode 600. Its
// public half is that VM's only authorized key, so it logs in to no other.
func EnsureKey(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // devvm's own file
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if pub, ok := Public(b); ok {
		return pub, nil
	}
	privPEM, pub, err := newKey(Comment)
	if err != nil {
		return "", err
	}
	return pub, state.WriteFile(path, privPEM)
}

// Public returns the public half of an ed25519 key in OpenSSH's format,
// unencrypted (as devvm writes it), as an authorized_keys line.
func Public(privPEM []byte) (string, bool) {
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
		if fields[i], rest, ok = field(rest); !ok {
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
	kt, keyRest, ok := field(blob)
	if pk, _, pok := field(keyRest); !ok || !pok || string(kt) != "ssh-ed25519" || len(pk) != ed25519.PublicKeySize {
		return "", false
	}
	if inner, _, iok := field(rest); !iok || len(inner) == 0 {
		return "", false
	}
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " " + Comment, true
}

// field reads one ssh wire format string from b.
func field(b []byte) (f, rest []byte, ok bool) {
	if len(b) < 4 {
		return nil, nil, false
	}
	n := int64(binary.BigEndian.Uint32(b))
	if int64(len(b)-4) < n {
		return nil, nil, false
	}
	return b[4 : 4+n], b[4+n:], true
}

// pubLine is an ed25519 public key line, as newKey writes it.
var pubLine = regexp.MustCompile(`^ssh-ed25519 [A-Za-z0-9+/]+=* [A-Za-z0-9.-]+\n?$`)

// newKey makes an ed25519 key: the private one in OpenSSH's format,
// unencrypted, and the public one as an authorized_keys line.
func newKey(comment string) (privPEM []byte, pub string, err error) {
	pk, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	blob := wireString(nil, []byte("ssh-ed25519"))
	blob = wireString(blob, pk)
	var check [4]byte
	if _, err := rand.Read(check[:]); err != nil {
		return nil, "", err
	}
	inner := append(append([]byte{}, check[:]...), check[:]...)
	inner = wireString(inner, []byte("ssh-ed25519"))
	inner = wireString(inner, pk)
	inner = wireString(inner, priv)
	inner = wireString(inner, []byte(comment))
	for i := byte(1); len(inner)%8 != 0; i++ {
		inner = append(inner, i)
	}
	body := wireString([]byte("openssh-key-v1\x00"), []byte("none"))
	body = wireString(body, []byte("none"))
	body = wireString(body, nil)
	body = binary.BigEndian.AppendUint32(body, 1)
	body = wireString(body, blob)
	body = wireString(body, inner)
	privPEM = pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: body})
	return privPEM, "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " " + comment, nil
}

// wireString appends b to buf as the ssh wire format's string: its length,
// then its bytes.
func wireString(buf, b []byte) []byte {
	return append(binary.BigEndian.AppendUint32(buf, uint32(len(b))), b...) //nolint:gosec // devvm's own short fields
}

// hostKeyLine is an ed25519 public key line, as the VM's sshd has it.
var hostKeyLine = regexp.MustCompile(`^ssh-ed25519 [A-Za-z0-9+/]+=*(\s|$)`)

// HostKey returns the key type and base64 of the VM's sshd's host key, as
// cat of its .pub file gives them.
func HostKey(out []byte) (string, bool) {
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if !hostKeyLine.MatchString(line) {
		return "", false
	}
	f := strings.Fields(line)
	return f[0] + " " + f[1], true
}

// HostKeyPath is the VM's sshd's host key's public half, which services.sh
// makes at each start.
const HostKeyPath = "/etc/ssh/ssh_host_ed25519_key.pub"

// ServicesArgs are services.sh's arguments for a start that runs the VM's
// sshd: the user, devvm's public key, and the names of the variables its
// sessions get from the start's environment (first, then env's), values
// never in arguments.
func ServicesArgs(user, pub string, first []string, env map[string]string) []string {
	names := slices.Clone(first)
	for _, k := range slices.Sorted(maps.Keys(env)) {
		if shell.EnvName(k) {
			names = append(names, k)
		}
	}
	return append([]string{"ssh", user, pub}, names...)
}

// Capped keeps the first Max bytes written to it, and takes the rest
// without keeping it: what devvm reads of what a VM prints.
type Capped struct {
	bytes.Buffer
	Max int
}

func (w *Capped) Write(p []byte) (int, error) {
	if room := w.Max - w.Len(); room > 0 {
		w.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
