package build

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil { //nolint:gosec // the test's own folder
		t.Fatal(err)
	}
}

// testBuilder returns a builder on a fake backend with every feature, in a
// temporary folder with devvm's files and the user's layers foo and bar.
func testBuilder(t *testing.T) (*Builder, *backendtest.Fake) {
	t.Helper()
	dir := t.TempDir()
	d := paths.Dirs{Data: filepath.Join(dir, "data"), Config: filepath.Join(dir, "config"), Lima: filepath.Join(dir, "lima")}
	root := filepath.Join(dir, "root")
	write(t, filepath.Join(root, "base.yaml"), "cpus: 4\n")
	write(t, filepath.Join(root, "layers", "base.sh"), "#!/bin/bash\n")
	write(t, filepath.Join(d.Layers(), "foo.sh"), "#!/bin/bash\necho foo\n")
	write(t, filepath.Join(d.Layers(), "bar.sh"), "#!/bin/bash\necho bar\n")
	write(t, d.BaseFeatures(), "") // chosen: none
	caps := backend.Caps{}
	for _, f := range backend.Features {
		caps[f] = backend.Native
	}
	f := backendtest.New("fake", caps)
	f.Built = backend.Built{Origin: "https://example.com/ubuntu.img sha256:1111", Libc: "gnu"}
	// The shared tools: the repo's tools/, the stub mise resolving the
	// fixture lock.
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mise.toml", "build.sh"} {
		data, err := os.ReadFile(filepath.Join(repo, "tools", name)) //nolint:gosec // the repo's own files
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(root, "tools", name), string(data))
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "xdg")) // the run log
	t.Setenv("MISE_STUB_LOCK", filepath.Join(repo, "testdata", "mise.lock"))
	tl := tools.New(d, root)
	tl.Getenv = func(k string) string {
		if k == "DEVVM_HOST_MISE" {
			return filepath.Join(repo, "testdata", "stubs", "mise")
		}
		return ""
	}
	return &Builder{Dirs: d, Root: root, Backend: f, Tools: tl}, f
}

func isMsg(err error, id string) bool {
	m, ok := errors.AsType[msg.M](err)
	return ok && m.ID() == id
}

