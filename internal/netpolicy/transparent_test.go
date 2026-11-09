package netpolicy

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/proxy"
	"github.com/devvmhq/devvm/internal/state"
)

// A VM's transparent mode: its own value, else config.toml's, else on; an
// old state without the field follows config.toml.
func TestTransparentOf(t *testing.T) {
	d, st := sandbox(t, "base")
	config := func(s string) {
		t.Helper()
		if err := os.MkdirAll(d.Config, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(d.ConfigFile(), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		config, own string
		want        Transparent
	}{
		{"", "", Transparent{On: true, From: FromDefault}},
		{"transparent-proxy = false\n", "", Transparent{On: false, From: FromConfig}},
		{"transparent-proxy = true\n", "", Transparent{On: true, From: FromConfig}},
		{"transparent-proxy = true\n", TransparentOff, Transparent{On: false, From: FromVM}},
		{"transparent-proxy = false\n", TransparentOn, Transparent{On: true, From: FromVM}},
		{"", TransparentOff, Transparent{On: false, From: FromVM}},
	} {
		config(c.config)
		change(t, st, func(st *state.VM) error { return SetTransparent(st, c.own) })
		if got, err := TransparentOf(d, st); err != nil || got != c.want {
			t.Errorf("config %q, own %q: %+v, %v; want %+v", c.config, c.own, got, err, c.want)
		}
	}
	config("transparent-proxy = \"no\"\n")
	change(t, st, func(st *state.VM) error { return SetTransparent(st, "") })
	if _, err := TransparentOf(d, st); err == nil {
		t.Error("a config.toml that isn't true or false was taken")
	}
	b, err := os.ReadFile(filepath.Join(d.VM(st.Name()).Dir, "vm.json"))
	if err != nil || strings.Contains(string(b), "transparent") {
		t.Errorf("an unset value is written: %v %s", err, b)
	}
}

// The stand-in address goes to the setup only while the VM runs confined
// to its proxy with transparent mode on.
func TestTransparentAddr(t *testing.T) {
	on, off := Transparent{On: true}, Transparent{}
	for _, c := range []struct {
		conf backend.Confinement
		t    Transparent
		want string
	}{
		{backend.Confinement{Net: backend.NetProxy}, on, proxy.SentinelAddr},
		{backend.Confinement{Net: backend.NetProxy}, off, ""},
		{backend.Confinement{Net: backend.NetDirect, ProxyPort: 3128}, on, ""},
		{backend.Confinement{}, on, ""},
	} {
		if got := TransparentAddr(c.conf, c.t); got != c.want {
			t.Errorf("%+v, %+v: %q, want %q", c.conf, c.t, got, c.want)
		}
	}
}

// The self-check's direct connection is to an address on 443, never a
// name, which transparent mode would send to the proxy, nor port 53, whose
// DNS transparent mode answers in the VM.
func TestDirectCheckIsAnAddress(t *testing.T) {
	u := directArgv[len(directArgv)-1]
	if u != "https://1.1.1.1" || !slices.Contains(directArgv, "--noproxy") {
		t.Errorf("the direct check is %q", directArgv)
	}
}

// devvm network lock lists apart the hosts that only tools without proxy
// settings reached: one reached both ways is an ordinary one.
func TestLearnedNoProxy(t *testing.T) {
	l := &Log{Path: filepath.Join(t.TempDir(), "network.jsonl")}
	since := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	via := func(line string) string { return strings.TrimSuffix(line, "}") + `,"via":"dns"}` }
	writeLog(t, l.Path,
		at(since, learnEvent, "", 0, ""),
		via(at(since.Add(time.Second), "tunnel", "telemetry.test", 443, "")),
		via(at(since.Add(2*time.Second), "tunnel", "registry.npmjs.org", 443, "")),
		at(since.Add(3*time.Second), "tunnel", "registry.npmjs.org", 443, "npm/10.9.2"),
		via(at(since.Add(4*time.Second), "forward", "plain.test", 80, "")),
	)
	got, err := l.Learned(since, none())
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, h := range got.Hosts {
		rows = append(rows, h.Rule+" "+map[bool]string{true: "no proxy", false: "proxy"}[h.NoProxy])
	}
	want := []string{"registry.npmjs.org:443 proxy", "plain.test:80 no proxy", "telemetry.test:443 no proxy"}
	if !slices.Equal(rows, want) {
		t.Errorf("Learned: %q, want %q", rows, want)
	}
}
