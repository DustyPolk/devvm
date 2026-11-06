// Package ubuntu finds Ubuntu's cloud image for base: the newest dated
// release of the series that has an image for the host, read from
// cloud-images.ubuntu.com's listing. A release counts only when its
// SHA256SUMS carry a signature from Ubuntu's cloud image key, pinned in
// devvm's own files (layers/ubuntu-cloud.gpg), which devvm checks itself
// (pgp.go), and list the image: the image's SHA-256 is then the one they
// list. What's found is pinned on this machine (state.UbuntuPin), and
// base's key hashes the pin.
//
// The image itself isn't downloaded here: the backend fetches it and checks
// it against the pinned digest.
package ubuntu

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/devvmhq/devvm/internal/fetch"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/state"
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
)

// File is the cloud image's file name in a release, for an architecture
// as Go names it (arm64, amd64).
func File(arch string) string {
	return "ubuntu-" + Version + "-server-cloudimg-" + arch + ".img"
}

// PinArg is a pin as base's recipe on Lima holds it, which its key hashes:
// "<release> <file> <digest>".
func PinArg(p state.UbuntuPin) string {
	return p.Release + " " + p.File + " " + p.Digest
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

// ImageOf is p's image at url, where the dated releases are listed.
func ImageOf(url string, p state.UbuntuPin) Image {
	return Image{URL: strings.TrimRight(url, "/") + "/" + p.Release + "/" + p.File, Digest: p.Digest}
}

// Finder finds the image. New fills it in; tests change its fields.
type Finder struct {
	// URL lists the dated releases (release-YYYYMMDD/), each with its
	// SHA256SUMS and SHA256SUMS.gpg. http(s):// or file://.
	URL string
	// Assets holds devvm's own files, Keyring among them.
	Assets fs.FS
	// Arch is the host's, as Go and Ubuntu's image names have it (arm64,
	// amd64).
	Arch string
	// Client fetches the listing and the checksums.
	Client *fetch.Client
	// RetryDelay is the wait before the first retry; it doubles after each.
	RetryDelay time.Duration
	// Now is the time a signature's expiry is checked at.
	Now func() time.Time
}

// New returns a Finder with devvm's own files (assets), at url
// (DefaultURL when it's "", a file:// folder in tests).
func New(assets fs.FS, url string) *Finder {
	return &Finder{
		URL:        strings.TrimRight(cmp.Or(url, DefaultURL), "/"),
		Assets:     assets,
		Arch:       runtime.GOARCH,
		Client:     NewClient(),
		RetryDelay: time.Second,
		Now:        time.Now,
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

// errNoImage is a release whose checksums list no image for the host.
var errNoImage = errors.New("no image for this host")

// Newest returns the newest dated release, older than before
// (release-YYYYMMDD; "" for any), whose signed checksums list an image for
// the host, with that image's SHA-256: releases without one are passed
// over. It fails with ubuntu_unreachable when the listing can't be read,
// no_ubuntu_release when no release has an image, and with the first
// release's refusal (no_ubuntu_image, ubuntu_bad_signature) otherwise: a
// bad signature is never passed over for an older release.
func (f *Finder) Newest(ctx context.Context, before string) (state.UbuntuPin, error) {
	releases, err := f.releases(ctx)
	if err != nil {
		return state.UbuntuPin{}, err
	}
	for _, r := range releases {
		if before != "" && r >= before {
			continue
		}
		p, err := f.check(ctx, r)
		if errors.Is(err, errNoImage) {
			continue
		}
		return p, err
	}
	return state.UbuntuPin{}, msg.NoUbuntuRelease(f.URL, Version)
}

// Release returns release's image for the host, as Newest checks it: a
// release that has none fails with no_ubuntu_image.
func (f *Finder) Release(ctx context.Context, release string) (state.UbuntuPin, error) {
	p, err := f.check(ctx, release)
	if errors.Is(err, errNoImage) {
		return state.UbuntuPin{}, msg.NoUbuntuImage(release, f.URL, Version)
	}
	return p, err
}

var (
	release     = regexp.MustCompile(`release-[0-9]{8}`)
	releaseName = regexp.MustCompile(`^release-[0-9]{8}$`)
	// listed is a link to a dated release in the listing: Apache's, or
	// Go's file server's in tests.
	listed = regexp.MustCompile(`href="(release-[0-9]{8})/?"`)
)

// releases lists the dated releases the listing links to, newest first.
func (f *Finder) releases(ctx context.Context) ([]string, error) {
	page, err := f.fetch(ctx, f.URL+"/")
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, msg.UbuntuUnreachable(f.URL, Version).Because(err)
	}
	var out []string
	for _, m := range listed.FindAllSubmatch(page, -1) {
		if r := string(m[1]); !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	slices.Reverse(out)
	return out, nil
}

// check returns rel's image for the host once rel's SHA256SUMS are signed
// by Ubuntu's key and list it; errNoImage when they don't list it.
func (f *Finder) check(ctx context.Context, rel string) (state.UbuntuPin, error) {
	if !releaseName.MatchString(rel) {
		return state.UbuntuPin{}, msg.NoUbuntuImage(rel, f.URL, Version)
	}
	sums, err := f.fetch(ctx, f.URL+"/"+rel+"/SHA256SUMS")
	if err != nil {
		if ctx.Err() != nil {
			return state.UbuntuPin{}, ctx.Err()
		}
		return state.UbuntuPin{}, msg.NoUbuntuImage(rel, f.URL, Version).Because(err)
	}
	if err := f.verify(ctx, rel, sums); err != nil {
		return state.UbuntuPin{}, err
	}
	file := File(f.Arch)
	sum := Checksum(sums, file)
	if sum == "" {
		return state.UbuntuPin{}, errNoImage
	}
	return state.UbuntuPin{Release: rel, File: file, Digest: "sha256:" + sum, Pinned: f.Now().UTC().Truncate(time.Second)}, nil
}

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

// verify checks that sums, rel's SHA256SUMS, carry a valid signature by
// Ubuntu's key (Keyring) and no other.
func (f *Finder) verify(ctx context.Context, rel string, sums []byte) error {
	bad := msg.UbuntuBadSignature(rel, f.URL)
	sig, err := f.fetch(ctx, f.URL+"/"+rel+"/SHA256SUMS.gpg")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return bad.Because(err)
	}
	key, err := fs.ReadFile(f.Assets, Keyring)
	if err != nil {
		return fmt.Errorf("checking Ubuntu's checksums: %w", err)
	}
	if err := verifyDetached(key, sig, sums, f.Now()); err != nil {
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
