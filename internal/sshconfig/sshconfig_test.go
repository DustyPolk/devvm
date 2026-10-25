package sshconfig

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
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

// pinned is what every entry turns off, whatever a Host * says.
const pinned = `  ForwardX11 no
  ForwardX11Trusted no
  Tunnel no
  PermitRemoteOpen none
  AddKeysToAgent no
  ProxyJump none
  ProxyCommand none
  UpdateHostKeys no
  HashKnownHosts no
`

// testConfig is a config on a fake backend whose VMs answer with a
// made-up host key each, and an "other" key line.
func testConfig(t *testing.T) (*Config, *backendtest.Fake) {
	t.Helper()
	root := t.TempDir()
	f := backendtest.New("fake", backend.Caps{backend.FeatureSSH: backend.Native})
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
	quiet := func(msg.M) {}
	return &Config{
		Dirs: paths.Dirs{Config: filepath.Join(root, "config"), Data: filepath.Join(root, "data")}, Backends: []backend.Backend{f},
		Info: quiet, Warn: quiet, Say: quiet, Terminal: func() bool { return false },
	}, f
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // the test's own file
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(data)
}

func setAgent(t *testing.T, c *Config, vm, mode string) {
	t.Helper()
	st, err := state.ReadVM(c.Dirs, vm)
	if err == nil {
		err = st.Set(sshagent.Key, mode)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// Agent forwarding is what the VM's state says, whatever its backend
// config has: devvm's own agent by its socket, as the agent too, so it's
// forwarded however the editor was started.
func TestWriteAgent(t *testing.T) {
	c, f := testConfig(t)
	f.Instances = map[string]backend.Instance{web: {Name: web, Status: backend.StatusRunning, AgentForwarding: true}}
	sock := `"` + c.Dirs.AgentSock() + `"`
	for _, tc := range []struct {
		mode  string
		want  string
		agent bool
	}{
		{sshagent.Devvm, "  ForwardAgent " + sock + "\n", true},
		{sshagent.On, "  ForwardAgent yes\n", false},
		{sshagent.Off, "  ForwardAgent no\n", false},
		{"", "  ForwardAgent no\n", false},
	} {
		setAgent(t, c, web, tc.mode)
		if err := c.Write(t.Context()); err != nil {
			t.Fatal(err)
		}
		got := read(t, c.Dirs.SSHConfig())
		if !strings.Contains(got, tc.want) || strings.Count(got, "ForwardAgent") != 1 {
			t.Errorf("mode %q:\n%s", tc.mode, got)
		}
		if has := strings.Contains(got, "  ControlPath none\n  IdentityAgent "+sock+"\n"+pinned+"  Hostname "); has != tc.agent {
			t.Errorf("mode %q: IdentityAgent %v, want %v:\n%s", tc.mode, has, tc.agent, got)
		}
	}
}

func TestWrite(t *testing.T) {
	c, f := testConfig(t)
	f.Instances = map[string]backend.Instance{
		web:                      {Name: web, Status: backend.StatusRunning},
		api:                      {Name: api, Status: backend.StatusStopped},
		imgish:                   {Name: imgish, Status: backend.StatusRunning},
		"devvm-img-0123456789ab": {Name: "devvm-img-0123456789ab", Status: backend.StatusRunning},
		"devvm-tools-build":      {Name: "devvm-tools-build", Status: backend.StatusRunning},
	}
	setAgent(t, c, imgish, sshagent.On)
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
` + pinned + `  Hostname 127.0.0.1
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
` + pinned + `  Hostname 127.0.0.1
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

// A backend whose ssh is an emulation not yet reviewed gets no entries,
// unless a test build lets it through (DEVVM_UNREVIEWED); none whose
// backend lacks it.
func TestWriteUnreviewed(t *testing.T) {
	t.Parallel()
	c, f := testConfig(t)
	f.Instances = map[string]backend.Instance{web: {Name: web, Status: backend.StatusRunning}}
	for _, tc := range []struct {
		caps backend.Caps
		let  map[string]bool
		want bool
	}{
		{backend.Caps{backend.FeatureSSH: backend.Emulated(false)}, nil, false},
		{backend.Caps{backend.FeatureSSH: backend.Emulated(false)}, map[string]bool{"fake:ssh": true}, true},
		{backend.Caps{backend.FeatureSSH: backend.Emulated(true)}, nil, true},
		{nil, map[string]bool{"fake:ssh": true}, false},
	} {
		f.Capabilities, c.Review.Unreviewed = tc.caps, tc.let
		if err := c.Write(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(read(t, c.Dirs.SSHConfig()), "Host devvm-web "); got != tc.want {
			t.Errorf("%v, let through %v: entry %v, want %v", tc.caps, tc.let, got, tc.want)
		}
	}
}

// The real ssh reads devvm's entries under a user's config whose Host *
// has an IgnoreUnknown of its own (as macOS users' UseKeychain needs):
// devvm's entries set no IgnoreUnknown, which would win and replace it.
func TestEntriesWithRealSSH(t *testing.T) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("no ssh")
	}
	c, f := testConfig(t)
	f.Instances = map[string]backend.Instance{web: {Name: web, Status: backend.StatusRunning}}
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(t.TempDir(), "config")
	conf := "Include " + c.Dirs.SSHConfig() + "\nHost *\n  IgnoreUnknown FooBarOpt\n  FooBarOpt yes\n  ForwardX11 yes\n  AddKeysToAgent yes\n"
	if err := os.WriteFile(user, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(t.Context(), sshPath, "-G", "-F", user, "devvm-web").CombinedOutput() //nolint:gosec // the test's own config
	if err != nil {
		t.Fatalf("ssh -G: %v\n%s", err, out)
	}
	for _, want := range []string{"forwardx11 no", "addkeystoagent false", "permitremoteopen none", "port 60022", "hostkeyalias " + web} {
		if !slices.Contains(strings.Split(string(out), "\n"), want) {
			t.Errorf("ssh -G lacks %q:\n%s", want, out)
		}
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
	t.Parallel()
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
