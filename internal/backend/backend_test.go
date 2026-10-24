package backend

import (
	"errors"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

// A logged failure is still its message, for errors.As and printing.
func TestLoggedError(t *testing.T) {
	m := msg.StartFailed("v")
	err := Logged(m)
	if le, ok := errors.AsType[*LoggedError](err); !ok || le.Msg.ID() != "start_failed" {
		t.Errorf("as a LoggedError: %v", err)
	}
	if got, ok := errors.AsType[msg.M](err); !ok || got.ID() != m.ID() || err.Error() != m.Error() {
		t.Errorf("as its message: %v", err)
	}
}
