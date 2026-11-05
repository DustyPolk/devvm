package vm

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
)

// startRun is what a start path did: the backend's starts and the
// confinement of the last, the self-checks, the warnings, the error, and
// whether the VM's proxy runs after it.
type startRun struct {
	starts     int
	net        backend.Net
	selfChecks int
	warned     []string
	err        string
	proxy      bool
}

// startPath runs one way devvm starts or opens the folder's VM.
type startPath struct {
	name string
	run  func(ctx context.Context, e *Env) error
}

// The ways an existing VM is started: a launch (devvm), ssh's (devvm
// host), and a command that only needs it running (ensureRunning).
func startPaths() []startPath {
	return []startPath{
		{"open", func(ctx context.Context, e *Env) error { return Launch{e}.Up(ctx, Options{}) }},
		{"ready", func(ctx context.Context, e *Env) error { return Launch{e}.ShowHost(ctx) }},
		{"ensureRunning", func(ctx context.Context, e *Env) error {
			t, err := finder{e}.target(ctx, need{has: hasMade})
			if err != nil {
				return err
			}
			return t.ensureRunning(ctx)
		}},
	}
}

// recordStart runs path on e, recording what it did (startRun).
func recordStart(t *testing.T, e *Env, f *backendtest.Fake, path func(context.Context, *Env) error) startRun {
	t.Helper()
	var r startRun
	before := len(f.Calls())
	var warned msg.Recorder
	e.Warn = warned.Add
	err := path(t.Context(), e)
	r.err = msg.NamesOf(err)
	for _, c := range f.Calls()[before:] {
		switch c.Method {
		case "Start":
			r.starts++
			conf, _ := c.Args[2].(backend.Confinement)
			r.net = conf.Net
		case "Exec":
			if argv, _ := c.Args[1].([]string); len(argv) > 0 && argv[0] == "curl" {
				r.selfChecks++
			}
		}
	}
	// ssh's Include line, which ready asks for, isn't the start's.
	r.warned = slices.DeleteFunc(warned.Names(), func(n string) bool { return n == "ssh_include_yourself" })
	vm := vmName(t, e)
	_, r.proxy = e.Proxy.PID(vm)
	return r
}

