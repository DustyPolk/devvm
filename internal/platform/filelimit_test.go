//go:build unix

package platform

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// fileLimitChild, set in the environment, makes TestShareFileLimit the
// child it starts: started with a soft limit of 256 open files (macOS's
// default), it shares its own limit and logs the one its child gets.
const fileLimitChild = "DEVVM_PLATFORM_TEST_FILE_LIMIT_CHILD"

// A process started with a low soft limit, which Go raises for itself,
// gives the processes it starts its raised limit once it shares it: an ssh
// master carrying 256 connections needs more than 256 descriptors.
func TestShareFileLimit(t *testing.T) {
	if os.Getenv(fileLimitChild) != "" {
		if err := ShareFileLimit(); err != nil {
			t.Fatal(err)
		}
		out, err := exec.CommandContext(t.Context(), "sh", "-c", "ulimit -S -n").Output()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("child-limit=%s", out)
		return
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "sh", "-c", //nolint:gosec // the test binary itself
		`ulimit -S -n 256 && exec "$0" -test.v -test.run='^TestShareFileLimit$'`, self)
	cmd.Env = append(os.Environ(), fileLimitChild+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	_, after, ok := strings.Cut(string(out), "child-limit=")
	if !ok {
		t.Fatalf("no limit printed: %s", out)
	}
	first, _, _ := strings.Cut(after, "\n")
	n, err := strconv.Atoi(strings.TrimSpace(first))
	if first == "unlimited" {
		n, err = 1<<31-1, nil
	}
	if err != nil || n <= 256 {
		t.Errorf("the child's soft limit is %q, want more than 256", first)
	}
}
