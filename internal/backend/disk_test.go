package backend

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

// stored is a backend whose disks are in dirs; nothing else of it is
// called.
type stored struct {
	Backend
	dirs []string
}

func (s stored) DiskDirs() []string { return s.dirs }

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

func TestSpaceCheck(t *testing.T) {
	const need = 8 << 30
	statErr := &fs.PathError{Op: "statfs", Path: "/gone", Err: fs.ErrPermission}
	b := stored{dirs: []string{"/lima"}}
	for _, tc := range []struct {
		name  string
		b     Backend
		free  func(string) (uint64, error)
		extra []string
		want  string // the message's names, "" for none
	}{
		{"below", b, freeOf(map[string]uint64{"/lima": need - 1}, nil), nil, "disk_low dir=/lima free=7.9 name=node need=8"},
		{"at", b, freeOf(map[string]uint64{"/lima": need}, nil), nil, ""},
		{"above", b, freeOf(map[string]uint64{"/lima": 100 << 30}, nil), nil, ""},
		{
			"the lowest volume",
			stored{dirs: []string{"/a", "/b"}},
			freeOf(map[string]uint64{"/a": 50 << 30, "/b": 1 << 30}, nil), nil,
			"disk_low dir=/b free=1.0 name=node need=8",
		},
		{
			"an extra folder", b, freeOf(map[string]uint64{"/lima": 50 << 30, "/data": 2 << 29}, nil),
			[]string{"/data"},
			"disk_low dir=/data free=1.0 name=node need=8",
		},
		{"statfs fails", b, freeOf(nil, statErr), nil, "disk_free_unknown dir=/lima"},
		{"a system without it", b, freeOf(nil, errors.ErrUnsupported), nil, ""},
		{"no free space func", b, nil, nil, ""},
		{"a backend whose disks aren't the host's", nil, freeOf(nil, statErr), nil, ""},
		{
			"not the host's, with an extra folder", nil, freeOf(map[string]uint64{"/data": 0}, nil),
			[]string{"/data"},
			"disk_low dir=/data free=0.0 name=node need=8",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Space{Free: tc.free}.Check(tc.b, "node", need, tc.extra...)
			got := ""
			if err != nil {
				m, ok := errors.AsType[msg.M](err)
				if !ok {
					t.Fatalf("not a message: %v", err)
				}
				got = m.Names()
			}
			if got != tc.want {
				t.Errorf("Check = %q, want %q", got, tc.want)
			}
			if tc.name == "statfs fails" && !errors.Is(err, statErr) {
				t.Errorf("statfs's error isn't passed on: %v", err)
			}
		})
	}
}

func TestSpaceWarning(t *testing.T) {
	b := stored{dirs: []string{"/lima"}}
	for _, tc := range []struct {
		name string
		free func(string) (uint64, error)
		want string
	}{
		{"below base's", freeOf(map[string]uint64{"/lima": FreeBase - 1<<30}, nil), "disk_low_warning dir=/lima free=11.0 need=12"},
		{"at base's", freeOf(map[string]uint64{"/lima": FreeBase}, nil), ""},
		{"unreadable", freeOf(nil, fs.ErrPermission), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, low := Space{Free: tc.free}.Warning(b)
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
		if got := GiB(n); got != want {
			t.Errorf("GiB(%d) = %s, want %s", n, got, want)
		}
	}
}
