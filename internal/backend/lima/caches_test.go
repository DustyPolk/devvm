package lima

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// devvm's share of Lima's download cache: Ubuntu's cloud images from
// devvm's address that no image record names, unchanged for an hour. Never
// another download (yours), one a record names, one being written, or an
// entry whose url doesn't hash to its name.
func TestDevvmCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	dir, err := downloadCache()
	if err != nil {
		t.Fatal(err)
	}
	d := paths.Dirs{Data: filepath.Join(home, "data"), Lima: filepath.Join(home, "lima")}
	l := &Lima{Dirs: d, Getenv: func(string) string { return "" }}
	cloud := func(date string) string {
		return ubuntu.DefaultURL + "/release-" + date + "/ubuntu-" + ubuntu.Version + "-server-cloudimg-arm64.img"
	}
	old := time.Now().Add(-2 * time.Hour)
	entry := func(url, name string, recent bool) string {
		if name == "" {
			sum := sha256.Sum256([]byte(url))
			name = hex.EncodeToString(sum[:])
		}
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		for f, data := range map[string]string{"url": url, "data": "x"} {
			if err := os.WriteFile(filepath.Join(p, f), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if !recent {
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	stale := entry(cloud("20260101"), "", false)
	kept := entry(cloud("20260201"), "", false)
	entry(cloud("20260301"), "", true)                                // a build downloading it
	entry("https://example.com/yours.img", "", false)                 // your own download
	entry(cloud("20260401"), "0123456789abcdef", false)               // not Lima's layout
	entry(ubuntu.DefaultURL+"/release-20260101/other.img", "", false) // not a cloud image of devvm's
	if err := state.WriteImage(d, "0123456789ab", state.Image{Name: "base", Digest: cloud("20260201") + " sha256:" + hex.EncodeToString(make([]byte, 32))}); err != nil {
		t.Fatal(err)
	}
	got, err := l.DevvmCache(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, e := range got {
		paths = append(paths, e.Path)
	}
	if !slices.Equal(paths, []string{stale}) {
		t.Fatalf("DevvmCache = %v, want only %s", paths, stale)
	}
	if err := l.DeleteCache(t.Context(), got); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("the stale entry stayed: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the recorded entry went: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 5 {
		t.Errorf("left %d entries, want 5", len(entries))
	}
}
