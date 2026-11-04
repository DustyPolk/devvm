package ubuntu

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/assets"
)

// realRelease is a release Ubuntu signed, kept in testdata as it was
// published: its SHA256SUMS and their signature.
const realRelease = "testdata/resolute/release-20261016"

func ubuntuKey(t *testing.T) []byte {
	t.Helper()
	key, err := fs.ReadFile(assets.FS, Keyring)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // the test's own files
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// testKey is an RSA key, as a keyring of one public key packet, that
// tests sign with (sign).
type testKey struct {
	priv    *rsa.PrivateKey
	keyring []byte
}

func newTestKey(t *testing.T) testKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte{4, 0, 0, 0, 0, pubRSA}
	body = append(body, mpiOf(priv.N.Bytes())...)
	body = append(body, mpiOf(big.NewInt(int64(priv.E)).Bytes())...)
	return testKey{priv: priv, keyring: newPacket(tagPublicKey, body)}
}

// mpiOf is b as an MPI.
func mpiOf(b []byte) []byte {
	b = bytes.TrimLeft(b, "\x00")
	bits := 0
	if len(b) > 0 {
		bits = (len(b)-1)*8 + big.NewInt(int64(b[0])).BitLen()
	}
	return append([]byte{byte(bits >> 8), byte(bits)}, b...)
}

// newPacket is a new-format packet with a 5-byte length.
func newPacket(tag byte, body []byte) []byte {
	out := []byte{0xc0 | tag, 255, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(out[2:], uint32(len(body))) //nolint:gosec // test data
	return append(out, body...)
}

// be16 is n's two bytes, big-endian.
func be16(n int) []byte { return binary.BigEndian.AppendUint16(nil, uint16(n)) } //nolint:gosec // test data, under 64 KiB

// sign returns a detached signature of data by k, with hashAlgo and the
// hashed subpackets given (a creation time when none are), armored.
func (k testKey) sign(t *testing.T, data []byte, algo byte, sigType byte, sub []byte) []byte {
	t.Helper()
	if sub == nil {
		sub = []byte{5, subCreated, 0x60, 0, 0, 0}
	}
	signed := append([]byte{4, sigType, pubRSA, algo}, be16(len(sub))...)
	signed = append(signed, sub...)
	h, _ := hashOf(algo)
	d := h.new()
	d.Write(data)
	d.Write(signed)
	d.Write(append([]byte{4, 0xff, 0, 0}, be16(len(signed))...))
	digest := d.Sum(nil)
	value, err := rsa.SignPKCS1v15(rand.Reader, k.priv, h.id, digest)
	if err != nil {
		t.Fatal(err)
	}
	body := append(append(signed, 0, 0), digest[:2]...)
	body = append(body, mpiOf(value)...)
	return armor(newPacket(tagSignature, body))
}

func armor(b []byte) []byte {
	return []byte("-----BEGIN PGP SIGNATURE-----\n\n" + base64.StdEncoding.EncodeToString(b) + "\n=" +
		base64.StdEncoding.EncodeToString(crc24(b)) + "\n-----END PGP SIGNATURE-----\n")
}

func TestVerifyUbuntusSignature(t *testing.T) {
	sums := readFile(t, filepath.Join(realRelease, "SHA256SUMS"))
	sig := readFile(t, filepath.Join(realRelease, "SHA256SUMS.gpg"))
	now := time.Now()
	if err := verifyDetached(ubuntuKey(t), sig, sums, now); err != nil {
		t.Fatalf("Ubuntu's own signature: %v", err)
	}
	other := newTestKey(t)
	tampered := bytes.Replace(sums, []byte("0"), []byte("1"), 1)
	for name, err := range map[string]error{
		"a changed file":   verifyDetached(ubuntuKey(t), sig, tampered, now),
		"another key":      verifyDetached(other.keyring, sig, sums, now),
		"no signature":     verifyDetached(ubuntuKey(t), nil, sums, now),
		"text, not armor":  verifyDetached(ubuntuKey(t), []byte("signature\n"), sums, now),
		"a broken armor":   verifyDetached(ubuntuKey(t), bytes.Replace(sig, []byte("iQ"), []byte("jQ"), 1), sums, now),
		"another's signed": verifyDetached(ubuntuKey(t), other.sign(t, sums, 10, sigBinary, nil), sums, now),
	} {
		if !errors.Is(err, errBadSignature) {
			t.Errorf("%s: %v, want a bad signature", name, err)
		}
	}
}

func TestVerifyDetached(t *testing.T) {
	k := newTestKey(t)
	data := []byte("abc *ubuntu.img\n")
	now := time.Now()
	expired := []byte{5, subCreated, 0x60, 0, 0, 0, 5, subExpires, 0, 0, 0, 1}
	unknownCritical := []byte{5, subCreated, 0x60, 0, 0, 0, 2, 0x80 | 99, 0}
	issuer := []byte{5, subCreated, 0x60, 0, 0, 0, 9, 0x80 | subIssuer, 1, 2, 3, 4, 5, 6, 7, 8}
	for _, c := range []struct {
		name string
		sig  []byte
		ok   bool
	}{
		{"SHA-512", k.sign(t, data, 10, sigBinary, nil), true},
		{"SHA-256", k.sign(t, data, 8, sigBinary, nil), true},
		{"SHA-384", k.sign(t, data, 9, sigBinary, nil), true},
		{"binary, not armored", unarmored(t, k.sign(t, data, 10, sigBinary, nil)), true},
		{"a critical issuer", k.sign(t, data, 10, sigBinary, issuer), true},
		{"a text signature", k.sign(t, data, 10, 0x01, nil), false},
		{"expired", k.sign(t, data, 10, sigBinary, expired), false},
		{"an unknown critical subpacket", k.sign(t, data, 10, sigBinary, unknownCritical), false},
	} {
		err := verifyDetached(k.keyring, c.sig, data, now)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v, want ok %v", c.name, err, c.ok)
		}
	}
	// A weak hash (SHA-1) is refused before anything is hashed.
	weak := k.sign(t, data, 10, sigBinary, nil)
	raw := unarmored(t, weak)
	raw[6+3] = 2
	if err := verifyDetached(k.keyring, raw, data, now); err == nil {
		t.Error("SHA-1: accepted")
	}
}

