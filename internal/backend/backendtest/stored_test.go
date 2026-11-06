package backendtest

import (
	"slices"
	"testing"

	"github.com/devvmhq/devvm/internal/backend"
)

// A Stored reports its folders and its floors, the defaults unless it
// sets its own, and is still its fake.
func TestStored(t *testing.T) {
	f := New("fake", backend.Caps{})
	s := Stored{Fake: f, Dirs: []string{"/a", "/b"}}
	if got := s.DiskDirs(); !slices.Equal(got, []string{"/a", "/b"}) {
		t.Errorf("DiskDirs = %q", got)
	}
	if got := s.Floors(); got != DefaultFloors() || got.Base != 12<<30 || got.PerTool != 1<<30 {
		t.Errorf("Floors = %+v", got)
	}
	own := backend.Floors{Base: 1, PerTool: 2}
	if got := (Stored{Floor: own}).Floors(); got != own {
		t.Errorf("own Floors = %+v", got)
	}
	if s.Name() != "fake" {
		t.Errorf("Name = %q", s.Name())
	}
}
