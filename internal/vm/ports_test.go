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

func TestParsePorts(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for in, want := range map[string]string{
		"all": "all", "none": "none", "5173, 8000-8010,5173": "5173,8000-8010", "0080,3000-3000": "80,3000",
		" all ": "all",
	} {
		if got, err := ParsePorts(in); got != want || err != nil {
			t.Errorf("ParsePorts(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", " ", "0", "65536", "8010-8000", "5173,,80", "abc", "80-", "1-2-3", "123456"} {
		if _, err := ParsePorts(bad); names(err) != "port_invalid value="+`"`+bad+`"` {
			t.Errorf("ParsePorts(%q) = %v", bad, names(err))
		}
	}
}

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
	} {
		f, name := AddPorts, "AddPorts"
		if c.rm {
			f, name = RemovePorts, "RemovePorts"
		}
		got, err := f(c.old, c.list)
		if got != c.out || names(err) != c.err {
			t.Errorf("%s(%q, %q) = %q, %v; want %q, %s", name, c.old, c.list, got, names(err), c.out, c.err)
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
	if got := portsValue(taken); got != "5174,8080-8081" {
		t.Errorf("portsValue = %q", got)
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
			var warned []string
			e.Warn = func(m msg.M) { warned = append(warned, m.Names()) }
			st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true, Ports: tc.ports})
			var got []backend.PortRange
			for _, c := range f.Calls() {
				if c.Method == "Start" {
					conf := c.Args[1].(backend.Confinement)
					if conf.HostKey == nil {
						t.Error("started without its host key")
					}
					got = conf.Forwarded(t.Context(), PortsBackend(tc.ports))
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("started with %+v, want %+v", got, tc.want)
			}
			want := []string(nil)
			if tc.warned != "" {
				want = []string{tc.warned + st.Name()}
			}
			if taken := slices.DeleteFunc(warned, func(w string) bool { return !strings.HasPrefix(w, "ports_taken") }); !reflect.DeepEqual(taken, want) {
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
	var infos, warned []string
	e.Info = func(m msg.M) { infos = append(infos, m.Names()) }
	e.Warn = func(m msg.M) { warned = append(warned, m.Names()) }
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true, Ports: "5173"})
	vm := st.Name()
	if err := st.Set("proxy_port", "30001"); err != nil {
		t.Fatal(err)
	}
	e.PortInUse = func(_ context.Context, p int) bool { return p == 8080 }
	n := len(f.Calls())
	infos, warned = nil, nil
	s := Settings{e}
	if err := s.SetPorts(t.Context(), PortsChange{Op: "add", List: "8080-8081"}); err != nil {
		t.Fatal(names(err))
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
	if !slices.Equal(infos, []string{"ports_forwarding ports=5173,8080-8081 vm=" + vm}) || !slices.Equal(warned, []string{"ports_taken count=1 ports=8080 vm=" + vm}) {
		t.Errorf("said %q, warned %q", infos, warned)
	}
	var b strings.Builder
	if err := s.Ports(t.Context(), &b, false); err != nil || b.String() != "5173,8080-8081\n" {
		t.Errorf("ports: %q, %v", b.String(), err)
	}
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	n = len(f.Calls())
	infos = nil
	if err := s.SetPorts(t.Context(), PortsChange{List: "none"}); err != nil {
		t.Fatal(names(err))
	}
	if args := setPorts(n); !reflect.DeepEqual(args[2], backend.Confinement{}) || !slices.Equal(infos, []string{"ports_now ports=none vm=" + vm}) {
		t.Errorf("stopped: SetPorts %v, said %q", args, infos)
	}
	// Its backend failing: its state as it was, nothing said.
	f.Errs = map[string]error{"SetPorts": msg.VMNotStopped(vm)}
	infos = nil
	if err := s.SetPorts(t.Context(), PortsChange{List: "5173"}); names(err) != "vm_not_stopped vm="+vm || len(infos) != 0 {
		t.Errorf("backend failed: %v, said %q", err, infos)
	}
	if now, err := state.ReadVM(e.Dirs, vm); err != nil || now.Get(KeyPorts) != "none" {
		t.Errorf("backend failed: state ports=%q, %v", now.Get(KeyPorts), err)
	}
}

func TestPortsBackend(t *testing.T) {
	for in, want := range map[string]backend.Ports{
		"":               {All: true},
		"all":            {All: true},
		"none":           {},
		"5173,8000-8010": {Ranges: []backend.PortRange{{From: 5173, To: 5173}, {From: 8000, To: 8010}}},
	} {
		if got := PortsBackend(in); !reflect.DeepEqual(got, want) {
			t.Errorf("PortsBackend(%q) = %+v, want %+v", in, got, want)
		}
	}
}
