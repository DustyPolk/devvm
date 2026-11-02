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
	"testing/fstest"

	"github.com/w3cj/devvm/assets"
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

// fakeGPGV stands in for gpgv: it records each call and the keyring it
// was given, and a signature is good unless bad is set.
type fakeGPGV struct {
	bad      bool
	calls    [][]string
	homes    []string
	keyrings []string
}

func (g *fakeGPGV) run(_ context.Context, cmd *exec.Cmd) error {
	g.calls = append(g.calls, cmd.Args)
	if i := slices.Index(cmd.Args, "--keyring"); i >= 0 && i+1 < len(cmd.Args) {
		key, _ := os.ReadFile(cmd.Args[i+1])
		g.keyrings = append(g.keyrings, string(key))
	}
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

// testAssets is devvm's files with a stand-in for Ubuntu's key.
var testAssets = fstest.MapFS{Keyring: {Data: []byte("key")}}

func finder(url, arch string, g *fakeGPGV, notes *[]msg.M) *Finder {
	pin := map[string]string{"arm64": "sha256:" + sumArm64, "amd64": "sha256:" + sumAmd64}[arch]
	return &Finder{
		URL: url, Release: "release-20260101", Digest: pin,
		Assets: testAssets, Arch: arch, Client: NewClient(), GPGV: "gpgv", Run: g.run,
		Note: func(m msg.M) { *notes = append(*notes, m) },
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

// The pinned release's image for this host, with its pinned digest, once
// the release's SHA256SUMS lists it; a newer release is never looked at.
func TestImage(t *testing.T) {
	t.Parallel()
	served(t, func(t *testing.T, dir, url string) {
		newer := filepath.Join(dir, "release-20260301")
		if err := os.Mkdir(newer, 0o750); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(newer, "SHA256SUMS"), strings.Repeat("3", 64)+" *ubuntu-"+Version+"-server-cloudimg-arm64.img\n")
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
	})
}

// Anything but the pinned checksum in the pinned release's SHA256SUMS is
// refused, never put in the backend's config; a missing release, or an
// architecture without a pin, finds no image.
func TestImageRefused(t *testing.T) {
	t.Parallel()
	file := " *ubuntu-" + Version + "-server-cloudimg-arm64.img\n"
	tests := []struct {
		name, sums string // "" for no SHA256SUMS
		release    string // the Finder's, when not release-20260101
		noPin      bool
		want       string
	}{
		{name: "another checksum", sums: strings.Repeat("3", 64) + file, want: "ubuntu_checksum_unpinned"},
		{name: "the amd64 one only", sums: sumAmd64 + " *ubuntu-" + Version + "-server-cloudimg-amd64.img\n", want: "ubuntu_checksum_unpinned"},
		{name: "not a checksum", sums: sumArm64 + `"}]|.mounts=[{"location":"~","writable":true}]` + file, want: "ubuntu_checksum_unpinned"},
		{name: "no release", release: "release-20250101", want: "no_ubuntu_image"},
		{name: "no pin", noPin: true, want: "no_ubuntu_image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			served(t, func(t *testing.T, dir, url string) {
				if tt.sums != "" {
					write(t, filepath.Join(dir, "release-20260101", "SHA256SUMS"), tt.sums)
				}
				var notes []msg.M
				f := finder(url, "arm64", &fakeGPGV{}, &notes)
				if tt.release != "" {
					f.Release = tt.release
				}
				if tt.noPin {
					f.Digest = ""
				}
				_, err := f.Image(t.Context())
				switch tt.want {
				case "no_ubuntu_image":
					wantMsg(t, err, tt.want, "release="+f.Release, "url="+url, "version="+Version)
				default:
					wantMsg(t, err, tt.want, "file=ubuntu-"+Version+"-server-cloudimg-arm64.img", "release=release-20260101", "url="+url)
				}
			})
		})
	}
}

// The pins: each architecture's digest, 64 hex digits, and what base's key
// on Lima hashes; none for another architecture.
func TestCloudPin(t *testing.T) {
	t.Parallel()
	if !release.MatchString(CloudRelease) || ReleaseDate(CloudRelease) == "" {
		t.Errorf("CloudRelease %q isn't a dated release", CloudRelease)
	}
	for arch, sum := range map[string]string{"arm64": CloudSHA256Arm64, "amd64": CloudSHA256Amd64} {
		if !digest.MatchString(sum) {
			t.Errorf("%s: %q isn't a SHA-256", arch, sum)
		}
		if got := CloudDigest(arch); got != "sha256:"+sum {
			t.Errorf("CloudDigest(%s) = %q", arch, got)
		}
		want := CloudRelease + " ubuntu-" + Version + "-server-cloudimg-" + arch + ".img sha256:" + sum
		if got := CloudPin(arch); got != want {
			t.Errorf("CloudPin(%s) = %q, want %q", arch, got, want)
		}
	}
	if CloudPin("arm64") == CloudPin("amd64") {
		t.Error("both architectures pin the same image")
	}
	if got := CloudDigest("riscv64"); got != "" {
		t.Errorf("riscv64: %q", got)
	}
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
		want := []string{"gpgv", "--keyring", filepath.Join(tmp, "ubuntu-cloud.gpg"), filepath.Join(tmp, "SHA256SUMS.gpg"), filepath.Join(tmp, "SHA256SUMS")}
		if !slices.Equal(args, want) {
			t.Errorf("gpgv %q, want %q", args, want)
		}
		if !slices.Equal(g.keyrings, []string{"key"}) {
			t.Errorf("keyrings %q, want devvm's key", g.keyrings)
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

		// Without gpgv, the pin still decides.
		write(t, filepath.Join(dir, "release-20260101", "SHA256SUMS"), strings.Repeat("3", 64)+" *ubuntu-"+Version+"-server-cloudimg-arm64.img\n")
		_, err = f.Image(t.Context())
		wantMsg(t, err, "ubuntu_checksum_unpinned")
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
	f := New(assets.FS, "file://"+src, testUI{t})
	f.GPGV, f.Arch = gpgv, "arm64"
	f.Release, f.Digest = "release-20261016", "sha256:8eb876c2ccc4b6ff89e4a65d511815923b285e04503ba465809bf587f763da24"
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
	f := New(testAssets, "", testUI{t})
	if f.URL != DefaultURL || f.Assets == nil || f.Release != CloudRelease || f.Digest != CloudDigest(f.Arch) {
		t.Errorf("default: %+v", f)
	}
	f = New(testAssets, "file:///tmp/ubuntu/", testUI{t})
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
	if _, err := f.fetch(t.Context(), f.URL+"/"); !errors.Is(err, errRedirect) {
		t.Errorf("to file://: got %v, want a refused redirect", err)
	}
	if _, err := f.Image(t.Context()); err == nil {
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
	var notes []msg.M
	if _, err := finder("file://"+mirror(t), "arm64", &fakeGPGV{}, &notes).Image(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}
