package backend_test

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
)

// stored is a backend whose disks are in dirs, with half a GiB for each
// tool; nothing else of it is called.
func stored(dirs ...string) backendtest.Stored {
	f := backendtest.DefaultFloors()
	f.PerTool = 512 << 20
	return backendtest.Stored{Dirs: dirs, Floor: f}
}

// freeOf answers each folder's free space from a table, and errs for the
// rest.
func freeOf(free map[string]uint64, err error) func(string) (uint64, error) {
	return func(dir string) (uint64, error) {
		if n, ok := free[dir]; ok {
			return n, nil
		}
		return 0, err
	}
}

func imageOf(f backend.Floors) uint64 { return f.Image }

func TestSpaceCheck(t *testing.T) {
	const need = 8 << 30
	statErr := &fs.PathError{Op: "statfs", Path: "/gone", Err: fs.ErrPermission}
	b := stored("/lima")
	for _, tc := range []struct {
		name string
		b    backend.Backend
		need func(backend.Floors) uint64
		free func(string) (uint64, error)
		want string // the message's names, "" for none
	}{
		{"below", b, imageOf, freeOf(map[string]uint64{"/lima": need - 1}, nil), "disk_low dir=/lima free=7.9 name=node need=8"},
		{"at", b, imageOf, freeOf(map[string]uint64{"/lima": need}, nil), ""},
		{"above", b, imageOf, freeOf(map[string]uint64{"/lima": 100 << 30}, nil), ""},
		{
			"the lowest volume",
			stored("/a", "/b"),
			imageOf,
			freeOf(map[string]uint64{"/a": 50 << 30, "/b": 1 << 30}, nil),
			"disk_low dir=/b free=1.0 name=node need=8",
		},
		{
			"the backend's own floor", b, func(f backend.Floors) uint64 { return f.Base },
			freeOf(map[string]uint64{"/lima": need}, nil),
			"disk_low dir=/lima free=8.0 name=node need=12",
		},
		{
			// 3 GiB and 0.5 more for each of three tools, 4.5, is rounded up.
			"rounded up to whole GiB", b, func(f backend.Floors) uint64 { return f.ForTools(3) },
			freeOf(map[string]uint64{"/lima": 9 << 29}, nil),
			"disk_low dir=/lima free=4.5 name=node need=5",
		},
		{"statfs fails", b, imageOf, freeOf(nil, statErr), "disk_free_unknown dir=/lima"},
		{"a system without it", b, imageOf, freeOf(nil, errors.ErrUnsupported), ""},
		{"no free space func", b, imageOf, nil, ""},
		{"a backend whose disks aren't the host's", nil, imageOf, freeOf(nil, statErr), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := backend.Space{Free: tc.free}.Check(tc.b, "node", tc.need)
			if got := msg.NamesOf(err); got != tc.want {
				t.Errorf("Check = %q, want %q", got, tc.want)
			}
			if tc.name == "statfs fails" && !errors.Is(err, statErr) {
				t.Errorf("statfs's error isn't passed on: %v", err)
			}
		})
	}
}

func TestSpaceCheckDir(t *testing.T) {
	free := freeOf(map[string]uint64{"/data": 1<<30 - 1}, nil)
	if got := msg.NamesOf(backend.Space{Free: free}.CheckDir("/data", "node", backend.ToolImages(1))); got != "disk_low dir=/data free=0.9 name=node need=1" {
		t.Errorf("CheckDir = %q", got)
	}
	enough := freeOf(map[string]uint64{"/data": 1 << 30}, nil)
	if err := (backend.Space{Free: enough}).CheckDir("/data", "node", backend.ToolImages(1)); err != nil {
		t.Errorf("CheckDir with enough = %v", err)
	}
}

func TestForTools(t *testing.T) {
	f := backend.Floors{Tools: 3 << 30, PerTool: 512 << 20}
	if got := f.ForTools(10); got != 8<<30 {
		t.Errorf("ForTools(10) = %d", got)
	}
	if got := backend.ToolImages(10); got != 3<<30 {
		t.Errorf("ToolImages(10) = %d", got)
	}
}

func TestSpaceWarning(t *testing.T) {
	b := stored("/lima")
	for _, tc := range []struct {
		name string
		b    backend.Backend
		free func(string) (uint64, error)
		want string
	}{
		{"below base's", b, freeOf(map[string]uint64{"/lima": 11 << 30}, nil), "disk_low_warning dir=/lima free=11.0 need=12"},
		{"at base's", b, freeOf(map[string]uint64{"/lima": 12 << 30}, nil), ""},
		{"unreadable", b, freeOf(nil, fs.ErrPermission), ""},
		{"a backend whose disks aren't the host's", nil, freeOf(nil, nil), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, low := backend.Space{Free: tc.free}.Warning(tc.b)
			got := ""
			if low {
				got = m.Names()
			}
			if got != tc.want {
				t.Errorf("Warning = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGiB(t *testing.T) {
	for n, want := range map[uint64]string{0: "0.0", 1 << 30: "1.0", 12<<30 - 1: "11.9", 3<<29 + 1: "1.5"} {
		if got := backend.GiB(n); got != want {
			t.Errorf("GiB(%d) = %s, want %s", n, got, want)
		}
	}
}
