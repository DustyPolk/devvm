// Package ubuntu pins Ubuntu's container image and cloud image for base.
// The cloud image is Lima's: a pinned dated release of the series, and its
// image's SHA-256 for each architecture. Finding it reads that release's
// SHA256SUMS, which must list the pinned checksum; when gpgv is installed,
// SHA256SUMS must also be signed by Ubuntu's cloud image key, pinned in
// devvm's own files (layers/ubuntu-cloud.gpg).
//
// The image itself isn't downloaded here: the backend fetches it and checks
// it against the digest.
package ubuntu

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/fetch"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

const (
	// Series is the Ubuntu release base is built on, by its code name.
	Series = "resolute"
	// Version is Series' version number, as Ubuntu's image names have it.
	Version = "26.04"
	// DefaultURL is where the series' dated releases are listed.
	DefaultURL = "https://cloud-images.ubuntu.com/releases/" + Series
	// Keyring is Ubuntu's cloud image signing key, by its path in devvm's
	// own files.
	Keyring = "layers/ubuntu-cloud.gpg"
	// ContainerRepo is Ubuntu's container image, which smolvm's base
	// starts from without --from: Version's, by ContainerDigest.
	ContainerRepo = "docker.io/library/ubuntu"
)

// ContainerDigest pins ContainerRepo's image: its index's digest, every
// platform's. scripts/bump.sh moves it with layers/base.sh's package
// snapshot, to an image built before the snapshot: one with newer packages
// than the snapshot has leaves apt unable to install what depends on their
// exact versions.
const ContainerDigest = "sha256:19fcd1bf268f02b60aca3bd30bd8a6d9a11759a430e32921833e65f16d87348a"

// ContainerRef is the pinned image by its digest, as it's pulled.
const ContainerRef = ContainerRepo + "@" + ContainerDigest

// ContainerOrigin is how base's record names the pinned image: its tag and
// digest.
const ContainerOrigin = ContainerRepo + ":" + Version + "@" + ContainerDigest

// CloudRelease pins the cloud image Lima's base starts from: Series' dated
// release on DefaultURL. scripts/bump.sh moves it with layers/base.sh's
// package snapshot, to the newest release dated before the snapshot, for
// the reason ContainerDigest moves with it.
const CloudRelease = "release-20261016"

// CloudSHA256Arm64 is CloudRelease's arm64 image's SHA-256, from its
// signed SHA256SUMS.
const CloudSHA256Arm64 = "8eb876c2ccc4b6ff89e4a65d511815923b285e04503ba465809bf587f763da24"

// CloudSHA256Amd64 is CloudRelease's amd64 image's SHA-256, from its
// signed SHA256SUMS.
const CloudSHA256Amd64 = "5645d0318ad21f643ebb1ac4567514620e9893380b07b0201fda163baa5b7eb5"

// CloudDigest is CloudRelease's image's digest for an architecture (arm64,
// amd64), sha256:<hex>; "" for one without an image.
func CloudDigest(arch string) string {
	switch arch {
	case "arm64":
		return "sha256:" + CloudSHA256Arm64
	case "amd64":
		return "sha256:" + CloudSHA256Amd64
	}
	return ""
}

// CloudPin is the pinned cloud image for an architecture, as base's key on
// Lima hashes it: "<release> <file> <digest>".
func CloudPin(arch string) string {
	return CloudRelease + " " + cloudFile(arch) + " " + CloudDigest(arch)
}

func cloudFile(arch string) string {
	return "ubuntu-" + Version + "-server-cloudimg-" + arch + ".img"
}

const (
	timeout = 60 * time.Second // per request, body included
	maxBody = 4 << 20          // the listing and SHA256SUMS are a few KB
	retries = 3
)

// Image is Ubuntu's cloud image for the host.
type Image struct {
	URL    string // the image file's address
	Digest string // sha256:<64 hex digits>, from the release's SHA256SUMS
}

// String is how base's record keeps it: "URL sha256:DIGEST".
func (i Image) String() string { return i.URL + " " + i.Digest }

// Finder looks up the image. New fills it in; tests change its fields.
type Finder struct {
	// URL holds the dated releases (release-YYYYMMDD/), each with its
	// SHA256SUMS and SHA256SUMS.gpg. http(s):// or file://.
	URL string
	// Release is the dated release to use: CloudRelease.
	Release string
	// Digest is the release's image's for Arch, sha256:<hex>: CloudDigest's;
	// "" when there's none.
	Digest string
	// Assets holds devvm's own files, Keyring among them.
	Assets fs.FS
	// Arch is the host's, as Go and Ubuntu's image names have it (arm64,
	// amd64).
	Arch string
	// Client fetches the listing and the checksums.
	Client *fetch.Client
	// RetryDelay is the wait before the first retry; it doubles after each.
	RetryDelay time.Duration
	// GPGV is gpgv's path, "" when it isn't installed.
	GPGV string
	// Run runs gpgv into the run log.
	Run func(ctx context.Context, cmd *exec.Cmd) error
	// Note prints a note.
	Note func(msg.M)
}

// UI is the run log gpgv runs into, and where a Finder prints.
type UI interface {
	LoggedCmd(ctx context.Context, step msg.M, cmd *exec.Cmd) error
	Note(msg.M)
}

