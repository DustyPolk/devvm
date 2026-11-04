package redact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	t.Parallel()
	key := strings.Repeat("0123456789abcdef", 2)
	for _, tc := range []struct{ in, want string }{
		{"plain line", "plain line"},
		{"http://devvm:" + key + "@host:3128", "http://devvm:<key>@host:3128"},
		{"HTTPS_PROXY=http://alice:s3cr%40t@proxy.corp:8080", "HTTPS_PROXY=http://alice:<password>@proxy.corp:8080"},
		{"value `http://bob:pw@127.0.0.1:3128` and \"https://c:d@e\"", "value `http://bob:<password>@127.0.0.1:3128` and \"https://c:<password>@e\""},
		{"no user https://host:443/a@b", "no user https://host:443/a@b"},
		{"user only http://alice@proxy", "user only http://alice@proxy"},
	} {
		if got := String(tc.in); got != tc.want {
			t.Errorf("String(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// File overwrites each secret with as many stars in place, so the file keeps its length, and a
// writer that still has it open at its own offset, as Lima's hostagent does, goes on where it was.
func TestFile(t *testing.T) {
	t.Parallel()
	pass := "e2e-not-a-secret" //nolint:gosec // a fake password
	key := strings.Repeat("fedcba9876543210", 4)
	path := filepath.Join(t.TempDir(), "ha.stderr.log")
	before := `level=info msg="Replacing \"HTTPS_PROXY\" value ` + "`http://alice:" + pass + "@127.0.0.1:3128`" + `"` + "\n" +
		"proxy http://devvm:" + key + "@192.168.5.2:3129 kept\nno user https://host:443/a@b\n"
	w, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	if _, err := w.WriteString(before); err != nil {
		t.Fatal(err)
	}
	if err := File(path); err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("later\n"); err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(pass, strings.Repeat("*", len(pass)), key, strings.Repeat("*", len(key))).Replace(before) + "later\n"
	b, err := os.ReadFile(path) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != want {
		t.Errorf("got  %q\nwant %q", b, want)
	}
	// Once more changes nothing; a file without secrets, or none, is left alone.
	if err := File(path); err != nil {
		t.Fatal(err)
	}
	if b2, _ := os.ReadFile(path); string(b2) != want { //nolint:gosec // the test's own file
		t.Errorf("again: %q", b2)
	}
	if err := File(filepath.Join(t.TempDir(), "missing.log")); err != nil {
		t.Errorf("a missing file: %v", err)
	}
	if err := File(t.TempDir()); err == nil {
		t.Error("a folder: no error")
	}
}