// Layers are the user's scripts with names image names can hold; nothing
// else gets past CheckLayers.
func TestLayersAndCheckLayers(t *testing.T) {
	b, _ := testBuilder(t)
	write(t, filepath.Join(b.Dirs.Layers(), "Bad_Name.sh"), "")
	write(t, filepath.Join(b.Dirs.Layers(), "base.sh"), "")
	write(t, filepath.Join(b.Root, "layers", "stray.sh"), "") // devvm's own folder holds only base
	if err := os.Mkdir(filepath.Join(b.Dirs.Layers(), "dir.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A layer with a shared tool's name isn't one.
	write(t, filepath.Join(b.Dirs.Layers(), "node.sh"), "")
	if got, want := b.Layers(), []string{"base", "bar", "foo"}; !slices.Equal(got, want) {
		t.Errorf("Layers() = %q, want %q", got, want)
	}
	if got, want := b.Environments(), []string{"base", "node", "python", "rust", "go", "bar", "foo"}; !slices.Equal(got, want) {
		t.Errorf("Environments() = %q, want %q", got, want)
	}
	if err := b.CheckLayers("stray"); err == nil || err.Error() != msg.UnknownLayer("claude codex opencode", "base node python rust go bar foo", "stray").Error() {
		t.Errorf("CheckLayers(stray) = %v", err)
	}
	for _, ok := range []string{"base", "foo", "foo+bar", "base+foo", "node+foo+claude", "go"} {
		if err := b.CheckLayers(ok); err != nil {
			t.Errorf("CheckLayers(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"", "stray", "Bad_Name", "dir", "foo+", "../../x", "foo+nope", "-x"} {
		if err := b.CheckLayers(bad); !isMsg(err, "unknown_layer") {
			t.Errorf("CheckLayers(%q) = %v, want unknown_layer", bad, err)
		}
	}
}

// A name with tools builds its image (base and its layers, not the tools)
// and the missing tool images, from base's image, for base's libc.
func TestBuildTools(t *testing.T) {
	b, f := testBuilder(t)
	f.Built.Libc = "musl"
	if err := b.Build(t.Context(), "claude+foo+node"); err != nil {
		t.Fatal(err)
	}
	if ms := f.Methods(); !slices.Equal(ms, []string{"Images", "Images", "BuildImage", "BuildImage", "BuildTools"}) {
		t.Fatalf("calls = %q", ms)
	}
	chain, err := b.Chain("foo")
	if err != nil {
		t.Fatal(err)
	}
	var tools backend.ToolsSpec
	for _, c := range f.Calls() {
		if c.Method == "BuildTools" {
			tools, _ = c.Args[0].(backend.ToolsSpec)
		}
	}
	if tools.Base != chain[0].Key || len(tools.Args) != 12 || tools.Args[0] != "node" || tools.Args[9] != "claude" {
		t.Errorf("tools spec = %+v", tools)
	}
	tg, err := b.ToolTarget()
	if tg.Libc != "musl" || tg.Format != "" || err != nil {
		t.Errorf("ToolTarget = %+v, %v", tg, err)
	}
	files, err := b.Tools.Files(t.Context(), []string{"node", "claude"}, tg)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if !b.Tools.Built(file) || !slices.Contains(tools.Args, file) {
			t.Errorf("%s: not built for musl", file)
		}
	}
	// Built: nothing to do.
	n := len(f.Calls())
	if err := b.Build(t.Context(), "node+foo+claude"); err != nil || slices.Contains(f.Methods()[n:], "BuildTools") {
		t.Errorf("built again: %v %q", err, f.Methods()[n:])
	}
}

// The shared tools aren't images: the lock versions them, so they aren't
// rebuilt.
func TestRebuildRefusesTools(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	b, f := testBuilder(t)
	if err := b.Rebuild(t.Context(), "node+claude"); err == nil || err.Error() != "tools_not_images tools=node claude" {
		t.Errorf("Rebuild = %v", err)
	}
	if len(f.Calls()) > 0 {
		t.Errorf("calls = %q", f.Methods())
	}
}

// Before base is built, its libc is gnu.
func TestBaseLibcUnbuilt(t *testing.T) {
	b, _ := testBuilder(t)
	if libc, err := b.BaseLibc(); libc != "gnu" || err != nil {
		t.Errorf("BaseLibc = %q, %v", libc, err)
	}
}

func TestBuild(t *testing.T) {
	b, f := testBuilder(t)
	ctx := t.Context()
	if err := b.Build(ctx, "foo+bar"); err != nil {
		t.Fatal(err)
	}
	chain, err := b.Chain("foo+bar")
	if err != nil {
		t.Fatal(err)
	}
	var specs []backend.ImageSpec
	for _, c := range f.Calls() {
		if c.Method == "BuildImage" {
			specs = append(specs, c.Args[0].(backend.ImageSpec))
		}
	}
	if len(specs) != 3 {
		t.Fatalf("built %d images, want 3: %+v", len(specs), specs)
	}
	for i, want := range []backend.ImageSpec{
		{Image: "base", Key: chain[0].Key, Scripts: []backend.Script{{Name: "base.sh", Path: filepath.Join(b.Root, "layers", "base.sh")}}},
		{Image: "foo", Key: chain[1].Key, Parent: chain[0].Key, Scripts: []backend.Script{{Name: "foo.sh", Path: filepath.Join(b.Dirs.Layers(), "foo.sh")}}},
		{Image: "foo+bar", Key: chain[2].Key, Parent: chain[1].Key, Scripts: []backend.Script{{Name: "bar.sh", Path: filepath.Join(b.Dirs.Layers(), "bar.sh")}}},
	} {
		want.Net = backend.NetInternet
		got := specs[i]
		if got.Image != want.Image || got.Key != want.Key || got.Parent != want.Parent || got.Net != want.Net ||
			got.From != nil || got.MemoryGiB != 0 || !slices.Equal(got.Scripts, want.Scripts) {
			t.Errorf("build %d: %+v, want %+v", i, got, want)
		}
	}
	base, err := state.ReadImage(b.Dirs, chain[0].Key)
	if err != nil || base != (state.Image{Name: "base", Chain: chain[:1].String(), Digest: f.Built.Origin, Libc: "gnu"}) {
		t.Errorf("base's record: %+v, %v", base, err)
	}
	top, err := state.ReadImage(b.Dirs, chain[2].Key)
	if err != nil || top != (state.Image{Name: "foo+bar", Chain: chain.String(), Libc: "gnu"}) {
		t.Errorf("foo+bar's record: %+v, %v", top, err)
	}

	// Built: nothing to do.
	n := len(f.Calls())
	if err := b.Build(ctx, "base+foo+bar"); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls()[n:] {
		if c.Method != "Images" {
			t.Errorf("a built image called %s", c.Method)
		}
	}
}

// An image above base keeps its parent's C library.
func TestBuildKeepsTheLibc(t *testing.T) {
	b, f := testBuilder(t)
	f.Built.Libc = "musl"
	if err := b.Build(t.Context(), "foo"); err != nil {
		t.Fatal(err)
	}
	key, err := b.Key("foo")
	if err != nil {
		t.Fatal(err)
	}
	if rec, err := state.ReadImage(b.Dirs, key); err != nil || rec.Libc != "musl" || rec.Digest != "" {
		t.Errorf("foo's record: %+v, %v", rec, err)
	}
}

// A backend that can't build, or not confined, is refused before anything
// is built.
func TestBuildRequires(t *testing.T) {
	b, f := testBuilder(t)
	delete(f.Capabilities, backend.FeatureBuildConfinement)
	if err := b.Build(t.Context(), "foo"); !isMsg(err, "backend_lacks") {
		t.Errorf("Build = %v, want backend_lacks", err)
	}
	if slices.Contains(f.Methods(), "BuildImage") {
		t.Error("built anyway")
	}
}

// A failed build writes no record, so the next build starts over.
func TestFailedBuildHasNoRecord(t *testing.T) {
	b, f := testBuilder(t)
	f.Errs = map[string]error{"BuildImage": backend.Logged(msg.ScriptFailed("base.sh", "x"))}
	if err := b.Build(t.Context(), "base"); err == nil {
		t.Fatal("Build succeeded")
	}
	if entries, _ := os.ReadDir(b.Dirs.Images()); len(entries) != 0 {
		t.Errorf("records after a failed build: %v", entries)
	}
}

func TestList(t *testing.T) {
	b, f := testBuilder(t)
	ctx := t.Context()
	if err := b.Build(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	f.ImageKeys = append(f.ImageKeys, "0123456789ab") // no record: unfinished
	write(t, filepath.Join(b.Dirs.Layers(), "foo.sh"), "#!/bin/bash\necho edited\n")
	imgs, err := b.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, img := range imgs {
		got[img.Name] = img.Current
	}
	if want := map[string]bool{"base": true, "foo": false, "": false}; !maps.Equal(got, want) {
		t.Errorf("List: %v, want %v", got, want)
	}
}

// Rebuild gives the image a new key, and deletes the old one and those on
// it, never the others.
func TestRebuild(t *testing.T) {
	b, f := testBuilder(t)
	ctx := t.Context()
	if err := b.Build(ctx, "foo+bar"); err != nil {
		t.Fatal(err)
	}
	old, err := b.Chain("foo+bar")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Rebuild(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	newFoo, err := b.Key("foo")
	if err != nil {
		t.Fatal(err)
	}
	if newFoo == old[1].Key {
		t.Error("foo kept its key")
	}
	var deleted []string
	for _, c := range f.Calls() {
		if c.Method == "DeleteImage" {
			deleted = append(deleted, c.Args[0].(string))
		}
	}
	if want := []string{old[1].Key, old[2].Key}; !slices.Equal(deleted, want) {
		t.Errorf("deleted %q, want %q", deleted, want)
	}
	if want := []string{old[0].Key, newFoo}; !slices.Equal(f.ImageKeys, want) {
		t.Errorf("images left %q, want %q", f.ImageKeys, want)
	}
	for _, k := range []string{old[1].Key, old[2].Key} {
		if _, err := state.ReadImage(b.Dirs, k); err == nil {
			t.Errorf("%s kept its record", k)
		}
	}
}

// An old image the backend won't delete (a VM uses it) stays.
func TestRebuildKeepsImagesInUse(t *testing.T) {
	b, f := testBuilder(t)
	ctx := t.Context()
	if err := b.Build(ctx, "base"); err != nil {
		t.Fatal(err)
	}
	old := f.ImageKeys[0]
	f.Errs = map[string]error{"DeleteImage": msg.ImageInUse(old, "devvm-p-0123456789abcd")}
	if err := b.Rebuild(ctx, "base"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ReadImage(b.Dirs, old); err != nil {
		t.Errorf("the image in use lost its record: %v", err)
	}
}

// fromBuilder is testBuilder with --from node:24, the conversion's
// scripts, and a registry whose digest for it is *digest; lookups counts
// what it's asked.
func fromBuilder(t *testing.T, digest *string, lookups *int) (*Builder, *backendtest.Fake) {
	t.Helper()
	b, f := testBuilder(t)
	for _, s := range []string{"build.sh", "bootify.sh", "kernel.sh"} {
		write(t, filepath.Join(b.Root, "oci", s), "#!/bin/bash\n")
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*lookups++
		w.Header().Set("Docker-Content-Digest", *digest)
	}))
	t.Cleanup(srv.Close)
	b.From = "node:24"
	b.Lookup = &from.Lookup{
		Dirs:     b.Dirs,
		Registry: &from.Registry{Client: srv.Client(), Base: func(string) string { return srv.URL }},
		DockerID: func(context.Context, string) (string, bool) { t.Error("Docker asked"); return "", false },
		Warn:     func(m msg.M) { t.Errorf("warned: %s", m.Names()) },
	}
	return b, f
}

// --from: base starts from the image's digest, looked up when building;
// every image on it records both.
func TestBuildFrom(t *testing.T) {
	digest, lookups := "sha256:"+strings.Repeat("a", 64), 0
	b, f := fromBuilder(t, &digest, &lookups)
	f.Built = backend.Built{Origin: "node@" + digest, Libc: "musl"}
	if err := b.Build(t.Context(), "foo"); err != nil {
		t.Fatal(err)
	}
	if lookups != 1 {
		t.Errorf("looked up %d times", lookups)
	}
	chain, err := b.Chain("foo")
	if err != nil {
		t.Fatal(err)
	}
	var specs []backend.ImageSpec
	for _, c := range f.Calls() {
		if c.Method == "BuildImage" {
			specs = append(specs, c.Args[0].(backend.ImageSpec))
		}
	}
	if len(specs) != 2 || specs[0].From == nil || *specs[0].From != (backend.From{Ref: "node:24", Digest: "node@" + digest}) || specs[1].From != nil {
		t.Fatalf("specs: %+v", specs)
	}
	for i, name := range []string{"base", "foo"} {
		rec, err := state.ReadImage(b.Dirs, chain[i].Key)
		if want := (state.Image{Name: name, Chain: chain[:i+1].String(), From: "node:24", Digest: "node@" + digest, Libc: "musl"}); err != nil || rec != want {
			t.Errorf("%s's record: %+v, %v; want %+v", name, rec, err, want)
		}
	}
	// Ubuntu's base is another image.
	ubuntu := &Builder{Dirs: b.Dirs, Root: b.Root, Backend: b.Backend, Tools: b.Tools}
	if k, err := ubuntu.Key("base"); err != nil || k == chain[0].Key {
		t.Errorf("Ubuntu's base: %s, %v", k, err)
	}
	// Keys, lists and outdated checks use the recorded digest: no lookups.
	if imgs, err := b.List(t.Context()); err != nil || len(imgs) != 2 || !imgs[0].Current || !imgs[1].Current || imgs[0].From != "node:24" {
		t.Errorf("List: %+v, %v", imgs, err)
	}
	if err := b.WarnIfOutdated(chain.String()); err != nil || lookups != 1 {
		t.Errorf("WarnIfOutdated: %v, %d lookups", err, lookups)
	}

	// The tag moved: the next build looks it up, and base and foo are new.
	digest = "sha256:" + strings.Repeat("b", 64)
	again := &Builder{Dirs: b.Dirs, Root: b.Root, Backend: b.Backend, Tools: b.Tools, From: b.From, Lookup: b.Lookup}
	if err := again.Build(t.Context(), "foo"); err != nil {
		t.Fatal(err)
	}
	moved, err := again.Chain("foo")
	if err != nil {
		t.Fatal(err)
	}
	if moved[0].Key == chain[0].Key || moved[1].Key == chain[1].Key {
		t.Errorf("a moved tag kept the keys: %s, %s", moved, chain)
	}
	if imgs, err := again.List(t.Context()); err != nil || len(imgs) != 4 || imgs[0].Current || imgs[1].Current {
		t.Errorf("after the move, the old images are stale: %+v, %v", imgs, err)
	}
}

// A pinned digest is never looked up, and rebuilds count apart from
// other bases'.
func TestBuildFromPinned(t *testing.T) {
	digest, lookups := "", 0
	b, f := fromBuilder(t, &digest, &lookups)
	b.From = "node:24@sha256:" + strings.Repeat("c", 64)
	if err := b.Rebuild(t.Context(), "base"); err != nil {
		t.Fatal(err)
	}
	if lookups != 0 {
		t.Errorf("looked up %d times", lookups)
	}
	if _, err := os.Stat(b.Dirs.RebuildFile("base", b.From)); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(b.Dirs.RebuildFile("base", "")); err == nil {
		t.Error("Ubuntu's base was rebuilt")
	}
	for _, c := range f.Calls() {
		if c.Method == "BuildImage" && c.Args[0].(backend.ImageSpec).From.Digest != b.From {
			t.Errorf("built %+v", c.Args[0])
		}
	}
}

// A backend without --from is refused before anything is built.
func TestBuildFromRequires(t *testing.T) {
	digest, lookups := "sha256:"+strings.Repeat("a", 64), 0
	b, f := fromBuilder(t, &digest, &lookups)
	delete(f.Capabilities, backend.FeatureFrom)
	if err := b.Build(t.Context(), "foo"); !isMsg(err, "backend_lacks") {
		t.Errorf("Build = %v, want backend_lacks", err)
	}
	if slices.Contains(f.Methods(), "BuildImage") {
		t.Error("built anyway")
	}
}