// Each start path on an existing VM, as the VM is: stopped, it starts
// confined as its state says, and the self-check runs with network
// policy; running, it isn't started again, and keeps or gets back its
// proxy; network policy turned off while it ran confined keeps it on its
// proxy until its next start, said, with no self-check; SSH agent
// forwarding changed while it runs is said, and refused where the backend
// can't change it.
func TestStartPaths(t *testing.T) {
	for _, tc := range []struct {
		name string
		caps func(backend.Caps)
		net  bool
		// prep makes the VM, made and running, what the case needs.
		prep func(v startVM)
		want startRun
	}{
		{
			name: "stopped, network policy", net: true,
			prep: func(v startVM) { v.stop() },
			want: startRun{starts: 1, net: backend.NetProxy, selfChecks: 1, proxy: true},
		},
		{
			name: "stopped, no network policy",
			prep: func(v startVM) { v.stop() },
			want: startRun{starts: 1, net: backend.NetDirect},
		},
		{
			name: "running with its proxy", net: true,
			want: startRun{selfChecks: 1, proxy: true},
		},
		{
			name: "running, its proxy gone", net: true,
			prep: func(v startVM) { v.e.Proxy.Stop(v.st.Name()) },
			want: startRun{selfChecks: 1, proxy: true},
		},
		{
			name: "running, no network policy",
			want: startRun{},
		},
		{
			name: "network policy turned off while running", net: true,
			prep: func(v startVM) { v.set(keyNet, "off") },
			want: startRun{warned: []string{"net_off_next_start vm=%s"}, proxy: true},
		},
		{
			name: "network policy turned off, then stopped", net: true,
			prep: func(v startVM) { v.set(keyNet, "off"); v.stop() },
			want: startRun{starts: 1, net: backend.NetDirect},
		},
		{
			name: "agent turned on while running",
			prep: func(v startVM) { v.set(keyAgent, sshagent.On) },
			want: startRun{warned: []string{"agent_next_start vm=%s"}},
		},
		{
			name: "agent turned on, then stopped",
			prep: func(v startVM) { v.set(keyAgent, sshagent.On); v.stop() },
			want: startRun{starts: 1, net: backend.NetDirect},
		},
		{
			name: "agent turned on, no agent-toggle",
			caps: func(c backend.Caps) { c[backend.FeatureAgentToggle] = backend.None },
			prep: func(v startVM) { v.set(keyAgent, sshagent.On) },
			want: startRun{err: "backend_lacks backend=fake feature=agent-toggle vm=%s"},
		},
		{
			name: "agent forwarding, no agent-forwarding",
			prep: func(v startVM) {
				v.set(keyAgent, sshagent.On)
				v.stop()
				v.f.Capabilities[backend.FeatureAgentForwarding] = backend.None
			},
			want: startRun{err: "backend_lacks backend=fake feature=agent-forwarding vm=%s"},
		},
		{
			name: "network policy, no egress-proxy", net: true,
			prep: func(v startVM) {
				v.stop()
				v.e.Proxy.Stop(v.st.Name())
				v.f.Capabilities[backend.FeatureEgressProxy] = backend.None
			},
			want: startRun{err: "backend_lacks backend=fake feature=egress-proxy vm=%s"},
		},
		{
			name: "ports, no ports-all",
			prep: func(v startVM) { v.stop(); v.f.Capabilities[backend.FeaturePortsAll] = backend.None },
			want: startRun{err: "backend_lacks backend=fake feature=ports-all vm=%s"},
		},
	} {
		for _, p := range startPaths() {
			// No commas: the folder's path, which has the test's name,
			// would be one no VM can be made for.
			t.Run(strings.ReplaceAll(tc.name, ",", "")+"/"+p.name, func(t *testing.T) {
				caps := allNative()
				if tc.caps != nil {
					tc.caps(caps)
				}
				e, f := startEnv(t, caps)
				st := existingVM(t, e, Options{NetworkPolicy: tc.net, NoNetworkPolicy: !tc.net})
				if tc.prep != nil {
					tc.prep(startVM{t, e, f, st})
				}
				got := recordStart(t, e, f, p.run)
				want := tc.want
				want.err = withVM(want.err, st.Name())
				want.warned = nil
				for _, w := range tc.want.warned {
					want.warned = append(want.warned, withVM(w, st.Name()))
				}
				if !slices.Equal(got.warned, want.warned) || got.starts != want.starts || got.net != want.net ||
					got.selfChecks != want.selfChecks || got.err != want.err || got.proxy != want.proxy {
					t.Errorf("got %+v\nwant %+v", got, want)
				}
			})
		}
	}
}

