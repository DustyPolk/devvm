package vm

import (
	"errors"
	"reflect"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// ls lists project VMs on every backend, from their state; images, build
// VMs and a VM without state aside.
func TestList(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	d := paths.Dirs{Data: t.TempDir()}
	const (
		web   = "devvm-web-0123456789abcd"
		img   = "devvm-img-foo-0123456789abcd" // a folder named img-foo
		clone = "devvm-cl-0123456789abcd"
		half  = "devvm-half-0123456789abcd"
		other = "devvm-api-0123456789abcd"
	)
	for _, v := range []struct {
		name string
		n    state.NewVM
		kv   map[string]string
	}{
		{
			web,
			state.NewVM{Image: "docker", Tools: []string{"node", "claude"}, Dir: "/p/web", Backend: "lima"},
			map[string]string{netpolicy.KeyNet: "on", gitprotect.KeyGitProtect: "on"},
		},
		{
			img,
			state.NewVM{Image: "base", Dir: "/p/img-foo", From: "node:24", Backend: "lima"},
			map[string]string{netpolicy.KeyNet: "off", gitprotect.KeyGitProtect: "off"},
		},
		{
			clone,
			state.NewVM{Image: "base", Dir: "/p/cl", Backend: "lima"},
			map[string]string{netpolicy.KeyNet: "on", netpolicy.KeyCheck: "unenforced", KeyMode: ModeClone},
		},
		{
			other,
			state.NewVM{Image: "base", Tools: []string{"go"}, Dir: "/p/api", Backend: "other"},
			map[string]string{netpolicy.KeyNet: "off", gitprotect.KeyGitProtect: "on"},
		},
	} {
		st, err := state.CreateVM(d, v.name, v.n)
		if err != nil {
			t.Fatal(err)
		}
		for k, val := range v.kv {
			if err := st.Set(k, val); err != nil {
				t.Fatal(err)
			}
		}
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
		{Name: other, Status: "Running", Image: "base", Tools: []string{"go"}, Git: "on", Net: "off", Backend: "other", Folder: "/p/api"},
		{Name: clone, Status: "Stopped", Image: "base", Git: "clone", Net: "unenforced", Backend: "lima", Folder: "/p/cl"},
		{Name: half, Status: "Stopped", Backend: "lima"},
		{Name: img, Status: "Stopped", Image: "base", From: "node:24", Git: "off", Net: "off", Backend: "lima", Folder: "/p/img-foo"},
		{Name: web, Status: "Running", Image: "docker", Tools: []string{"node", "claude"}, Git: "on", Net: "unenforced", Backend: "lima", Folder: "/p/web"},
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
	if _, err := state.CreateVM(d, "devvm-b-0123456789abcd", state.NewVM{Backend: "broken"}); err != nil {
		t.Fatal(err)
	}
	if _, err := List(t.Context(), d, reg); err == nil || err.Error() != "list failed" {
		t.Errorf("used broken backend: %v", err)
	}
	broken.Errs["Check"] = errors.New("not installed")
	if _, err := List(t.Context(), d, reg); err == nil || err.Error() != "not installed" {
		t.Errorf("used broken backend, check failing: %v", err)
	}
}
