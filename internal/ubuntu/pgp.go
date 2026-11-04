package ubuntu

import (
	"bufio"
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"slices"
	"strings"
	"time"
)

// An OpenPGP signature's check (RFC 4880), as much of it as Ubuntu's
// signed files need and no more: version 4 signatures by an RSA key of a
// pinned keyring, hashed with SHA-256 or a stronger SHA-2, either detached
// over binary data (the cloud images' SHA256SUMS) or over a clearsigned
// message's text (the archive's InRelease). The cryptography is the
// standard library's; this only reads the packets. Anything else is
// refused.

// errBadSignature is every refusal: the caller decides what to say.
var errBadSignature = errors.New("bad OpenPGP signature")

// OpenPGP's packet tags and algorithm numbers that this reads.
const (
	tagSignature = 2
	tagPublicKey = 6
	tagSubkey    = 14

	pubRSA         = 1
	pubRSASignOnly = 3

	sigBinary = 0x00
	sigText   = 0x01

	maxSignatures = 4 // a signature's most packets that verify tries

	subCreated  = 2
	subExpires  = 3
	subIssuer   = 16
	subIssuerFP = 33
)

// hashAlgo is a hash algorithm a signature may use.
type hashAlgo struct {
	id  crypto.Hash
	new func() hash.Hash
}

// hashOf is the hash algorithm OpenPGP's number n names, when a signature
// may use it: SHA-256 or a stronger SHA-2.
func hashOf(n byte) (hashAlgo, bool) {
	switch n {
	case 8:
		return hashAlgo{crypto.SHA256, sha256.New}, true
	case 9:
		return hashAlgo{crypto.SHA384, sha512.New384}, true
	case 10:
		return hashAlgo{crypto.SHA512, sha512.New}, true
	}
	return hashAlgo{}, false
}

// packet is one OpenPGP packet: its tag and body.
type packet struct {
	tag  int
	body []byte
}

// packets splits data into its packets. Partial lengths (streamed
// packets) aren't used by keys or signatures, and are refused.
func packets(data []byte) ([]packet, error) {
	var out []packet
	for len(data) > 0 {
		first := data[0]
		if first&0x80 == 0 {
			return nil, errBadSignature
		}
		var tag, n, hdr int
		if first&0x40 != 0 { // the new format
			tag = int(first & 0x3f)
			if len(data) < 2 {
				return nil, errBadSignature
			}
			switch l := int(data[1]); {
			case l < 192:
				n, hdr = l, 2
			case l < 224:
				if len(data) < 3 {
					return nil, errBadSignature
				}
				n, hdr = (l-192)<<8+int(data[2])+192, 3
			case l == 255:
				if len(data) < 6 {
					return nil, errBadSignature
				}
				n, hdr = int(binary.BigEndian.Uint32(data[2:6])), 6
			default:
				return nil, errBadSignature
			}
		} else { // the old format
			tag = int(first>>2) & 0x0f
			switch first & 3 {
			case 0:
				if len(data) < 2 {
					return nil, errBadSignature
				}
				n, hdr = int(data[1]), 2
			case 1:
				if len(data) < 3 {
					return nil, errBadSignature
				}
				n, hdr = int(binary.BigEndian.Uint16(data[1:3])), 3
			case 2:
				if len(data) < 5 {
					return nil, errBadSignature
				}
				n, hdr = int(binary.BigEndian.Uint32(data[1:5])), 5
			default:
				n, hdr = len(data)-1, 1
			}
		}
		if n < 0 || hdr+n > len(data) {
			return nil, errBadSignature
		}
		out = append(out, packet{tag: tag, body: data[hdr : hdr+n]})
		data = data[hdr+n:]
	}
	return out, nil
}

// mpi reads a multiprecision integer from the start of b: its bytes and
// what follows it.
func mpi(b []byte) (n, rest []byte, err error) {
	if len(b) < 2 {
		return nil, nil, errBadSignature
	}
	bits := int(binary.BigEndian.Uint16(b))
	size := (bits + 7) / 8
	if len(b) < 2+size {
		return nil, nil, errBadSignature
	}
	return b[2 : 2+size], b[2+size:], nil
}

// rsaKeys reads the RSA keys, primary keys and subkeys, of a keyring's
// packets (binary, as assets' ubuntu-cloud.gpg is). Its user IDs and
// self-signatures go unread: the keyring is pinned in devvm's own files.
func rsaKeys(keyring []byte) ([]*rsa.PublicKey, error) {
	ps, err := packets(keyring)
	if err != nil {
		return nil, err
	}
	var keys []*rsa.PublicKey
	for _, p := range ps {
		if p.tag != tagPublicKey && p.tag != tagSubkey {
			continue
		}
		// version 4, its creation time, its algorithm, n, e
		b := p.body
		if len(b) < 6 || b[0] != 4 || (b[5] != pubRSA && b[5] != pubRSASignOnly) {
			continue
		}
		n, rest, err := mpi(b[6:])
		if err != nil {
			return nil, err
		}
		e, _, err := mpi(rest)
		if err != nil {
			return nil, err
		}
		if len(e) == 0 || len(e) > 4 {
			return nil, errBadSignature
		}
		exp := 0
		for _, x := range e {
			exp = exp<<8 | int(x)
		}
		keys = append(keys, &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp})
	}
	if len(keys) == 0 {
		return nil, errors.New("the keyring has no RSA key")
	}
	return keys, nil
}

