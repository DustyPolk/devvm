package ubuntu

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/w3cj/devvm/assets"
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
	if !r.Lists(mainArm64, 4539808) || r.Lists(mainArm64, 4539807) {
		t.Error("main's arm64 Packages: not listed with its size")
	}
	if r.Lists("b2c5060084dc1345085fae9745b378ad97177006db8d40761d7d6dd73874fd82", 457597198) {
		t.Error("Contents-amd64 is listed as a package list")
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
		case c.ok && !r.Lists(sum, 12):
			t.Errorf("%s: its list isn't listed", c.name)
		case c.ok && r.Lists(strings.Repeat("cd", 32), 9):
			t.Errorf("%s: a compressed list is listed", c.name)
		}
	}
	if _, err := VerifyRelease(fstest.MapFS{}, k.clearsign(t, good), time.Now()); err == nil {
		t.Error("no keyring: verified")
	}
}
