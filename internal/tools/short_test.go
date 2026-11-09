package tools

import (
	"encoding/binary"
	"path/filepath"
	"testing"
	"time"

	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/state"
)

// erofsImage is an EROFS image's start, its superblock saying it has
// blocks blocks of 4 KiB, cut to size bytes.
func erofsImage(blocks uint32, size int) []byte {
	b := make([]byte, max(size, erofsSuper+erofsBlocks+4))
	binary.LittleEndian.PutUint32(b[erofsSuper:], erofsMagic)
	b[erofsSuper+erofsBlkszBits] = 12
	binary.LittleEndian.PutUint32(b[erofsSuper+erofsBlocks:], blocks)
	return b[:size]
}

// A tool image cut short, as Linux's 9p client could leave one built into
// a mount with mmap, is empty or shorter than its superblock says; one that
// isn't EROFS's is left to the guest's mount.
func TestCutShort(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name  string
		data  []byte
		short bool
	}{
		{"whole", erofsImage(3, 3*4096), false},
		{"longer", erofsImage(3, 4*4096), false},
		{"empty", nil, true},
		{"cut to a page", erofsImage(3, 4096), true},
		{"not EROFS", append([]byte("image node 24\n"), make([]byte, 2000)...), false},
		{"a few bytes", []byte("image node 24\n"), true},
		{"ending in its superblock", erofsImage(1, erofsSuper+8), true},
		{"a bad block size", func() []byte { b := erofsImage(3, 4096); b[erofsSuper+erofsBlkszBits] = 40; return b }(), false},
	} {
		p := filepath.Join(dir, tc.name)
		write(t, p, tc.data)
		if got := cutShort(p); got != tc.short {
			t.Errorf("%s: cut short %v, want %v", tc.name, got, tc.short)
		}
	}
	if cutShort(filepath.Join(dir, "missing")) {
		t.Error("a missing file is cut short")
	}
}

// Built counts an EROFS tool image cut short as not built, so the next
// build makes it again; another backend's format isn't read.
func TestBuiltCutShort(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	s := Share{Env: &Env{Dirs: d}}
	file := "node-24.1.0-0123456789ab.erofs"
	for _, tc := range []struct {
		data   []byte
		format string
		built  bool
	}{
		{erofsImage(2, 2*4096), "", true},
		{erofsImage(2, 4096), "", false},
		{nil, "", false},
		{nil, "other", true},
	} {
		write(t, d.ToolImage(file), tc.data)
		if err := state.WriteToolImage(d, file, state.ToolImage{SHA256: "x", Libc: "gnu", Format: tc.format, Built: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if got := s.Built(file); got != tc.built {
			t.Errorf("%d bytes, format %q: built %v, want %v", len(tc.data), tc.format, got, tc.built)
		}
	}
}
