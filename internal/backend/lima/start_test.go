package lima

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
)

// envOf returns the value the call's environment gives key, and whether
// it's set.
func envOf(c call, key string) (string, bool) {
	for _, e := range slices.Backward(c.env) {
		if v, ok := strings.CutPrefix(e, key+"="); ok {
			return v, true
		}
	}
	return "", false
}

// startCall returns the call that started the VM.
func startCall(t *testing.T, f *fakeRunner) call {
	t.Helper()
	for _, c := range f.calls {
		if slices.Contains(c.argv, "start") {
			return c
		}
	}
	t.Fatalf("no start in %q", f.lines())
	return call{}
}

const stoppedVM = `{"name":"v","status":"Stopped","config":{"ssh":{"forwardAgent":true}}}`

func TestStartConfined(t *testing.T) {
	proxy := backend.Confinement{
		Net: backend.NetProxy, ProxyPort: 23456, ProxyUser: "devvm", ProxyPassword: "0123456789abcdef0123456789abcdef",
		SSHPort: 2222, Env: map[string]string{"NO_PROXY": "localhost"},
	}
	for _, tc := range []struct {
		name, os string
		c        backend.Confinement
		edits    []string // the --set values
		wrapped  bool     // under sandbox-exec
		qemu     bool     // through the QEMU wrapper
		guestfwd string   // DEVVM_GUESTFWD; "" for unset
	}{
		{
			name: "macOS, the proxy", os: "darwin", c: proxy, wrapped: true,
			edits: []string{
				".ssh.localPort=2222",
				`.env = {"HTTPS_PROXY":"http://devvm:devvm-proxy-password@192.168.5.2:23456","HTTP_PROXY":"http://devvm:devvm-proxy-password@192.168.5.2:23456","NO_PROXY":"localhost","http_proxy":"http://devvm:devvm-proxy-password@192.168.5.2:23456","https_proxy":"http://devvm:devvm-proxy-password@192.168.5.2:23456"}`,
				".propagateProxyEnv=false", ".hostResolver.enabled=false", `.dns=["127.0.0.1"]`, noForwardsSet(),
			},
		},
		{
			name: "Linux, the proxy", os: "linux", c: proxy, qemu: true,
			guestfwd: `'/opt/my devvm/devvm' _qemu connect 127.0.0.1:23456`,
			edits: []string{
				".ssh.localPort=2222",
				`.env = {"HTTPS_PROXY":"http://devvm:devvm-proxy-password@192.168.5.4:3128","HTTP_PROXY":"http://devvm:devvm-proxy-password@192.168.5.4:3128","NO_PROXY":"localhost","http_proxy":"http://devvm:devvm-proxy-password@192.168.5.4:3128","https_proxy":"http://devvm:devvm-proxy-password@192.168.5.4:3128"}`,
				".propagateProxyEnv=false", ".hostResolver.enabled=false", `.dns=["127.0.0.1"]`, noForwardsSet(),
			},
		},
		{
			name: "macOS, a build with no network", os: "darwin", c: backend.Confinement{Net: backend.NetNone, SSHPort: 2223}, wrapped: true,
			edits: []string{".ssh.localPort=2223", ".propagateProxyEnv=false", noForwardsSet()},
		},
		{
			name: "Linux, a build with no network", os: "linux", c: backend.Confinement{Net: backend.NetNone}, qemu: true,
			edits: []string{".propagateProxyEnv=false", noForwardsSet()},
		},
		{
			// Not the host's proxy settings: its image would keep them.
			name: "Linux, a build with the internet", os: "linux", c: backend.Confinement{Net: backend.NetInternet},
			edits: []string{".propagateProxyEnv=false", noForwardsSet()},
		},
		{name: "macOS, unconfined", os: "darwin", c: backend.Confinement{}, edits: []string{noForwardsSet()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, f, _ := testLima(t, tc.os)
			t.Setenv(GuestFwdEnv, "left over")
			yaml := stoppedWithYAML(t, l, f)
			if err := l.Start(t.Context(), "v", tc.c); err != nil {
				t.Fatal(err)
			}
			if edits := editSets(f); !slices.Equal(edits, tc.edits) {
				t.Errorf("edits\n%s\nwant\n%s", strings.Join(edits, "\n"), strings.Join(tc.edits, "\n"))
			}
			// The password is in no command's arguments: devvm writes it into
			// lima.yaml, which the setup's proxy address matches.
			for _, c := range f.calls {
				if strings.Contains(c.line(), proxy.ProxyPassword) {
					t.Errorf("the password is in %q", c.line())
				}
			}
			checkLimaYAML(t, l, yaml, tc.c)
			s := startCall(t, f)
			checkStartCall(t, l, s, tc.wrapped)
			// Only the proxy's VMs have their binds denied: builds keep
			// Lima's host resolver, which binds loopback ports.
			if tc.wrapped {
				sb, err := os.ReadFile(l.profileFile("v"))
				if err != nil || strings.Contains(string(sb), "(deny network-bind)") != (tc.c.Net == backend.NetProxy) {
					t.Errorf("profile (%v):\n%s", err, sb)
				}
			}
			w, ok := envOf(s, "QEMU_SYSTEM_AARCH64")
			if ok != tc.qemu || ok && w != filepath.Join(l.Dirs.Run(), "qemu", "qemu-system-aarch64") {
				t.Errorf("QEMU_SYSTEM_AARCH64=%q (%v)", w, ok)
			}
			if fwd, _ := envOf(s, GuestFwdEnv); tc.qemu && fwd != tc.guestfwd {
				t.Errorf("%s=%q, want %q", GuestFwdEnv, fwd, tc.guestfwd)
			}
			if ok, err := l.Confined(t.Context(), "v"); err != nil {
				t.Fatal(err)
			} else if ok {
				t.Error("Confined with a hostagent that doesn't run")
			}
		})
	}
}

