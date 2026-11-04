package trust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/w3cj/devvm/internal/state"
)

// devvm's own CA, which the network policy proxy intercepts with, lasts
// caLife and is replaced when it has less than caRenew left: a key that
// leaks (in a backup, say) stops working within weeks. VMs trust a new CA
// from their next launch, which starts their proxy again with it.
const (
	caLife  = 30 * 24 * time.Hour
	caRenew = 7 * 24 * time.Hour
)

// InitCA writes devvm's CA into dir (paths.Dirs.CA) unless a valid one with
// over caRenew left is there: ca.pem, the certificate, mode 644, and
// ca.key, its ECDSA key ("EC PRIVATE KEY"), mode 600, each written whole.
// One valid for longer than caLife is replaced too. The CA names no host:
// every VM sees it.
func InitCA(dir string) error {
	certPath, keyPath := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key")
	if current(certPath, keyPath) {
		return nil
	}
	// crypto/rand never fails: a system whose randomness does crashes Go.
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "devvm proxy CA", Organization: []string{"devvm"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(caLife),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	kder, kerr := x509.MarshalECPrivateKey(key)
	if err := errors.Join(err, kerr); err != nil {
		return err
	}
	// The key first: a crash between the two leaves a pair that doesn't
	// match, which the next InitCA replaces.
	if err := state.WriteFileMode(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder}), 0o600); err != nil {
		return err
	}
	return state.WriteFileMode(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// current: the CA in certPath and keyPath is a pair, with over caRenew
// left and a life of at most caLife.
func current(certPath, keyPath string) bool {
	if _, err := os.Stat(keyPath); err != nil {
		return false
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return false
	}
	c, err := x509.ParseCertificate(pair.Certificate[0])
	return err == nil && time.Until(c.NotAfter) > caRenew && c.NotAfter.Sub(c.NotBefore) <= caLife+time.Hour
}
