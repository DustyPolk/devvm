package ubuntu

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/devvmhq/devvm/assets"
)

// realInRelease is an InRelease Ubuntu signed, kept in testdata as
// ports.ubuntu.com published it.
const realInRelease = "testdata/resolute/dists/resolute-updates/InRelease"

// clearsign is text clearsigned by k, as gpg --clearsign writes it: its
// lines starting with a dash escaped, the signature over them joined by
// CR LF without trailing whitespace.
func (k testKey) clearsign(t *testing.T, text string) []byte {
	t.Helper()
	var lines, out []string
	for line := range strings.Lines(strings.TrimSuffix(text, "\n")) {
		line = strings.TrimSuffix(line, "\n")
		lines = append(lines, strings.TrimRight(line, " \t"))
		if strings.HasPrefix(line, "-") {
			line = "- " + line
		}
		out = append(out, line)
	}
	sig := k.sign(t, []byte(strings.Join(lines, "\r\n")), 10, sigText, nil)
	return []byte(clearBegin + "\nHash: SHA512\n\n" + strings.Join(out, "\n") + "\n" + string(sig))
}

func keyFS(keyring []byte) fstest.MapFS {
	return fstest.MapFS{ArchiveKeyring: {Data: keyring}}
}

func TestVerifyUbuntusRelease(t *testing.T) {
	in := readFile(t, realInRelease)
	r, err := VerifyRelease(assets.FS, in, time.Now())
	if err != nil {
		t.Fatalf("Ubuntu's own InRelease: %v", err)
	}
	if r.Suite != "resolute-updates" || !r.Date.Equal(time.Date(2026, 11, 4, 18, 15, 15, 0, time.UTC)) {
		t.Errorf("read %s, %s", r.Suite, r.Date)
	}
	const mainArm64 = "32df8f754a7bf1f448c75dd951f3b553e36a615c726617c40a8199972785e41a"
	if l := r.Lists(mainArm64, 4539808); len(l) != 1 || l[0].Path != "main/binary-arm64/Packages" || l[0].Arch() != "arm64" ||
		len(r.Lists(mainArm64, 4539807)) != 0 {
		t.Errorf("main's arm64 Packages: not listed with its size: %v", l)
	}
	if len(r.Lists("b2c5060084dc1345085fae9745b378ad97177006db8d40761d7d6dd73874fd82", 457597198)) != 0 {
		t.Error("Contents-amd64 is listed as a package list")
	}
	var arm64 []string
	for _, l := range r.ListsFor("arm64") {
		arm64 = append(arm64, l.Path)
	}
	if want := []string{
		"main/binary-arm64/Packages", "multiverse/binary-arm64/Packages", "restricted/binary-arm64/Packages",
		"universe/binary-arm64/Packages",
	}; !slices.Equal(arm64, want) {
		t.Errorf("arm64's lists: %q, want %q", arm64, want)
	}
	if r.Backports() || !(Release{Suite: "resolute-backports"}).Backports() {
		t.Error("backports: not told apart")
	}
	other := newTestKey(t)
	unsigned, _, _ := bytes.Cut(in, []byte(sigBegin))
	for name, data := range map[string][]byte{
		"a changed line":     bytes.Replace(in, []byte("Suite: resolute-updates"), []byte("Suite: resolute-updatez"), 1),
		"a line added":       bytes.Replace(in, []byte("\nSHA256:\n"), []byte("\nSHA256:\n "+strings.Repeat("0", 64)+" 1 main/binary-arm64/Packages\n"), 1),
		"text before":        append([]byte("Date: Thu, 01 Jan 2099 00:00:00 UTC\n"), in...),
		"text after":         append(bytes.Clone(in), "Suite: resolute-security\n"...),
		"another header":     bytes.Replace(in, []byte("Hash: SHA512\n"), []byte("Hash: SHA512\nComment: x\n"), 1),
		"no signature":       unsigned,
		"no signature's end": bytes.TrimSuffix(bytes.TrimSpace(in), []byte(sigEnd)),
		"another key's":      other.clearsign(t, string(unsigned[len(clearBegin)+15:])),
		"not clearsigned":    []byte("Suite: resolute\n"),
	} {
		if _, err := VerifyRelease(assets.FS, data, time.Now()); err == nil {
			t.Errorf("%s: verified", name)
		}
	}
}

