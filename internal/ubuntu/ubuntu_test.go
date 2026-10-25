package ubuntu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

var (
	sumArm64 = strings.Repeat("1", 64)
	sumAmd64 = strings.Repeat("2", 64)
)

// testUI runs commands as they are, and logs notes.
type testUI struct{ t *testing.T }

func (testUI) LoggedCmd(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }

func (u testUI) Note(m msg.M) { u.t.Log(m.Names()) }

// mirror makes a folder of releases: release-20260101 with a checksum for
// each arch and a signature (which the fake gpgv checks).
func mirror(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	rel := filepath.Join(dir, "release-20260101")
	if err := os.MkdirAll(rel, 0o750); err != nil {
		t.Fatal(err)
	}
	sums := sumArm64 + " *ubuntu-" + Version + "-server-cloudimg-arm64.img\n" +
		sumAmd64 + " *ubuntu-" + Version + "-server-cloudimg-amd64.img\n"
	write(t, filepath.Join(rel, "SHA256SUMS"), sums)
	write(t, filepath.Join(rel, "SHA256SUMS.gpg"), "signature")
	return dir
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil { //nolint:gosec // the test's own files
		t.Fatal(err)
	}
}

// fakeGPGV stands in for gpgv: it records each call, and a signature is
// good unless bad is set.
type fakeGPGV struct {
	bad   bool
	calls [][]string
	homes []string
}

func (g *fakeGPGV) run(_ context.Context, cmd *exec.Cmd) error {
	g.calls = append(g.calls, cmd.Args)
	for _, e := range cmd.Env {
		if h, ok := strings.CutPrefix(e, "GNUPGHOME="); ok {
			g.homes = append(g.homes, h)
		}
	}
	if g.bad {
		return errors.New("exit status 1")
	}
	return nil
}

// served runs each test against the folder as a file:// URL and over HTTP.
func served(t *testing.T, test func(t *testing.T, dir, url string)) {
	t.Helper()
	t.Run("file", func(t *testing.T) {
		dir := mirror(t)
		test(t, dir, "file://"+dir)
	})
	t.Run("http", func(t *testing.T) {
		dir := mirror(t)
		srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
		t.Cleanup(srv.Close)
		test(t, dir, srv.URL)
	})
}

func finder(url, arch string, g *fakeGPGV, notes *[]msg.M) *Finder {
	return &Finder{
		URL: url, Keyring: "/devvm/" + Keyring, Arch: arch, Client: NewClient(), GPGV: "gpgv", Run: g.run,
		Note: func(m msg.M) { *notes = append(*notes, m) },
	}
}

func wantMsg(t *testing.T, err error, id string, vals ...string) {
	t.Helper()
	m, ok := errors.AsType[msg.M](err)
	if !ok || m.ID() != id {
		t.Fatalf("got %v, want %s", err, id)
	}
	if got := m.Names(); !strings.HasPrefix(got, strings.Join(append([]string{id}, vals...), " ")) {
		t.Errorf("got %s, want %s %s", got, id, strings.Join(vals, " "))
	}
}

// The newest dated release with an image for this host, and its checksum;
// a newer release without files is skipped. No release: no_ubuntu_image.
func TestImage(t *testing.T) {
	t.Parallel()
	served(t, func(t *testing.T, dir, url string) {
		if err := os.Mkdir(filepath.Join(dir, "release-20260301"), 0o750); err != nil {
			t.Fatal(err)
		}
		for arch, sum := range map[string]string{"arm64": sumArm64, "amd64": sumAmd64} {
			var notes []msg.M
			img, err := finder(url, arch, &fakeGPGV{}, &notes).Image(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			want := Image{URL: url + "/release-20260101/ubuntu-" + Version + "-server-cloudimg-" + arch + ".img", Digest: "sha256:" + sum}
			if img != want {
				t.Errorf("%s: got %+v, want %+v", arch, img, want)
			}
			if img.String() != want.URL+" "+want.Digest {
				t.Errorf("String: %s", img)
			}
		}
		for _, r := range []string{"release-20260101", "release-20260301"} {
			if err := os.RemoveAll(filepath.Join(dir, r)); err != nil {
				t.Fatal(err)
			}
		}
		var notes []msg.M
		_, err := finder(url, "arm64", &fakeGPGV{}, &notes).Image(t.Context())
		wantMsg(t, err, "no_ubuntu_image", "url="+url, "version="+Version)
	})
}

// A checksum that isn't 64 hex digits is skipped, never put in the
// backend's config.
func TestImageSkipsBadChecksums(t *testing.T) {
	t.Parallel()
	served(t, func(t *testing.T, dir, url string) {
		bad := strings.Repeat("a", 64) + `"}]|.mounts=[{"location":"~","writable":true}]|.x=[{"y":"`
		write(t, filepath.Join(dir, "release-20260101", "SHA256SUMS"), bad+" *ubuntu-"+Version+"-server-cloudimg-arm64.img\n")
		var notes []msg.M
		_, err := finder(url, "arm64", &fakeGPGV{}, &notes).Image(t.Context())
		wantMsg(t, err, "no_ubuntu_image", "url="+url, "version="+Version)
	})
}

// The checksums must be signed by Ubuntu's key, checked with gpgv in a
// home of its own; without gpgv, a note.
func TestImageSignature(t *testing.T) {
	t.Parallel()
	served(t, func(t *testing.T, dir, url string) {
		var notes []msg.M
		g := &fakeGPGV{}
		if _, err := finder(url, "arm64", g, &notes).Image(t.Context()); err != nil {
			t.Fatal(err)
		}
		if len(g.calls) != 1 || len(g.homes) != 1 {
			t.Fatalf("gpgv calls %q, homes %q", g.calls, g.homes)
		}
		args, tmp := g.calls[0], filepath.Dir(g.homes[0])
		want := []string{"gpgv", "--keyring", "/devvm/layers/ubuntu-cloud.gpg", filepath.Join(tmp, "SHA256SUMS.gpg"), filepath.Join(tmp, "SHA256SUMS")}
		if !slices.Equal(args, want) {
			t.Errorf("gpgv %q, want %q", args, want)
		}
		if _, err := os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there: %v", tmp, err)
		}
		if len(notes) != 0 {
			t.Errorf("notes: %v", notes)
		}

		_, err := finder(url, "arm64", &fakeGPGV{bad: true}, &notes).Image(t.Context())
		wantMsg(t, err, "ubuntu_bad_signature", "release=release-20260101", "url="+url)

		if err := os.Remove(filepath.Join(dir, "release-20260101", "SHA256SUMS.gpg")); err != nil {
			t.Fatal(err)
		}
		g = &fakeGPGV{}
		_, err = finder(url, "arm64", g, &notes).Image(t.Context())
		wantMsg(t, err, "ubuntu_bad_signature", "release=release-20260101", "url="+url)
		if len(g.calls) != 0 {
			t.Errorf("gpgv ran without a signature: %q", g.calls)
		}

		f := finder(url, "arm64", g, &notes)
		f.GPGV = ""
		if _, err := f.Image(t.Context()); err != nil {
			t.Fatal(err)
		}
		if len(notes) != 1 || notes[0].ID() != "ubuntu_unsigned" || len(g.calls) != 0 {
			t.Errorf("without gpgv: notes %v, calls %q", notes, g.calls)
		}
	})
}

