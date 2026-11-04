package lima

import (
	"io/fs"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
)

// ownFiles is a copy of devvm's files, for a test to edit.
func ownFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	err := fs.WalkDir(assets.FS, ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil || !e.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(assets.FS, name)
		m[name] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// limaBuild is what a build of base on Lima is made with: devvm's files,
// the host's architecture and user, and --from ("" for Ubuntu's).
type limaBuild struct {
	own  fstest.MapFS
	arch string
	user user.User
	from string
}

// build builds base as lb says, every feature chosen, as devvm does:
// build's Builds on Lima, whose commands only the fake runner sees. It
// returns what that sent into the build, its paths of the moment written
// as OUT (each limactl create's arguments and input, each script's
// arguments and input, and with --from, the files written for the
// conversion), and base's key. Commands that only start, stop or look at
// a VM aren't what it sent.
func (lb limaBuild) build(t *testing.T) (sent, key string) {
	t.Helper()
	l, f := buildLima(t, "darwin", "")
	l.Assets, l.Arch = lb.own, lb.arch
	l.User = func() (*user.User, error) { return &lb.user, nil }
	var out []string
	tmp := "" // the conversion's own folder
	do := f.do
	f.do = func(c call) {
		do(c)
		i := slices.Index(c.argv, "bash")
		if i < 0 || i+3 >= len(c.argv) || !strings.HasSuffix(c.argv[i+1], "/oci/build.sh") {
			return
		}
		oci := c.argv[i+2]
		tmp = filepath.Dir(oci)
		writeConverted(t, c.argv[i+3])
		err := filepath.WalkDir(oci, func(p string, e fs.DirEntry, err error) error {
			if err == nil && e.Type().IsRegular() {
				data, err := os.ReadFile(p) //nolint:gosec // the conversion's own folder
				out = append(out, p+"\n"+string(data))
				return err
			}
			return err
		})
		if err != nil {
			t.Error(err)
		}
	}
	none := func(msg.M) {}
	b := &build.Env{
		Dirs: l.Dirs, Assets: lb.own, Backend: l, From: lb.from, Version: "0.1.0-test",
		Info: none, Warn: none, Say: none, LogNote: func() {},
		Confirm:  func(msg.M) bool { return true },
		Pick:     func(fs []build.Feature) []build.Feature { return fs },
		Terminal: func() bool { return false },
	}
	if err := (build.Builds{Env: b}).Build(t.Context(), image.Base); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c.line(), "limactl create") || slices.Contains(c.argv, "bash") {
			out = append(out, c.line()+"\n"+c.stdin)
		}
	}
	sent = strings.Join(out, "\n---\n")
	if tmp != "" {
		sent = strings.ReplaceAll(sent, tmp, "OUT")
	}
	key, err := b.Key(image.Base)
	if err != nil {
		t.Fatal(err)
	}
	// Each create, clone and script run names its instance by key.
	return strings.ReplaceAll(sent, key, "KEY"), key
}

// Every byte and argument Lima sends into base's build changes base's
// key: each of devvm's files, the host's architecture, the user a --from
// base's conversion makes, and --from's digest. Each change is checked on
// its own, from Ubuntu and with --from.
func TestKeyHashesWhatLimaSends(t *testing.T) {
	debian := "debian:12@sha256:" + strings.Repeat("a", 64)
	for _, from := range []string{"", debian} {
		lb := limaBuild{own: ownFiles(t), arch: "arm64", user: user.User{Username: "me", Uid: "501"}, from: from}
		before, key := lb.build(t)
		differed := false
		check := func(change string, c limaBuild) {
			t.Helper()
			sent, k := c.build(t)
			if sent == before {
				return
			}
			differed = true
			if k == key {
				t.Errorf("%q: %s changes what base's build sends, not its key", from, change)
			}
		}
		for name, f := range lb.own {
			c := lb
			c.own = maps.Clone(lb.own)
			c.own[name] = &fstest.MapFile{Data: append(slices.Clone(f.Data), "\n# edited\n"...)}
			check("editing "+name, c)
		}
		c := lb
		c.arch = "amd64"
		check("another architecture", c)
		c = lb
		c.user = user.User{Username: "you", Uid: "502"}
		check("another user", c)
		if from != "" {
			c = lb
			c.from = "debian:12@sha256:" + strings.Repeat("b", 64)
			check("--from's digest", c)
		}
		if !differed {
			t.Errorf("%q: no change sent anything else", from)
		}
	}
}
