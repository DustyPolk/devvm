package netpolicy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
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
			if m, ok := errors.AsType[msg.M](err); !ok || m.ID() != "not_a_rule" {
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
}

// WriteRules lists the files with rules, comments and blank lines left
// out; missing and comment-only files not at all.
func TestWriteRules(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c")
	if err := os.WriteFile(a, []byte("# head\n\nx.test # why\n  !y.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("# only a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := ReadRules([]string{a, b, c})
	if err != nil || len(files) != 1 || files[0].Path != a || strings.Join(files[0].Rules, " ") != "x.test !y.test" {
		t.Fatalf("ReadRules = %+v, %v", files, err)
	}
	var out strings.Builder
	if err := WriteRules(&out, files); err != nil {
		t.Fatal(err)
	}
	if want := "# " + a + "\n  x.test\n  !y.test\n"; out.String() != want {
		t.Errorf("WriteRules = %q; want %q", out.String(), want)
	}
}
