//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // G505: a key's identifier, as RFC 5280's method 1 has it, not a signature
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// CA is the run's throwaway certificate authority, never one of yours:
// its certificate, PEM, and a server certificate
// it signed for host.lima.internal, localhost and 127.0.0.1, which Serve
// serves, in Dir. A run makes it once, the first time a section asks, and
// deletes it at its end, with devvm's CA folder (Trust).
type CA struct {
	Dir   string // the CA's files: ca.pem, srv.pem and srv.key, and what Sign makes
	PEM   string // the CA's certificate
	cert  *x509.Certificate
	key   *rsa.PrivateKey
	srv   *http.Server
	paths Paths
	ctx   context.Context // the run's, which Serve listens in
}

// NewCA is the run's CA, made the first time a section asks.
func NewCA(s *S) (*CA, error) {
	if s.sb.ca != nil {
		return s.sb.ca, nil
	}
	dir := filepath.Join(s.sb.work, "e2e-ca")
	madeFile(s.sb, dir)
	madeFile(s.sb, s.Paths.CAs)
	c, err := makeCA(dir, s.Paths)
	if err != nil {
		return nil, err
	}
	c.ctx = s.r.ctx
	s.Defer(func(context.Context) { c.StopServing() })
	s.sb.ca = c
	return c, nil
}

// makeCA makes a CA in dir, an earlier one there replaced: the CA's
// certificate, valid two days, and srv's.
func makeCA(dir string, p Paths) (*CA, error) {
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	tmpl := certTemplate("devvm e2e CA", &key.PublicKey)
	tmpl.IsCA, tmpl.BasicConstraintsValid = true, true
	tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	c := &CA{Dir: dir, PEM: filepath.Join(dir, "ca.pem"), cert: cert, key: key, paths: p}
	if err := writePEM(c.PEM, "CERTIFICATE", der); err != nil {
		return nil, err
	}
	if _, _, err := c.Sign("srv", "host.lima.internal", "localhost", "127.0.0.1"); err != nil {
		return nil, err
	}
	return c, nil
}

// certTemplate is a certificate for cn's key pub: valid from an hour ago
// (a VM's clock may lag) for two days, its key's identifier set.
func certTemplate(cn string, pub *rsa.PublicKey) *x509.Certificate {
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	ski := sha1.Sum(x509.MarshalPKCS1PublicKey(pub)) //nolint:gosec // G401: an identifier, not a signature
	now := time.Now()
	return &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: cn},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(48 * time.Hour), SubjectKeyId: ski[:],
	}
}

// Sign makes a server certificate the CA signed, for hosts (names, or
// addresses), as name.pem and name.key in Dir, and is their paths.
func (c *CA) Sign(name string, hosts ...string) (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	tmpl := certTemplate(hosts[0], &key.PublicKey)
	tmpl.BasicConstraintsValid = true
	tmpl.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		return "", "", err
	}
	cf, kf := filepath.Join(c.Dir, name+".pem"), filepath.Join(c.Dir, name+".key")
	if err := writePEM(cf, "CERTIFICATE", der); err != nil {
		return "", "", err
	}
	return cf, kf, writePEM(kf, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key))
}

func writePEM(f, kind string, der []byte) error {
	return os.WriteFile(f, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600)
}

// Line is line n of the CA's PEM, from 1: a line of its base64, which a
// check looks for in a VM's bundle.
func (c *CA) Line(n int) string { return pemLine(c.PEM, n) }

// Trust puts the CA in devvm's CA folder, as e2e.pem: VMs trust it from
// their next launch, and builds while they run.
func (c *CA) Trust() error {
	b, err := os.ReadFile(c.PEM)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.paths.CAs, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.paths.CAs, "e2e.pem"), b, 0o600)
}

// Untrust deletes devvm's CA folder.
func (c *CA) Untrust() error { return os.RemoveAll(c.paths.CAs) }

// Serve serves HTTPS with srv's certificate on port, on every address,
// answering 200 to anything, until StopServing or the run's end. One serves at a time: Serve again does
// nothing.
func (c *CA) Serve(port int) error {
	if c.srv != nil {
		return nil
	}
	pair, err := tls.LoadX509KeyPair(filepath.Join(c.Dir, "srv.pem"), filepath.Join(c.Dir, "srv.key"))
	if err != nil {
		return err
	}
	// On every address, so a VM reaches it as host.lima.internal.
	ln, err := (&net.ListenConfig{}).Listen(c.ctx, "tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return err
	}
	c.srv = &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("devvm e2e\n")) }),
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func(srv *http.Server) {
		if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = ln.Close()
		}
	}(c.srv)
	return nil
}

// StopServing ends Serve's server, so its port is free for later
// sections.
func (c *CA) StopServing() {
	if c.srv != nil {
		_ = c.srv.Close()
		c.srv = nil
	}
}

// GoneFrom fails unless the running VM has none of devvm's CA files and
// the CA isn't in its system's bundle: what an image keeps.
func (c *CA) GoneFrom(s *S, vm string) error {
	_, err := s.Run(Cmd{Args: []string{
		"limactl", "shell", "--workdir", "/", vm, "sh", "-c",
		`! ls /usr/local/share/ca-certificates/devvm-* >/dev/null 2>&1 &&
    [ ! -e /etc/ssl/devvm.pem ] && [ ! -e /var/lib/devvm-ca ] && ! grep -qF "$1" /etc/ssl/certs/ca-certificates.crt`,
		"_", c.Line(4),
	}})
	if err != nil {
		return errors.New(vm + " keeps devvm's CA files or the run's CA")
	}
	return nil
}
