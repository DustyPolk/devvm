package build

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// tarEntry is one entry of a test's tar stream: a file, or with link set,
// a symlink.
type tarEntry struct {
	name, data, link string
}

func tarOf(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.data)), Typeflag: tar.TypeReg}
		if e.link != "" {
			h = &tar.Header{Name: e.name, Linkname: e.link, Typeflag: tar.TypeSymlink}
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// untar reads a tar stream into its files' names and contents.
func untar(t *testing.T, data []byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[h.Name] = string(b)
	}
}

func sumOf(data string) string {
	s := sha256.Sum256([]byte(data))
	return hex.EncodeToString(s[:])
}

// deb is a package's file and its entry in apt's lists, as a build gives
// them back.
type deb struct{ name, file string }

func (d deb) sum() string { return sumOf(d.file) }

func (d deb) entry() string {
	return "Package: " + d.name + "\nArchitecture: arm64\nVersion: 1.0-1ubuntu1\nDepends: libc6 (>= 2.34),\n libfoo\n" +
		"Filename: pool/main/" + d.name + ".deb\nSize: " + strconv.Itoa(len(d.file)) + "\nSHA256: " + d.sum() + "\n"
}

// archiveFixture is one of testdata/archive's files: a test archive key
// (archive.gpg, standing in for Ubuntu's), a package list of the debs
// these tests use, git's, curl's, libc's, an old git's and a new git's
// (Packages), and InRelease files that list it, clearsigned by that key
// with gpg (gpg --quick-gen-key, then --export-options export-minimal
// --export and --clearsign; the private key wasn't kept).
func archiveFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "archive", name)) //nolint:gosec // the test's own files
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// signedLists is the InRelease and the package list a recording's build
// gives back with its debs.
func signedLists(t *testing.T) []tarEntry {
	t.Helper()
	return []tarEntry{
		{name: "./1.InRelease", data: string(archiveFixture(t, "InRelease"))},
		{name: "./2.Packages", data: string(archiveFixture(t, "Packages"))},
	}
}

// given is what a recording's build gives back for debs, with the
// archive's signed lists.
func given(t *testing.T, debs ...deb) []byte {
	t.Helper()
	return tarOf(t, givenEntries(t, debs...)...)
}

func givenEntries(t *testing.T, debs ...deb) []tarEntry {
	t.Helper()
	var list strings.Builder
	entries := []tarEntry{{name: "./", data: ""}}
	for _, d := range debs {
		list.WriteString(d.entry() + "\n")
		entries = append(entries, tarEntry{name: "./" + d.sum() + ".deb", data: d.file})
	}
	entries[0] = tarEntry{name: "./Packages", data: list.String()}
	return append(entries, signedLists(t)...)
}