func TestVerifyClearsigned(t *testing.T) {
	k := newTestKey(t)
	text := "Suite: resolute\n- a dashed line\n--- another\ntrailing \t\n\nlast\n"
	want := "Suite: resolute\n- a dashed line\n--- another\ntrailing\n\nlast\n"
	signed := k.clearsign(t, text)
	got, err := verifyClearsigned(k.keyring, signed, time.Now())
	if err != nil || string(got) != want {
		t.Fatalf("got %q, %v", got, err)
	}
	// CR LF line endings and trailing whitespace aren't signed: the same.
	crlf := bytes.ReplaceAll(signed, []byte("\n"), []byte("\r\n"))
	if got, err := verifyClearsigned(k.keyring, crlf, time.Now()); err != nil || string(got) != want {
		t.Errorf("CR LF: %q, %v", got, err)
	}
	unsigned, _, _ := bytes.Cut(signed, []byte(sigBegin))
	// More signature packets than verify tries, the good one last.
	canonical := []byte(strings.Join(strings.Split(strings.TrimSuffix(want, "\n"), "\n"), "\r\n"))
	var many []byte
	for range maxSignatures {
		many = append(many, unarmored(t, newTestKey(t).sign(t, canonical, 10, sigText, nil))...)
	}
	many = append(many, unarmored(t, k.sign(t, canonical, 10, sigText, nil))...)
	if err := verify(k.keyring, many, canonical, sigText, time.Now()); !errors.Is(err, errBadSignature) {
		t.Errorf("%d signatures: %v, want a bad signature", maxSignatures+1, err)
	}
	if err := verify(k.keyring, unarmored(t, k.sign(t, canonical, 10, sigText, nil)), canonical, sigText, time.Now()); err != nil {
		t.Errorf("one signature: %v", err)
	}
	for name, data := range map[string][]byte{
		"an unescaped dash": bytes.Replace(signed, []byte("- --- another"), []byte("--- another"), 1),
		"a changed line":    bytes.Replace(signed, []byte("last"), []byte("lost"), 1),
		"a binary signature": append(bytes.Clone(unsigned),
			k.sign(t, []byte(strings.Join(strings.Split(strings.TrimSuffix(want, "\n"), "\n"), "\r\n")), 10, sigBinary, nil)...),
		"no blank line": bytes.Replace(signed, []byte("Hash: SHA512\n\n"), []byte("Hash: SHA512\n"), 1),
		"empty":         nil,
	} {
		if _, err := verifyClearsigned(k.keyring, data, time.Now()); !errors.Is(err, errBadSignature) {
			t.Errorf("%s: %v, want a bad signature", name, err)
		}
	}
}

// A pocket that keeps changing is stale when it's more than a day older
// than the pinned image's release; the release pocket, frozen at
// Released, when it's older than that.
func TestReleaseStale(t *testing.T) {
	at := func(s string) time.Time {
		d, err := time.Parse(time.DateTime, s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	for _, c := range []struct {
		suite, date, release string
		stale                bool
	}{
		{"resolute-updates", "2026-11-04 18:15:15", "release-20261016", false},
		{"resolute-updates", "2026-10-15 00:00:00", "release-20261016", false},
		{"resolute-security", "2026-10-14 23:59:59", "release-20261016", true},
		{"resolute-backports", "2026-01-01 00:00:00", "release-20261016", true},
		{"resolute-updates", "2026-11-04 18:15:15", "not a release", true},
		{"resolute", "2026-04-23 17:07:15", "release-20261016", false},
		{"resolute", "2026-04-22 23:59:59", "release-20261016", true},
	} {
		r := Release{Suite: c.suite, Date: at(c.date)}
		if got := r.Stale(c.release); got != c.stale {
			t.Errorf("%s of %s, %s: stale %v, want %v", c.suite, c.date, c.release, got, c.stale)
		}
	}
}

// What a verified InRelease must say: one of Series' suites, a date.
func TestReadRelease(t *testing.T) {
	k := newTestKey(t)
	sum := strings.Repeat("ab", 32)
	good := "Suite: resolute-security\nDate: Wed, 04 Nov 2026 19:59:16 UTC\nSHA256:\n " + sum + " 12 main/binary-arm64/Packages\n " +
		strings.Repeat("cd", 32) + " 9 main/binary-arm64/Packages.xz\n"
	for _, c := range []struct {
		name, text string
		ok         bool
	}{
		{"good", good, true},
		{"a numeric zone", strings.Replace(good, "UTC", "+0000", 1), true},
		{"the release pocket", strings.Replace(good, "resolute-security", "resolute", 1), true},
		{"backports", strings.Replace(good, "resolute-security", "resolute-backports", 1), true},
		{"proposed", strings.Replace(good, "resolute-security", "resolute-proposed", 1), false},
		{"another series", strings.Replace(good, "resolute-security", "noble-security", 1), false},
		{"a series that starts the same", strings.Replace(good, "resolute-security", "resolutex", 1), false},
		{"no date", strings.Replace(good, "Date: Wed, 04 Nov 2026 19:59:16 UTC\n", "", 1), false},
		{"not a date", strings.Replace(good, "Wed, 04 Nov 2026", "yesterday", 1), false},
		{"a size too big", strings.Replace(good, " 12 ", " 99999999999999999999 ", 1), false},
	} {
		r, err := VerifyRelease(keyFS(k.keyring), k.clearsign(t, c.text), time.Now())
		switch {
		case (err == nil) != c.ok:
			t.Errorf("%s: %v, want ok %v", c.name, err, c.ok)
		case c.ok && len(r.Lists(sum, 12)) != 1:
			t.Errorf("%s: its list isn't listed", c.name)
		case c.ok && len(r.Lists(strings.Repeat("cd", 32), 9)) != 0:
			t.Errorf("%s: a compressed list is listed", c.name)
		}
	}
	if _, err := VerifyRelease(fstest.MapFS{}, k.clearsign(t, good), time.Now()); err == nil {
		t.Error("no keyring: verified")
	}
}
