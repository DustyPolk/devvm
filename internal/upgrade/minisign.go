package upgrade

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// minisign's formats (https://jedisct1.github.io/minisign/): a public
// key is base64 of "Ed", its 8-byte key ID and its Ed25519 public key; a
// signature file is an untrusted comment line, base64 of the algorithm,
// the key ID and the signature, a trusted comment line, and base64 of the
// global signature, over the signature and the trusted comment.
const (
	keyAlg       = "Ed"
	prehashedAlg = "ED" // the signature is over the file's BLAKE2b-512, minisign's default
	keyIDLen     = 8
	pubKeyLen    = 2 + keyIDLen + ed25519.PublicKeySize
	sigLen       = 2 + keyIDLen + ed25519.SignatureSize

	untrustedPrefix = "untrusted comment: "
	trustedPrefix   = "trusted comment: "
)

var (
	// errKey is a public key that isn't minisign's.
	errKey = errors.New("not a minisign public key")
	// errSignature is a signature file that isn't minisign's, or that
	// doesn't verify.
	errSignature = errors.New("the signature doesn't verify")
)

// A PublicKey is a minisign public key: its key ID and its Ed25519 key.
type PublicKey struct {
	ID  [keyIDLen]byte
	Key ed25519.PublicKey
}

// ParsePublicKey reads a minisign public key: its base64 line, as
// minisign -P takes it, or a minisign.pub file, whose second line it is.
func ParsePublicKey(s string) (PublicKey, error) {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	line := lines[0]
	if len(lines) == 2 && strings.HasPrefix(lines[0], untrustedPrefix) {
		line = lines[1]
	} else if len(lines) != 1 {
		return PublicKey{}, errKey
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(line))
	if err != nil || len(raw) != pubKeyLen || string(raw[:2]) != keyAlg {
		return PublicKey{}, errKey
	}
	var k PublicKey
	copy(k.ID[:], raw[2:2+keyIDLen])
	k.Key = ed25519.PublicKey(bytes.Clone(raw[2+keyIDLen:]))
	return k, nil
}

// Verify checks sig, a minisign signature file, against data, signed with
// k: a prehashed signature (ED) by k's key ID over data's BLAKE2b-512, and
// its global signature over the signature and the trusted comment, which
// it returns. A legacy signature (Ed, over the data itself) is refused,
// as minisign's docs say a new implementation must do by default.
func (k PublicKey) Verify(data, sig []byte) (trusted string, err error) {
	lines := strings.Split(strings.TrimRight(string(sig), "\n"), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], untrustedPrefix) || !strings.HasPrefix(lines[2], trustedPrefix) {
		return "", fmt.Errorf("%w: not a minisign signature file", errSignature)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(raw) != sigLen {
		return "", fmt.Errorf("%w: not a minisign signature", errSignature)
	}
	if string(raw[:2]) != prehashedAlg {
		return "", fmt.Errorf("%w: not a prehashed (ED) signature", errSignature)
	}
	if !bytes.Equal(raw[2:2+keyIDLen], k.ID[:]) {
		return "", fmt.Errorf("%w: made with another key", errSignature)
	}
	signature := raw[2+keyIDLen:]
	sum := blake2b.Sum512(data)
	if len(k.Key) != ed25519.PublicKeySize || !ed25519.Verify(k.Key, sum[:], signature) {
		return "", errSignature
	}
	trusted = strings.TrimSuffix(strings.TrimPrefix(lines[2], trustedPrefix), "\r")
	global, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[3]))
	if err != nil || len(global) != ed25519.SignatureSize ||
		!ed25519.Verify(k.Key, append(bytes.Clone(signature), trusted...), global) {
		return "", fmt.Errorf("%w: the trusted comment's signature", errSignature)
	}
	return trusted, nil
}
