package ubuntu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/fetch"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

func sumOf(c byte) string { return strings.Repeat(string(c), 64) }

func line(sum, arch string) string { return sum + " *" + File(arch) + "\n" }

// publish writes a dated release into dir: its SHA256SUMS, signed by k.
func publish(t *testing.T, dir, name, sums string, k testKey) {
	t.Helper()
	rel := filepath.Join(dir, name)
	if err := os.MkdirAll(rel, 0o750); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(rel, "SHA256SUMS"), sums)
	write(t, filepath.Join(rel, "SHA256SUMS.gpg"), string(k.sign(t, []byte(sums), 10, sigBinary, nil)))
}

// mirror makes a folder of releases signed by k: release-20260101 with an
// image for each architecture, and release-20260201, newer, with amd64's
// only.
func mirror(t *testing.T, k testKey) string {
	t.Helper()
	dir := t.TempDir()
	publish(t, dir, "release-20260101", line(sumOf('1'), "arm64")+line(sumOf('2'), "amd64"), k)
	publish(t, dir, "release-20260201", line(sumOf('3'), "amd64"), k)
	return dir
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

// served runs each test against the folder as a file:// URL and over HTTP,
// both of which list it as a page of links.
func served(t *testing.T, test func(t *testing.T, k testKey, dir, url string)) {
	t.Helper()
	k := newTestKey(t)
	t.Run("file", func(t *testing.T) {
		dir := mirror(t, k)
		test(t, k, dir, "file://"+dir)
	})
	t.Run("http", func(t *testing.T) {
		dir := mirror(t, k)
		srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
		t.Cleanup(srv.Close)
		test(t, k, dir, srv.URL)
	})
}

// then is the time the tests' Finders check signatures at, and pin.
func then() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) }

func finder(url, arch string, k testKey) *Finder {
	return &Finder{
		URL: url, Assets: fstest.MapFS{Keyring: {Data: k.keyring}}, Arch: arch,
		Client: NewClient(), Now: then,
	}
}

func wantMsg(t *testing.T, err error, id string, vals ...string) {
	t.Helper()
	if msg.IDOf(err) != id {
		t.Fatalf("got %v, want %s", err, id)
	}
	if got := msg.NamesOf(err); !strings.HasPrefix(got, strings.Join(append([]string{id}, vals...), " ")) {
		t.Errorf("got %s, want %s %s", got, id, strings.Join(vals, " "))
	}
}

func pinOf(rel, arch, sum string) state.UbuntuPin {
	return state.UbuntuPin{Release: rel, File: File(arch), Digest: "sha256:" + sum, Pinned: then()}
}

// The newest release with an image for the host, with the checksum its
// signed SHA256SUMS list; one without an image for the host is passed
// over, and so is anything not older than before.
func TestNewest(t *testing.T) {
	t.Parallel()
	served(t, func(t *testing.T, k testKey, _, url string) {
		for _, c := range []struct {
			arch, before string
			want         state.UbuntuPin
		}{
			{"amd64", "", pinOf("release-20260201", "amd64", sumOf('3'))},
			{"arm64", "", pinOf("release-20260101", "arm64", sumOf('1'))},
			{"amd64", "release-20260201", pinOf("release-20260101", "amd64", sumOf('2'))},
		} {
			got, err := finder(url, c.arch, k).Newest(t.Context(), c.before)
			if err != nil {
				t.Fatalf("%s before %q: %v", c.arch, c.before, err)
			}
			if got != c.want {
				t.Errorf("%s before %q: got %+v, want %+v", c.arch, c.before, got, c.want)
			}
		}
		_, err := finder(url, "arm64", k).Newest(t.Context(), "release-20260101")
		wantMsg(t, err, "no_ubuntu_release", "url="+url, "version="+Version)
		_, err = finder(url, "riscv64", k).Newest(t.Context(), "")
		wantMsg(t, err, "no_ubuntu_release")
	})
}

