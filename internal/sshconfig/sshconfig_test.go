package sshconfig

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
)

const (
	web    = "devvm-web-aaaaaaaaaaaaaa"
	web2   = "devvm-web-bbbbbbbbbbbbbb"
	api    = "devvm-api-cccccccccccccc"
	imgish = "devvm-img-foo-abc123abc123ab" // a project folder named img-foo
)

func TestAlias(t *testing.T) {
	for _, tc := range []struct {
		vm   string
		all  []string
		want string
	}{
		{web, []string{web, api}, "devvm-web"},
		// Two folders with the same name: the hash tells them apart.
		{web, []string{web, web2, api}, web},
		{api, []string{web, web2, api}, "devvm-api"},
		{imgish, []string{imgish, "devvm-img-0123456789ab"}, "devvm-img-foo"},
		// A folder whose name has a dash is another folder.
		{web, []string{web, "devvm-web-app-dddddddddddddd"}, "devvm-web"},
	} {
		if got := Alias(tc.vm, tc.all); got != tc.want {
			t.Errorf("Alias(%s, %q) = %s, want %s", tc.vm, tc.all, got, tc.want)
		}
	}
}

// testConfig is a config on a fake backend whose VMs answer with a
// made-up host key each, and an "other" key line.
func testConfig(t *testing.T) (*Config, *backendtest.Fake) {
	t.Helper()
	root := t.TempDir()
	f := backendtest.New("fake", nil)
	f.Target = backend.SSHTarget{Host: "127.0.0.1", Port: 60022, User: "u", Options: []backend.SSHOption{
		{Key: "IdentityFile", Value: `"/x/key"`}, {Key: "IdentitiesOnly", Value: "yes"},
	}}
	f.ExecFunc = func(vm string, argv []string, o backend.ExecOpts) error {
		if len(argv) != 3 || !strings.Contains(argv[2], "/etc/ssh/ssh_host_") {
			return errors.New("unexpected command")
		}
		_, err := io.WriteString(o.Stdout, "ssh-ed25519 AAAAkey"+strings.ReplaceAll(vm, "-", "")+" root@x\n"+
			"garbage\nssh-rsa not;base64\n*-evil AAAA\n")
		return err
	}
	return &Config{Dirs: paths.Dirs{Config: filepath.Join(root, "config"), Data: filepath.Join(root, "data")}, Backends: []backend.Backend{f}}, f
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // the test's own file
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(data)
}

func TestWrite(t *testing.T) {
	c, f := testConfig(t)
	f.Instances = map[string]backend.Instance{
		web:                      {Name: web, Status: backend.StatusRunning},
		api:                      {Name: api, Status: backend.StatusStopped},
		imgish:                   {Name: imgish, Status: backend.StatusRunning, AgentForwarding: true},
		"devvm-img-0123456789ab": {Name: "devvm-img-0123456789ab", Status: backend.StatusRunning},
		"devvm-tools-build":      {Name: "devvm-tools-build", Status: backend.StatusRunning},
	}
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	kh := c.Dirs.KnownHosts()
	want := header + `
Host devvm-img-foo ` + imgish + `
  HostKeyAlias ` + imgish + `
  UserKnownHostsFile "` + kh + `"
  StrictHostKeyChecking yes
  NoHostAuthenticationForLocalhost no
  ForwardAgent yes
  ControlMaster no
  ControlPath none
  Hostname 127.0.0.1
  Port 60022
  User u
  IdentityFile "/x/key"
  IdentitiesOnly yes

Host devvm-web ` + web + `
  HostKeyAlias ` + web + `
  UserKnownHostsFile "` + kh + `"
  StrictHostKeyChecking yes
  NoHostAuthenticationForLocalhost no
  ForwardAgent no
  ControlMaster no
  ControlPath none
  Hostname 127.0.0.1
  Port 60022
  User u
  IdentityFile "/x/key"
  IdentitiesOnly yes
`
	if got := read(t, c.Dirs.SSHConfig()); got != want {
		t.Errorf("ssh config:\n%s\nwant:\n%s", got, want)
	}
	// Only the running project VMs' keys, under their names, nothing but
	// host keys.
	wantKH := imgish + " ssh-ed25519 AAAAkeydevvmimgfooabc123abc123ab\n" + web + " ssh-ed25519 AAAAkeydevvmwebaaaaaaaaaaaaaa\n"
	if got := read(t, kh); got != wantKH {
		t.Errorf("known hosts = %q, want %q", got, wantKH)
	}
	if fi, err := os.Stat(c.Dirs.SSHConfig()); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("ssh config's mode: %v, %v", fi, err)
	}

	// Keys are read once; a stopped VM drops out, its keys kept.
	f.Instances[imgish] = backend.Instance{Name: imgish, Status: backend.StatusStopped}
	execs := 0
	for _, m := range f.Methods() {
		if m == "Exec" {
			execs++
		}
	}
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, m := range f.Methods() {
		if m == "Exec" {
			execs--
		}
	}
	if execs != 0 {
		t.Errorf("keys read again: %d more Execs", -execs)
	}
	got := read(t, c.Dirs.SSHConfig())
	if strings.Contains(got, imgish) || !strings.Contains(got, "Host devvm-web "+web+"\n") {
		t.Errorf("ssh config with %s stopped:\n%s", imgish, got)
	}
	if read(t, kh) != wantKH {
		t.Errorf("known hosts changed: %q", read(t, kh))
	}

	// Two web folders: each by its full name only.
	f.Instances[web2] = backend.Instance{Name: web2, Status: backend.StatusStopped}
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c.Dirs.SSHConfig()); !strings.Contains(got, "\nHost "+web+"\n") {
		t.Errorf("ssh config with two web folders:\n%s", got)
	}

	// Forgotten keys are read again; forgetting keeps the others.
	if err := c.ForgetHostKeys(web); err != nil {
		t.Fatal(err)
	}
	if got := read(t, kh); got != imgish+" ssh-ed25519 AAAAkeydevvmimgfooabc123abc123ab\n" {
		t.Errorf("known hosts after forgetting %s = %q", web, got)
	}
	if err := c.ForgetHostKeys(imgish); err != nil {
		t.Fatal(err)
	}
	if got := read(t, kh); got != "" {
		t.Errorf("known hosts after forgetting both = %q", got)
	}
	if err := c.ForgetHostKeys(api); err != nil {
		t.Errorf("forgetting a VM with no keys: %v", err)
	}
}

