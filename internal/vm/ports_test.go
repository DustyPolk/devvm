package vm

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

func TestAddRemovePorts(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, c := range []struct {
		rm             bool
		old, list, out string
		err            string
	}{
		{old: "none", list: "5173", out: "5173"},
		{old: "5173", list: "8080,5173", out: "5173,8080"},
		{old: "all", list: "8080", out: "all"},
		{old: "5173", list: "all", err: `ports_list_needed value="all"`},
		{old: "5173", list: "0", err: `port_invalid value="0"`},
		{rm: true, old: "5173,8000-8010", list: "5173", out: "8000-8010"},
		{rm: true, old: "5173", list: " 5173 ", out: "none"},
		{rm: true, old: "all", list: "5173", err: "ports_rm_all"},
		{rm: true, old: "5173", list: "8080", err: "port_not_listed port=8080 ports=5173"},
		{rm: true, old: "none", list: "5173", err: "port_not_listed port=5173 ports=none"},
		{rm: true, old: "5173", list: "nope", err: `port_invalid value="nope"`},
		{rm: true, old: "5173", list: "none", err: `ports_list_needed value="none"`},
		{rm: true, old: "80,x", list: "80", err: `port_invalid value="80,x"`},
	} {
		f, name := AddPorts, "AddPorts"
		if c.rm {
			f, name = RemovePorts, "RemovePorts"
		}
		got, err := f(c.old, c.list)
		if got != c.out || msg.NamesOf(err) != c.err {
			t.Errorf("%s(%q, %q) = %q, %v; want %q, %s", name, c.old, c.list, got, msg.NamesOf(err), c.out, c.err)
		}
	}
}

// A listed range splits around the ports in use, never joining another;
// one wholly free stays as listed.
func TestSplitPorts(t *testing.T) {
	r := func(from, to int) backend.PortRange { return backend.PortRange{From: from, To: to} }
	inUse := func(p int) bool { return p == 5174 || p == 8080 || p == 8081 }
	free, taken := splitPorts([]backend.PortRange{r(5173, 5173), r(5174, 5176), r(5177, 5177), r(8079, 8082)}, inUse)
	if want := []backend.PortRange{r(5173, 5173), r(5175, 5176), r(5177, 5177), r(8079, 8079), r(8082, 8082)}; !reflect.DeepEqual(free, want) {
		t.Errorf("free %v, want %v", free, want)
	}
	if want := []backend.PortRange{r(5174, 5174), r(8080, 8081)}; !reflect.DeepEqual(taken, want) {
		t.Errorf("taken %v, want %v", taken, want)
	}
}