// A named release (e2e's) is checked as the newest is.
func TestRelease(t *testing.T) {
	t.Parallel()
	served(t, func(t *testing.T, k testKey, _, url string) {
		got, err := finder(url, "amd64", k).Release(t.Context(), "release-20260101")
		if err != nil || got != pinOf("release-20260101", "amd64", sumOf('2')) {
			t.Errorf("got %+v, %v", got, err)
		}
		_, err = finder(url, "arm64", k).Release(t.Context(), "release-20260201")
		wantMsg(t, err, "no_ubuntu_image", "release=release-20260201", "url="+url)
		_, err = finder(url, "arm64", k).Release(t.Context(), "release-20250101")
		wantMsg(t, err, "no_ubuntu_image", "release=release-20250101")
		_, err = finder(url, "arm64", k).Release(t.Context(), "../release-20260101")
		wantMsg(t, err, "no_ubuntu_image")
	})
}

// A release whose checksums aren't signed by the pinned key is refused,
// never passed over for an older one; a checksum that isn't 64 hex digits
// is no image, as it would go into the backend's config.
func TestNewestRefused(t *testing.T) {
	t.Parallel()
	served(t, func(t *testing.T, k testKey, dir, url string) {
		other := newTestKey(t)
		publish(t, dir, "release-20260301", line(sumOf('4'), "amd64"), other)
		_, err := finder(url, "amd64", k).Newest(t.Context(), "")
		wantMsg(t, err, "ubuntu_bad_signature", "release=release-20260301", "url="+url)

		if err := os.Remove(filepath.Join(dir, "release-20260301", "SHA256SUMS.gpg")); err != nil {
			t.Fatal(err)
		}
		_, err = finder(url, "amd64", k).Newest(t.Context(), "")
		wantMsg(t, err, "ubuntu_bad_signature", "release=release-20260301")

		if err := os.Remove(filepath.Join(dir, "release-20260301", "SHA256SUMS")); err != nil {
			t.Fatal(err)
		}
		_, err = finder(url, "amd64", k).Newest(t.Context(), "")
		wantMsg(t, err, "no_ubuntu_image", "release=release-20260301")

		publish(t, dir, "release-20260301", sumOf('4')+`"}]|.mounts=[{"location":"~","writable":true}] *`+File("amd64")+"\n", k)
		got, err := finder(url, "amd64", k).Newest(t.Context(), "")
		if err != nil || got.Release != "release-20260201" {
			t.Errorf("past a release without a checksum: %+v, %v", got, err)
		}
	})
}

// The listing can't be read: no answer, or nothing there.
func TestNewestUnreachable(t *testing.T) {
	t.Parallel()
	k := newTestKey(t)
	url := "file://" + filepath.Join(t.TempDir(), "gone")
	_, err := finder(url, "arm64", k).Newest(t.Context(), "")
	wantMsg(t, err, "ubuntu_unreachable", "url="+url, "version="+Version)
	empty := "file://" + t.TempDir()
	_, err = finder(empty, "arm64", k).Newest(t.Context(), "")
	wantMsg(t, err, "no_ubuntu_release")
}

