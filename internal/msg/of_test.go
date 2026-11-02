package msg

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
)

// IDOf and NamesOf find the message an error is or wraps; an error
// without one has no ID and shows its own text.
func TestIDOfNamesOf(t *testing.T) {
	m := NoVM("/x")
	for _, tc := range []struct {
		name      string
		err       error
		id, names string
	}{
		{"nil", nil, "", ""},
		{"a message", m, "no_vm", "no_vm dir=/x"},
		{"a wrapped message", fmt.Errorf("up: %w", m), "no_vm", "no_vm dir=/x"},
		{"a message with a cause", m.Because(errors.New("gone")), "no_vm", "no_vm dir=/x"},
		{"another error", errors.New("boom"), "", "boom"},
	} {
		if got := IDOf(tc.err); got != tc.id {
			t.Errorf("%s: IDOf = %q, want %q", tc.name, got, tc.id)
		}
		if got := NamesOf(tc.err); got != tc.names {
			t.Errorf("%s: NamesOf = %q, want %q", tc.name, got, tc.names)
		}
	}
}

// A Recorder keeps what it's given in order, from any goroutine, until
// it's reset.
func TestRecorder(t *testing.T) {
	var r Recorder
	if r.Names() != nil || r.IDs() != nil || len(r.Messages()) != 0 {
		t.Fatalf("empty recorder has %q", r.Names())
	}
	warn := r.Add
	warn(NoVM("/a"))
	warn(HomeNotAbsolute())
	if got, want := r.Names(), []string{"no_vm dir=/a", "home_not_absolute"}; !slices.Equal(got, want) {
		t.Errorf("Names = %q, want %q", got, want)
	}
	if got, want := r.IDs(), []string{"no_vm", "home_not_absolute"}; !slices.Equal(got, want) {
		t.Errorf("IDs = %q, want %q", got, want)
	}
	ms := r.Messages()
	ms[0] = HomeNotAbsolute()
	if r.IDs()[0] != "no_vm" {
		t.Error("Messages shares the recorder's own slice")
	}
	r.Reset()
	if r.Names() != nil {
		t.Errorf("after Reset: %q", r.Names())
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { r.Add(NoVM("/b")) })
	}
	wg.Wait()
	if n := len(r.Messages()); n != 20 {
		t.Errorf("kept %d of 20 concurrent messages", n)
	}
}