// A VM that doesn't show its host keys, or has no ssh target, gets no
// entry: ssh couldn't check it.
func TestWriteUnchecked(t *testing.T) {
	c, f := testConfig(t)
	f.Instances = map[string]backend.Instance{web: {Name: web, Status: backend.StatusRunning}}
	f.ExecFunc = func(string, []string, backend.ExecOpts) error { return errors.New("no answer") }
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c.Dirs.SSHConfig()); got != header {
		t.Errorf("ssh config = %q", got)
	}
	if got := read(t, c.Dirs.KnownHosts()); got != "" {
		t.Errorf("known hosts = %q", got)
	}
	f.ExecFunc = func(_ string, _ []string, o backend.ExecOpts) error {
		_, err := io.WriteString(o.Stdout, "nothing here\n")
		return err
	}
	f.Errs = map[string]error{"SSH": errors.New("no ssh")}
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.Errs = nil
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c.Dirs.SSHConfig()); got != header {
		t.Errorf("ssh config = %q", got)
	}
}

func TestParseHostKeys(t *testing.T) {
	out := "ssh-ed25519 AAAAC3Nza root@vm\necdsa-sha2-nistp256 AAAAE2Vj= c\n\nssh-rsa\nrsa AAAA\nssh-x AA AA\nssh-y A\"A\n"
	want := "v ssh-ed25519 AAAAC3Nza\nv ecdsa-sha2-nistp256 AAAAE2Vj=\nv ssh-x AA\n"
	if got := parseHostKeys("v", []byte(out)); got != want {
		t.Errorf("parseHostKeys = %q, want %q", got, want)
	}
	w := &capped{max: 4}
	if n, err := w.Write([]byte("abcdef")); n != 6 || err != nil || w.String() != "abcd" {
		t.Errorf("capped: %d, %v, %q", n, err, w.String())
	}
}

// Without a terminal, the Include line isn't added: it says to. Once
// there, anywhere, nothing is asked.
func TestEnsureInclude(t *testing.T) {
	if ui.StdinIsTerminal() {
		t.Skip("it would ask: run without a terminal on stdin")
	}
	c, _ := testConfig(t)
	home := t.TempDir()
	cfg := filepath.Join(home, ".ssh", "config")
	ok, err := c.EnsureInclude(home)
	if ok || err != nil {
		t.Errorf("EnsureInclude without a terminal = %v, %v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("~/.ssh made: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	mine := "Host *\n  ForwardAgent no\n" + c.IncludeLine() + "\n"
	if err := os.WriteFile(cfg, []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.EnsureInclude(home); !ok || err != nil {
		t.Errorf("EnsureInclude with the line = %v, %v", ok, err)
	}
	if got := read(t, cfg); got != mine {
		t.Errorf("~/.ssh/config changed: %q", got)
	}
	if want := "Include " + filepath.Join(c.Dirs.Config, "ssh_config"); c.IncludeLine() != want {
		t.Errorf("IncludeLine = %q, want %q", c.IncludeLine(), want)
	}
}