// Ubuntu's real key checks a real release's signature, and refuses its
// checksums once they're changed.
func TestRealRelease(t *testing.T) {
	t.Parallel()
	src, err := filepath.Abs(filepath.Join("testdata", "resolute"))
	if err != nil {
		t.Fatal(err)
	}
	f := New(assets.FS, "file://"+src)
	f.Arch = "arm64"
	got, err := f.Newest(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Release != "release-20261016" || got.Digest != "sha256:8eb876c2ccc4b6ff89e4a65d511815923b285e04503ba465809bf587f763da24" {
		t.Errorf("got %+v", got)
	}
	if img := ImageOf(f.URL, got); img.URL != "file://"+src+"/release-20261016/ubuntu-26.04-server-cloudimg-arm64.img" ||
		img.String() != img.URL+" "+got.Digest {
		t.Errorf("image %+v", img)
	}

	// The same files with one checksum changed.
	dir := t.TempDir()
	rel := filepath.Join(dir, "release-20261016")
	if err := os.Mkdir(rel, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SHA256SUMS", "SHA256SUMS.gpg"} {
		data := readFile(t, filepath.Join(src, "release-20261016", name))
		if name == "SHA256SUMS" {
			data = []byte(strings.Replace(string(data), "8eb876c2", "8eb876c3", 1))
		}
		write(t, filepath.Join(rel, name), string(data))
	}
	f.URL = "file://" + dir
	_, err = f.Newest(t.Context(), "")
	wantMsg(t, err, "ubuntu_bad_signature", "release=release-20261016")
}

func TestPinArg(t *testing.T) {
	t.Parallel()
	p := pinOf("release-20260101", "arm64", sumOf('1'))
	if got, want := PinArg(p), "release-20260101 ubuntu-26.04-server-cloudimg-arm64.img sha256:"+sumOf('1'); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	f := New(assets.FS, "")
	if f.URL != DefaultURL || f.Assets == nil || f.Now == nil || f.Client == nil {
		t.Errorf("default: %+v", f)
	}
	f = New(assets.FS, "file:///tmp/ubuntu/")
	if f.URL != "file:///tmp/ubuntu" {
		t.Errorf("moved: %s", f.URL)
	}
}

func TestReleaseDate(t *testing.T) {
	t.Parallel()
	for url, want := range map[string]string{
		"https://cloud-images.ubuntu.com/releases/resolute/release-20261001/ubuntu-26.04-server-cloudimg-arm64.img sha256:ab": "20261001",
		"https://example.com/release-2026/x.img": "",
		"":                                       "",
	} {
		if got := ReleaseDate(url); got != want {
			t.Errorf("ReleaseDate(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestChecksum(t *testing.T) {
	t.Parallel()
	hex := strings.Repeat("ab", 32)
	tests := []struct {
		name, sums, want string
	}{
		{"binary mode", hex + " *f.img\n", hex},
		{"text mode", hex + "  f.img\n", hex},
		{"another file", hex + " *f.img.manifest\n" + hex + " *g.img\n", ""},
		{"too short", hex[:63] + " *f.img\n", ""},
		{"uppercase", strings.ToUpper(hex) + " *f.img\n", ""},
		{"the first good one", "zz *f.img\n" + hex + " *f.img\n", hex},
		{"no trailing newline", hex + " *f.img", hex},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		if got := Checksum([]byte(tt.sums), "f.img"); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

// A redirect to another scheme is refused (so an HTTP server can't send
// devvm to a file on disk); one on the same scheme is followed.
func TestRedirects(t *testing.T) {
	t.Parallel()
	k := newTestKey(t)
	dir := mirror(t, k)
	files := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(files.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/file/") {
			http.Redirect(w, r, "file://"+dir+"/", http.StatusFound)
			return
		}
		http.Redirect(w, r, files.URL+strings.TrimPrefix(r.URL.Path, "/same"), http.StatusFound) //nolint:gosec // the test's own server
	}))
	t.Cleanup(srv.Close)
	if _, err := finder(srv.URL+"/same", "arm64", k).Newest(t.Context(), ""); err != nil {
		t.Errorf("same scheme: %v", err)
	}
	f := finder(srv.URL+"/file", "arm64", k)
	if _, err := f.fetch(t.Context(), f.URL+"/"); !errors.Is(err, fetch.ErrRedirect) {
		t.Errorf("to file://: got %v, want a refused redirect", err)
	}
	if _, err := f.Newest(t.Context(), ""); err == nil {
		t.Error("to file://: found an image")
	}
}

// Failures that may pass are tried again; a missing file isn't.
func TestRetries(t *testing.T) {
	t.Parallel()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := n.Add(1)
		switch {
		case r.URL.Path == "/missing":
			http.NotFound(w, r)
		case c < 3:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = w.Write([]byte("release-20260101"))
		}
	}))
	t.Cleanup(srv.Close)
	f := &Finder{URL: srv.URL, Client: NewClient()}
	body, err := f.fetch(t.Context(), srv.URL+"/")
	if err != nil || string(body) != "release-20260101" || n.Load() != 3 {
		t.Errorf("got %q, %v after %d requests", body, err, n.Load())
	}
	n.Store(0)
	if _, err := f.fetch(t.Context(), srv.URL+"/missing"); err == nil || n.Load() != 1 {
		t.Errorf("missing: %v after %d requests", err, n.Load())
	}
}

// A canceled lookup stops, with the context's error.
func TestCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	k := newTestKey(t)
	if _, err := finder("file://"+mirror(t, k), "arm64", k).Newest(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}