// A new VM starts once, confined as it was made, with the self-check
// with network policy.
func TestStartNew(t *testing.T) {
	for _, tc := range []struct {
		name string
		net  bool
		want startRun
	}{
		{"network policy", true, startRun{starts: 1, net: backend.NetProxy, selfChecks: 1, proxy: true}},
		{"no network policy", false, startRun{starts: 1, net: backend.NetDirect}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, f := startEnv(t, allNative())
			got := recordStart(t, e, f, func(ctx context.Context, e *Env) error {
				return Launch{e}.Up(ctx, Options{NetworkPolicy: tc.net, NoNetworkPolicy: !tc.net})
			})
			t.Cleanup(func() { e.Proxy.Stop(vmName(t, e)) })
			if got.starts != tc.want.starts || got.net != tc.want.net || got.selfChecks != tc.want.selfChecks ||
				got.err != "" || got.proxy != tc.want.proxy || len(got.warned) != 0 {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// requirements lists each feature what a VM asks for needs, once, in
// order: a new machine's, its ports', its network's, its layers', its
// agent forwarding and its toggle, and ssh for a clone.
func TestRequirements(t *testing.T) {
	spec := backend.VMSpec{Tools: "/t", Want: backend.Want{Project: "/p", AgentForwarding: true}}
	for _, tc := range []struct {
		name string
		n    needs
		want []backend.Feature
	}{
		{"nothing", needs{}, nil},
		{"direct", needs{net: backend.NetDirect}, nil},
		{"network policy", needs{net: backend.NetProxy}, []backend.Feature{backend.FeatureEgressProxy}},
		{"all ports", needs{ports: backend.Ports{All: true}}, []backend.Feature{backend.FeaturePorts, backend.FeaturePortsAll}},
		{"some ports", needs{ports: backend.Ports{Ranges: []backend.PortRange{{From: 80, To: 80}}}}, []backend.Feature{backend.FeaturePorts}},
		{"layers", needs{img: image.Name(image.DockerLayer + "+" + image.NixLayer)}, []backend.Feature{backend.FeatureDockerLayer, backend.FeatureNixLayer}},
		{"agent", needs{agent: true}, []backend.Feature{backend.FeatureAgentForwarding}},
		{"agent turned off", needs{toggle: true}, []backend.Feature{backend.FeatureAgentToggle}},
		{"agent turned on", needs{agent: true, toggle: true}, []backend.Feature{backend.FeatureAgentForwarding, backend.FeatureAgentToggle}},
		{"clone", needs{ssh: true}, []backend.Feature{backend.FeatureSSH}},
		{"a new machine, its agent once", needs{spec: &spec, net: backend.NetProxy, agent: true, ssh: true}, []backend.Feature{
			backend.FeatureClone, backend.FeatureProjectMount, backend.FeatureToolsShare, backend.FeatureAgentForwarding,
			backend.FeatureEgressProxy, backend.FeatureSSH,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := requirements(tc.n); !slices.Equal(got, tc.want) {
				t.Errorf("requirements = %q, want %q", got, tc.want)
			}
		})
	}
}

// Opening a running VM that gets changes only at its next start (its
// size, agent forwarding, agent folder mounts and network policy turned
// off) asks one question after at most two lines naming them, past two
// the first and how many more: yes by default ([Y/n], Confirm) when its
// agent folder mounts wait, else no ([y/N], Ask). Yes stops it and
// starts it with them, then opens its shell; no, and no question without
// a terminal, warn of each and open the shell. A running VM without such
// changes asks nothing.
func TestOpenAsksToRestart(t *testing.T) {
	all := []string{"size", "agent", "mounts", "net"}
	for _, tc := range []struct {
		name        string
		changes     []string
		canAsk, yes bool
		// confirm: asked with Confirm ([Y/n]), not Ask ([y/N]); noted:
		// the context's names, before vm= (none for next_start_more).
		confirm bool
		noted   []string
	}{
		{"all, yes", all, true, true, true, []string{"next_start_size", "next_start_more count=3"}},
		{"all, no", all, true, false, true, []string{"next_start_size", "next_start_more count=3"}},
		{"all, no terminal", all, false, true, true, nil},
		{"size and agent, yes", []string{"size", "agent"}, true, true, false, []string{"next_start_size", "next_start_agent"}},
		{"size, no", []string{"size"}, true, false, false, []string{"next_start_size"}},
		{"mounts, yes", []string{"mounts"}, true, true, true, []string{"next_start_mounts"}},
		{"nothing waits", nil, true, true, false, nil},
	} {
		t.Run(strings.ReplaceAll(tc.name, ",", ""), func(t *testing.T) {
			e, f := startEnv(t, allNative())
			e.Dirs.Home = t.TempDir()
			st := existingVM(t, e, Options{NetworkPolicy: true, Toolchains: &[]string{}, Agents: &[]string{"claude"}})
			vm := st.Name()
			skills := filepath.Join(e.Dirs.Home, ".claude", "skills")
			v := startVM{t, e, f, st}
			later := map[string]string{
				"size": "size_next_start", "agent": "agent_next_start", "mounts": "mounts_next_start", "net": "net_off_next_start",
			}
			var wantLater []string
			for _, c := range tc.changes {
				wantLater = append(wantLater, later[c]+" vm="+vm)
				switch c {
				case "size":
					v.set(keyCPUs, "2")
				case "agent":
					v.set(keyAgent, sshagent.On)
				case "mounts":
					mkdirs(t, e.Dirs.Home, ".claude/skills")
				case "net":
					v.set(keyNet, "off")
				}
			}
			var asked []string
			e.CanAsk = func() bool { return tc.canAsk }
			e.Ask = func(m msg.M) bool { asked = append(asked, "ask "+m.Names()); return tc.yes }
			e.Confirm = func(m msg.M) bool { asked = append(asked, "confirm "+m.Names()); return tc.yes }
			var noted msg.Recorder
			e.Note = noted.Add
			before := len(f.Calls())
			run := recordStart(t, e, f, func(ctx context.Context, e *Env) error { return Launch{e}.Up(ctx, Options{}) })
			if run.err != "" {
				t.Fatal(run.err)
			}
			ms := f.Methods()[before:]
			stop, start := slices.Index(ms, "Stop"), slices.Index(ms, "Start")
			restarted := stop >= 0 && start > stop
			asks := tc.canAsk && len(tc.changes) > 0
			if want := asks && tc.yes; restarted != want {
				t.Errorf("restarted = %v, want %v: calls %q", restarted, want, ms)
			}
			var wantAsked, wantNoted, wantWarned []string
			if asks {
				how := "ask "
				if tc.confirm {
					how = "confirm "
				}
				wantAsked = []string{how + "restart_now count=" + strconv.Itoa(len(tc.changes)) + " vm=" + vm}
				for _, n := range tc.noted {
					if !strings.HasPrefix(n, "next_start_more ") {
						n += " vm=" + vm
					}
					wantNoted = append(wantNoted, n)
				}
			}
			if !restarted {
				wantWarned = wantLater
			}
			if restarted && slices.Equal(tc.changes, all) {
				w, _ := f.Calls()[before+start].Args[1].(backend.Want)
				if w.Size.CPUs != 2 || !w.AgentForwarding || !slices.Equal(w.AgentDirs, []string{skills}) || run.net != backend.NetDirect {
					t.Errorf("started wanting %+v on %v", w, run.net)
				}
			}
			// The launch's other warnings, of the agent folder found and
			// the agent's login, aren't the question's.
			warned := slices.DeleteFunc(run.warned, func(n string) bool { return !strings.Contains(n, "_next_start ") })
			if !slices.Equal(asked, wantAsked) || !slices.Equal(noted.Names(), wantNoted) || !slices.Equal(warned, wantWarned) {
				t.Errorf("asked %q, noted %q, warned %q", asked, noted.Names(), warned)
			}
		})
	}
}

// Opening an existing VM asks its backend for its instance once in the
// finder and once as it starts it.
func TestOpenInspectsOnce(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		e, f := startEnv(t, allNative())
		st := existingVM(t, e, Options{NetworkPolicy: true})
		if stopped {
			stopVM(t, f, st)
		}
		before := len(f.Calls())
		if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
			t.Fatal(msg.NamesOf(err))
		}
		n := 0
		for _, m := range f.Methods()[before:] {
			if m == "Inspect" {
				n++
			}
		}
		if n != 2 {
			t.Errorf("stopped %v: %d inspects: %q", stopped, n, f.Methods()[before:])
		}
	}
}

// startEnv is upEnv for the start paths: the VM's proxy follows a
// lifeline, and a VM's direct connections fail, as network policy has
// them, its forwarder reading its host keys.
func startEnv(t *testing.T, caps backend.Caps) (*Env, *backendtest.Fake) {
	t.Helper()
	e, f, _ := upEnv(t, caps)
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	hostKeys := f.ExecFunc
	f.ExecFunc = func(vm string, argv []string, o backend.ExecOpts) error {
		if err := noDirect(vm, argv, o); err != nil {
			return err
		}
		return hostKeys(vm, argv, o)
	}
	return e, f
}

// startVM is a made VM that TestStartPaths readies for its case.
type startVM struct {
	t  *testing.T
	e  *Env
	f  *backendtest.Fake
	st *state.VM
}

// stop stops the VM on the fake.
func (v startVM) stop() { stopVM(v.t, v.f, v.st) }

// set sets one field of its state (setField's keys).
func (v startVM) set(key, val string) {
	if err := set(v.t, v.st, key, val); err != nil {
		v.t.Fatal(err)
	}
}

// stopVM stops st's VM on the fake.
func stopVM(t *testing.T, f *backendtest.Fake, st *state.VM) {
	t.Helper()
	if err := f.Stop(t.Context(), st.Name()); err != nil {
		t.Fatal(err)
	}
}

// withVM fills a want's %s with the VM's name.
func withVM(s, vm string) string { return strings.ReplaceAll(s, "%s", vm) }