// New returns a Finder with devvm's own files (assets), at url
// (DefaultURL when it's "", a file:// folder in tests), running gpgv and
// printing through u.
func New(assets fs.FS, url string, u UI) *Finder {
	if url == "" {
		url = DefaultURL
	}
	gpgv, err := exec.LookPath("gpgv")
	if err != nil {
		gpgv = ""
	}
	return &Finder{
		URL:        strings.TrimRight(url, "/"),
		Release:    CloudRelease,
		Digest:     CloudDigest(runtime.GOARCH),
		Assets:     assets,
		Arch:       runtime.GOARCH,
		Client:     NewClient(),
		RetryDelay: time.Second,
		GPGV:       gpgv,
		Run:        func(ctx context.Context, cmd *exec.Cmd) error { return u.LoggedCmd(ctx, msg.M{}, cmd) },
		Note:       u.Note,
	}
}

// NewClient returns the HTTP client a Finder uses: a timeout per request,
// file:// URLs read from disk, and no redirects to another scheme.
func NewClient() *fetch.Client {
	c := fetch.NewClient(timeout, fetch.SameScheme)
	if t := c.Transport(); t != nil {
		// Only DEVVM_UBUNTU_URL can name a file: redirects can't switch to it.
		t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/"))) //nolint:gosec // file:// URLs are absolute paths
	}
	return c
}

// File is the image's file name in a release.
func (f *Finder) File() string { return cloudFile(f.Arch) }

// Image returns the pinned release's image for the host, with its pinned
// digest, once the release's SHA256SUMS (signature-checked when gpgv is
// installed) lists that digest for it. It fails with no_ubuntu_image when
// the host's architecture has no pin or the release's SHA256SUMS can't be
// fetched, ubuntu_bad_signature when they aren't signed by Ubuntu's key,
// and ubuntu_checksum_unpinned when they list another checksum.
func (f *Finder) Image(ctx context.Context) (Image, error) {
	if f.Digest == "" {
		return Image{}, msg.NoUbuntuImage(f.Release, f.URL, Version)
	}
	sums, err := f.fetch(ctx, f.URL+"/"+f.Release+"/SHA256SUMS")
	if err != nil {
		if ctx.Err() != nil {
			return Image{}, ctx.Err()
		}
		return Image{}, msg.NoUbuntuImage(f.Release, f.URL, Version)
	}
	if err := f.verify(ctx, f.Release, sums); err != nil {
		return Image{}, err
	}
	file := f.File()
	if "sha256:"+Checksum(sums, file) != f.Digest {
		return Image{}, msg.UbuntuChecksumUnpinned(file, f.Release, f.URL)
	}
	return Image{URL: f.URL + "/" + f.Release + "/" + file, Digest: f.Digest}, nil
}

var release = regexp.MustCompile(`release-[0-9]{8}`)

// ReleaseDate is the date of the dated release an image's address names
// (20260101 for …/release-20260101/…), "" when it names none.
func ReleaseDate(url string) string {
	r := release.FindString(url)
	return strings.TrimPrefix(r, "release-")
}

var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Checksum returns file's SHA-256 in a SHA256SUMS file ("<hex> *<file>" or
// "<hex>  <file>" lines), or "" when it has none. Anything that isn't 64
// lowercase hex digits isn't a checksum, and is skipped: it goes into the
// backend's config.
func Checksum(sums []byte, file string) string {
	for line := range strings.Lines(string(sums)) {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.TrimPrefix(fields[1], "*") != file {
			continue
		}
		if digest.MatchString(fields[0]) {
			return fields[0]
		}
	}
	return ""
}

// verify checks that sums, release's SHA256SUMS, is signed by Ubuntu's
// key, with gpgv in a home of its own, so no other key counts; the key is
// written beside them. Without gpgv it notes that the signature goes
// unchecked; the pinned SHA-256 still decides.
func (f *Finder) verify(ctx context.Context, release string, sums []byte) error {
	if f.GPGV == "" {
		f.Note(msg.UbuntuUnsigned())
		return nil
	}
	bad := msg.UbuntuBadSignature(release, f.URL)
	sig, err := f.fetch(ctx, f.URL+"/"+release+"/SHA256SUMS.gpg")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return bad
	}
	dir, err := os.MkdirTemp("", "devvm-ubuntu.")
	if err != nil {
		return fmt.Errorf("checking Ubuntu's checksums: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	key, err := fs.ReadFile(f.Assets, Keyring)
	if err != nil {
		return fmt.Errorf("checking Ubuntu's checksums: %w", err)
	}
	sumsFile, sigFile, home := filepath.Join(dir, "SHA256SUMS"), filepath.Join(dir, "SHA256SUMS.gpg"), filepath.Join(dir, "gnupg")
	keyring := filepath.Join(dir, filepath.Base(Keyring))
	for file, data := range map[string][]byte{sumsFile: sums, sigFile: sig, keyring: key} {
		if err := state.WriteFile(file, data); err != nil {
			return fmt.Errorf("checking Ubuntu's checksums: %w", err)
		}
	}
	if err := os.Mkdir(home, 0o700); err != nil {
		return fmt.Errorf("checking Ubuntu's checksums: %w", err)
	}
	cmd := exec.CommandContext(ctx, f.GPGV, "--keyring", keyring, sigFile, sumsFile) //nolint:gosec // gpgv, found on PATH
	cmd.Env = append(os.Environ(), "GNUPGHOME="+home)
	if err := f.Run(ctx, cmd); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return bad
	}
	return nil
}

// fetch returns what's at u, trying again after failures that may pass
// (fetch.Transient: no answer, 408, 429, 5xx).
func (f *Finder) fetch(ctx context.Context, u string) ([]byte, error) {
	var body []byte
	err := fetch.Retry(ctx, retries, f.RetryDelay, func() (bool, error) {
		var err error
		body, err = fetch.Get(ctx, f.Client, u, maxBody)
		return fetch.Transient(err), err
	})
	if err != nil {
		return nil, err
	}
	return body, nil
}