// A start turns Lima's own forwards off where the VM's rules, as limactl
// list fills them in, forward anything, whatever ports it's given: those
// are the forwarder's.
func TestStartSetsNoForwards(t *testing.T) {
	// As limactl list --json has noForwards: the host side filled in.
	const ignore = `{"guestIPMustBeZero":false,"guestIP":"127.0.0.1","guestPortRange":[1,65535],"hostIP":"127.0.0.1","hostPortRange":[1,65535],"proto":"any","ignore":true},
		{"guestIPMustBeZero":true,"guestIP":"0.0.0.0","guestPortRange":[1,65535],"hostIP":"127.0.0.1","hostPortRange":[1,65535],"proto":"any","ignore":true},
		{"guestIPMustBeZero":false,"guestIP":"::","guestPortRange":[1,65535],"hostIP":"127.0.0.1","hostPortRange":[1,65535],"proto":"any","ignore":true}`
	const listed = `{"guestIPMustBeZero":false,"guestIP":"127.0.0.1","guestPortRange":[5173,5173],"hostIP":"127.0.0.1","hostPortRange":[5173,5173],"proto":"any"}`
	l, _, _ := testLima(t, "darwin")
	set := []string{"--set", noForwardsSet()}
	for _, tc := range []struct {
		name, cfg string
		want      []string
	}{
		{"off", `{"portForwards":[` + ignore + `]}`, nil},
		{"Lima's own rule", `{"portForwards":[]}`, set},
		{"a listed port of Lima's", `{"portForwards":[` + listed + `,` + ignore + `]}`, set},
		{"another address", `{"portForwards":[` + strings.Replace(ignore, `"::"`, `"::1"`, 1) + `]}`, set},
		{"TCP alone", `{"portForwards":[` + strings.ReplaceAll(ignore, `"any"`, `"tcp"`) + `]}`, set},
	} {
		var cfg config
		if err := json.Unmarshal([]byte(tc.cfg), &cfg); err != nil {
			t.Fatal(err)
		}
		if got, _ := l.startSets(&cfg, backend.Confinement{}, 0); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A start with network policy edits the VM's environment only when it
// differs from the one the VM has: the same NO_PROXY changes nothing, so
// its ssh host keys stay, and another edits it.
func TestStartSetsChangedEnv(t *testing.T) {
	l, _, _ := testLima(t, "darwin")
	conf := func(noProxy string) backend.Confinement {
		return backend.Confinement{
			Net: backend.NetProxy, ProxyPort: 20001, ProxyUser: "devvm", ProxyPassword: "pw",
			Env: map[string]string{"NO_PROXY": noProxy, "no_proxy": noProxy},
		}
	}
	const was = "localhost,127.0.0.1,::1"
	f, enabled := false, false
	cfg := config{Env: l.guestEnv(conf(was)), DNS: []string{"127.0.0.1"}, PropagateProxyEnv: &f}
	cfg.HostResolver.Enabled = &enabled
	if err := json.Unmarshal([]byte(`[{"guestIPMustBeZero":false,"guestIP":"127.0.0.1","guestPortRange":[1,65535],"hostIP":"127.0.0.1","hostPortRange":[1,65535],"proto":"any","ignore":true},
		{"guestIPMustBeZero":true,"guestIP":"0.0.0.0","guestPortRange":[1,65535],"hostIP":"127.0.0.1","hostPortRange":[1,65535],"proto":"any","ignore":true},
		{"guestIPMustBeZero":false,"guestIP":"::","guestPortRange":[1,65535],"hostIP":"127.0.0.1","hostPortRange":[1,65535],"proto":"any","ignore":true}]`), &cfg.PortForwards); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.startSets(&cfg, conf(was), 0); len(got) != 0 {
		t.Errorf("the same NO_PROXY: %q", got)
	}
	got, fill := l.startSets(&cfg, conf(was+",.corp"), 0)
	if len(got) != 2 || !strings.HasPrefix(got[1], ".env = ") || !strings.Contains(got[1], "::1,.corp") || !fill {
		t.Errorf("another NO_PROXY: %q, fill %v", got, fill)
	}
}

// stoppedWithYAML makes v a stopped VM with a stale hostagent pid and a
// lima.yaml that limactl edit writes the environment it's given into,
// and returns lima.yaml's path.
func stoppedWithYAML(t *testing.T, l *Lima, f *fakeRunner) string {
	t.Helper()
	f.out["limactl list --json"] = stoppedVM
	if err := os.MkdirAll(l.Dirs.LimaVM("v"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.haPIDFile("v"), []byte("12345\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	yaml := filepath.Join(l.Dirs.LimaVM("v"), "lima.yaml")
	if err := os.WriteFile(yaml, []byte("cpus: 4\n"), 0o644); err != nil { //nolint:gosec // as Lima writes it
		t.Fatal(err)
	}
	f.do = func(c call) {
		if i := slices.IndexFunc(c.argv, func(a string) bool { return strings.HasPrefix(a, ".env = ") }); i >= 0 && c.argv[1] == "edit" {
			if err := os.WriteFile(yaml, []byte("cpus: 4\nenv: "+strings.TrimPrefix(c.argv[i], ".env = ")+"\n"), 0o644); err != nil { //nolint:gosec // as Lima writes it
				t.Error(err)
			}
		}
	}
	return yaml
}

// editSets returns the --set values of every limactl edit, in order.
func editSets(f *fakeRunner) []string {
	var edits []string
	for _, c := range f.calls {
		if c.argv[1] != "edit" {
			continue
		}
		for i, a := range c.argv {
			if a == "--set" {
				edits = append(edits, c.argv[i+1])
			}
		}
	}
	return edits
}

// checkLimaYAML checks that a start with c left lima.yaml with the proxy's
// address and password (owner-only), or untouched without the proxy.
func checkLimaYAML(t *testing.T, l *Lima, yaml string, c backend.Confinement) {
	t.Helper()
	data, err := os.ReadFile(yaml) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	u := l.ProxyURL(c)
	if c.Net != backend.NetProxy {
		if u != "" || string(data) != "cpus: 4\n" {
			t.Errorf("ProxyURL = %q, lima.yaml:\n%s", u, data)
		}
		return
	}
	fi, err := os.Stat(yaml)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"HTTP_PROXY":"`+u+`"`) || strings.Contains(string(data), passwordStandIn) || fi.Mode().Perm() != 0o600 {
		t.Errorf("lima.yaml (%v):\n%s", fi.Mode(), data)
	}
}

// checkStartCall checks limactl start's arguments, folder and, when
// wrapped, its sandbox-exec profile.
func checkStartCall(t *testing.T, l *Lima, s call, wrapped bool) {
	t.Helper()
	args := s.argv[slices.Index(s.argv, "start"):]
	if !slices.Equal(args, []string{"start", "--tty=false", "v"}) {
		t.Errorf("start's arguments: %q", args)
	}
	if s.dir != l.Dirs.Data {
		t.Errorf("ran in %s", s.dir)
	}
	if (s.argv[0] == "sandbox-exec") != wrapped {
		t.Errorf("under sandbox-exec: %q", s.argv)
		return
	}
	if wrapped {
		lc := s.argv[3]
		if !slices.Equal(s.argv[1:3], []string{"-f", l.profileFile("v")}) || !filepath.IsAbs(lc) || filepath.Base(lc) != "limactl" {
			t.Errorf("sandbox-exec's arguments: %q", s.argv)
		}
	}
}

// fillPassword fails, changing nothing, when lima.yaml lacks the stand-in or
// the URL isn't plain YAML.
func TestFillPassword(t *testing.T) {
	l, _, _ := testLima(t, "darwin")
	yaml := filepath.Join(l.Dirs.LimaVM("v"), "lima.yaml")
	if err := os.MkdirAll(filepath.Dir(yaml), 0o700); err != nil {
		t.Fatal(err)
	}
	c := backend.Confinement{Net: backend.NetProxy, ProxyPort: 1, ProxyUser: "devvm", ProxyPassword: "pw"}
	for _, tc := range []struct{ name, yaml, password string }{
		{"no stand-in", "env: {}\n", "pw"},
		{"an anchor", "env: {HTTP_PROXY: http://devvm:devvm-proxy-password@192.168.5.2:1}\n", "p&w"},
	} {
		if err := os.WriteFile(yaml, []byte(tc.yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		c.ProxyPassword = tc.password
		if err := l.fillPassword("v", c); err == nil {
			t.Errorf("%s: filled", tc.name)
		}
		if b, _ := os.ReadFile(yaml); string(b) != tc.yaml { //nolint:gosec // the test's own file
			t.Errorf("%s: changed to %q", tc.name, b)
		}
	}
}

// A VM whose config already matches gets no edit: an edit means new ssh host
// keys at its next boot.
func TestStartEditsOnlyWhatDiffers(t *testing.T) {
	l, f, _ := testLima(t, "linux")
	c := backend.Confinement{Net: backend.NetProxy, ProxyPort: 23456, SSHPort: 2222}
	f.out["limactl list --json"] = `{"name":"v","status":"Stopped","config":{"ssh":{"localPort":2222},"propagateProxyEnv":false,"portForwards":` + yqJSON(noForwards()) + `,` +
		`"hostResolver":{"enabled":false},"dns":["127.0.0.1"],"env":{"HTTPS_PROXY":"http://192.168.5.4:3128","HTTP_PROXY":"http://192.168.5.4:3128",` +
		`"http_proxy":"http://192.168.5.4:3128","https_proxy":"http://192.168.5.4:3128"}}}`
	if err := os.MkdirAll(l.Dirs.LimaVM("v"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.haPIDFile("v"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(t.Context(), "v", c); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(f.lines(), func(l string) bool { return strings.HasPrefix(l, "limactl edit") }) {
		t.Errorf("edited: %q", f.lines())
	}
	// Started without the proxy, the proxy's settings go.
	f.calls = nil
	if err := l.Start(t.Context(), "v", backend.Confinement{SSHPort: 2222}); err != nil {
		t.Fatal(err)
	}
	want := "limactl edit --tty=false v --set del(.env) --set .propagateProxyEnv=true --set del(.hostResolver) --set del(.dns)"
	if got := f.lines(); len(got) == 0 || got[0] != want {
		t.Errorf("ran %q, want %q first", got, want)
	}
}

// A running VM is left as it is.
func TestStartLeavesARunningVM(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	f.out["limactl list --json"] = `{"name":"v","status":"Running"}`
	if err := l.Start(t.Context(), "v", backend.Confinement{Net: backend.NetProxy}); err != nil {
		t.Fatal(err)
	}
	if got := f.lines(); len(got) > 0 {
		t.Errorf("ran %q", got)
	}
}

// The profile lets the hostagent reach unix sockets only in the VM's Lima
// folder and the ssh agent, with links resolved, and loopback only at the
// VM's ssh port and its proxy.
func TestProfile(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"real/agent", "real/lima/vm1"} {
		if err := os.MkdirAll(filepath.Join(tmp, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(tmp, "real"), filepath.Join(tmp, "link")); err != nil {
		t.Fatal(err)
	}
	p := profile{LimaDir: filepath.Join(tmp, "link", "lima", "vm1"), AgentSocket: filepath.Join(tmp, "link", "agent", "s"), SSHPort: 2222, ProxyPort: 3128}
	got := strings.Split(p.render(), "\n")
	for _, line := range []string{
		"(deny network-outbound)",
		`(allow network-outbound (remote unix-socket (subpath "` + tmp + `/real/lima/vm1")))`,
		`(allow network-outbound (remote unix-socket (path-literal "` + tmp + `/real/agent/s")))`,
		`(allow network-outbound (remote ip "localhost:2222"))`,
		`(allow network-outbound (remote ip "localhost:3128"))`,
		`(deny mach-lookup (global-name "com.apple.dnssd.service"))`,
	} {
		if !slices.Contains(got, line) {
			t.Errorf("no %s in\n%s", line, p.render())
		}
	}
	if slices.Contains(got, "(allow network-outbound (remote unix-socket))") {
		t.Error("every unix socket allowed")
	}
	if strings.Contains(p.render(), "network-bind") {
		t.Error("binds denied without NoBinds")
	}
	p.AgentSocket, p.ProxyPort = "", 0
	if n := strings.Count(p.render(), "unix-socket"); n != 1 {
		t.Errorf("%d unix-socket rules without an agent", n)
	}
	if strings.Contains(p.render(), "localhost:3128") {
		t.Error("a proxy port without a proxy")
	}
	// NoBinds: no bind but the ssh port's and the VM's own unix sockets',
	// after every other rule, so none allows more.
	p.NoBinds = true
	wantBinds := "(deny network-bind)\n(allow network-bind (local tcp \"localhost:2222\"))\n" +
		"(allow network-bind (local unix-socket (subpath \"" + tmp + "/real/lima/vm1\")))\n"
	if got := p.render(); !strings.HasSuffix(got, wantBinds) || strings.Count(got, "network-bind") != 3 {
		t.Errorf("NoBinds profile:\n%s\nwant it to end\n%s", got, wantBinds)
	}
	// A build with the internet: everything but host loopback.
	p = profile{Internet: true, LimaDir: "/l/vm", SSHPort: 2224}
	want := "(version 1)\n(allow default)\n(deny network-outbound (remote ip \"localhost:*\"))\n(allow network-outbound (remote ip \"localhost:2224\"))\n"
	if got := p.render(); got != want {
		t.Errorf("internet profile:\n%s\nwant\n%s", got, want)
	}
}

// A VM using devvm's own agent may reach that agent, not the user's; one
// without forwarding, neither.
func TestProfileAgent(t *testing.T) {
	for _, tc := range []struct {
		forwarding bool
		socket     string
		want       string
	}{
		{true, "/d/agent.sock", "/d/agent.sock"},
		{true, "", "/user/agent"},
		{false, "/d/agent.sock", ""},
	} {
		l, f, _ := testLima(t, "darwin")
		t.Setenv("SSH_AUTH_SOCK", "/user/agent")
		l.Getenv = func(k string) string {
			if k == "SSH_AUTH_SOCK" {
				return "/user/agent"
			}
			return ""
		}
		f.out["limactl list --json"] = `{"name":"v","status":"Stopped","config":{"ssh":{"forwardAgent":` + map[bool]string{true: "true", false: "false"}[tc.forwarding] + `}}}`
		if err := os.MkdirAll(l.Dirs.LimaVM("v"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(l.haPIDFile("v"), []byte("1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := l.Start(t.Context(), "v", backend.Confinement{Net: backend.NetProxy, ProxyPort: 1, SSHPort: 2, AgentSocket: tc.socket}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(l.profileFile("v"))
		if err != nil {
			t.Fatal(err)
		}
		lits := regexp.MustCompile(`path-literal "([^"]*)"`).FindAllStringSubmatch(string(b), -1)
		got := ""
		if len(lits) > 0 {
			got = lits[0][1]
		}
		if got != tc.want || len(lits) > 1 {
			t.Errorf("forwarding %v, socket %q: agent %q in\n%s", tc.forwarding, tc.socket, got, b)
		}
		wantSock := tc.socket
		if wantSock == "" {
			wantSock = "/user/agent" // the user's, as it is
		}
		if v, _ := envOf(startCall(t, f), "SSH_AUTH_SOCK"); v != wantSock {
			t.Errorf("socket %q: SSH_AUTH_SOCK=%q, want %q", tc.socket, v, wantSock)
		}
		if fi, err := os.Stat(l.Dirs.Run()); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("run folder: %v, %v", fi.Mode(), err)
		}
	}
}

func TestSBString(t *testing.T) {
	if got, want := sbString(`/a "b" \c`), `"/a \"b\" \\c"`; got != want {
		t.Errorf("sbString = %s, want %s", got, want)
	}
}

// Lima can report a failed start for a VM that's running: carry on with a
// warning, unless it doesn't answer over ssh; then it's stopped.
func TestDegradedStart(t *testing.T) {
	l, warned := stubLima(t)
	vm := "devvm-img-0123456789ab"
	writeStub(t, "instances", vm+" Stopped\n")
	writeStub(t, "start-fails", "")
	if err := l.Start(t.Context(), vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"started_with_warnings vm=" + vm}; !slices.Equal(*warned, want) {
		t.Errorf("warned %q, want %q", *warned, want)
	}
	writeStub(t, "instances", vm+" Stopped\n")
	writeStub(t, "shell-hangs", "")
	l.SSHTimeout = 200 * time.Millisecond
	err := l.Start(t.Context(), vm, backend.Confinement{})
	le, ok := errors.AsType[*backend.LoggedError](err)
	if !ok || le.Msg.Names() != "start_no_ssh console="+filepath.Join(l.Dirs.LimaVM(vm), "serial*.log")+" vm="+vm {
		t.Errorf("no ssh: %v", err)
	}
	if !slices.Contains(stubCalls(t), "limactl stop --force "+vm) {
		t.Errorf("not stopped: %q", stubCalls(t))
	}
	// Not running after a failed start: it failed.
	f := &fakeRunner{out: map[string]string{"limactl list --json": `{"name":"v","status":"Stopped"}`}, fail: []string{"limactl start"}}
	l.Runner = f
	err = l.Start(t.Context(), "v", backend.Confinement{})
	if le, ok := errors.AsType[*backend.LoggedError](err); !ok || le.Msg.ID() != "start_failed" {
		t.Errorf("a failed start: %v", err)
	}
}

// Confined: the VM still runs on the hostagent Start started, which runs.
func TestConfined(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	f.out["limactl list --json"] = `{"name":"v","status":"Stopped"}`
	if ok, err := l.Confined(t.Context(), "v"); ok || err != nil {
		t.Errorf("never started: %v, %v", ok, err)
	}
	if err := os.MkdirAll(l.Dirs.LimaVM("v"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.haPIDFile("v"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(t.Context(), "v", backend.Confinement{Net: backend.NetProxy, ProxyPort: 1, SSHPort: 2}); err != nil {
		t.Fatal(err)
	}
	if ok, err := l.Confined(t.Context(), "v"); !ok || err != nil {
		t.Errorf("after Start: %v, %v", ok, err)
	}
	// Started again some other way: a new hostagent.
	if err := os.WriteFile(l.haPIDFile("v"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := l.Confined(t.Context(), "v"); ok || err != nil {
		t.Errorf("another hostagent: %v, %v", ok, err)
	}
	// Started by Start, but without its proxy (network policy off then).
	if err := os.WriteFile(l.haPIDFile("v"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(t.Context(), "v", backend.Confinement{Net: backend.NetDirect}); err != nil {
		t.Fatal(err)
	}
	if ok, err := l.Confined(t.Context(), "v"); ok || err != nil {
		t.Errorf("started direct: %v, %v", ok, err)
	}
}
