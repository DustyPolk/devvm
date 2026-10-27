// Package ubuntu pins Ubuntu's container image and finds Ubuntu's cloud
// image for base. The cloud image is Lima's: the newest dated
// release of the series with an image for the host, and the image's
// checksum from that release's SHA256SUMS. When gpgv is installed,
// SHA256SUMS must be signed by Ubuntu's cloud image key, pinned in devvm's
// own files (layers/ubuntu-cloud.gpg).
//
// The image itself isn't downloaded here: the backend fetches it and checks
// it against the digest.
package ubuntu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/msg"
)

const (
	// Series is the Ubuntu release base is built on, by its code name.
	Series = "resolute"
	// Version is Series' version number, as Ubuntu's image names have it.
	Version = "26.04"
	// DefaultURL is where the series' dated releases are listed.
	DefaultURL = "https://cloud-images.ubuntu.com/releases/" + Series
	// URLEnv moves DefaultURL, to a mirror or a file:// folder (tests).
	URLEnv = "DEVVM_UBUNTU_URL"
	// Keyring is Ubuntu's cloud image signing key, by its path in devvm's
	// own files.
	Keyring = "layers/ubuntu-cloud.gpg"
	// ContainerRepo is Ubuntu's container image, which smolvm's and
	// Docker's base start from without --from: Version's, by
	// ContainerDigest.
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
	// URL lists the dated releases (release-YYYYMMDD/), each with its
	// SHA256SUMS and SHA256SUMS.gpg. http(s):// or file://.
	URL string
	// Assets holds devvm's own files, Keyring among them.
	Assets fs.FS
	// Arch is the host's, as Go and Ubuntu's image names have it (arm64,
	// amd64).
	Arch string
	// Client fetches the listing and the checksums.
	Client *http.Client
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

// New returns a Finder with devvm's own files (assets), the URL from
// DEVVM_UBUNTU_URL when it's set, running gpgv and printing through u.
func New(assets fs.FS, getenv func(string) string, u UI) *Finder {
	url := DefaultURL
	if v := getenv(URLEnv); v != "" {
		url = v
	}
	gpgv, err := exec.LookPath("gpgv")
	if err != nil {
		gpgv = ""
	}
	return &Finder{
		URL:        strings.TrimRight(url, "/"),
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
func NewClient() *http.Client {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t = &http.Transport{}
	}
	t = t.Clone()
	// Only DEVVM_UBUNTU_URL can name a file: redirects can't switch to it.
	t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/"))) //nolint:gosec // file:// URLs are absolute paths
	return &http.Client{Transport: t, Timeout: timeout, CheckRedirect: sameScheme}
}

// errRedirect is a redirect refused: trying again won't help.
var errRedirect = errors.New("redirect refused")

func sameScheme(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("%w: too many", errRedirect)
	}
	if req.URL.Scheme != via[0].URL.Scheme {
		return fmt.Errorf("%w: from %s to %s", errRedirect, via[0].URL.Scheme, req.URL.Scheme)
	}
	return nil
}

// File is the image's file name in a release.
func (f *Finder) File() string {
	return "ubuntu-" + Version + "-server-cloudimg-" + f.Arch + ".img"
}

// Image returns the newest dated release's image for the host, checked
// against that release's SHA256SUMS (signature-checked when gpgv is
// installed). A release without the image or a usable checksum is skipped:
// a new release's folder can appear before its files. It fails with
// no_ubuntu_image when no release has one, and ubuntu_bad_signature when
// the newest that has one isn't signed by Ubuntu's key.
func (f *Finder) Image(ctx context.Context) (Image, error) {
	releases, _ := f.Releases(ctx) // a failed listing finds no image
	file := f.File()
	for _, r := range releases {
		sums, err := f.fetch(ctx, f.URL+"/"+r+"/SHA256SUMS")
		if err != nil {
			if ctx.Err() != nil {
				return Image{}, ctx.Err()
			}
			continue
		}
		sum := Checksum(sums, file)
		if sum == "" {
			continue
		}
		if err := f.verify(ctx, r, sums); err != nil {
			return Image{}, err
		}
		return Image{URL: f.URL + "/" + r + "/" + file, Digest: "sha256:" + sum}, nil
	}
	if ctx.Err() != nil {
		return Image{}, ctx.Err()
	}
	return Image{}, msg.NoUbuntuImage(f.URL, Version)
}

var release = regexp.MustCompile(`release-[0-9]{8}`)

// ReleaseDate is the date of the dated release an image's address names
// (20260101 for …/release-20260101/…), "" when it names none.
func ReleaseDate(url string) string {
	r := release.FindString(url)
	return strings.TrimPrefix(r, "release-")
}

// Releases returns the dated releases in the listing at URL, newest first.
func (f *Finder) Releases(ctx context.Context) ([]string, error) {
	page, err := f.fetch(ctx, f.URL+"/")
	if err != nil {
		return nil, err
	}
	return Releases(page), nil
}

// Releases returns the dated release names (release-20260101) in a
// listing, newest first, each once.
func Releases(listing []byte) []string {
	var rs []string
	for _, m := range release.FindAll(listing, -1) {
		rs = append(rs, string(m))
	}
	slices.Sort(rs)
	rs = slices.Compact(rs)
	slices.Reverse(rs)
	return rs
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
// written beside them. Without gpgv it notes that only the download
// vouches for them.
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
		if err := os.WriteFile(file, data, 0o600); err != nil {
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
// (no answer, 408, 429, 5xx).
func (f *Finder) fetch(ctx context.Context, u string) ([]byte, error) {
	delay := f.RetryDelay
	for try := 0; ; try++ {
		body, again, err := f.get(ctx, u)
		if err == nil || !again || try == retries {
			return body, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

func (f *Finder) get(ctx context.Context, u string) (body []byte, again bool, err error) {
	if err := ctx.Err(); err != nil { // file:// reads don't watch the context
		return nil, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, false, err
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, ctx.Err() == nil && !errors.Is(err, errRedirect), err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		c := resp.StatusCode
		return nil, c == http.StatusRequestTimeout || c == http.StatusTooManyRequests || c >= 500,
			fmt.Errorf("%s: %s", u, resp.Status)
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, ctx.Err() == nil, err
	}
	if len(body) > maxBody {
		return nil, false, fmt.Errorf("%s: more than %d bytes", u, maxBody)
	}
	return body, false, nil
}
