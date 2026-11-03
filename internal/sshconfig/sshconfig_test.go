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

// made makes each VM's state, with the alias ChooseAlias gives it then,
// as up does, in order.
func made(t *testing.T, d paths.Dirs, vms ...string) {
	t.Helper()
	for _, vm := range vms {
		alias, err := ChooseAlias(d, vm)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := state.CreateVM(d, vm, state.NewVM{Image: "base", Alias: alias}); err != nil {
			t.Fatal(err)
		}
	}
}

// A VM's alias is devvm-<folder> unless another VM holds that; it's
// chosen once, so the first VM of a folder's name keeps it when a second
// comes, and the second keeps its full name when the first goes.
func TestChooseAlias(t *testing.T) {
	c, _ := testConfig(t)
	d := c.Dirs
	dashed := "devvm-web-app-dddddddddddddd" // a folder whose name has a dash is another folder
	made(t, d, web, api, imgish, dashed)
	for vm, want := range map[string]string{web: "devvm-web", api: "devvm-api", imgish: "devvm-img-foo", dashed: "devvm-web-app"} {
		if got := c.Host(vm); got != want {
			t.Errorf("Host(%s) = %s, want %s", vm, got, want)
		}
	}
	// Another folder with the same name: the hash tells them apart.
	made(t, d, web2)
	if c.Host(web) != "devvm-web" || c.Host(web2) != web2 {
		t.Errorf("with two web folders: %s, %s", c.Host(web), c.Host(web2))
	}
	st, err := state.ReadVM(d, web)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Remove(); err != nil {
		t.Fatal(err)
	}
	if c.Host(web2) != web2 {
		t.Errorf("once the first is gone: %s", c.Host(web2))
	}
	// Made again, the VM of a folder takes the alias no other VM holds.
	if got, err := ChooseAlias(d, web); err != nil || got != "devvm-web" {
		t.Errorf("ChooseAlias(%s) again = %s, %v", web, got, err)
	}
	// A VM without state, or one recording none, is reached by its name.
	if got := c.Host("devvm-none-eeeeeeeeeeeeee"); got != "devvm-none-eeeeeeeeeeeeee" {
		t.Errorf("Host without state = %s", got)
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
		Dirs: paths.Dirs{Config: filepath.Join(root, "config"), Data: filepath.Join(root, "data")}, Backends: func() ([]backend.Backend, error) { return []backend.Backend{f}, nil },
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

// A VM devvm started forwards the agent its start was made with: a change
// waits for its next start. One started some other way since forwards the
// one its state says.
func TestWriteAgentStarted(t *testing.T) {
	c, f := testConfig(t)
	f.Instances = map[string]backend.Instance{web: {Name: web, Status: backend.StatusRunning, AgentForwarding: true}}
	setAgent(t, c, web, sshagent.Off)
	st, err := state.ReadVM(c.Dirs, web)
	if err != nil {
		t.Fatal(err)
	}
	start, err := f.StartID(t.Context(), web)
	if err == nil {
		err = sshagent.RecordStart(st, sshagent.Off, start)
	}
	if err != nil {
		t.Fatal(err)
	}
	setAgent(t, c, web, sshagent.On)
	for _, tc := range []struct {
		name, want string
		do         func()
	}{
		{"changed while running", "  ForwardAgent no\n", func() {}},
		{"started outside devvm", "  ForwardAgent yes\n", func() { f.Restart(web) }},
	} {
		tc.do()
		if err := c.Write(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := read(t, c.Dirs.SSHConfig()); !strings.Contains(got, tc.want) {
			t.Errorf("%s:\n%s", tc.name, got)
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
	made(t, c.Dirs, web, api, imgish)
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

	// Two web folders: the first keeps its alias, the second has its full
	// name only. Two that record the same alias (made at once) give it to
	// the first entry alone.
	made(t, c.Dirs, web2)
	f.Instances[web2] = backend.Instance{Name: web2, Status: backend.StatusRunning}
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c.Dirs.SSHConfig()); !strings.Contains(got, "\nHost devvm-web "+web+"\n") || !strings.Contains(got, "\nHost "+web2+"\n") {
		t.Errorf("ssh config with two web folders:\n%s", got)
	}
	if st, err := state.ReadVM(c.Dirs, web2); err != nil || st.Set(state.KeyAlias, "devvm-web") != nil {
		t.Fatal(err)
	}
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c.Dirs.SSHConfig()); strings.Count(got, "Host devvm-web ") != 1 || !strings.Contains(got, "\nHost "+web2+"\n") {
		t.Errorf("ssh config with one alias twice:\n%s", got)
	}
	delete(f.Instances, web2)
	if err := c.ForgetHostKeys(web2); err != nil {
		t.Fatal(err)
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
	made(t, c.Dirs, web)
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
	made(t, c.Dirs, web)
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

// A backend that reaches its VMs through a command of its own (Docker's)
// gets it as ProxyCommand, in place of none, and no Port; ssh reads it so,
// whatever a Host * of the user's says. One whose command has a newline
// gets no entry.
func TestWriteProxyCommand(t *testing.T) {
	c, f := testConfig(t)
	f.Instances = map[string]backend.Instance{web: {Name: web, Status: backend.StatusRunning}}
	made(t, c.Dirs, web)
	proxy := "'/bin/devvm' '_docker-connect' '-data' '/a b' '" + web + "'"
	f.Target = backend.SSHTarget{Host: web, User: "u", ProxyCommand: proxy}
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := read(t, c.Dirs.SSHConfig())
	want := strings.Replace(pinned, "  ProxyCommand none\n", "  ProxyCommand "+proxy+"\n", 1) + "  Hostname " + web + "\n  User u\n"
	if !strings.HasSuffix(got, want) || strings.Contains(got, "  Port ") || strings.Count(got, "  ProxyCommand ") != 1 {
		t.Errorf("ssh config:\n%s", got)
	}
	if sshPath, err := exec.LookPath("ssh"); err == nil {
		user := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(user, []byte("Include "+c.Dirs.SSHConfig()+"\nHost *\n  ProxyCommand nc %h %p\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.CommandContext(t.Context(), sshPath, "-G", "-F", user, "devvm-web").CombinedOutput() //nolint:gosec // the test's own config
		if err != nil || !slices.Contains(strings.Split(string(out), "\n"), "proxycommand "+proxy) {
			t.Errorf("ssh -G: %v\n%s", err, out)
		}
	}
	f.Target.ProxyCommand = "x\ny"
	if err := c.Write(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c.Dirs.SSHConfig()); got != header {
		t.Errorf("a newline: %q", got)
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

// A VM's host key, for its port forwarder: read from it and pinned when
// none is, else the pinned one, ed25519 first; none when it can't show
// one.
func TestHostKey(t *testing.T) {
	c, f := testConfig(t)
	if got, err := c.HostKey(t.Context(), f, web); err != nil || got != "ssh-ed25519 AAAAkey"+strings.ReplaceAll(web, "-", "") {
		t.Errorf("read: %q, %v", got, err)
	}
	if !strings.Contains(read(t, c.Dirs.KnownHosts()), web+" ssh-ed25519 ") {
		t.Error("not pinned")
	}
	if err := os.WriteFile(c.Dirs.KnownHosts(), []byte(api+" ecdsa-sha2-nistp256 AAAAe\n"+api+" ssh-ed25519 AAAAp\n"+api+" ssh-rsa AAAAr\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.ExecFunc = func(string, []string, backend.ExecOpts) error { return errors.New("not asked") }
	if got, err := c.HostKey(t.Context(), f, api); err != nil || got != "ssh-ed25519 AAAAp" {
		t.Errorf("pinned: %q, %v", got, err)
	}
	if got, err := c.HostKey(t.Context(), f, web); err == nil {
		t.Errorf("unreadable: %q", got)
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
	if want := "Include " + filepath.Join(c.Dirs.Data, "ssh_config"); c.IncludeLine() != want {
		t.Errorf("IncludeLine = %q, want %q", c.IncludeLine(), want)
	}
}