// A first build of base records what it downloaded: each file in the
// cache under its SHA-256, the set under base's key, the mirror from
// config.toml sent first. A later build of the same key replays the set:
// the files from the cache, apt's entries naming them, nothing taken back.
func TestBasePackagesRecordThenReplay(t *testing.T) {
	b, f := testEnv(t)
	write(t, b.Dirs.ConfigFile(), "ubuntu-mirror = \"https://mirror.example/ubuntu\"\n")
	git, curl := deb{"git", "git's file"}, deb{"curl", "curl's file"}
	f.PackagesOut = given(t, git, curl)
	ctx := t.Context()
	if err := (Builds{b}).Build(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	if got := untar(t, f.PackagesSent); got["in/mirrors"] != "https://mirror.example/ubuntu\n" || len(got) != 1 {
		t.Errorf("a recording was sent %q", got)
	}
	key, err := b.Key(image.Base)
	if err != nil {
		t.Fatal(err)
	}
	set, err := state.ReadPackageSet(b.Dirs, key)
	if err != nil || len(set.Packages) != 2 {
		t.Fatalf("set %+v, %v", set, err)
	}
	for _, d := range []deb{git, curl} {
		if data, err := os.ReadFile(b.Dirs.PackageFile(d.sum())); err != nil || string(data) != d.file {
			t.Errorf("%s in the cache: %q, %v", d.name, data, err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(b.Dirs.Packages(), ".taken-*")); len(left) != 0 {
		t.Errorf("left %q", left)
	}

	// base is built again: its image gone, its set kept.
	f.ImageKeys = slices.DeleteFunc(f.ImageKeys, func(k string) bool { return k == key })
	if err := state.RemoveImage(b.Dirs, key); err != nil {
		t.Fatal(err)
	}
	f.PackagesOut = []byte("not a tar stream: a replay takes nothing back")
	if err := (Builds{b}).Build(ctx, "base"); err != nil {
		t.Fatal(err)
	}
	got := untar(t, f.PackagesSent)
	if got["in/"+git.sum()+".deb"] != git.file || got["in/"+curl.sum()+".deb"] != curl.file || len(got) != 3 {
		t.Errorf("a replay was sent %q", got)
	}
	list := got["in/Packages"]
	for _, d := range []deb{git, curl} {
		if !strings.Contains(list, "Package: "+d.name+"\n") || !strings.Contains(list, "Filename: ./"+d.sum()+".deb\n") {
			t.Errorf("the replay's list lacks %s:\n%s", d.name, list)
		}
	}
	if strings.Contains(list, "pool/") || !strings.Contains(list, "\n libfoo\n") {
		t.Errorf("the replay's list:\n%s", list)
	}
	if after, _ := state.ReadPackageSet(b.Dirs, key); after.Recorded != set.Recorded {
		t.Error("the replay recorded the set again")
	}
}

// A replay whose file is gone from the cache, or changed, fails before
// anything is built, naming the package.
func TestBasePackagesMissing(t *testing.T) {
	git := deb{"git", "git's file"}
	for name, change := range map[string]func(path string) error{
		"gone":    os.Remove,
		"changed": func(p string) error { return os.WriteFile(p, []byte("another file"), 0o600) },
	} {
		t.Run(name, func(t *testing.T) {
			b, f := testEnv(t)
			f.PackagesOut = given(t, git)
			if err := (Builds{b}).Build(t.Context(), "base"); err != nil {
				t.Fatal(err)
			}
			key, _ := b.Key(image.Base)
			f.ImageKeys = nil
			if err := change(b.Dirs.PackageFile(git.sum())); err != nil {
				t.Fatal(err)
			}
			builds := len(f.Methods())
			err := (Builds{b}).Build(t.Context(), "base")
			if msg.NamesOf(err) != "packages_missing name=git version=1.0-1ubuntu1" {
				t.Errorf("got %v", err)
			}
			if slices.Contains(f.Methods()[builds:], "BuildImage") {
				t.Error("built without its packages")
			}
			if _, err := state.ReadPackageSet(b.Dirs, key); err != nil {
				t.Errorf("the set: %v", err)
			}
		})
	}
}

// What a build gives back that isn't exactly packages with their entries
// is refused, and none of it is kept: no set, nothing in the cache, no
// image record.
func TestBasePackagesRefused(t *testing.T) {
	git := deb{"git", "git's file"}
	list := git.entry() + "\n"
	sumName := "./" + git.sum() + ".deb"
	for name, out := range map[string][]tarEntry{
		"a symlink":            {{name: "./Packages", data: list}, {name: sumName, link: "/etc/passwd"}},
		"another name":         {{name: "./Packages", data: list}, {name: "./git.deb", data: git.file}},
		"a path":               {{name: "./Packages", data: list}, {name: "./../" + git.sum() + ".deb", data: git.file}},
		"another SHA-256":      {{name: "./Packages", data: list}, {name: sumName, data: "another file"}},
		"no file":              {{name: "./Packages", data: list}},
		"no entry":             {{name: "./Packages", data: ""}, {name: sumName, data: git.file}},
		"twice":                {{name: "./Packages", data: list + list}, {name: sumName, data: git.file}},
		"another size":         {{name: "./Packages", data: strings.Replace(list, "Size: 10", "Size: 11", 1)}, {name: sumName, data: git.file}},
		"a bad entry":          {{name: "./Packages", data: "Package: git\nnot a field\n"}, {name: sumName, data: git.file}},
		"two lists":            {{name: "./Packages", data: list}, {name: "./Packages", data: list}, {name: sumName, data: git.file}},
		"an entry without one": {{name: "./Packages", data: strings.Replace(list, "Version: 1.0-1ubuntu1\n", "", 1)}, {name: sumName, data: git.file}},
	} {
		t.Run(name, func(t *testing.T) {
			checkRefused(t, append(out, signedLists(t)...))
		})
	}
}

// checkRefused checks that a recording whose build gives back out is
// refused, and none of it is kept: no set, nothing in the cache, no image
// record.
func checkRefused(t *testing.T, out []tarEntry) {
	t.Helper()
	b, f := testEnv(t)
	f.PackagesOut = tarOf(t, out...)
	err := (Builds{b}).Build(t.Context(), "base")
	if msg.IDOf(err) != "packages_refused" {
		t.Fatalf("got %v", err)
	}
	t.Log(msg.NamesOf(err))
	key, _ := b.Key(image.Base)
	if _, err := state.ReadPackageSet(b.Dirs, key); err == nil {
		t.Error("a set was recorded")
	}
	if _, err := state.ReadImage(b.Dirs, key); err == nil {
		t.Error("the image was recorded")
	}
	if left, _ := os.ReadDir(b.Dirs.Packages()); len(left) != 0 {
		t.Errorf("the cache holds %v", left)
	}
}

// Each package a recording gives back must be one the archive's package
// lists give the SHA-256 of, each list one an InRelease signed by Ubuntu's
// archive key gives the SHA-256 of: a forged InRelease, a list it doesn't
// give, a package no list has or apt's entry for another package is
// refused, and nothing is kept.
func TestBasePackagesSigned(t *testing.T) {
	git := deb{"git", "git's file"}
	inRelease := string(archiveFixture(t, "InRelease"))
	packages := string(archiveFixture(t, "Packages"))
	files := func(debs ...deb) []tarEntry { return givenEntries(t, debs...)[:1+len(debs)] }
	withLists := func(in, list string, debs ...deb) []tarEntry {
		return append(files(debs...), tarEntry{name: "./1.InRelease", data: in}, tarEntry{name: "./2.Packages", data: list})
	}
	for name, out := range map[string][]tarEntry{
		"a forged InRelease":      withLists(strings.Replace(inRelease, "Suite: resolute-updates", "Suite: resolute-security", 1), packages, git),
		"another signature":       withLists(strings.Replace(inRelease, "iQ", "iR", 1), packages, git),
		"not clearsigned":         withLists(inRelease[strings.Index(inRelease, "Origin:"):strings.Index(inRelease, "-----BEGIN PGP SIGNATURE")], packages, git),
		"a changed list":          withLists(inRelease, strings.Replace(packages, "libfoo", "libbar", 1), git),
		"a package no list has":   withLists(inRelease, packages, deb{"git", "a made-up git's file"}),
		"another package's entry": withLists(inRelease, packages, deb{"vim", "git's file"}),
		"no lists":                files(git),
		"no InRelease":            append(files(git), tarEntry{name: "./2.Packages", data: packages}),
		"a list twice":            append(withLists(inRelease, packages, git), tarEntry{name: "./2.Packages", data: packages}),
		"an InRelease twice":      append(withLists(inRelease, packages, git), tarEntry{name: "./1.InRelease", data: inRelease}),
		"a line over 1 MiB":       withLists(inRelease, packages+"Description: "+strings.Repeat("x", 1<<20)+"\n", git),
		"another list's name":     append(files(git), tarEntry{name: "./1.InRelease", data: inRelease}, tarEntry{name: "./lists/2.Packages", data: packages}),
	} {
		t.Run(name, func(t *testing.T) { checkRefused(t, out) })
	}

	// A good set is kept, each package with the archive's entry for it.
	b, f := testEnv(t)
	f.PackagesOut = tarOf(t, withLists(inRelease, packages, git)...)
	if err := (Builds{b}).Build(t.Context(), "base"); err != nil {
		t.Fatal(err)
	}
	key, _ := b.Key(image.Base)
	set, err := state.ReadPackageSet(b.Dirs, key)
	if err != nil || len(set.Packages) != 1 {
		t.Fatalf("set %+v, %v", set, err)
	}
	if p := set.Packages[0]; p.Name != "git" || p.SHA256 != git.sum() || !strings.Contains(packages, p.Entry) {
		t.Errorf("kept %+v, not the archive's entry", p)
	}
}

// gc's view of the cache: a set is kept while its key is base's as it is
// now or an image the backend has, and so are the files it names; the
// rest goes, files another set shares staying. A backend that can't list
// its images keeps everything.
func TestStalePackages(t *testing.T) {
	b, f := testEnv(t)
	ctx := t.Context()
	shared, old := deb{"libc6", "libc's file"}, deb{"git", "an old git's file"}
	f.PackagesOut = given(t, shared, old)
	if err := (Builds{b}).Build(ctx, "base"); err != nil {
		t.Fatal(err)
	}
	oldKey, _ := b.Key(image.Base)
	// A new base, its old image gone: its key is no longer current.
	f.Pinned = "release-20260201"
	f.PackagesOut = given(t, shared, deb{"git", "a new git's file"})
	b.Force = true
	f.ImageKeys = nil
	if err := (Builds{b}).Rebuild(ctx, "base"); err != nil {
		t.Fatal(err)
	}
	images := Images{b}
	s, err := images.Stale(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Sets, []string{oldKey}) || !slices.Equal(s.Files, []string{old.sum()}) || s.Size != int64(len(old.file)) {
		t.Fatalf("stale %+v; want %s's set and %s", s, oldKey, old.sum())
	}
	if err := images.DeletePackages(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.Dirs.PackageFile(old.sum())); err == nil {
		t.Error("the old git is still there")
	}
	if _, err := os.Stat(b.Dirs.PackageFile(shared.sum())); err != nil {
		t.Errorf("the shared file: %v", err)
	}
	if s, err := images.Stale(ctx); err != nil || len(s.Sets)+len(s.Files) != 0 {
		t.Errorf("after: %+v, %v", s, err)
	}
	// A file gc planned to delete that a set has named since stays, and a
	// build's folder left over a day goes.
	write(t, b.Dirs.PackageFile(old.sum()), old.file)
	s = StalePackages{Files: []string{old.sum(), shared.sum()}}
	taken := filepath.Join(b.Dirs.Packages(), ".taken-left")
	write(t, filepath.Join(taken, "x.deb"), "x")
	day := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(taken, day, day); err != nil {
		t.Fatal(err)
	}
	if err := images.DeletePackages(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.Dirs.PackageFile(shared.sum())); err != nil {
		t.Errorf("a file a set names went: %v", err)
	}
	if _, err := os.Stat(b.Dirs.PackageFile(old.sum())); err == nil {
		t.Error("a file no set names stayed")
	}
	if _, err := os.Stat(taken); err == nil {
		t.Error("the old build's folder stayed")
	}
	f.Errs = map[string]error{"Images": os.ErrPermission}
	write(t, b.Dirs.PackageSet("0123456789ab"), `{"packages": []}`)
	if s, err := images.Stale(ctx); err != nil || len(s.Sets) != 0 {
		t.Errorf("unlisted: %+v, %v", s, err)
	}
}

// Only base from the backend's own Ubuntu gets packages of devvm's: not a
// layer, and not a --from base, whose image's own sources it uses.
func TestBasePackagesOnlyForBase(t *testing.T) {
	digest, lookups := "sha256:"+strings.Repeat("a", 64), 0
	ubuntu, uf := testEnv(t)
	from, ff := fromEnv(t, &digest, &lookups)
	for name, c := range map[string]struct {
		b *Env
		f *backendtest.Fake
	}{"ubuntu": {ubuntu, uf}, "--from": {from, ff}} {
		if err := (Builds{c.b}).Build(t.Context(), "foo"); err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, call := range c.f.Calls() {
			if call.Method == "BuildImage" {
				spec := call.Args[0].(backend.ImageSpec)
				got[spec.Image] = spec.Packages != nil
			}
		}
		if want := map[string]bool{"base": name == "ubuntu", "foo": false}; !maps.Equal(got, want) {
			t.Errorf("%s: packages %v, want %v", name, got, want)
		}
	}
}
