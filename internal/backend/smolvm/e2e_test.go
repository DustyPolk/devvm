package smolvm

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/paths"
)

// TestE2EExecQuoting checks, on a real running machine, that what Exec
// runs reaches it as given: arguments with spaces, quotes, $, newlines and
// globs, and a folder with a space; and that it runs as the VM's user, with
// a login's environment. tests/e2e.sh runs it on a VM it made, named by
// DEVVM_E2E_VM, in its sandbox; otherwise it's skipped.
func TestE2EExecQuoting(t *testing.T) {
	vm := os.Getenv("DEVVM_E2E_VM")
	if vm == "" {
		t.Skip("tests/e2e.sh runs it, on a VM of its own")
	}
	d, err := paths.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	s := New(d, assets.FS, testUI{})
	args := []string{
		"a b", "  two  spaces ", `"double"`, "'single'", `it's`, "$HOME", "${HOME}", "$(id -u)", "`id -u`",
		"a\nb", "tab\there", "*", "?", "[ab]", "/*", "~", "~root", `back\slash`, `\n`, "", "-n", "--",
		";", "&&", "|", ">x", "<x", "#c", "!", "%s", "é ünï",
	}
	var out, errOut bytes.Buffer
	argv := append([]string{"printf", `%s\0`}, args...)
	if err := s.Exec(t.Context(), vm, argv, backend.ExecOpts{Stdout: &out, Stderr: &errOut}); err != nil {
		t.Fatalf("Exec: %v\n%s", err, errOut.String())
	}
	got := strings.Split(strings.TrimSuffix(out.String(), "\x00"), "\x00")
	if !slices.Equal(got, args) {
		t.Errorf("the guest got %q,\nwant %q", got, args)
	}

	dir := "/tmp/devvm e2e 'q' $x"
	if err := s.Exec(t.Context(), vm, []string{"mkdir", "-p", dir}, backend.ExecOpts{Stderr: &errOut}); err != nil {
		t.Fatalf("mkdir: %v\n%s", err, errOut.String())
	}
	out.Reset()
	if err := s.Exec(t.Context(), vm, []string{"pwd"}, backend.ExecOpts{Dir: dir, Stdout: &out, Stderr: &errOut}); err != nil {
		t.Fatalf("pwd: %v\n%s", err, errOut.String())
	}
	if got := strings.TrimSuffix(out.String(), "\n"); got != dir {
		t.Errorf("pwd in %q = %q", dir, got)
	}
	out.Reset()
	if err := s.Exec(t.Context(), vm, []string{"cat"}, backend.ExecOpts{Stdin: strings.NewReader("in\n$x\n"), Stdout: &out}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "in\n$x\n" {
		t.Errorf("cat = %q", out.String())
	}
	// The VM's user, its home, and /etc/environment's BASH_ENV.
	out.Reset()
	if err := s.Exec(t.Context(), vm, []string{"sh", "-c", `echo "$(id -un) $HOME $USER $BASH_ENV"`}, backend.ExecOpts{Stdout: &out}); err != nil {
		t.Fatal(err)
	}
	name, _, err := s.user()
	if err != nil {
		t.Fatal(err)
	}
	if want := name + " " + home(name) + " " + name + " /etc/profile.d/devvm-tools.sh\n"; out.String() != want {
		t.Errorf("as %q, want %q", out.String(), want)
	}
}
