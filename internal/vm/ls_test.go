package vm

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// A folder is gone when nothing is at its path: a file there, or a VM
// that records no folder, isn't.
func TestFolderGone(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		dir: false, file: false, "": false, filepath.Join(dir, "gone"): true, filepath.Join(dir, "gone", "deeper"): true,
	} {
		if got := FolderGone(&state.VM{Dir: path}); got != want {
			t.Errorf("FolderGone(%q) = %v, want %v", path, got, want)
		}
	}
}

// ls lists project VMs on every backend, from their state; images, build
// VMs and a VM without state aside.
func TestList(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	const (
		web   = "devvm-web-0123456789abc4"
		img   = "devvm-img-foo-0123456789abcc" // a folder named img-foo
		clone = "devvm-cl-0123456789abc9"
		half  = "devvm-half-0123456789abce"
		other = "devvm-api-0123456789abc3"
	)
	// web's folder is there; the others' are gone.
	webDir := t.TempDir()
	for _, v := range []struct {
		name string
		kv   map[string]string
	}{
		{web, map[string]string{keyImage: "docker", keyToolNames: "node claude", keyDir: webDir, KeyNet: "on", keyGitProtect: "on"}},
		{img, map[string]string{keyDir: "/p/img-foo", keyFrom: "node:24", KeyNet: "off", keyGitProtect: "off"}},
		{clone, map[string]string{keyDir: "/p/cl", KeyNet: "on", keyCheck: "unenforced", KeyMode: ModeClone, keyGitProtect: gitprotect.Clone}},
		{other, map[string]string{keyToolNames: "go", keyDir: "/p/api", KeyNet: "off", keyGitProtect: "on"}},
	} {
		be := "lima"
		if v.name == other {
			be = "other"
		}
		vmState(t, d, v.name, func(st *state.VM) error {
			st.Backend = be
			for k, val := range v.kv {
				if err := setField(st, k, val); err != nil {
					return err
				}
			}
			return nil
		})
	}
	lima := backendtest.New("lima", nil)
	lima.Unconfined = true
	for _, in := range []backend.Instance{
		{Name: web, Status: backend.StatusRunning},
		{Name: img, Status: backend.StatusStopped},
		{Name: clone, Status: backend.StatusStopped},
		{Name: half, Status: backend.StatusStopped},
		{Name: "devvm-tools-build", Status: backend.StatusStopped},
	} {
		lima.Instances[in.Name] = in
	}
	oth := backendtest.New("other", nil)
	oth.Instances[other] = backend.Instance{Name: other, Status: backend.StatusRunning}
	reg, err := backend.NewRegistry(lima, oth)
	if err != nil {
		t.Fatal(err)
	}
	got, err := List(t.Context(), d, reg)
	if err != nil {
		t.Fatal(err)
	}
	want := []Listed{
		{Name: other, Status: "Running", Image: "base", Tools: []string{"go"}, Git: "on", Net: "off", Backend: "other", Folder: "/p/api", FolderGone: true},
		{Name: clone, Status: "Stopped", Image: "base", Git: "clone", Net: "unenforced", Backend: "lima", Folder: "/p/cl", FolderGone: true},
		{Name: half, Status: "Stopped", Backend: "lima"},
		{Name: img, Status: "Stopped", Image: "base", From: "node:24", Git: "off", Net: "off", Backend: "lima", Folder: "/p/img-foo", FolderGone: true},
		{Name: web, Status: "Running", Image: "docker", Tools: []string{"node", "claude"}, Git: "on", Net: "unenforced", Backend: "lima", Folder: webDir},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List =\n%+v\nwant\n%+v", got, want)
	}

	// A backend that can't list: skipped when no VM's state names it,
	// else the failure, its own check's first.
	broken := backendtest.New("broken", nil)
	broken.Errs = map[string]error{"List": errors.New("list failed")}
	reg, _ = backend.NewRegistry(broken)
	if got, err := List(t.Context(), d, reg); err != nil || got != nil {
		t.Errorf("unused broken backend: %v, %v", got, err)
	}
	vmState(t, d, "devvm-b-0123456789abcd", func(st *state.VM) error { st.Backend = "broken"; return nil })
	if _, err := List(t.Context(), d, reg); err == nil || err.Error() != "list failed" {
		t.Errorf("used broken backend: %v", err)
	}
	broken.Errs["Check"] = errors.New("not installed")
	if _, err := List(t.Context(), d, reg); err == nil || err.Error() != "not installed" {
		t.Errorf("used broken backend, check failing: %v", err)
	}
}