func unarmored(t *testing.T, armored []byte) []byte {
	t.Helper()
	b, err := dearmor(armored)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPacketsRefuseTruncated(t *testing.T) {
	sig := unarmored(t, readFile(t, filepath.Join(realRelease, "SHA256SUMS.gpg")))
	for n := range len(sig) - 1 {
		if err := verifyDetached(ubuntuKey(t), sig[:n], []byte("x"), time.Now()); err == nil {
			t.Fatalf("a signature cut to %d bytes: accepted", n)
		}
	}
	for _, keyring := range [][]byte{nil, []byte("x"), {0xc6, 1, 4}} {
		if _, err := rsaKeys(keyring); err == nil {
			t.Errorf("keyring %q: read", keyring)
		}
	}
}

// Every length a packet header can give, in both formats; a partial
// length, a header cut short or a length past the end is refused.
func TestPackets(t *testing.T) {
	body := bytes.Repeat([]byte{7}, 300)
	for _, c := range []struct {
		name string
		data []byte
		want int // the body's length; -1 for a refusal
	}{
		{"new, 1 byte", append([]byte{0xc2, 5}, body[:5]...), 5},
		{"new, 2 bytes", append([]byte{0xc2, 192, 108}, body[:300]...), 300},
		{"new, 5 bytes", append([]byte{0xc2, 255, 0, 0, 0, 3}, body[:3]...), 3},
		{"old, 1 byte", append([]byte{0x88, 4}, body[:4]...), 4},
		{"old, 2 bytes", append([]byte{0x89, 1, 44}, body[:300]...), 300},
		{"old, 4 bytes", append([]byte{0x8a, 0, 0, 0, 2}, body[:2]...), 2},
		{"old, to the end", append([]byte{0x8b}, body[:9]...), 9},
		{"new, partial", append([]byte{0xc2, 224}, body[:1]...), -1},
		{"not a packet", []byte{0x02, 1, 1}, -1},
		{"past the end", append([]byte{0xc2, 9}, body[:5]...), -1},
		{"new, cut", []byte{0xc2}, -1},
		{"new 2, cut", []byte{0xc2, 192}, -1},
		{"new 5, cut", []byte{0xc2, 255, 0}, -1},
		{"old 1, cut", []byte{0x88}, -1},
		{"old 2, cut", []byte{0x89, 1}, -1},
		{"old 4, cut", []byte{0x8a, 0, 0}, -1},
	} {
		ps, err := packets(c.data)
		switch {
		case c.want < 0 && err == nil:
			t.Errorf("%s: read %d packets", c.name, len(ps))
		case c.want >= 0 && (err != nil || len(ps) != 1 || len(ps[0].body) != c.want || ps[0].tag != tagSignature):
			t.Errorf("%s: %v, %+v", c.name, err, ps)
		}
	}
}

// Subpackets with 2- and 5-byte lengths read; ones cut short are refused.
func TestSubpackets(t *testing.T) {
	long := append([]byte{192, 1, 20}, bytes.Repeat([]byte{0}, 192)...) // 193 bytes: a notation, not critical
	five := []byte{255, 0, 0, 0, 5, subCreated, 0x60, 0, 0, 0}
	for _, c := range []struct {
		name string
		sub  []byte
		ok   bool
	}{
		{"2-byte length", long, true},
		{"5-byte length", five, true},
		{"2-byte, cut", []byte{192}, false},
		{"5-byte, cut", []byte{255, 0, 0}, false},
		{"empty", []byte{0}, false},
		{"past the end", []byte{9, subCreated}, false},
	} {
		if err := checkSubpackets(c.sub, time.Now()); (err == nil) != c.ok {
			t.Errorf("%s: %v, want ok %v", c.name, err, c.ok)
		}
	}
}

// Keys and signatures cut anywhere, or with a byte changed anywhere but
// the signature's unhashed subpackets (which OpenPGP leaves unsigned: the
// issuer's key ID), never verify, and never panic.
func TestVerifyDamaged(t *testing.T) {
	sums := readFile(t, filepath.Join(realRelease, "SHA256SUMS"))
	sig := unarmored(t, readFile(t, filepath.Join(realRelease, "SHA256SUMS.gpg")))
	key := ubuntuKey(t)
	// An old-format packet with a 2-byte length: 3 bytes, then the body.
	hashedEnd := 3 + 6 + int(binary.BigEndian.Uint16(sig[3+4:]))
	unhashed := hashedEnd + 2 + int(binary.BigEndian.Uint16(sig[hashedEnd:]))
	for i := range sig {
		if i >= hashedEnd+2 && i < unhashed {
			continue
		}
		bad := bytes.Clone(sig)
		bad[i] ^= 0x41
		if err := verifyDetached(key, bad, sums, time.Now()); err == nil {
			t.Fatalf("byte %d of the signature changed: verified", i)
		}
	}
	// The key's modulus: after its packet's 3-byte header, version, time,
	// algorithm and the MPI's bit count.
	for i := 3 + 6 + 2; i < 3+6+2+512; i += 7 {
		bad := bytes.Clone(key)
		bad[i] ^= 0x41
		if err := verifyDetached(bad, sig, sums, time.Now()); err == nil {
			t.Fatalf("byte %d of the key changed: verified", i)
		}
	}
	for i := range len(key) {
		if err := verifyDetached(key[:i], sig, sums, time.Now()); err == nil && i < 3+6+2+512+5 {
			t.Fatalf("the key cut to %d bytes: verified", i)
		}
	}
}

func TestDearmorHeaders(t *testing.T) {
	k := newTestKey(t)
	data := []byte("x")
	plain := string(k.sign(t, data, 10, sigBinary, nil))
	withHeader := strings.Replace(plain, "-----\n\n", "-----\nVersion: GnuPG v2\n\n", 1)
	noBlank := strings.Replace(plain, "-----\n\n", "-----\n", 1)
	noSum := plain[:strings.LastIndex(plain, "\n=")] + "\n-----END PGP SIGNATURE-----\n"
	for name, s := range map[string]string{"a header": withHeader, "no blank line": noBlank, "no checksum": noSum} {
		if err := verifyDetached(k.keyring, []byte(s), data, time.Now()); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	unended := strings.Replace(plain, "-----END PGP SIGNATURE-----", "", 1)
	if err := verifyDetached(k.keyring, []byte(unended), data, time.Now()); err == nil {
		t.Error("an armor without its end: accepted")
	}
}
