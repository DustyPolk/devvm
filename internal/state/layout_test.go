package state

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

// This devvm has a migration from each layout version before its own.
func TestMigrations(t *testing.T) {
	if n := len(migrations()); n != Layout-1 {
		t.Errorf("%d migrations for layout version %d", n, Layout)
	}
}

// Folders without version files get this devvm's; the same version goes
// on; an older one is migrated, once, then written; a newer one in
// either folder is refused, naming its file, and nothing is written.
func TestCheckLayout(t *testing.T) {
	tests := []struct {
		name         string
		data, config string // the version files, "" for none
		current      int
		wantRun      int    // the migrations run
		wantErr      string // the message's names, or "corrupt"
		want         string // both files after
	}{
		{name: "new folders", current: 1, want: "1\n"},
		{name: "the same version", data: "1\n", config: "1\n", current: 1, want: "1\n"},
		{name: "config folder without one", data: "1\n", current: 1, want: "1\n"},
		{name: "older", data: "1\n", config: "1\n", current: 2, wantRun: 1, want: "2\n"},
		{name: "older config folder", data: "2\n", config: "1\n", current: 2, wantRun: 1, want: "2\n"},
		{name: "older without a config version", data: "1\n", current: 3, wantRun: 2, want: "3\n"},
		{name: "newer data folder", data: "2\n", config: "1\n", current: 1, wantErr: "layout_newer current=1 path=DATA version=2"},
		{name: "newer config folder", data: "1\n", config: "3\n", current: 2, wantErr: "layout_newer current=2 path=CONFIG version=3"},
		{name: "not a version", data: "one\n", current: 1, wantErr: "corrupt"},
		{name: "zero", data: "0\n", current: 1, wantErr: "corrupt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDirs(t)
			for f, v := range map[string]string{d.Version(): tt.data, d.ConfigVersion(): tt.config} {
				if v != "" {
					write(t, f, v)
				}
			}
			var ran []int
			var ms []Migration
			for i := range tt.current - 1 {
				ms = append(ms, func(paths.Dirs) error { ran = append(ran, i+1); return nil })
			}
			err := checkLayout(t.Context(), d, tt.current, ms)
			if tt.wantErr == "corrupt" {
				if _, ok := errors.AsType[*CorruptError](err); !ok {
					t.Fatalf("checkLayout = %v, want a CorruptError", err)
				}
				return
			}
			if tt.wantErr != "" {
				m, ok := errors.AsType[msg.M](err)
				if want := replacePaths(tt.wantErr, d); !ok || m.Names() != want {
					t.Fatalf("checkLayout = %v, want %s", err, want)
				}
				if got := read(t, d.Version()); got != tt.data {
					t.Errorf("a refused check wrote the data folder's version: %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(ran) != tt.wantRun {
				t.Errorf("ran migrations %v, want %d", ran, tt.wantRun)
			}
			if !slices.IsSorted(ran) || (len(ran) > 0 && ran[len(ran)-1] != tt.current-1) {
				t.Errorf("ran migrations %v, want the last ones in order", ran)
			}
			for _, f := range []string{d.Version(), d.ConfigVersion()} {
				if got := read(t, f); got != tt.want {
					t.Errorf("%s = %q, want %q", f, got, tt.want)
				}
			}
		})
	}
}

// replacePaths puts d's version files for DATA and CONFIG in names.
func replacePaths(names string, d paths.Dirs) string {
	return strings.NewReplacer("path=DATA", "path="+d.Version(), "path=CONFIG", "path="+d.ConfigVersion()).Replace(names)
}

// A failed migration fails the check, and the versions stay as they were,
// so the next run migrates again.
func TestCheckLayoutMigrationFails(t *testing.T) {
	d := testDirs(t)
	write(t, d.Version(), "1\n")
	failed := errors.New("failed")
	if err := checkLayout(t.Context(), d, 2, []Migration{func(paths.Dirs) error { return failed }}); !errors.Is(err, failed) {
		t.Fatalf("checkLayout = %v, want the migration's error", err)
	}
	if got := read(t, d.Version()); got != "1\n" {
		t.Errorf("version = %q, want 1", got)
	}
	if err := checkLayout(t.Context(), d, 2, nil); err == nil {
		t.Error("a layout without its migrations was checked")
	}
}

// doctor's findings in the VMs' folder: what isn't a VM's folder, and a
// VM's folder without vm.json; a VM's folder with one is fine.
func TestVMFolderProblems(t *testing.T) {
	d := testDirs(t)
	if ms, err := VMFolderProblems(d); err != nil || len(ms) != 0 {
		t.Fatalf("no VMs' folder: %v, %v", ms, err)
	}
	const good, lockOnly = "0123456789abcd", "0123456789abce"
	write(t, d.VM("devvm-a-"+good).State(), "{}")
	write(t, d.VM("devvm-b-"+lockOnly).Lock(), "")
	write(t, filepath.Join(d.VMs(), "notes.txt"), "x")
	if err := os.MkdirAll(filepath.Join(d.VMs(), "devvm-old"), 0o700); err != nil {
		t.Fatal(err)
	}
	ms, err := VMFolderProblems(d)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(ms))
	for i, m := range ms {
		got[i] = m.Names()
	}
	want := []string{
		"vm_folder_stateless path=" + d.VMDir(lockOnly),
		"vms_stray path=" + filepath.Join(d.VMs(), "devvm-old") + " vms=" + d.VMs(),
		"vms_stray path=" + filepath.Join(d.VMs(), "notes.txt") + " vms=" + d.VMs(),
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("VMFolderProblems = %q, want %q", got, want)
	}
}
