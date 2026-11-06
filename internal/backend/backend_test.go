package backend

import (
	"errors"
	"testing"

	"github.com/devvmhq/devvm/internal/msg"
)

// A start refuses only a disk smaller than the VM's: zero is the VM's
// own, and the rest changes.
func TestValidate(t *testing.T) {
	in := Instance{Name: "v", Size: Size{CPUs: 4, MemoryGiB: 8, DiskGiB: 100}}
	for _, tc := range []struct {
		want Want
		err  string
	}{
		{Want{}, ""},
		{Want{Size: Size{CPUs: 1, MemoryGiB: 2, DiskGiB: 100}, AgentForwarding: true}, ""},
		{Want{Size: Size{DiskGiB: 120.5}}, ""},
		{Want{Size: Size{DiskGiB: 99.5}}, "disk_shrink have=100 vm=v want=99.5"},
	} {
		if got := msg.NamesOf(Validate(tc.want, in)); got != tc.err {
			t.Errorf("Validate(%+v) = %q, want %q", tc.want, got, tc.err)
		}
	}
}

// A logged failure is still its message, for errors.As and printing.
func TestLoggedError(t *testing.T) {
	m := msg.StartFailed("v")
	err := Logged(m)
	if le, ok := errors.AsType[*LoggedError](err); !ok || le.Msg.ID() != "start_failed" {
		t.Errorf("as a LoggedError: %v", err)
	}
	if msg.IDOf(err) != m.ID() || err.Error() != m.Error() {
		t.Errorf("as its message: %v", err)
	}
}
