package state

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// A package set reads back as written, is listed by its key, and goes;
// one with a package that isn't one, or one twice, is neither written nor
// read.
func TestPackageSet(t *testing.T) {
	d := testDirs(t)
	p := Package{
		Name: "libc6", Version: "2.42-0ubuntu1", Arch: "arm64", SHA256: strings.Repeat("a", 64), Size: 10,
		Entry: "Package: libc6\nVersion: 2.42-0ubuntu1\n",
	}
	want := PackageSet{Recorded: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC), Packages: []Package{p}}
	if keys, err := PackageSetKeys(d); err != nil || len(keys) != 0 {
		t.Fatalf("none: %q, %v", keys, err)
	}
	if _, err := ReadPackageSet(d, "0123456789ab"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("none: %v", err)
	}
	if err := WritePackageSet(d, "0123456789ab", want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPackageSet(d, "0123456789ab")
	if err != nil || !got.Recorded.Equal(want.Recorded) || !slices.Equal(got.Packages, want.Packages) {
		t.Errorf("got %+v, %v", got, err)
	}
	if err := os.WriteFile(d.PackageSet("notakey"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if keys, err := PackageSetKeys(d); err != nil || !slices.Equal(keys, []string{"0123456789ab"}) {
		t.Errorf("keys %q, %v", keys, err)
	}
	for name, change := range map[string]func(*Package){
		"a name with a space":    func(p *Package) { p.Name = "lib c6" },
		"a version with a slash": func(p *Package) { p.Version = "1/2" },
		"an architecture":        func(p *Package) { p.Arch = "ARM" },
		"a short SHA-256":        func(p *Package) { p.SHA256 = "aa" },
		"no size":                func(p *Package) { p.Size = 0 },
		"no entry":               func(p *Package) { p.Entry = "" },
		"a blank line":           func(p *Package) { p.Entry += "\nPackage: another\n" },
		"a line of spaces":       func(p *Package) { p.Entry += " \t\nPackage: another\n" },
	} {
		bad := p
		change(&bad)
		if err := WritePackageSet(d, "0123456789ac", PackageSet{Packages: []Package{bad}}); err == nil {
			t.Errorf("%s: written", name)
		}
	}
	if err := WritePackageSet(d, "0123456789ac", PackageSet{Packages: []Package{p, p}}); err == nil {
		t.Error("twice: written")
	}
	if err := os.WriteFile(d.PackageSet("0123456789ac"), []byte(`{"packages": [{"name": "x"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var corrupt *CorruptError
	if _, err := ReadPackageSet(d, "0123456789ac"); !errors.As(err, &corrupt) {
		t.Errorf("a bad set: %v", err)
	}
	if _, err := ReadPackageSet(d, "../x"); err == nil {
		t.Error("a path for a key")
	}
	for _, k := range []string{"0123456789ab", "0123456789ab"} {
		if err := RemovePackageSet(d, k); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ReadPackageSet(d, "0123456789ab"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("removed: %v", err)
	}
}
