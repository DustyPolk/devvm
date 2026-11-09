package proxy

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// v010Files are the files of name in every VM's run folder of devvm 0.1.0's state
// (testdata/v0.1.0/GOOS, which 0.1.0 itself made), both systems': never regenerated.
func v010Files(t *testing.T, name string) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join("..", "..", "testdata", "v0.1.0", "*", "data", "devvm", "vms", "*", "run", name))
	if err != nil || len(found) == 0 {
		t.Fatalf("no %s in 0.1.0's state: %v", name, err)
	}
	return found
}

// Every bindings file 0.1.0 wrote reads as it was, of version 1, and writing its bindings again
// gives the same bytes: a version 1 file, which 0.1.0's proxy reads alike, so a launch after an
// upgrade, or a proxy still running from before it, keeps the VM's secrets.
func TestV010Bindings(t *testing.T) {
	t.Parallel()
	for _, f := range v010Files(t, "secrets.json") {
		b, err := os.ReadFile(f) //nolint:gosec // G304: the repo's frozen fixture
		must(t, err)
		l, err := parseBindings(f, b)
		if err != nil || l.Version != 1 {
			t.Errorf("%s: version %d, %v", f, l.Version, err)
			continue
		}
		if again := (BindingList{Bindings: l.Bindings}).Encode(); !bytes.Equal(again, b) {
			t.Errorf("%s: written again as\n%s\nnot\n%s", f, again, b)
		}
	}
}

// Every proxy config 0.1.0 wrote, as the file the user has (mode 600: git keeps no mode), reads
// with today's ReadConfig, which refuses a field it doesn't know, and records the binary that
// started its proxy, so devvm tells that proxy is another binary's (otherBinary).
func TestV010ProxyConfig(t *testing.T) {
	t.Parallel()
	// The SHA-256 of the devvm binary that made 0.1.0's state on each system.
	recorded := map[string]string{
		"darwin": "f8016a61f83f96d6ace74a381630be63d56979eeab3e789d29e4b09106d5d3a6",
		"linux":  "f6e713e44d65ca81d6d035e6f2fedf4534acc1ee72391702ff302dcd3ae53029",
	}
	dir := t.TempDir()
	for i, f := range v010Files(t, "proxy.json") {
		b, err := os.ReadFile(f) //nolint:gosec // G304: the repo's frozen fixture
		must(t, err)
		cp := filepath.Join(dir, filepath.Base(filepath.Dir(filepath.Dir(f)))+"-"+string(rune('a'+i)))
		must(t, os.WriteFile(cp, b, 0o600)) //nolint:gosec // G703: a file in the test's own temp folder
		c, err := ReadConfig(cp)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		// .../v0.1.0/GOOS/data/devvm/vms/HASH/run/proxy.json
		goos := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(f)))))))
		if want := recorded[goos]; c.DevvmSHA256 != want {
			t.Errorf("%s: devvm_sha256 %q, want %s's %q", f, c.DevvmSHA256, goos, want)
		}
	}
}

// devvm 0.1.0's grammar, frozen: its TestParsePolicy's lines and whether each is a rule, and its
// TestDecide's policy and verdicts, as `git show v0.1.0:internal/proxy/proxy_test.go` has them.
// Rules by method and path add to the grammar, so no line here changes meaning; this table is
// its own, so an edit to today's TestParsePolicy can't move it.
func TestV010Rules(t *testing.T) {
	t.Parallel()
	text := "# a comment\n" +
		"\n" +
		"example.com\n" +
		"  *.github.com   # with a comment\n" +
		"example.com:x\n" +
		"api.example.com:0\n" +
		"! bad.example.com\n" +
		"!!worse.example.com\n" +
		"a b\n" +
		"*\n" +
		"a*.example.com\n" +
		"[::1]\n" +
		"fe80::1%eth0\n" +
		"x/y\n" +
		"example.com:+22\n" +
		"*.10.0.0.1\n" +
		"a..example.com\n" +
		"10.0.0.0/8:8080\n" +
		"Example.ORG.\n" +
		"\r\n" +
		"last.example.com"
	want := []PolicyLine{
		{3, "example.com", true},
		{4, "*.github.com", true},
		{5, "example.com:x", false},
		{6, "api.example.com:0", false},
		{7, "! bad.example.com", false},
		{8, "!!worse.example.com", false},
		{9, "a b", false},
		{10, "*", false},
		{11, "a*.example.com", false},
		{12, "[::1]", false},
		{13, "fe80::1%eth0", false},
		{14, "x/y", false},
		{15, "example.com:+22", false},
		{16, "*.10.0.0.1", false},
		{17, "a..example.com", false},
		{18, "10.0.0.0/8:8080", true},
		{19, "Example.ORG.", true},
		{21, "last.example.com", true},
	}
	if got := ParsePolicy(text); !slices.Equal(got, want) {
		t.Errorf("ParsePolicy =\n%v\nwant 0.1.0's\n%v", got, want)
	}
	p := writePolicy(t, `
# comment
example.com
*.github.com          # any depth, not the apex
github.com:22
!evil.github.com
10.1.2.3
192.168.0.0/16:8080
**:9999
`)
	for _, c := range []struct {
		host string
		port int
		want bool
	}{
		{"example.com", 443, true},
		{"EXAMPLE.com", 80, true},
		{"example.com.", 443, true},
		{"example.com", 8443, false},
		{"www.example.com", 443, false},
		{"api.github.com", 443, true},
		{"a.b.github.com", 443, true},
		{"github.com", 443, false},
		{"github.com", 22, true},
		{"evil.github.com", 443, false},
		{"10.1.2.3", 443, true},
		{"10.1.2.4", 443, false},
		{"192.168.5.5", 8080, true},
		{"192.168.5.5", 443, false},
		{"anything.test", 9999, true},
		{"anything.test", 443, false},
	} {
		if got, why := p.decide(c.host, c.port); got != c.want {
			t.Errorf("decide(%s, %d) = %v (%s), want 0.1.0's %v", c.host, c.port, got, why, c.want)
		}
		// None of 0.1.0's rules narrows, so no host's requests are judged one by one.
		if p.byRequest(c.host, c.port) {
			t.Errorf("%s:%d: its requests are judged one by one", c.host, c.port)
		}
	}
}