// signature is a version 4 signature packet, read.
type signature struct {
	hash   byte   // the hash algorithm, by OpenPGP's number
	signed []byte // from its version to the end of its hashed subpackets
	left16 []byte // the digest's first two bytes
	value  []byte // the RSA signature
}

// readSignature reads a signature packet's body, refusing any but a
// version 4 RSA signature of sigType (sigBinary or sigText) with a hash of
// hashOf's, and one with a critical subpacket this doesn't know or that
// has expired at now.
func readSignature(b []byte, sigType byte, now time.Time) (signature, error) {
	if len(b) < 6 || b[0] != 4 || b[1] != sigType || (b[2] != pubRSA && b[2] != pubRSASignOnly) {
		return signature{}, errBadSignature
	}
	if _, ok := hashOf(b[3]); !ok {
		return signature{}, errBadSignature
	}
	hashedLen := int(binary.BigEndian.Uint16(b[4:6]))
	if len(b) < 6+hashedLen+2 {
		return signature{}, errBadSignature
	}
	s := signature{hash: b[3], signed: b[:6+hashedLen]}
	if err := checkSubpackets(b[6:6+hashedLen], now); err != nil {
		return signature{}, err
	}
	rest := b[6+hashedLen:]
	unhashedLen := int(binary.BigEndian.Uint16(rest))
	if len(rest) < 2+unhashedLen+2 {
		return signature{}, errBadSignature
	}
	rest = rest[2+unhashedLen:]
	s.left16 = rest[:2]
	var err error
	if s.value, _, err = mpi(rest[2:]); err != nil {
		return signature{}, err
	}
	return s, nil
}

// checkSubpackets reads a signature's hashed subpackets: one marked
// critical must be one this knows, and an expiry must not have passed.
func checkSubpackets(b []byte, now time.Time) error {
	var created, expires uint32
	for len(b) > 0 {
		var n, hdr int
		switch l := int(b[0]); {
		case l < 192:
			n, hdr = l, 1
		case l < 255:
			if len(b) < 2 {
				return errBadSignature
			}
			n, hdr = (l-192)<<8+int(b[1])+192, 2
		default:
			if len(b) < 5 {
				return errBadSignature
			}
			n, hdr = int(binary.BigEndian.Uint32(b[1:5])), 5
		}
		if n < 1 || hdr+n > len(b) {
			return errBadSignature
		}
		typ, data := b[hdr], b[hdr+1:hdr+n]
		critical, kind := typ&0x80 != 0, typ&0x7f
		switch {
		case kind == subCreated && len(data) == 4:
			created = binary.BigEndian.Uint32(data)
		case kind == subExpires && len(data) == 4:
			expires = binary.BigEndian.Uint32(data)
		case critical && kind != subIssuer && kind != subIssuerFP:
			return errBadSignature
		}
		b = b[hdr+n:]
	}
	if expires != 0 && now.Unix() > int64(created)+int64(expires) {
		return errBadSignature
	}
	return nil
}

// verifyDetached checks that sig, a detached signature (binary, or
// ASCII-armored as Ubuntu's SHA256SUMS.gpg is), holds at least one
// signature of data by a key of keyring that checks out, at now. It
// returns errBadSignature when none does.
func verifyDetached(keyring, sig, data []byte, now time.Time) error {
	return verify(keyring, sig, data, sigBinary, now)
}

