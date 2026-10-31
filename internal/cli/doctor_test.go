package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
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

// A machine below the recommended host gets a warning for each way it
// is: less memory, fewer CPUs. None when it isn't, or can't say.
func TestSmallHost(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, c := range []struct {
		host vm.Host
		err  error
		want []string
	}{
		{vm.Host{CPUs: 8, MemoryGiB: 16}, nil, nil},
		{vm.Host{CPUs: 4, MemoryGiB: 8}, nil, nil},
		{vm.Host{CPUs: 4, MemoryGiB: 7}, nil, []string{"host_memory_low have=7 want=8"}},
		{vm.Host{CPUs: 2, MemoryGiB: 8}, nil, []string{"host_cpus_few have=2 want=4"}},
		{vm.Host{CPUs: 1, MemoryGiB: 4}, nil, []string{"host_memory_low have=4 want=8", "host_cpus_few have=1 want=4"}},
		{vm.Host{}, errors.ErrUnsupported, nil},
	} {
		var got []string
		for _, m := range smallHost(func() (vm.Host, error) { return c.host, c.err }) {
			got = append(got, m.Names())
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("host %+v, %v: %q, want %q", c.host, c.err, got, c.want)
		}
	}
}

// doctor's warnings about a small host go to stderr and never change
// what it returns, and --fixes prints only the problems' lines either way.
func TestDoctorSmallHostWarnsOnly(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	notFound := func(string) (string, error) { return "", exec.ErrNotFound }
	for _, c := range []struct {
		name     string
		host     vm.Host
		problems []backend.Problem
		fixes    bool
		wantErr  string // the message doctor returns; "" for none
		wantOut  string // --fixes' output
		warned   bool   // whether stderr has the small host's warnings
	}{
		{"big, nothing missing", vm.Host{CPUs: 8, MemoryGiB: 16}, nil, false, "", "", false},
		{"small, nothing missing", vm.Host{CPUs: 2, MemoryGiB: 4}, nil, false, "", "", true},
		{"small, Lima missing", vm.Host{CPUs: 2, MemoryGiB: 4}, []backend.Problem{{Key: "lima", Msg: msg.LimaMissing()}}, false, "lima_missing", "", true},
		{"small, --fixes, nothing missing", vm.Host{CPUs: 2, MemoryGiB: 4}, nil, true, "", "", false},
		{"small, --fixes, Lima missing", vm.Host{CPUs: 2, MemoryGiB: 4}, []backend.Problem{{Key: "lima", Msg: msg.LimaMissing()}}, true, "", "lima\t\tlima_missing\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := backendtest.New("lima", backend.Caps{})
			b.Missing = c.problems
			u, out, errOut := testUI()
			d := doctor{
				Path: onPath{LookPath: notFound, Executable: os.Executable},
				Free: func(string) (uint64, error) { return 1 << 40, nil },
				Host: func() (vm.Host, error) { return c.host, nil },
			}
			err := d.run(t.Context(), u, []backend.Backend{b}, b, c.fixes)
			gotErr := ""
			if m, ok := errors.AsType[msg.M](err); ok {
				gotErr = m.Names()
			} else if err != nil {
				t.Fatal(err)
			}
			if gotErr != c.wantErr {
				t.Errorf("returned %q, want %q", gotErr, c.wantErr)
			}
			if c.fixes && out.String() != c.wantOut {
				t.Errorf("--fixes printed %q, want %q", out.String(), c.wantOut)
			}
			if warned := strings.Contains(errOut.String(), "devvm: host_cpus_few have=2 want=4\n") &&
				strings.Contains(errOut.String(), "devvm: host_memory_low have=4 want=8\n"); warned != c.warned {
				t.Errorf("stderr %q: warned %v, want %v", errOut.String(), warned, c.warned)
			}
			if !c.warned && !c.fixes && c.problems == nil && errOut.Len() > 0 {
				t.Errorf("stderr %q, want nothing", errOut.String())
			}
			if c.fixes && errOut.Len() > 0 {
				t.Errorf("--fixes: stderr %q, want nothing", errOut.String())
			}
		})
	}
}
