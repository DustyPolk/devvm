package image

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/w3cj/devvm/assets"
)

// env is a copy of the fixtures in testdata, and the inputs that read it.
type env struct {
	t   *testing.T
	dir string
	in  Inputs
}

func newEnv(t *testing.T, from string) *env {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "rebuilds"), 0o700); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, dir: dir, in: Inputs{
		Backend:      "lima",
		Assets:       os.DirFS(filepath.Join(dir, "root")),
		Layers:       os.DirFS(filepath.Join(dir, "layers")),
		Features:     os.DirFS(filepath.Join(dir, "features")),
		BaseFeatures: []string{"fzf", "mine"},
		Rebuilds:     os.DirFS(filepath.Join(dir, "rebuilds")),
	}}
	if from != "" {
		e.in.From = from
		e.in.FromDigest = "node@sha256:aaaa"
	}
	return e
}

// edit appends a line to a file in the copy, creating it if need be.
func (e *env) edit(name string) {
	t := e.t
	t.Helper()
	f, err := os.OpenFile(filepath.Join(e.dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // a fixture in the test's folder
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("# edit\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func (e *env) remove(name string) {
	e.t.Helper()
	if err := os.Remove(filepath.Join(e.dir, name)); err != nil {
		e.t.Fatal(err)
	}
}

var images = []string{"base", "foo", "foo+bar"}

func (e *env) keys(t *testing.T) map[string]string {
	t.Helper()
	keys := map[string]string{}
	for _, img := range images {
		k, err := e.in.Key(img)
		if err != nil {
			t.Fatalf("%s: %v", img, err)
		}
		keys[img] = k
	}
	return keys
}

func TestKeyChangesWithEachInput(t *testing.T) {
	all := images
	above := []string{"foo", "foo+bar"}
	tests := []struct {
		name   string
		from   string
		change func(e *env)
		want   []string // the images whose keys change
	}{
		{"base.yaml", "", func(e *env) { e.edit("root/base.yaml") }, all},
		{"layers/base.sh", "", func(e *env) { e.edit("root/layers/base.sh") }, all},
		{"the backend", "", func(e *env) { e.in.Backend = "smolvm" }, all},
		{"a backend's own file", "", func(e *env) { e.in.BackendFiles = map[string][]byte{"container/prepare.sh": []byte("x")} }, all},
		{"no backend files is none", "", func(e *env) { e.in.BackendFiles = map[string][]byte{} }, nil},
		{"a chosen built-in feature's script", "", func(e *env) { e.edit("root/features/fzf.sh") }, all},
		{"a chosen user feature's script", "", func(e *env) { e.edit("features/mine.sh") }, all},
		{"a chosen user feature deleted", "", func(e *env) { e.remove("features/mine.sh") }, all},
		{"a feature chosen", "", func(e *env) { e.in.BaseFeatures = append(e.in.BaseFeatures, "omb") }, all},
		{"a feature dropped", "", func(e *env) { e.in.BaseFeatures = []string{"fzf"} }, all},
		{"the features' order", "", func(e *env) { e.in.BaseFeatures = []string{"mine", "fzf"} }, all},
		{"--from", "", func(e *env) { e.in.From = "node:24"; e.in.FromDigest = "node@sha256:aaaa" }, all},
		{"base's rebuild counter", "", func(e *env) { e.edit("rebuilds/base") }, all},
		{"a layer's script", "", func(e *env) { e.edit("layers/foo.sh") }, above},
		{"a layer's script deleted", "", func(e *env) { e.remove("layers/foo.sh") }, above},
		{"the top layer's script", "", func(e *env) { e.edit("layers/bar.sh") }, []string{"foo+bar"}},
		{"a layer's rebuild counter", "", func(e *env) { e.edit("rebuilds/foo") }, above},
		{"the top layer's rebuild counter", "", func(e *env) { e.edit("rebuilds/foo+bar") }, []string{"foo+bar"}},

		{"an unchosen feature's script", "", func(e *env) { e.edit("root/features/omb.sh") }, nil},
		{"a user feature named like a built-in one", "", func(e *env) { e.edit("features/fzf.sh") }, nil},
		{"a policy file", "", func(e *env) { e.edit("root/layers/base.policy") }, nil},
		{"another layer", "", func(e *env) { e.edit("layers/baz.sh") }, nil},
		{"oci scripts without --from", "", func(e *env) { e.edit("root/oci/build.sh") }, nil},
		{"the digest without --from", "", func(e *env) { e.in.FromDigest = "x@sha256:bbbb" }, nil},
		{"a rebuild counter for --from", "", func(e *env) { e.edit("rebuilds/foo@node-24") }, nil},
		{"the tools' build script", "", func(e *env) { e.edit("root/shared-tools/build.sh") }, nil},

		{"the --from digest (a moved tag)", "node:24", func(e *env) { e.in.FromDigest = "node@sha256:bbbb" }, all},
		{"another --from", "node:24", func(e *env) { e.in.From = "node:25" }, all},
		{"oci/build.sh", "node:24", func(e *env) { e.edit("root/oci/build.sh") }, all},
		{"oci/bootify.sh", "node:24", func(e *env) { e.edit("root/oci/bootify.sh") }, all},
		{"oci/kernel.sh", "node:24", func(e *env) { e.edit("root/oci/kernel.sh") }, all},
		{"a rebuild counter with --from", "node:24", func(e *env) { e.edit("rebuilds/foo@node-24") }, above},
		{"oci/try.sh", "node:24", func(e *env) { e.edit("root/oci/try.sh") }, nil},
		{"a rebuild counter without --from", "node:24", func(e *env) { e.edit("rebuilds/foo") }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, tt.from)
			before := e.keys(t)
			tt.change(e)
			after := e.keys(t)
			for _, img := range images {
				changed := before[img] != after[img]
				if want := slices.Contains(tt.want, img); changed != want {
					t.Errorf("%s: changed %v, want %v", img, changed, want)
				}
			}
		})
	}
}

func TestKeyIsStable(t *testing.T) {
	a := newEnv(t, "")
	b := newEnv(t, "") // the same files, copied elsewhere
	first := a.keys(t)
	for _, keys := range []map[string]string{a.keys(t), b.keys(t)} {
		for _, img := range images {
			if keys[img] != first[img] {
				t.Errorf("%s: %s, then %s", img, first[img], keys[img])
			}
		}
	}
	shape := regexp.MustCompile(`^[0-9a-f]{12}$`)
	for img, k := range first {
		if !shape.MatchString(k) {
			t.Errorf("%s: key %q isn't 12 hex digits", img, k)
		}
	}
	if first["base"] == first["foo"] || first["foo"] == first["foo+bar"] {
		t.Errorf("keys repeat: %v", first)
	}
}

// Moving bytes from one input to the next must change the key: the inputs
// aren't simply joined.
func TestKeyFieldsAreSeparate(t *testing.T) {
	key := func(files fstest.MapFS, features []string) string {
		t.Helper()
		k, err := Inputs{Backend: "lima", Assets: files, BaseFeatures: features}.Key(Base)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	a := key(fstest.MapFS{"base.yaml": {Data: []byte("x\n")}, "layers/base.sh": {Data: []byte("y\n")}}, nil)
	b := key(fstest.MapFS{"base.yaml": {Data: []byte("x\ny\n")}, "layers/base.sh": {Data: []byte("")}}, nil)
	if a == b {
		t.Error("moving a line from base.sh to base.yaml kept the key")
	}
	files := fstest.MapFS{
		"base.yaml": {}, "layers/base.sh": {},
		"features/a.sh": {Data: []byte("b")}, "features/ab.sh": {},
	}
	if key(files, []string{"a"}) == key(files, []string{"ab"}) {
		t.Error("features a (script b) and ab (empty) have the same key")
	}
}

func TestChain(t *testing.T) {
	e := newEnv(t, "")
	keys := e.keys(t)
	c, err := e.in.Chain("foo+bar")
	if err != nil {
		t.Fatal(err)
	}
	want := Chain{{"base", keys["base"]}, {"foo", keys["foo"]}, {"foo+bar", keys["foo+bar"]}}
	if !slices.Equal(c, want) {
		t.Fatalf("got %v, want %v", c, want)
	}
	if s := c.String(); s != "base="+keys["base"]+" foo="+keys["foo"]+" foo+bar="+keys["foo+bar"] {
		t.Errorf("String: %q", s)
	}
	if c.Key() != keys["foo+bar"] || Chain(nil).Key() != "" {
		t.Errorf("Key: %q", c.Key())
	}
}

func TestParent(t *testing.T) {
	for name, want := range map[string]string{"base": "", "foo": "base", "foo+bar": "foo", "a+b+c": "a+b"} {
		if got := Parent(name); got != want {
			t.Errorf("Parent(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestKeyRefuses(t *testing.T) {
	e := newEnv(t, "")
	for _, name := range []string{"", "Foo", "foo+", "+foo", "base+foo", "foo+base", "foo/bar", "../x", "-x"} {
		if _, err := e.in.Key(name); err == nil {
			t.Errorf("%q: no error", name)
		}
	}

	noBackend := e.in
	noBackend.Backend = ""
	if _, err := noBackend.Key(Base); err == nil {
		t.Error("no backend: no error")
	}

	badFeature := e.in
	badFeature.BaseFeatures = []string{"../x"}
	if _, err := badFeature.Key(Base); err == nil {
		t.Error("feature ../x: no error")
	}

	e.remove("root/layers/base.sh")
	if _, err := e.in.Key("foo"); err == nil {
		t.Error("no layers/base.sh: no error")
	}

	from := newEnv(t, "node:24")
	from.remove("root/oci/kernel.sh")
	if _, err := from.in.Key(Base); err == nil {
		t.Error("--from without oci/kernel.sh: no error")
	}
}

// Without user layers, features or rebuild counters, keys still work, and
// a missing layer's key differs from an empty one's.
func TestKeyWithoutUserFolders(t *testing.T) {
	e := newEnv(t, "")
	in := e.in
	in.Layers, in.Features, in.Rebuilds = nil, nil, nil
	in.BaseFeatures = []string{"fzf"}
	missing, err := in.Key("foo")
	if err != nil {
		t.Fatal(err)
	}
	in.Layers = fstest.MapFS{"foo.sh": {}}
	empty, err := in.Key("foo")
	if err != nil {
		t.Fatal(err)
	}
	if missing == empty {
		t.Error("a missing layer and an empty one have the same key")
	}
}

// A built-in layer's key hashes devvm's script, never the user's of the
// same name; devvm's must be there.
func TestBuiltinLayerKey(t *testing.T) {
	e := newEnv(t, "")
	key := func() string {
		t.Helper()
		k, err := e.in.Key("foo+docker")
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	first := key()
	e.edit("layers/docker.sh") // the user's, made
	if key() != first {
		t.Error("a user's docker.sh changed the built-in layer's key")
	}
	e.edit("root/layers/docker.sh")
	if key() == first {
		t.Error("devvm's docker.sh didn't change its key")
	}
	e.remove("root/layers/docker.sh")
	if _, err := e.in.Key("docker"); err == nil {
		t.Error("no layers/docker.sh: no error")
	}
}

// devvm's own files, embedded, give base and every built-in layer a key.
func TestKeyOfTheRealFiles(t *testing.T) {
	in := Inputs{Backend: "lima", Assets: assets.FS}
	for _, name := range append([]string{Base}, Builtins...) {
		key, err := in.Key(name)
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(key) {
			t.Errorf("%s: key %q", name, key)
		}
	}
}

func TestParseChain(t *testing.T) {
	c := Chain{{"base", "0123456789ab"}, {"foo", "ba9876543210"}}
	if got := ParseChain(c.String()); !slices.Equal(got, c) {
		t.Errorf("ParseChain(%q) = %v", c.String(), got)
	}
	if got := ParseChain("  base=0123456789ab junk =x y= foo=ba9876543210 "); !slices.Equal(got, c) {
		t.Errorf("ParseChain skipping junk = %v", got)
	}
	if got := ParseChain(""); got != nil {
		t.Errorf("ParseChain(\"\") = %v", got)
	}
}

// A VM's chain is outdated where a key changed: that image and every one
// above it, never those below.
func TestOutdated(t *testing.T) {
	e := newEnv(t, "")
	chain, err := e.in.Chain("foo+bar")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.in.Outdated(chain); err != nil || got != nil {
		t.Errorf("Outdated before a change = %q, %v", got, err)
	}
	e.edit("layers/foo.sh")
	if got, err := e.in.Outdated(chain); err != nil || !slices.Equal(got, []string{"foo", "foo+bar"}) {
		t.Errorf("Outdated after editing foo = %q, %v", got, err)
	}
	if _, err := e.in.Outdated(Chain{{"Bad", "x"}}); err == nil {
		t.Error("Outdated took an invalid name")
	}
}

func TestLayerName(t *testing.T) {
	for name, want := range map[string]bool{
		"foo": true, "my-layer2": true, "0x": true,
		"base": false, "": false, "-x": false, "Foo": false, "a_b": false, "a+b": false, "../x": false,
		// Shared tools' names aren't layers'.
		"node": false, "go": false, "claude": false, "pnpm": true,
		// Command names are layers' too: devvm takes no image as a word.
		"build": true, "ls": true,
	} {
		if got := LayerName(name); got != want {
			t.Errorf("LayerName(%q) = %v", name, got)
		}
	}
}