// verify checks that sig, binary or armored, holds at least one signature
// of sigType over data by a key of keyring that checks out, at now.
func verify(keyring, sig, data []byte, sigType byte, now time.Time) error {
	keys, err := rsaKeys(keyring)
	if err != nil {
		return err
	}
	if sig, err = dearmor(sig); err != nil {
		return err
	}
	ps, err := packets(sig)
	if err != nil {
		return err
	}
	// Each signature hashes all of data, so only so many are tried: Ubuntu
	// signs with one or two keys.
	ps = slices.DeleteFunc(ps, func(p packet) bool { return p.tag != tagSignature })
	if len(ps) > maxSignatures {
		return errBadSignature
	}
	for _, p := range ps {
		s, err := readSignature(p.body, sigType, now)
		if err != nil {
			continue
		}
		h, _ := hashOf(s.hash)
		d := h.new()
		d.Write(data)
		d.Write(s.signed)
		// The trailer: version 4, 0xff, the length of what's signed.
		trailer := []byte{4, 0xff, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(trailer[2:], uint32(len(s.signed))) //nolint:gosec // at most 6 + 65535
		d.Write(trailer)
		digest := d.Sum(nil)
		if !bytes.Equal(digest[:2], s.left16) {
			continue
		}
		for _, k := range keys {
			value := s.value
			// A leading zero byte is dropped from an MPI: put it back.
			if size := k.Size(); len(value) < size {
				value = append(make([]byte, size-len(value)), value...)
			}
			if rsa.VerifyPKCS1v15(k, h.id, digest, value) == nil {
				return nil
			}
		}
	}
	return errBadSignature
}

const (
	clearBegin = "-----BEGIN PGP SIGNED MESSAGE-----"
	sigBegin   = "-----BEGIN PGP SIGNATURE-----"
	sigEnd     = "-----END PGP SIGNATURE-----"
)

// verifyClearsigned checks that msg, a clearsigned message (RFC 4880's
// cleartext signature framework, as an InRelease is), holds a signature of
// its text by a key of keyring that checks out at now, and returns that
// text: its lines dash-unescaped and without trailing spaces or tabs, as
// they were signed, each ending in "\n". Anything before the message's
// first line or after its signature, a header other than Hash, or a line
// starting with a dash that isn't escaped is refused.
func verifyClearsigned(keyring, msg []byte, now time.Time) ([]byte, error) {
	lines := strings.SplitAfter(string(msg), "\n")
	if len(lines) == 0 || trimLine(lines[0]) != clearBegin {
		return nil, errBadSignature
	}
	i := 1
	for ; i < len(lines) && trimLine(lines[i]) != ""; i++ {
		if !strings.HasPrefix(lines[i], "Hash: ") {
			return nil, errBadSignature
		}
	}
	var text []string
	for i++; i < len(lines); i++ {
		line := lines[i]
		if trimLine(line) == sigBegin {
			break
		}
		if strings.HasPrefix(line, "-") {
			var ok bool
			if line, ok = strings.CutPrefix(line, "- "); !ok {
				return nil, errBadSignature
			}
		}
		text = append(text, trimLine(line))
	}
	sig, err := signatureBlock(lines[min(i, len(lines)):])
	if err != nil {
		return nil, err
	}
	// What's signed: the lines joined by CR LF, without the last's.
	if err := verify(keyring, sig, []byte(strings.Join(text, "\r\n")), sigText, now); err != nil {
		return nil, err
	}
	if len(text) == 0 {
		return nil, nil
	}
	return []byte(strings.Join(text, "\n") + "\n"), nil
}

// signatureBlock is lines, a clearsigned message's from its signature's
// first line on, up to the signature's end; anything after it but blank
// lines is refused.
func signatureBlock(lines []string) ([]byte, error) {
	if len(lines) == 0 {
		return nil, errBadSignature
	}
	for j, line := range lines {
		if trimLine(line) != sigEnd {
			continue
		}
		for _, rest := range lines[j+1:] {
			if trimLine(rest) != "" {
				return nil, errBadSignature
			}
		}
		return []byte(strings.Join(lines[:j+1], "")), nil
	}
	return nil, errBadSignature
}

// trimLine is a line without its line ending and trailing spaces or tabs,
// which a cleartext signature doesn't cover.
func trimLine(line string) string { return strings.TrimRight(line, " \t\r\n") }

// dearmor returns an ASCII-armored signature's bytes (RFC 4880's radix-64,
// its checksum checked), or sig itself when it's binary.
func dearmor(sig []byte) ([]byte, error) {
	if len(sig) > 0 && sig[0]&0x80 != 0 {
		return sig, nil
	}
	const begin, end = sigBegin, sigEnd
	sc := bufio.NewScanner(bytes.NewReader(sig))
	state := 0 // 0: before begin, 1: headers, 2: body
	var body strings.Builder
	sum := ""
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t\r")
		switch {
		case state == 0 && line == begin:
			state = 1
		case state == 1 && line == "":
			state = 2
		case state == 1 && !strings.Contains(line, ": "):
			// No headers: this is the body's first line.
			state = 2
			body.WriteString(line)
		case state == 2 && line == end:
			return decodeArmor(body.String(), sum)
		case state == 2 && strings.HasPrefix(line, "="):
			sum = line[1:]
		case state == 2:
			body.WriteString(line)
		}
	}
	return nil, errBadSignature
}

// decodeArmor decodes an armored body, checking its CRC-24 when there
// is one.
func decodeArmor(body, sum string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(body)
	if err != nil || len(data) == 0 {
		return nil, errBadSignature
	}
	if sum != "" {
		want, err := base64.StdEncoding.DecodeString(sum)
		if err != nil || len(want) != 3 {
			return nil, errBadSignature
		}
		if !bytes.Equal(want, crc24(data)) {
			return nil, fmt.Errorf("%w: its armor's checksum", errBadSignature)
		}
	}
	return data, nil
}

// crc24 is OpenPGP's armor checksum, as its 3 bytes.
func crc24(data []byte) []byte {
	const init, poly = 0xB704CE, 0x1864CFB
	c := uint32(init)
	for _, b := range data {
		c ^= uint32(b) << 16
		for range 8 {
			c <<= 1
			if c&0x1000000 != 0 {
				c ^= poly
			}
		}
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], c&0xFFFFFF)
	return b[1:]
}
