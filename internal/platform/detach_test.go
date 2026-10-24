//go:build unix

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A detached process leads a session of its own, gets no input and writes
// to the file it's given.
func TestStartDetached(t *testing.T) {
	out, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	pid, err := StartDetached("/bin/sh", []string{"-c", "read x; echo ran $?; exec sleep 30"}, out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	var got string
	for range 100 {
		data, _ := os.ReadFile(out.Name())
		if got = string(data); strings.Contains(got, "ran") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got != "ran 1\n" {
		t.Errorf("output = %q, want the read to fail on no input", got)
	}
	sid, err := unix.Getsid(pid)
	if err != nil {
		t.Fatal(err)
	}
	if sid != pid {
		t.Errorf("session = %d, want its own (%d)", sid, pid)
	}
	if own, _ := unix.Getsid(0); own == sid {
		t.Error("it shares the test's session")
	}
}
