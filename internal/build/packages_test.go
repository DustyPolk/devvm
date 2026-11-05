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

func (d deb) entry() string { return d.entryAt("1.0-1ubuntu1") }

// entryAt is d's entry as version.
func (d deb) entryAt(version string) string {
	return "Package: " + d.name + "\nArchitecture: arm64\nVersion: " + version + "\nDepends: libc6 (>= 2.34),\n libfoo\n" +
		"Filename: pool/main/" + d.name + ".deb\nSize: " + strconv.Itoa(len(d.file)) + "\nSHA256: " + d.sum() + "\n"
}

// archiveFixture is one of testdata/archive's files: a test archive key
// (archive.gpg, standing in for Ubuntu's); package lists: the debs these
// tests use, git's, curl's, libc's, an old git's and a new git's, all
// 1.0-1ubuntu1 (Packages), git 1.0-1ubuntu1.1 (Packages-newer), and
// openssl and git 1.0-1ubuntu0.1 (Packages-security); and InRelease files,
// clearsigned by that key with gpg (gpg --quick-gen-key rsa2048, then
// --export-options export-minimal --export and --clearsign --digest-algo
// SHA512; the private key wasn't kept). resolute-updates lists Packages
// (InRelease; InRelease-old, dated before the pinned release) or
// Packages-newer (InRelease-newer), resolute Packages (InRelease-release),
// resolute-backports Packages-newer (InRelease-backports), and
// resolute-security Packages-security as arm64's main, with an empty
// universe and an amd64 list (InRelease-security) or that amd64 list
// alone (InRelease-security-amd64).
func archiveFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "archive", name)) //nolint:gosec // the test's own files
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// signedLists is the InRelease files and package lists a recording's
// build gives back with its debs: resolute-updates' and
// resolute-security's.
func signedLists(t *testing.T) []tarEntry {
	t.Helper()
	return append([]tarEntry{
		{name: "./1.InRelease", data: string(archiveFixture(t, "InRelease"))},
		{name: "./2.Packages", data: string(archiveFixture(t, "Packages"))},
	}, securityLists(t)...)
}

