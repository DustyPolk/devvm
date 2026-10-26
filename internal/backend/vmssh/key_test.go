package vmssh

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A VM's key is OpenSSH's ed25519 format, which ssh-keygen reads back to
// the same public key, as devvm reads it from the private key; it's made
// once per VM, one file, mode 600, and again when it's gone or isn't one.
func TestKey(t *testing.T) {
	priv, pub, err := newKey(Comment)
	if err != nil {
		t.Fatal(err)
	}
	if !pubLine.MatchString(pub) || !strings.HasPrefix(string(priv), "-----BEGIN OPENSSH PRIVATE KEY-----\n") {
		t.Fatalf("key: %q\n%s", pub, priv)
	}
	if got, ok := Public(priv); !ok || got != pub {
		t.Errorf("Public = %q, %v; want %q", got, ok, pub)
	}
	for _, bad := range [][]byte{nil, []byte("garbage"), priv[:len(priv)/2], []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n")} {
		if _, ok := Public(bad); ok {
			t.Errorf("Public(%q) = ok", bad)
		}
	}
	if kg, err := exec.LookPath("ssh-keygen"); err == nil {
		f := filepath.Join(t.TempDir(), "key")
		if err := os.WriteFile(f, priv, 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.CommandContext(t.Context(), kg, "-y", "-f", f).Output() //nolint:gosec // the test's own file
		if err == nil && len(strings.Fields(string(out))) > 1 && strings.Fields(string(out))[1] != strings.Fields(pub)[1] {
			t.Errorf("ssh-keygen reads %q, want %q", out, pub)
		}
	}

	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.key"), filepath.Join(dir, "b.key")
	first, err := EnsureKey(a)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(a); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("%s: %v, %v", a, fi, err)
	}
	if again, err := EnsureKey(a); err != nil || again != first {
		t.Errorf("again: %q, %v", again, err)
	}
	if other, err := EnsureKey(b); err != nil || other == first {
		t.Errorf("another VM's: %q, %v", other, err)
	}
	if err := os.WriteFile(a, []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fresh, err := EnsureKey(a); err != nil || fresh == first || !pubLine.MatchString(fresh) {
		t.Errorf("not a key: %q, %v", fresh, err)
	}
}

// A host key is read as its type and key, ed25519 alone.
func TestHostKey(t *testing.T) {
	for out, want := range map[string]string{
		"ssh-ed25519 AAAAC3Nz root@x\n": "ssh-ed25519 AAAAC3Nz",
		"ssh-ed25519 AAAA\nssh-rsa B\n": "ssh-ed25519 AAAA",
		"ssh-rsa AAAA x\n":              "",
		"ssh-ed25519 AA$A\n":            "",
		"":                              "",
	} {
		if got, ok := HostKey([]byte(out)); got != want || ok != (want != "") {
			t.Errorf("HostKey(%q) = %q, %v", out, got, ok)
		}
	}
}

// services.sh gets the user, the key and the variables' names, never their
// values: first's, then env's sorted, names that aren't left out.
func TestServicesArgs(t *testing.T) {
	got := ServicesArgs("u", "ssh-ed25519 AA c", []string{"HTTP_PROXY"}, map[string]string{"B": "1", "A": "2", "1x": "3"})
	if !slices.Equal(got, []string{"ssh", "u", "ssh-ed25519 AA c", "HTTP_PROXY", "A", "B"}) {
		t.Errorf("ServicesArgs = %q", got)
	}
}
