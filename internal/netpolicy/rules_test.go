package netpolicy

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/proxy"
)

func TestRule(t *testing.T) {
	for _, c := range []struct {
		in   string
		deny bool
		want string
	}{
		{"example.com", false, "example.com"},
		{"example.com", true, "!example.com"},
		{"!example.com", true, "!example.com"},
		{"[fd00::1]:22", true, "![fd00::1]:22"},
		{"!example.com", false, ""},
		{"a b", false, ""},
		{"!!x", true, ""},
	} {
		got, err := Rule(c.in, c.deny)
		if c.want == "" {
			if msg.IDOf(err) != "not_a_rule" {
				t.Errorf("Rule(%q, %v): %v; want not_a_rule", c.in, c.deny, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("Rule(%q, %v) = %q, %v; want %q", c.in, c.deny, got, err, c.want)
		}
	}
}

// Adding and removing keep the other lines, comments included, match a
// rule in any case and beside a comment, and write through a link.
func TestAddRemoveRule(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("# mine\nExample.com # site\nother.test"), 0o600); err != nil {
		t.Fatal(err)
	}
	content := func(p string) string {
		b, _ := os.ReadFile(p) //nolint:gosec // the test's own files
		return string(b)
	}
	path := filepath.Join(dir, "sub", "policy")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	step := func(name string, ok, want bool, err error) {
		t.Helper()
		if err != nil || ok != want {
			t.Fatalf("%s: %v, %v; want %v", name, ok, err, want)
		}
	}
	ok, err := AddRule(path, "example.com")
	step("add a rule there", ok, false, err)
	ok, err = AddRule(path, "!bad.test")
	step("add", ok, true, err)
	ok, err = RemoveRule(path, "EXAMPLE.com")
	step("remove", ok, true, err)
	ok, err = RemoveRule(path, "nothere.test")
	step("remove a rule not there", ok, false, err)
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v", err)
	}
	if got, want := content(target), "# mine\nother.test\n!bad.test\n"; got != want {
		t.Errorf("file = %q; want %q", got, want)
	}

	missing := filepath.Join(dir, "new", "policy")
	ok, err = RemoveRule(missing, "x.test")
	step("remove from a missing file", ok, false, err)
	ok, err = AddRule(missing, "x.test")
	step("add to a missing file", ok, true, err)
	if got := content(missing); got != "x.test\n" {
		t.Errorf("new file = %q", got)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ok, err = AddRule(empty, "x.test")
	step("add to an empty file", ok, true, err)
	if got := content(empty); got != "x.test\n" {
		t.Errorf("empty file = %q", got)
	}

	// rm takes every line with the rule, never a commented-out one; lines
	// that aren't rules and CRLF endings are kept as they are.
	mixed := filepath.Join(dir, "mixed")
	if err := os.WriteFile(mixed, []byte("# a.test\na.test\r\nexample.com:x # bad\r\nA.test # again\nb.test"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, err = AddRule(mixed, "c.test")
	step("add beside a line that isn't a rule", ok, true, err)
	ok, err = RemoveRule(mixed, "a.test")
	step("remove duplicates", ok, true, err)
	if got, want := content(mixed), "# a.test\nexample.com:x # bad\r\nb.test\nc.test\n"; got != want {
		t.Errorf("mixed file = %q; want %q", got, want)
	}
}

// WriteRules lists the built-in policies, by name and labelled as
// devvm's, then the files with rules, comments and blank lines left out,
// and marks each line that isn't a rule with its number; missing and
// comment-only files not at all. ReadRules reads the built-in ones from
// devvm's files alone, never from disk.
func TestWriteRules(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c")
	if err := os.WriteFile(a, []byte("# head\n\nx.test # why\n  !y.test\nexample.com:x\na\x1b[31mb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("# only a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	own := fstest.MapFS{"layers/base.policy": {Data: []byte("# devvm's\nbase.test\n")}, "shared-tools/none.policy": {Data: []byte("# none\n")}}
	files, err := ReadRules(own, []string{"layers/base.policy", "shared-tools/none.policy"}, []string{a, b, c})
	if err != nil || len(files) != 2 || files[0].Path != "layers/base.policy" || !files[0].Builtin ||
		files[1].Path != a || files[1].Builtin || strings.Join(files[1].Rules(), " ") != "x.test !y.test" {
		t.Fatalf("ReadRules = %+v, %v", files, err)
	}
	if bad := files[1].NotRules(); len(bad) != 2 || bad[0].N != 5 || bad[0].Text != "example.com:x" {
		t.Errorf("NotRules = %+v", bad)
	}
	var out strings.Builder
	if err := WriteRules(&out, files); err != nil {
		t.Fatal(err)
	}
	// A line's control characters are dropped.
	if want := "# devvm: layers/base.policy\n  base.test\n# " + a + "\n  x.test\n  !y.test\n  example.com:x" + notRuleMark + " (line 5)\n" +
		"  a[31mb" + notRuleMark + " (line 6)\n"; out.String() != want {
		t.Errorf("WriteRules = %q; want %q", out.String(), want)
	}
	// A built-in devvm's files lack is an error, whatever is on disk.
	if err := os.MkdirAll(filepath.Join(dir, "layers"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "layers", "gone.policy"), []byte("**\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, err := ReadRules(own, []string{"layers/gone.policy"}, nil); err == nil {
		t.Error("a missing built-in policy was read")
	}
}

// Every policy file devvm ships parses cleanly: each line a rule, a
// comment or nothing. Only the desktop layer's has no rules, as its
// packages come from base's hosts, and its comment says so.
func TestBuiltinPolicies(t *testing.T) {
	n := 0
	err := fs.WalkDir(assets.FS, ".", func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || filepath.Ext(path) != ".policy" {
			return err
		}
		n++
		b, err := fs.ReadFile(assets.FS, path)
		if err != nil {
			return err
		}
		lines := proxy.ParsePolicy(string(b))
		if len(lines) == 0 && (path != "layers/desktop.policy" || !strings.HasPrefix(string(b), "# desktop: ")) {
			t.Errorf("%s has no rules", path)
		}
		for _, l := range lines {
			if !l.OK {
				t.Errorf("%s:%d isn't a rule: %q", path, l.N, l.Text)
			}
		}
		return nil
	})
	if err != nil || n == 0 {
		t.Fatalf("%d policy files: %v", n, err)
	}
}

// NotRules warns of each line that isn't a rule in the policy files you
// edit: every VM's, each VM's own and your layers'; one that reads as a
// deny warns that its VMs' proxies deny everything.
func TestNotRules(t *testing.T) {
	root := t.TempDir()
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config")}
	files := map[string]string{
		d.Policy():                                "ok.test\na b\n",
		filepath.Join(d.Layers(), "pg.policy"):    "# pg\n*\n",
		d.VM("devvm-x-0123456789abcd").State():    `{"version": 1, "id": "x", "name": "devvm-x-0123456789abcd", "image": "base", "dir": "/x", "backend": "fake"}`,
		d.VM("devvm-x-0123456789abcd").Policy():   "!!x.test\nfine.test\n",
		filepath.Join(d.Layers(), "pg.sh"):        "not a policy\n",
		filepath.Join(d.VMs(), "other", "policy"): "!!not.a.vm\n",
	}
	for f, s := range files {
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, m := range NotRules(d) {
		got = append(got, m.Names())
	}
	want := []string{
		"policy_not_rule file=" + d.Policy() + " line=2 rule=a b",
		"policy_not_rule file=" + filepath.Join(d.Layers(), "pg.policy") + " line=2 rule=*",
		"policy_broken_deny file=" + d.VM("devvm-x-0123456789abcd").Policy() + " line=1 rule=!!x.test",
	}
	if !slices.Equal(got, want) {
		t.Errorf("NotRules =\n%q\nwant\n%q", got, want)
	}
}