// securityLists is resolute-security's InRelease and package list.
func securityLists(t *testing.T) []tarEntry {
	t.Helper()
	return []tarEntry{
		{name: "./11.InRelease", data: string(archiveFixture(t, "InRelease-security"))},
		{name: "./12.Packages", data: string(archiveFixture(t, "Packages-security"))},
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

// A recording keeps a file already in the cache only when its SHA-256 is
// still its name: a changed one is replaced with what the build gave
// back, which the run log notes.
func TestBasePackagesReplaceChanged(t *testing.T) {
	b, f := testEnv(t)
	var log bytes.Buffer
	b.LogWriter = func() io.Writer { return &log }
	git, curl := deb{"git", "git's file"}, deb{"curl", "curl's file"}
	write(t, b.Dirs.PackageFile(git.sum()), "a corrupted file")
	write(t, b.Dirs.PackageFile(curl.sum()), curl.file)
	f.PackagesOut = given(t, git, curl)
	if err := (Builds{b}).Build(t.Context(), "base"); err != nil {
		t.Fatal(err)
	}
	for _, d := range []deb{git, curl} {
		if data, err := os.ReadFile(b.Dirs.PackageFile(d.sum())); err != nil || string(data) != d.file {
			t.Errorf("%s in the cache: %q, %v", d.name, data, err)
		}
	}
	want := "package cache: replaced " + b.Dirs.PackageFile(git.sum()) + " (git 1.0-1ubuntu1), which had changed\n"
	if log.String() != want {
		t.Errorf("the run log: %q", log.String())
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
	if err := keptNone(t, "", out); msg.IDOf(err) != "packages_refused" {
		t.Fatalf("got %v", err)
	}
}

// keptNone returns the error a recording whose build gives back out
// fails with, config.toml holding config, and checks that none of it is
// kept.
func keptNone(t *testing.T, config string, out []tarEntry) error {
	t.Helper()
	b, f := testEnv(t)
	if config != "" {
		write(t, b.Dirs.ConfigFile(), config)
	}
	f.PackagesOut = tarOf(t, out...)
	err := (Builds{b}).Build(t.Context(), "base")
	if err == nil {
		t.Fatal("kept")
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
	return err
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
		out := append(files(debs...), tarEntry{name: "./1.InRelease", data: in}, tarEntry{name: "./2.Packages", data: list})
		return append(out, securityLists(t)...)
	}
	for name, out := range map[string][]tarEntry{
		"a forged InRelease":      withLists(strings.Replace(inRelease, "Suite: resolute-updates", "Suite: resolute-security", 1), packages, git),
		"another signature":       withLists(strings.Replace(inRelease, "iQ", "iR", 1), packages, git),
		"not clearsigned":         withLists(inRelease[strings.Index(inRelease, "Origin:"):strings.Index(inRelease, "-----BEGIN PGP SIGNATURE")], packages, git),
		"a changed list":          withLists(inRelease, strings.Replace(packages, "libfoo", "libbar", 1), git),
		"a package no list has":   withLists(inRelease, packages, deb{"git", "a made-up git's file"}),
		"another package's entry": withLists(inRelease, packages, deb{"vim", "git's file"}),
		"no InRelease":            append(withLists(inRelease, packages, git), tarEntry{name: "./9.Packages", data: string(archiveFixture(t, "Packages-newer"))}),
		"a list twice":            append(withLists(inRelease, packages, git), tarEntry{name: "./2.Packages", data: packages}),
		"an InRelease twice":      append(withLists(inRelease, packages, git), tarEntry{name: "./1.InRelease", data: inRelease}),
		"a line over 1 MiB":       withLists(inRelease, packages+"Description: "+strings.Repeat("x", 1<<20)+"\n", git),
		"another list's name":     append(files(git), tarEntry{name: "./1.InRelease", data: inRelease}, tarEntry{name: "./lists/2.Packages", data: packages}),
	} {
		t.Run(name, func(t *testing.T) { checkRefused(t, out) })
	}

	// A good set is kept, each package with the archive's entry for it,
	// the release pocket's InRelease as old as the release.
	b, f := testEnv(t)
	f.PackagesOut = tarOf(t, append(withLists(inRelease, packages, git),
		tarEntry{name: "./3.InRelease", data: string(archiveFixture(t, "InRelease-release"))})...)
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

// An InRelease signed by Ubuntu's archive key but older than the pinned
// cloud image's release, which a stale mirror serves, is refused, naming
// the mirror set, or Ubuntu's archive with none, and nothing is kept.
func TestBasePackagesStale(t *testing.T) {
	git := deb{"git", "git's file"}
	out := givenEntries(t, git)[:2]
	out = append(out,
		tarEntry{name: "./1.InRelease", data: string(archiveFixture(t, "InRelease-old"))},
		tarEntry{name: "./2.Packages", data: string(archiveFixture(t, "Packages"))})
	for config, want := range map[string]string{
		"": "packages_stale date=2026-01-01 release=" + testRelease + " suite=resolute-updates",
		"ubuntu-mirror = \"https://mirror.example/ubuntu-ports\"\n": "packages_stale_mirror date=2026-01-01 mirror=https://mirror.example/ubuntu-ports release=" +
			testRelease + " suite=resolute-updates",
	} {
		if got := msg.NamesOf(keptNone(t, config, out)); got != want {
			t.Errorf("%q: got %s, want %s", config, got, want)
		}
	}
}

// A recording must give back resolute-updates' and resolute-security's
// signed InRelease files, and every list a signed InRelease gives for the
// architecture its packages are listed for, but an empty one: else apt
// may have had a newer version of a package that devvm can't see. Lists
// for another architecture aren't needed.
func TestBasePackagesListsMissing(t *testing.T) {
	git := deb{"git", "git's file"}
	files := givenEntries(t, git)[:2]
	updates := []tarEntry{
		{name: "./1.InRelease", data: string(archiveFixture(t, "InRelease"))},
		{name: "./2.Packages", data: string(archiveFixture(t, "Packages"))},
	}
	release := []tarEntry{
		{name: "./5.InRelease", data: string(archiveFixture(t, "InRelease-release"))},
		{name: "./6.Packages", data: string(archiveFixture(t, "Packages"))},
	}
	security := securityLists(t)
	backports := []tarEntry{{name: "./7.InRelease", data: string(archiveFixture(t, "InRelease-backports"))}}
	for name, c := range map[string]struct {
		out  [][]tarEntry
		want string
	}{
		"no lists":                 {[][]tarEntry{files}, "resolute-updates"},
		"no resolute-security":     {[][]tarEntry{files, updates}, "resolute-security"},
		"no resolute-updates":      {[][]tarEntry{files, release, security}, "resolute-updates"},
		"resolute-security's list": {[][]tarEntry{files, updates, security[:1]}, "resolute-security"},
		"resolute-backports' list": {[][]tarEntry{files, updates, security, backports}, "resolute-backports"},
		// Another archive's InRelease (archive.ubuntu.com's, not ports'),
		// which has no lists for the packages' architecture.
		"another archive's resolute-security": {[][]tarEntry{
			files, updates,
			{{name: "./11.InRelease", data: string(archiveFixture(t, "InRelease-security-amd64"))}},
		}, "resolute-security"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := msg.NamesOf(keptNone(t, "", slices.Concat(c.out...))); got != "packages_lists_missing suite="+c.want {
				t.Errorf("got %s, want packages_lists_missing suite=%s", got, c.want)
			}
		})
	}
}

// A recorded package that a signed list of the release pocket,
// resolute-updates or resolute-security has a newer version of is
// refused, as apt would have installed that one; an older version there,
// or a newer one in resolute-backports, which apt installs only when
// asked, is fine, and so is the newer version itself.
func TestBasePackagesNewest(t *testing.T) {
	git, newer := deb{"git", "git's file"}, deb{"git", "a newer build"}
	release := []tarEntry{
		{name: "./5.InRelease", data: string(archiveFixture(t, "InRelease-release"))},
		{name: "./6.Packages", data: string(archiveFixture(t, "Packages"))},
	}
	updatesNewer := []tarEntry{
		{name: "./1.InRelease", data: string(archiveFixture(t, "InRelease-newer"))},
		{name: "./2.Packages", data: string(archiveFixture(t, "Packages-newer"))},
	}
	backports := []tarEntry{
		{name: "./7.InRelease", data: string(archiveFixture(t, "InRelease-backports"))},
		{name: "./8.Packages", data: string(archiveFixture(t, "Packages-newer"))},
	}
	recorded := func(d deb, version string) []tarEntry {
		return []tarEntry{{name: "./Packages", data: d.entryAt(version) + "\n"}, {name: "./" + d.sum() + ".deb", data: d.file}}
	}
	old := recorded(git, "1.0-1ubuntu1")
	err := keptNone(t, "", slices.Concat(old, release, updatesNewer, securityLists(t)))
	if got, want := msg.NamesOf(err), "packages_not_newest name=git newer=1.0-1ubuntu1.1 suite=resolute-updates version=1.0-1ubuntu1"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}

	for name, out := range map[string][]tarEntry{
		"the newer version":  slices.Concat(recorded(newer, "1.0-1ubuntu1.1"), release, updatesNewer, securityLists(t)),
		"newer in backports": slices.Concat(old, signedLists(t), release, backports),
		"older in -security": slices.Concat(old, signedLists(t)),
	} {
		t.Run(name, func(t *testing.T) {
			b, f := testEnv(t)
			f.PackagesOut = tarOf(t, out...)
			if err := (Builds{b}).Build(t.Context(), "base"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// checkNewest compares Debian versions, for the architectures that list
// a package only, and refuses a version that isn't one.
func TestCheckNewest(t *testing.T) {
	s := &signed{
		arches: map[string][]string{"a": {"arm64"}},
		newest: map[string]map[string]newest{
			"arm64": {"git": {"1:1.0", "resolute-security"}},
			"amd64": {"vim": {"9.0", "resolute-updates"}},
		},
	}
	for version, want := range map[string]string{
		"1:1.0":     "",
		"1:1.0+b1":  "",
		"2.0":       "packages_not_newest name=git newer=1:1.0 suite=resolute-security version=2.0",
		"1:1.0~rc1": "packages_not_newest name=git newer=1:1.0 suite=resolute-security version=1:1.0~rc1",
		"1.0_1":     "packages_refused why=git's version isn't one: \"1.0_1\"",
	} {
		set := state.PackageSet{Packages: []state.Package{{Name: "git", Version: version, SHA256: "a"}, {Name: "vim", Version: "1.0", SHA256: "a"}}}
		if got := msg.NamesOf(s.checkNewest(set)); got != want {
			t.Errorf("%s: got %q, want %q", version, got, want)
		}
	}
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1.0", "", true}, {"1.0_1", "", false}, {"1.1", "1.0", true}, {"1.0", "1.0", false}, {"1.0_1", "1.0", false}} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
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
