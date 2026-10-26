package vm

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
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
// warned; every port is left to the backend.
func TestStartPorts(t *testing.T) {
	for _, tc := range []struct {
		ports, warned string
		want          *backend.Ports
	}{
		{"5173-5175,8080", "ports_taken count=2 ports=5174,8080 vm=", &backend.Ports{Ranges: []backend.PortRange{{From: 5173, To: 5173}, {From: 5175, To: 5175}}}},
		{"5173", "", &backend.Ports{Ranges: []backend.PortRange{{From: 5173, To: 5173}}}},
		{"none", "", nil},
		{"all", "", nil},
	} {
		t.Run(strings.ReplaceAll(tc.ports, ",", "+"), func(t *testing.T) { // a comma isn't a project path's
			e, f, _ := upEnv(t, allNative())
			e.PortInUse = func(_ context.Context, p int) bool { return p == 5174 || p == 8080 }
			var warned []string
			e.Warn = func(m msg.M) { warned = append(warned, m.Names()) }
			st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true, Ports: tc.ports})
			var got *backend.Ports
			for _, c := range f.Calls() {
				if c.Method == "Start" {
					got = c.Args[1].(backend.Confinement).Ports
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
