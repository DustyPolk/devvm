package cli

import (
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