// Ubuntu's real key, with the real gpgv, checks a real release's signature,
// and refuses its checksums once they're changed.
func TestRealSignature(t *testing.T) {
	t.Parallel()
	gpgv, err := exec.LookPath("gpgv")
	if err != nil {
		t.Skip("gpgv isn't installed")
	}
	src, err := filepath.Abs(filepath.Join("testdata", "resolute"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	f := New(root, func(string) string { return "file://" + src }, testUI{t})
	f.GPGV, f.Arch = gpgv, "arm64"
	img, err := f.Image(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := Image{
		URL:    "file://" + src + "/release-20261016/ubuntu-26.04-server-cloudimg-arm64.img",
		Digest: "sha256:8eb876c2ccc4b6ff89e4a65d511815923b285e04503ba465809bf587f763da24",
	}
	if img != want {
		t.Errorf("got %+v, want %+v", img, want)
	}

	// The same files with one checksum changed.
	dir := t.TempDir()
	rel := filepath.Join(dir, "release-20261016")
	if err := os.Mkdir(rel, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SHA256SUMS", "SHA256SUMS.gpg"} {
		data, err := os.ReadFile(filepath.Join(src, "release-20261016", name)) //nolint:gosec // the test's fixtures
		if err != nil {
			t.Fatal(err)
		}
		if name == "SHA256SUMS" {
			data = []byte(strings.Replace(string(data), "8eb876c2", "8eb876c3", 1))
		}
		write(t, filepath.Join(rel, name), string(data))
	}
	f.URL = "file://" + dir
	_, err = f.Image(t.Context())
	wantMsg(t, err, "ubuntu_bad_signature", "release=release-20261016")
}

func TestNew(t *testing.T) {
	t.Parallel()
	f := New("/devvm", func(string) string { return "" }, testUI{t})
	if f.URL != DefaultURL || f.Keyring != "/devvm/layers/ubuntu-cloud.gpg" {
		t.Errorf("default: %+v", f)
	}
	f = New("/devvm", func(k string) string {
		if k == URLEnv {
			return "file:///tmp/ubuntu/"
		}
		return ""
	}, testUI{t})
	if f.URL != "file:///tmp/ubuntu" {
		t.Errorf("%s: %s", URLEnv, f.URL)
	}
}

func TestReleases(t *testing.T) {
	t.Parallel()
	listing := `<a href="release-20260101/">release-20260101/</a>
<a href="release-20260927/">release-20260927/</a>
<a href="current/">current/</a> <a href="release-2026/">release-2026/</a>`
	got := Releases([]byte(listing))
	if want := []string{"release-20260927", "release-20260101"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := Releases(nil); len(got) != 0 {
		t.Errorf("empty listing: %q", got)
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
	dir := mirror(t)
	files := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(files.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file/" {
			http.Redirect(w, r, "file://"+dir+"/", http.StatusFound)
			return
		}
		http.Redirect(w, r, files.URL+strings.TrimPrefix(r.URL.Path, "/same"), http.StatusFound) //nolint:gosec // the test's own server
	}))
	t.Cleanup(srv.Close)
	var notes []msg.M
	if _, err := finder(srv.URL+"/same", "arm64", &fakeGPGV{}, &notes).Image(t.Context()); err != nil {
		t.Errorf("same scheme: %v", err)
	}
	f := finder(srv.URL+"/file", "arm64", &fakeGPGV{}, &notes)
	if _, err := f.Releases(t.Context()); !errors.Is(err, errRedirect) {
		t.Errorf("to file://: got %v, want a refused redirect", err)
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
	rs, err := f.Releases(t.Context())
	if err != nil || !slices.Equal(rs, []string{"release-20260101"}) || n.Load() != 3 {
		t.Errorf("got %q, %v after %d requests", rs, err, n.Load())
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
	var notes []msg.M
	if _, err := finder("file://"+mirror(t), "arm64", &fakeGPGV{}, &notes).Image(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}
