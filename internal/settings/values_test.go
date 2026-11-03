package settings

import (
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

func TestParseSizes(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, tc := range []struct{ in, want, err string }{
		{"6", "6", ""},
		{"1.5", "", `cpus_not_number flag=--cpus value="1.5"`},
		{"0", "", `cpus_not_number flag=--cpus value="0"`},
		{"many", "", `cpus_not_number flag=--cpus value="many"`},
	} {
		got, err := ParseCPUs("--cpus", tc.in)
		if got != tc.want || msg.NamesOf(err) != tc.err {
			t.Errorf("ParseCPUs(%q) = %q, %v", tc.in, got, msg.NamesOf(err))
		}
	}
	for _, tc := range []struct{ in, want, err string }{
		{"16", "16", ""},
		{"16G", "16", ""},
		{"16g", "16", ""},
		{"200GiB", "200", ""},
		{"1.5", "1.5", ""},
		{"0.5", "0.5", ""},
		{"0", "", `size_not_gib flag=--memory value="0"`},
		{"16M", "", `size_not_gib flag=--memory value="16M"`},
		{"lots", "", `size_not_gib flag=--memory value="lots"`},
		{"1$(touch x)", "", `size_not_gib flag=--memory value="1$(touch x)"`},
	} {
		got, err := ParseGiB("--memory", tc.in)
		if got != tc.want || msg.NamesOf(err) != tc.err {
			t.Errorf("ParseGiB(%q) = %q, %v", tc.in, got, msg.NamesOf(err))
		}
	}
}

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
		if _, err := ParsePorts(bad); msg.NamesOf(err) != "port_invalid value="+`"`+bad+`"` {
			t.Errorf("ParsePorts(%q) = %v", bad, msg.NamesOf(err))
		}
	}
}

// devvm retention's KEY=VALUE words: a known key with a value it takes,
// or off; anything else is refused.
func TestParseRetention(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	got, err := ParseRetention([]string{"history.age=30d", "logs.runs=50", "tools.unused-age=off"})
	if err != nil || len(got) != 3 || got[RetainHistoryAge] != "30d" || got[RetainRunLogs] != "50" || got[RetainToolsAge] != "" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, bad := range [][]string{
		{"nope=1"},
		{"history.age=30"},
		{"logs.runs=0"},
		{"logs.runs=05"},
		{"history.count=x"},
		{"novalue"},
		{"history.age=1d", "orphans.age=-1d"},
		{"history.age="},
		{"nope=off"},
		{"nix-cache.size=20g"},
	} {
		if _, err := ParseRetention(bad); err == nil {
			t.Errorf("ParseRetention(%q): want an error", bad)
		}
	}
	var r Retention
	if r.NixCacheCap() != NixCacheDefault || !r.Off() || r.Source(RetainRunLogs) != FromBuiltin {
		t.Errorf("off: %+v", r)
	}
	if err := r.set(RetainNixCacheSize, "2G"); err != nil || r.NixCacheCap() != 2<<30 || r.Off() {
		t.Errorf("2G: %+v, %v", r, err)
	}
}
