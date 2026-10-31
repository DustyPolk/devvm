package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
)

// The features table has a column per backend; a feature a backend
// doesn't list is none.
func TestWriteFeatures(t *testing.T) {
	a := backendtest.New("lima", backend.Caps{backend.FeatureSSH: backend.Native, backend.FeatureDisk: backend.Native})
	b := backendtest.New("vmx", backend.Caps{backend.FeatureSSH: backend.Emulated(true), backend.FeatureDisk: backend.Emulated(false)})
	var out strings.Builder
	if err := writeFeatures(&out, []backend.Backend{a, b}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != len(backend.Features)+1 {
		t.Fatalf("%d lines:\n%s", len(lines), out.String())
	}
	for _, want := range []string{"FEATURE LIMA VMX", "ssh native emulated", "disk native unreviewed", "cpus none none"} {
		found := false
		for _, l := range lines {
			found = found || strings.Join(strings.Fields(l), " ") == want
		}
		if !found {
			t.Errorf("no line %q in\n%s", want, out.String())
		}
	}
}

// doctor warns when the devvm first on PATH isn't this one, links
// resolved; not when it is, or when there's none on PATH.
func TestOnPathOther(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(dir, "lib", "devvm")
	brew := filepath.Join(dir, "brew", "devvm")
	link := filepath.Join(dir, "bin", "devvm")
	for _, f := range []string{self, brew} {
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(self, link); err != nil {
		t.Fatal(err)
	}
	found := func(p string) func(string) (string, error) {
		return func(string) (string, error) { return p, nil }
	}
	notFound := func(string) (string, error) { return "", exec.ErrNotFound }
	for _, c := range []struct {
		name     string
		lookPath func(string) (string, error)
		self     string
		want     string
	}{
		{"this one, through its link", found(link), self, ""},
		{"this one, run through its link", found(self), link, ""},
		{"another", found(brew), link, "devvm_other_first first=" + brew + " self=" + self},
		{"none on PATH", notFound, self, ""},
		{"a broken link on PATH", found(filepath.Join(dir, "gone")), self, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, ok := onPath{LookPath: c.lookPath, Executable: func() (string, error) { return c.self, nil }}.other()
			got := ""
			if ok {
				got = m.Names()
			}
			if got != c.want {
				t.Errorf("other: %q, want %q", got, c.want)
			}
		})
	}
}

// --fixes prints a KEY, FIX and message line for each problem.
func TestWriteFixes(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	var out strings.Builder
	err := writeFixes(&out, []backend.Problem{
		{Key: "lima", Fix: "brew install lima", Msg: msg.LimaMissingFix("brew install lima")},
		{Key: "kvm", Msg: msg.NoKVM()},
	})
	want := "lima\tbrew install lima\tlima_missing_fix fix=brew install lima\nkvm\t\tno_kvm\n"
	if err != nil || out.String() != want {
		t.Errorf("writeFixes: %q, %v; want %q", out.String(), err, want)
	}
}
