package smolvm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// A VM's key is OpenSSH's ed25519 format, which ssh-keygen reads back to
// the same public key, as devvm reads it from the private key; it's made
// once per VM, one file, mode 600, and again when it's gone or isn't one.
func TestSSHKey(t *testing.T) {
	priv, pub, err := newSSHKey(sshComment)
	if err != nil {
		t.Fatal(err)
	}
	if !sshPubLine.MatchString(pub) || !strings.HasPrefix(string(priv), "-----BEGIN OPENSSH PRIVATE KEY-----\n") {
		t.Fatalf("key: %q\n%s", pub, priv)
	}
	if got, ok := sshPublic(priv); !ok || got != pub {
		t.Errorf("sshPublic = %q, %v; want %q", got, ok, pub)
	}
	for _, bad := range [][]byte{nil, []byte("garbage"), priv[:len(priv)/2], []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n")} {
		if _, ok := sshPublic(bad); ok {
			t.Errorf("sshPublic(%q) = ok", bad)
		}
	}
	if kg, err := exec.LookPath("ssh-keygen"); err == nil {
		f := filepath.Join(t.TempDir(), "key")
		if err := os.WriteFile(f, priv, 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.CommandContext(t.Context(), kg, "-y", "-f", f).Output() //nolint:gosec // the test's own file
		if err != nil {
			t.Fatalf("ssh-keygen -y: %v", err)
		}
		if strings.Fields(string(out))[1] != strings.Fields(pub)[1] {
			t.Errorf("ssh-keygen reads %q, want %q", out, pub)
		}
	}

	s := newSandbox(t)
	a, b := "devvm-a-0123456789abcd", "devvm-b-0123456789abcd"
	first, err := s.ensureSSHKey(a)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(s.sshKey(a)); err != nil || fi.Mode().Perm() != 0o600 || filepath.Dir(s.sshKey(a)) != filepath.Dir(s.Dirs.SmolvmVM(a)) {
		t.Errorf("%s: %v, %v", s.sshKey(a), fi, err)
	}
	if again, err := s.ensureSSHKey(a); err != nil || again != first {
		t.Errorf("again: %q, %v", again, err)
	}
	if other, err := s.ensureSSHKey(b); err != nil || other == first {
		t.Errorf("another VM's: %q, %v", other, err)
	}
	if err := os.WriteFile(s.sshKey(a), []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fresh, err := s.ensureSSHKey(a); err != nil || fresh == first || !sshPubLine.MatchString(fresh) {
		t.Errorf("not a key: %q, %v", fresh, err)
	}
}

// A start with ssh forwards it from a host port, kept while it's free and
// never one of the VM's own; services.sh gets the user, the VM's key and
// the names of the variables its sessions get, never their values. SSH
// answers once services.sh ran. A start without ssh takes the forward away.
func TestStartSSH(t *testing.T) {
	s := newSandbox(t)
	vm := "devvm-s-0123456789abcd"
	s.machine(t, vm, "stopped 4 8192 20 true")
	if err := s.writeRecord(vm, record{Ports: []string{"61022:61022"}}); err != nil {
		t.Fatal(err)
	}
	next := []int{61022, 61023}
	s.FreePort = func() (int, error) { p := next[0]; next = next[1:]; return p, nil }
	if _, err := s.SSH(t.Context(), vm); names(err) != "vm_not_running vm="+vm {
		t.Errorf("stopped: %v", err)
	}
	if err := s.Start(t.Context(), vm, backend.Confinement{SSH: true}); err != nil {
		t.Fatal(err)
	}
	pub, err := s.ensureSSHKey(vm)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"smolvm machine update --name " + vm + " -p 61023:22",
		"smolvm machine start --name " + vm,
		"smolvm machine exec --name " + vm + " -- sh -c " + sshdProbe,
		"smolvm machine exec --name " + vm + " -i -- sh -s -- ssh u " + pub,
	}
	if got := s.calls(t); !slices.Equal(got, want) {
		t.Errorf("calls:\n%q\nwant\n%q", got, want)
	}
	tgt, err := s.SSH(t.Context(), vm)
	if err != nil {
		t.Fatal(err)
	}
	wantOpts := []backend.SSHOption{
		{Key: "IdentityFile", Value: `"` + s.sshKey(vm) + `"`},
		{Key: "IdentitiesOnly", Value: "yes"},
		{Key: "PreferredAuthentications", Value: "publickey"},
	}
	if tgt.Host != "127.0.0.1" || tgt.Port != 61023 || tgt.User != "u" || !slices.Equal(tgt.Options, wantOpts) || tgt.ConfigFile != "" {
		t.Errorf("SSH = %+v", tgt)
	}
	// Running, but its sshd not yet up (another devvm's start): no answer,
	// so nothing reads host keys about to be replaced.
	r, _ := s.readRecord(vm)
	r.SSHUp = false
	if err := s.writeRecord(vm, r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SSH(t.Context(), vm); names(err) != "smolvm_no_ssh vm="+vm {
		t.Errorf("sshd not up: %v", err)
	}

	// Again: the same port while it's free, else a new one.
	for _, tc := range []struct {
		free bool
		want string
	}{
		{true, ""},
		{false, " --remove-port 61023:22 -p 61099:22"},
	} {
		if err := s.Stop(t.Context(), vm); err != nil {
			t.Fatal(err)
		}
		s.reset(t)
		s.PortFree = func(int) bool { return tc.free }
		next = []int{61099}
		if err := s.Start(t.Context(), vm, backend.Confinement{SSH: true}); err != nil {
			t.Fatal(err)
		}
		got := s.calls(t)
		if tc.want == "" && strings.HasPrefix(got[0], "smolvm machine update") || tc.want != "" && got[0] != "smolvm machine update --name "+vm+tc.want {
			t.Errorf("free %v: %q", tc.free, got)
		}
	}

	// Network policy: its port, and the proxy's and Env's names for the
	// sessions; a port that's one of the VM's own forwards is refused.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	c := backend.Confinement{
		Net: backend.NetProxy, ProxyPort: 3128, ProxyUser: "devvm", ProxyPassword: "pw", SSH: true, SSHPort: 61022,
		Env: map[string]string{"NO_PROXY": "localhost"}, Decider: "127.0.0.1:4000", DeciderToken: strings.Repeat("ab", 32),
	}
	if err := s.Start(t.Context(), vm, c); names(err) != "smolvm_ssh_port_forwarded port=61022 vm="+vm {
		t.Errorf("forwarded port: %v", err)
	}
	c.SSHPort = 40022
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	got := s.calls(t)
	if !slices.Contains(got, "smolvm machine update --name "+vm+" --reset-allow-list --allow-cidr 100.96.0.1/32 --allow-host-pattern devvm.invalid --remove-port 61099:22 -p 40022:22") ||
		!strings.HasSuffix(got[len(got)-1], " -i -- sh -s -- ssh u "+pub+" HTTP_PROXY HTTPS_PROXY http_proxy https_proxy NO_PROXY") {
		t.Errorf("network policy: %q", got)
	}
	if strings.Contains(strings.Join(got, " "), "pw") {
		t.Error("the password is in smolvm's arguments")
	}

	// Without ssh: the forward goes, and so does SSH's target.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	if got := s.calls(t); got[0] != "smolvm machine update --name "+vm+" --net --reset-allow-list --no-egress-interceptor" ||
		!slices.Contains(got, "smolvm machine update --name "+vm+" --remove-port 40022:22") ||
		got[len(got)-1] != "smolvm machine exec --name "+vm+" -i -- sh -s --" {
		t.Errorf("without ssh: %q", got)
	}
	if _, err := s.SSH(t.Context(), vm); names(err) != "smolvm_no_ssh vm="+vm {
		t.Errorf("without ssh: %v", err)
	}
	s.touch(t, "smolvm-foreign-"+vm)
	if _, err := s.SSH(t.Context(), vm); !errors.Is(err, backend.ErrNotFound) {
		t.Errorf("someone else's: %v", err)
	}
}

// An image without sshd (made before devvm ran one): the VM starts and
// runs without ssh, warned; it isn't stopped. rm deletes its key.
func TestStartNoSSHD(t *testing.T) {
	s := newSandbox(t)
	vm := "devvm-o-0123456789abcd"
	s.machine(t, vm, "stopped 4 8192 20 true")
	s.touch(t, "no-sshd")
	var warned []string
	s.Warn = func(m msg.M) { warned = append(warned, m.Names()) }
	if err := s.Start(t.Context(), vm, backend.Confinement{SSH: true}); err != nil {
		t.Fatal(err)
	}
	if m, err := s.inspect(t.Context(), vm); err != nil || m.status() != backend.StatusRunning {
		t.Errorf("not running: %+v, %v", m, err)
	}
	if !slices.Equal(warned, []string{"smolvm_no_sshd vm=" + vm}) {
		t.Errorf("warned %q", warned)
	}
	// services.sh ran the rest, asked for no sshd.
	if got := s.calls(t); got[len(got)-1] != "smolvm machine exec --name "+vm+" -i -- sh -s --" {
		t.Errorf("calls: %q", got)
	}
	if _, err := s.SSH(t.Context(), vm); names(err) != "smolvm_no_sshd vm="+vm {
		t.Errorf("SSH: %v", err)
	}
	if err := s.Delete(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.sshKey(vm)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("key kept: %v", err)
	}
}

func TestForwards(t *testing.T) {
	ports := []string{"5173:5173", "8000-8002:8000-8002"}
	for p, want := range map[int]bool{5173: true, 8000: true, 8002: true, 8003: false, 22: false} {
		if forwards(ports, p) != want {
			t.Errorf("forwards(%d) = %v", p, !want)
		}
	}
	if sshForwardSpec(61022) != "61022:22" {
		t.Error(sshForwardSpec(61022))
	}
	if got := servicesArgs("u", "ssh-ed25519 AA c", map[string]string{"B": "1", "A": "2", "1x": "3"}, false); !slices.Equal(got, []string{"ssh", "u", "ssh-ed25519 AA c", "A", "B"}) {
		t.Errorf("servicesArgs = %q", got)
	}
}
