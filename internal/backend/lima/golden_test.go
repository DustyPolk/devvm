package lima

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/stubtest"
)

// goldenFixtures returns the golden fixtures named like pattern, from
// every system's folder (testdata/golden/<os>), by their path: what real
// Lima and QEMU printed, captured by tests/e2e.sh --golden.
func goldenFixtures(t *testing.T, pattern string) map[string][]byte {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "testdata", "golden", "*", pattern))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no golden fixture %s: run tests/e2e.sh --golden", pattern)
	}
	out := map[string][]byte{}
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // the repo's own fixtures
		if err != nil {
			t.Fatal(err)
		}
		out[f] = b
	}
	return out
}

// shapes adds each value's JSON shape in data, a stream of values, to
// into: "path kind" for each, an array's elements under path[], and the
// keys of a map whose keys are data (env, param) as *.
func shapes(t *testing.T, data []byte, into map[string]bool) {
	t.Helper()
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch v := v.(type) {
		case map[string]any:
			into[path+" object"] = true
			for k, e := range v {
				if strings.HasSuffix(path, ".env") || strings.HasSuffix(path, ".param") {
					k = "*"
				}
				walk(path+"."+k, e)
			}
		case []any:
			into[path+" array"] = true
			for _, e := range v {
				walk(path+"[]", e)
			}
		case string:
			into[path+" string"] = true
		case float64:
			into[path+" number"] = true
		case bool:
			into[path+" bool"] = true
		case nil:
			into[path+" null"] = true
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var v any
		if err := dec.Decode(&v); errors.Is(err, io.EOF) {
			return
		} else if err != nil {
			t.Fatal(err)
		}
		walk("", v)
	}
}

// notIn returns the shapes of a that b lacks, sorted.
func notIn(a, b map[string]bool) []string {
	var out []string
	for s := range a {
		if !b[s] {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}

// A shape is each field's path and kind, an env's keys as one; a field
// of another kind, or one Lima lacks, is found. A fixture holds nothing
// of this machine's.
func TestShapes(t *testing.T) {
	golden, stub := map[string]bool{}, map[string]bool{}
	shapes(t, []byte(`{"cpus":4,"config":{"env":{"A":"x"},"mounts":[{"location":"/x"}]}}`+"\n"), golden)
	shapes(t, []byte(`{"cpus":"4","config":{"env":{"B":"y"},"mounts":[]},"extra":null}`), stub)
	if got, want := notIn(stub, golden), []string{".cpus string", ".extra null"}; !slices.Equal(got, want) {
		t.Errorf("notIn = %q, want %q", got, want)
	}
	got := anonymous(`{"dir":"/tmp/x/lima/a","user":{"name":"cj","comment":"C J","home":"/home/cj.guest"},"timezone":"Europe/Oslo","env":{"HTTPS_PROXY":"http://u:p@h"},"cjx":1}`, "/tmp/x", "cj")
	if want := `{"dir":"/home/me/lima/a","user":{"name":"me","comment":"me","home":"/home/me.guest"},"timezone":"UTC","env":{"NAME":"value"},"cjx":1}`; got != want {
		t.Errorf("anonymous = %s, want %s", got, want)
	}
}

// versionRE is limactl --version's output.
var versionRE = regexp.MustCompile(`^limactl version \d+\.\d+\.\d+\n$`)

// Real limactl --version's output reads as a version devvm accepts, and
// the stub's has its form.
func TestGoldenLimactlVersion(t *testing.T) {
	for f, out := range goldenFixtures(t, "limactl-version.txt") {
		l, fake, _ := testLima(t, "darwin")
		fake.out["limactl --version"] = string(out)
		if v := l.limaVersion(t.Context()); major(v) < 2 || !versionRE.Match(out) {
			t.Errorf("%s: version %q from %q", f, v, out)
		}
	}
	stubtest.New(t)
	out, err := exec.CommandContext(t.Context(), "limactl", "--version").Output()
	if err != nil || !versionRE.Match(out) {
		t.Errorf("the stub's --version: %q, %v", out, err)
	}
}

// Real limactl list --json's output, stopped dummy instances' and a
// running VM's, reads as devvm needs it, and the stub's has no field
// Lima's lacks, nor one of another kind: the stub, through the scenario
// devvm's own steps make, against every fixture's fields.
func TestGoldenLimactlList(t *testing.T) {
	golden := map[string]bool{}
	for f, out := range goldenFixtures(t, "limactl-list-*.json") {
		l, fake, _ := testLima(t, "darwin")
		fake.out["limactl list --json"] = string(out)
		ins, err := l.list(t.Context())
		if err != nil || len(ins) == 0 {
			t.Fatalf("%s: %v, %d instances", f, err, len(ins))
		}
		for _, in := range ins {
			if in.Name == "" || (in.Status != "Stopped" && in.Status != "Running") || in.CPUs <= 0 || in.Memory <= 0 || in.Disk <= 0 || in.SSHConfigFile == "" {
				t.Errorf("%s: %+v", f, in)
			}
		}
		shapes(t, out, golden)
	}
	// The stub's listing, as the scenario leaves the most instances.
	stubtest.New(t)
	l, fx := dummyLima(t, sandboxDirs(t), stubtest.ExecRunner{})
	_, list := play(t, l, fx)
	stub := map[string]bool{}
	shapes(t, list, stub)
	if extra := notIn(stub, golden); len(extra) > 0 {
		t.Errorf("the stub lists what Lima doesn't:\n\t%s", strings.Join(extra, "\n\t"))
	}
}

// Real qemu-img info --output=json's output reads as devvm needs it, and
// the stub's has no field it lacks, nor one of another kind.
func TestGoldenQemuImgInfo(t *testing.T) {
	golden := map[string]bool{}
	for f, out := range goldenFixtures(t, "qemu-img-info*.json") {
		l, fake, _ := testLima(t, "linux")
		fake.out["qemu-img info -U --output=json /d"] = string(out)
		if info, err := l.diskInfo(t.Context(), "/d"); err != nil || info.Format != "qcow2" {
			t.Errorf("%s: %+v, %v", f, info, err)
		}
		shapes(t, out, golden)
	}
	stubs := stubtest.New(t)
	plain, err := exec.CommandContext(t.Context(), "qemu-img", "info", "-U", "--output=json", "/l/vm/disk").Output()
	if err != nil {
		t.Fatal(err)
	}
	stubs.Write(t, "backing-vm", "/l/img/disk")
	backed, err := exec.CommandContext(t.Context(), "qemu-img", "info", "-U", "--output=json", "/l/vm/disk").Output()
	if err != nil {
		t.Fatal(err)
	}
	stub := map[string]bool{}
	shapes(t, plain, stub)
	if golden[".backing-filename string"] {
		shapes(t, backed, stub)
	} else {
		t.Log("no fixture of an overlay's disk: the stub's backing file isn't checked")
	}
	if extra := notIn(stub, golden); len(extra) > 0 {
		t.Errorf("the stub says what qemu-img doesn't:\n\t%s", strings.Join(extra, "\n\t"))
	}
}
