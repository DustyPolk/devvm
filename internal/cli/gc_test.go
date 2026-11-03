package cli

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/vm"
)

// An image the backend refuses to delete with a message (on Linux hosts,
// one whose disk VMs use) stays, with its record, and gc goes on; any
// other failure ends it.
func TestDeleteStaleKeepsImagesInUse(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	dir := t.TempDir()
	d := paths.Dirs{Data: filepath.Join(dir, "data"), Config: filepath.Join(dir, "config"), Lima: filepath.Join(dir, "lima")}
	f := backendtest.New("fake", backend.Caps{})
	f.ImageKeys = []string{"0123456789ab", "ba9876543210"}
	if err := state.WriteImage(d, "0123456789ab", state.Image{Name: "base"}); err != nil {
		t.Fatal(err)
	}
	b := build.Images{Env: &build.Env{Dirs: d, Backend: f}}
	imgs := []listedImage{
		{Listed: build.Listed{Image: build.Image{Key: "0123456789ab", Name: "base"}}, images: b},
		{Listed: build.Listed{Image: build.Image{Key: "ba9876543210"}}, images: b},
	}
	f.Errs = map[string]error{"DeleteImage": msg.ImageInUse("0123456789ab", "devvm-p-0123456789abcd")}
	u, _, errOut := testUI()
	if err := deleteStale(t.Context(), u, imgs); err != nil {
		t.Fatal(err)
	}
	if got := f.Methods(); !slices.Equal(got, []string{"DeleteImage", "DeleteImage"}) {
		t.Errorf("calls: %v", got)
	}
	if !strings.HasPrefix(errOut.String(), "devvm: image_in_use ") {
		t.Errorf("stderr %q, want image_in_use", errOut)
	}
	if _, err := state.ReadImage(d, "0123456789ab"); err != nil {
		t.Errorf("the image in use lost its record: %v", err)
	}
	boom := errors.New("boom")
	f.Errs = map[string]error{"DeleteImage": boom}
	if err := deleteStale(t.Context(), u, imgs); !errors.Is(err, boom) {
		t.Errorf("deleteStale = %v, want %v", err, boom)
	}
}

// A skipped backend's orphans aren't forgotten: its VMs' state stays, and
// the backend is named once among those that couldn't be asked.
func TestWithoutSkipped(t *testing.T) {
	r := vm.LeftoverReport{
		Orphans: []vm.Orphan{{Name: "a", Backend: "lima"}, {Name: "b", Backend: "qemu"}, {Name: "c", Backend: "qemu"}, {Name: "d", Backend: "fake"}},
		Unknown: []string{"fake"},
	}
	got := withoutSkipped(r, map[string]bool{"qemu": true, "fake": true})
	if len(got.Orphans) != 1 || got.Orphans[0].Name != "a" {
		t.Errorf("orphans %+v, want a alone", got.Orphans)
	}
	if !slices.Equal(got.Unknown, []string{"fake", "qemu"}) {
		t.Errorf("unknown %v", got.Unknown)
	}
	if got := withoutSkipped(r, nil); len(got.Orphans) != 4 || len(got.Unknown) != 1 {
		t.Errorf("nothing skipped: %+v", got)
	}
}

// A destructive command asks before it deletes only in a terminal, never
// with --force, and without a terminal refuses unless --force.
func TestConfirmed(t *testing.T) {
	u, _, _ := testUI()
	asked := false
	ask := func() bool { asked = true; return true }
	if ok, err := confirmed(u, true, "devvm gc", ask); !ok || err != nil || asked {
		t.Errorf("--force: %v, %v, asked %v", ok, err, asked)
	}
	ok, err := confirmed(u, false, "devvm gc", ask)
	if ok || msg.NamesOf(err) != "force_needed command=devvm gc" || asked {
		t.Errorf("no terminal: %v, %v, asked %v", ok, err, asked)
	}
}
