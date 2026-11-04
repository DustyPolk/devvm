package state

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"
)

// The pinned cloud image reads back as written; none pinned is
// fs.ErrNotExist; a pin whose fields aren't a pin's is neither written nor
// read, as they go into base's key and Lima's config.
func TestUbuntuPin(t *testing.T) {
	d := testDirs(t)
	if _, err := ReadUbuntuPin(d); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("none pinned: %v", err)
	}
	want := UbuntuPin{
		Release: "release-20261016", File: "ubuntu-26.04-server-cloudimg-arm64.img",
		Digest: "sha256:" + strings.Repeat("a", 64), Pinned: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC),
	}
	if err := WriteUbuntuPin(d, want); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadUbuntuPin(d); err != nil || got != want {
		t.Errorf("got %+v, %v; want %+v", got, err, want)
	}
	for name, change := range map[string]func(*UbuntuPin){
		"a release that isn't dated": func(p *UbuntuPin) { p.Release = "current" },
		"a path for a file":          func(p *UbuntuPin) { p.File = "../ubuntu-26.04-server-cloudimg-arm64.img" },
		"a digest with more":         func(p *UbuntuPin) { p.Digest += `"}]` },
	} {
		p := want
		change(&p)
		var corrupt *CorruptError
		if err := WriteUbuntuPin(d, p); !errors.As(err, &corrupt) {
			t.Errorf("%s: written, %v", name, err)
		}
		data := `{"release": "` + p.Release + `", "file": "` + p.File + `", "digest": "sha256:x"}`
		if err := os.WriteFile(d.UbuntuPin(), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadUbuntuPin(d); !errors.As(err, &corrupt) {
			t.Errorf("%s: read, %v", name, err)
		}
	}
}
