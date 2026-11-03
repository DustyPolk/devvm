package smolvm

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/image"
)

// devvm's share of smolvm's cache: the disks smolvm unpacked of devvm's
// packs (a pack there, or one packsFile records), unchanged for an hour,
// that no machine of devvm's was made from. Never a disk of anyone
// else's pack, a recent one, a machine's folder or another cache.
func TestDevvmCache(t *testing.T) {
	s := newSandbox(t)
	s.Dirs.UserCache = filepath.Join(t.TempDir(), ".cache")
	seeds := filepath.Join(s.Dirs.UserCache, "smolvm", "vms", packSeeds)
	// Packs: a (no machine), b (a VM was made from it), and c, deleted
	// but recorded.
	for _, k := range []string{"aaaaaaaaaaaa", "bbbbbbbbbbbb"} {
		s.image(t, k)
		if err := os.WriteFile(s.packFile(k), []byte(k), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sum := func(k string) string {
		v, err := image.SHA256File(s.packFile(k))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	sa, sb, sc := sum("aaaaaaaaaaaa"), sum("bbbbbbbbbbbb"), "cc"+sum("aaaaaaaaaaaa")[2:]
	if err := os.WriteFile(packCache{s.Smolvm}.packsFile(), []byte("cccccccccccc "+sc+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.machine(t, "devvm-web-12345612345612", "running 4 8192 100 true")
	if err := s.writeRecord("devvm-web-12345612345612", record{Image: "bbbbbbbbbbbb"}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	seed := func(name string, recent bool) string {
		p := filepath.Join(seeds, name)
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "storage.raw"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p+".lock", nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if !recent {
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	free, deleted := seed("v2-"+sa+"-auto", false), seed("v2-"+sc+"-auto", false)
	seed("v2-"+sb+"-auto", false)                                                                                    // a VM's
	seed("v2-"+"dd"+sa[2:]+"-auto", false)                                                                           // someone else's pack
	seed("v2-"+sa[:62]+"ee-auto", true)                                                                              // not ours
	if err := os.MkdirAll(filepath.Join(s.Dirs.UserCache, "smolvm", "vms", "3d40063296df941a"), 0o700); err != nil { // a machine's
		t.Fatal(err)
	}
	got, err := s.DevvmCache(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var ps []string
	for _, e := range got {
		ps = append(ps, e.Path)
	}
	slices.Sort(ps)
	want := []string{free, deleted}
	slices.Sort(want)
	if !slices.Equal(ps, want) {
		t.Fatalf("DevvmCache = %v, want %v", ps, want)
	}
	if err := s.DeleteCache(t.Context(), got); err != nil {
		t.Fatal(err)
	}
	for _, p := range want {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s stayed", p)
		}
		if _, err := os.Stat(p + ".lock"); !os.IsNotExist(err) {
			t.Errorf("%s.lock stayed", p)
		}
	}
	if entries, _ := os.ReadDir(seeds); len(entries) != 6 { // 3 disks and their locks
		t.Errorf("left %d entries, want 6", len(entries))
	}
	// On Linux smolvm's caches are in devvm's own folder: none listed.
	s.OS = "linux"
	if got, _ := s.DevvmCache(t.Context()); len(got) != 0 {
		t.Errorf("Linux: %v", got)
	}
}

// Each new pack is recorded with its SHA-256.
func TestPackRecorded(t *testing.T) {
	s := newSandbox(t)
	s.image(t, "aaaaaaaaaaaa")
	if err := (packCache{s.Smolvm}).record("aaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	packs, err := packCache{s.Smolvm}.packs()
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 {
		t.Errorf("packs = %v", packs)
	}
}
