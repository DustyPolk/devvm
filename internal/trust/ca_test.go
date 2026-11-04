package trust

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// devvm's CA lasts 30 days, stays while it has over a week left, and is replaced after that; the
// key is the user's alone, and the pair is what the proxy loads (an ECDSA key, tls's pair).
func TestInitCA(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ca")
	if err := InitCA(dir); err != nil {
		t.Fatal(err)
	}
	read := func() (*x509.Certificate, *ecdsa.PrivateKey) {
		t.Helper()
		pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key"))
		if err != nil {
			t.Fatal(err)
		}
		key, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
		if !ok {
			t.Fatalf("the key is a %T", pair.PrivateKey)
		}
		c, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		return c, key
	}
	first, key := read()
	if life := first.NotAfter.Sub(first.NotBefore); life > 31*24*time.Hour || life < 29*24*time.Hour {
		t.Errorf("CA valid for %v, want about 30 days", life)
	}
	if !first.IsCA || !first.MaxPathLenZero || len(first.DNSNames) != 0 || first.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Errorf("CA = %+v", first)
	}
	for name, want := range map[string]os.FileMode{"ca.key": 0o600, "ca.pem": 0o644, ".": 0o700} {
		if st, err := os.Stat(filepath.Join(dir, name)); err != nil || st.Mode().Perm() != want {
			t.Errorf("%s: %v, %v; want %v", name, st.Mode().Perm(), err, want)
		}
	}
	if err := InitCA(dir); err != nil {
		t.Fatal(err)
	}
	if c, _ := read(); c.SerialNumber.Cmp(first.SerialNumber) != 0 {
		t.Error("a fresh CA was replaced")
	}
	// One with 3 days left, and one valid for 10 years (longer than a CA lasts), are replaced.
	for i, life := range [][2]time.Time{
		{time.Now().Add(-27 * 24 * time.Hour), time.Now().Add(3 * 24 * time.Hour)},
		{time.Now().Add(-time.Hour), time.Now().AddDate(10, 0, 0)},
	} {
		serial := big.NewInt(int64(7 + i))
		tmpl := &x509.Certificate{
			SerialNumber: serial, Subject: pkix.Name{CommonName: "old"}, NotBefore: life[0], NotAfter: life[1],
			IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := InitCA(dir); err != nil {
			t.Fatal(err)
		}
		var c *x509.Certificate
		c, key = read()
		if c.SerialNumber.Cmp(serial) == 0 || time.Until(c.NotAfter) < 29*24*time.Hour || c.NotAfter.Sub(c.NotBefore) > 31*24*time.Hour {
			t.Errorf("%d: an old CA was kept: %v until %v", i, c.SerialNumber, c.NotAfter)
		}
	}
	// A key without its certificate is replaced, with a certificate of its own.
	if err := os.Remove(filepath.Join(dir, "ca.pem")); err != nil {
		t.Fatal(err)
	}
	if err := InitCA(dir); err != nil {
		t.Fatal(err)
	}
	read()
}

// A CA folder InitCA can't write fails it.
func TestInitCAFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InitCA(filepath.Join(file, "ca")); err == nil {
		t.Error("InitCA wrote under a file")
	}
	// A certificate it can't write (a folder in its place) fails it too.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "ca.pem"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := InitCA(dir); err == nil {
		t.Error("InitCA wrote over a folder")
	}
}

// Bundle joins the CAs into one PEM file, none for none.
func TestBundle(t *testing.T) {
	if b := (CAs{}).Bundle(); len(b) != 0 {
		t.Errorf("no CAs = %q", b)
	}
	if b := (CAs{[]byte("a\n"), []byte("b\n")}).Bundle(); string(b) != "a\nb\n" {
		t.Errorf("Bundle = %q", b)
	}
}