// A start forwards the listed ports less those a host program has,
// warned, as the backend asks once its old forwarder stopped (Free);
// every port is left to the backend, and none forwards none.
func TestStartPorts(t *testing.T) {
	for _, tc := range []struct {
		ports, warned string
		want          []backend.PortRange
	}{
		{"5173-5175,8080", "ports_taken count=2 ports=5174,8080 vm=", []backend.PortRange{{From: 5173, To: 5173}, {From: 5175, To: 5175}}},
		{"5173", "", []backend.PortRange{{From: 5173, To: 5173}}},
		{"none", "", []backend.PortRange{}},
		{"all", "", nil},
	} {
		t.Run(strings.ReplaceAll(tc.ports, ",", "+"), func(t *testing.T) { // a comma isn't a project path's
			e, f, _ := upEnv(t, allNative())
			e.PortInUse = func(_ context.Context, p int) bool { return p == 5174 || p == 8080 }
			var warned msg.Recorder
			e.Warn = warned.Add
			st := existingVM(t, e, Options{NoNetworkPolicy: true, Ports: tc.ports})
			var got []backend.PortRange
			for _, c := range f.Calls() {
				if c.Method == "Start" {
					conf := c.Args[1].(backend.Confinement)
					if conf.HostKey == nil {
						t.Error("started without its host key")
					}
					p, err := PortsBackend(tc.ports)
					if err != nil {
						t.Fatal(err)
					}
					got = conf.Forwarded(t.Context(), p)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("started with %+v, want %+v", got, tc.want)
			}
			want := []string(nil)
			if tc.warned != "" {
				want = []string{tc.warned + st.Name()}
			}
			if taken := slices.DeleteFunc(warned.Names(), func(w string) bool { return !strings.HasPrefix(w, "ports_taken") }); !reflect.DeepEqual(taken, want) {
				t.Errorf("warned %q, want %q", taken, want)
			}
		})
	}
}

// A running VM's ports change at once: its backend gets the new ports with
// what its forwarder starts again with (its host key, its proxy's port,
// Free warning about taken ports), and devvm ports shows them. A stopped
// VM's backend gets them alone.
func TestSetPortsRunning(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	var infos, warned msg.Recorder
	e.Info = infos.Add
	e.Warn = warned.Add
	st := existingVM(t, e, Options{NoNetworkPolicy: true, Ports: "5173"})
	vm := st.Name()
	if err := set(t, st, "proxy_port", "30001"); err != nil {
		t.Fatal(err)
	}
	e.PortInUse = func(_ context.Context, p int) bool { return p == 8080 }
	n := len(f.Calls())
	infos.Reset()
	warned.Reset()
	s := Settings{e}
	if err := s.SetPorts(t.Context(), PortsChange{Op: "add", List: "8080-8081"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	setPorts := func(n int) []any {
		t.Helper()
		var args []any
		for _, c := range f.Calls()[n:] {
			switch c.Method {
			case "SetPorts":
				args = c.Args
			case "Start", "Stop":
				t.Errorf("%s called", c.Method)
			}
		}
		if args == nil {
			t.Fatal("no SetPorts")
		}
		return args
	}
	args := setPorts(n)
	want := backend.Ports{Ranges: []backend.PortRange{{From: 5173, To: 5173}, {From: 8080, To: 8081}}}
	conf := args[2].(backend.Confinement)
	if !reflect.DeepEqual(args[1], want) || conf.HostKey == nil || conf.ProxyPort != 30001 {
		t.Errorf("SetPorts(%v, %+v)", args[1], conf)
	}
	if got := conf.Forwarded(t.Context(), want); !reflect.DeepEqual(got, []backend.PortRange{{From: 5173, To: 5173}, {From: 8081, To: 8081}}) {
		t.Errorf("forwarded %v", got)
	}
	if !slices.Equal(infos.Names(), []string{"ports_forwarding ports=5173,8080-8081 vm=" + vm}) || !slices.Equal(warned.Names(), []string{"ports_taken count=1 ports=8080 vm=" + vm}) {
		t.Errorf("said %q, warned %q", infos.Names(), warned.Names())
	}
	if ports, err := s.Ports(t.Context()); err != nil || ports != "5173,8080-8081" {
		t.Errorf("ports: %q, %v", ports, err)
	}
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	n = len(f.Calls())
	infos.Reset()
	if err := s.SetPorts(t.Context(), PortsChange{List: "none"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if args := setPorts(n); !reflect.DeepEqual(args[2], backend.Confinement{}) || !slices.Equal(infos.Names(), []string{"ports_now ports=none vm=" + vm}) {
		t.Errorf("stopped: SetPorts %v, said %q", args, infos.Names())
	}
	// Its backend failing: its state as it was, nothing said.
	f.Errs = map[string]error{"SetPorts": msg.VMNotStopped(vm)}
	infos.Reset()
	if err := s.SetPorts(t.Context(), PortsChange{List: "5173"}); msg.NamesOf(err) != "vm_not_stopped vm="+vm || len(infos.Names()) != 0 {
		t.Errorf("backend failed: %v, said %q", err, infos.Names())
	}
	if now, err := state.ReadVM(e.Dirs, vm); err != nil || get(now, KeyPorts) != "none" {
		t.Errorf("backend failed: state ports=%q, %v", get(now, KeyPorts), err)
	}
}

func TestPortsBackend(t *testing.T) {
	for in, want := range map[string]backend.Ports{
		"":               {All: true},
		"all":            {All: true},
		"none":           {},
		"5173,8000-8010": {Ranges: []backend.PortRange{{From: 5173, To: 5173}, {From: 8000, To: 8010}}},
	} {
		if got, err := PortsBackend(in); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("PortsBackend(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	// A bad entry, as a hand-edited state file has it, is refused, never
	// port 0.
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, bad := range []string{"80,x", "x", "80-y", "5173,,80"} {
		got, err := PortsBackend(bad)
		if msg.NamesOf(err) != `port_invalid value="`+bad+`"` || len(got.Ranges) != 0 {
			t.Errorf("PortsBackend(%q) = %+v, %v", bad, got, msg.NamesOf(err))
		}
	}
}

// A VM whose state lists a port that isn't one doesn't start.
func TestStartBadPorts(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	e, f, _ := upEnv(t, allNative())
	st := existingVM(t, e, Options{NoNetworkPolicy: true, Ports: "5173"})
	if err := set(t, st, KeyPorts, "5173,x"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.start(t.Context(), f, st, backend.Confinement{}); msg.NamesOf(err) != `port_invalid value="5173,x"` {
		t.Errorf("start = %v", msg.NamesOf(err))
	}
}
