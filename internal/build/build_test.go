package build

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
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

// testEnv returns an Env on a fake backend with every feature, in a
// temporary folder with devvm's files and the user's layers foo and bar.
func testEnv(t *testing.T) (*Env, *backendtest.Fake) {
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
	// The shared tools: the repo's assets/shared-tools/, the stub mise resolving the
	// fixture lock.
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mise.toml", "build.sh"} {
		data, err := os.ReadFile(filepath.Join(repo, "assets", "shared-tools", name)) //nolint:gosec // the repo's own files
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(root, "shared-tools", name), string(data))
	}
	t.Setenv("MISE_STUB_LOCK", filepath.Join(repo, "testdata", "mise.lock"))
	tl := tools.New(d, os.DirFS(root), quietUI{}, platform.Host{})
	tl.FreeSpace = func(string) (uint64, error) { return 1 << 40, nil }
	tl.Getenv = func(string) string { return "" }
	tl.Mise = filepath.Join(repo, "testdata", "stubs", "mise")
	return quiet(&Env{Dirs: d, Assets: os.DirFS(root), Backend: f, Tools: tl}), f
}

// assetsDir is the folder of testEnv's devvm's files, its Assets.
func assetsDir(b *Env) string { return filepath.Join(filepath.Dir(b.Dirs.Data), "root") }

func script(name, data string) backend.Script { return backend.Script{Name: name, Data: []byte(data)} }

// sameScripts reports whether two lists of scripts have the same names
// and contents, in order.
func sameScripts(a, b []backend.Script) bool {
	return slices.EqualFunc(a, b, func(x, y backend.Script) bool { return x.Name == y.Name && bytes.Equal(x.Data, y.Data) })
}

// quiet makes b print nothing, keep the features chosen, and answer yes,
// without a terminal.
func quiet(b *Env) *Env {
	none := func(msg.M) {}
	b.Info, b.Warn, b.Say = none, none, none
	b.Confirm = func(msg.M) bool { return true }
	b.Pick = func(fs []Feature) []Feature { return fs }
	b.Terminal = func() bool { return false }
	b.LogNote = func() {}
	return b
}

// quietUI is the shared tools' UI: it prints nothing, runs commands as
// they are, their output dropped, and keeps no cleanups.
type quietUI struct{}

func (quietUI) Info(msg.M)                                                {}
func (quietUI) Warn(msg.M)                                                {}
func (quietUI) LogNote()                                                  {}
func (quietUI) Status(msg.M) func()                                       { return func() {} }
func (quietUI) AddCleanup(func(context.Context)) func()                   { return func() {} }
func (quietUI) LoggedCmd(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }

func isMsg(err error, id string) bool {
	m, ok := errors.AsType[msg.M](err)
	return ok && m.ID() == id
}

// Layers are devvm's built-in ones and the user's scripts with names image
// names can hold; nothing else gets past CheckLayers.
func TestLayersAndCheckLayers(t *testing.T) {
	b, _ := testEnv(t)
	write(t, filepath.Join(b.Dirs.Layers(), "Bad_Name.sh"), "")
	write(t, filepath.Join(b.Dirs.Layers(), "base.sh"), "")
	write(t, filepath.Join(assetsDir(b), "layers", "stray.sh"), "") // not a built-in layer
	if err := os.Mkdir(filepath.Join(b.Dirs.Layers(), "dir.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A layer with a shared tool's name isn't one; one with a built-in
	// layer's is the built-in one, listed once.
	write(t, filepath.Join(b.Dirs.Layers(), "node.sh"), "")
	write(t, filepath.Join(b.Dirs.Layers(), "docker.sh"), "")
	if got, want := b.Layers(), []string{"base", "bar", "docker", "foo", "nix"}; !slices.Equal(got, want) {
		t.Errorf("Layers() = %q, want %q", got, want)
	}
	if got, want := b.Environments(), []string{"base", "node", "python", "rust", "go", "bar", "docker", "foo", "nix"}; !slices.Equal(got, want) {
		t.Errorf("Environments() = %q, want %q", got, want)
	}
	if err := b.CheckLayers("stray"); err == nil || err.Error() != msg.UnknownLayer("claude codex opencode", "base node python rust go bar docker foo nix", "stray").Error() {
		t.Errorf("CheckLayers(stray) = %v", err)
	}
	for _, ok := range []string{"base", "foo", "foo+bar", "base+foo", "node+foo+claude", "go", "docker", "node+docker", "nix", "node+nix+claude"} {
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
	b, f := testEnv(t)
	f.Built.Libc = "musl"
	if err := (Builds{b}).Build(t.Context(), "claude+foo+node"); err != nil {
		t.Fatal(err)
	}
	if ms := f.Methods(); !slices.Equal(ms, []string{"Images", "Images", "BuildImage", "BuildImage", "BuildTools"}) {
		t.Fatalf("calls = %q", ms)
	}
	chain, err := b.Chain("foo")
	if err != nil {
		t.Fatal(err)
	}
	var spec backend.ToolsSpec
	for _, c := range f.Calls() {
		if c.Method == "BuildTools" {
			spec, _ = c.Args[0].(backend.ToolsSpec)
		}
	}
	if spec.Base != chain[0].Key || len(spec.Args) != 12 || spec.Args[0] != "node" || spec.Args[9] != "claude" {
		t.Errorf("tools spec = %+v", spec)
	}
	tg, err := b.ToolTarget()
	if tg.Libc != "musl" || tg.Format != "" || err != nil {
		t.Errorf("ToolTarget = %+v, %v", tg, err)
	}
	files, err := tools.Versions{Env: b.Tools}.Files(t.Context(), []string{"node", "claude"}, tg)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if !(tools.Share{Env: b.Tools}).Built(file) || !slices.Contains(spec.Args, file) {
			t.Errorf("%s: not built for musl", file)
		}
	}
	// Built: nothing to do.
	n := len(f.Calls())
	if err := (Builds{b}).Build(t.Context(), "node+foo+claude"); err != nil || slices.Contains(f.Methods()[n:], "BuildTools") {
		t.Errorf("built again: %v %q", err, f.Methods()[n:])
	}
}

// The shared tools aren't images: the lock versions them, so they aren't
// rebuilt.
func TestRebuildRefusesTools(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	b, f := testEnv(t)
	if err := (Builds{b}).Rebuild(t.Context(), "node+claude"); err == nil || err.Error() != "tools_not_images tools=node claude" {
		t.Errorf("Rebuild = %v", err)
	}
	if len(f.Calls()) > 0 {
		t.Errorf("calls = %q", f.Methods())
	}
}

// Before base is built, its libc is gnu.
func TestBaseLibcUnbuilt(t *testing.T) {
	b, _ := testEnv(t)
	if libc, err := b.BaseLibc(); libc != "gnu" || err != nil {
		t.Errorf("BaseLibc = %q, %v", libc, err)
	}
}

func TestBuild(t *testing.T) {
	b, f := testEnv(t)
	ctx := t.Context()
	if err := (Builds{b}).Build(ctx, "foo+bar"); err != nil {
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
		{Image: "base", Key: chain[0].Key, Scripts: []backend.Script{script("base.sh", "#!/bin/bash\n")}},
		{Image: "foo", Key: chain[1].Key, Parent: chain[0].Key, Scripts: []backend.Script{script("foo.sh", "#!/bin/bash\necho foo\n")}},
		{Image: "foo+bar", Key: chain[2].Key, Parent: chain[1].Key, Scripts: []backend.Script{script("bar.sh", "#!/bin/bash\necho bar\n")}},
	} {
		want.Net = backend.NetInternet
		got := specs[i]
		if got.Image != want.Image || got.Key != want.Key || got.Parent != want.Parent || got.Net != want.Net ||
			got.From != nil || got.MemoryGiB != 0 || !sameScripts(got.Scripts, want.Scripts) {
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
	if err := (Builds{b}).Build(ctx, "base+foo+bar"); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls()[n:] {
		if c.Method != "Images" {
			t.Errorf("a built image called %s", c.Method)
		}
	}
}

// A built-in layer runs devvm's script, not the user's of the same name.
func TestBuildBuiltinLayer(t *testing.T) {
	b, f := testEnv(t)
	write(t, filepath.Join(assetsDir(b), "layers", "docker.sh"), "#!/bin/bash\necho devvm's\n")
	write(t, filepath.Join(b.Dirs.Layers(), "docker.sh"), "#!/bin/bash\necho yours\n")
	if err := (Builds{b}).Build(t.Context(), "foo+docker"); err != nil {
		t.Fatal(err)
	}
	calls := f.Calls()
	last := calls[len(calls)-1]
	if last.Method != "BuildImage" {
		t.Fatalf("last call %s", last.Method)
	}
	got := last.Args[0].(backend.ImageSpec)
	if got.Image != "foo+docker" || !sameScripts(got.Scripts, []backend.Script{script("docker.sh", "#!/bin/bash\necho devvm's\n")}) {
		t.Errorf("built %+v", got)
	}
	if data, err := b.LayerData("docker"); err != nil || string(data) != "#!/bin/bash\necho devvm's\n" {
		t.Errorf("LayerData(docker) = %q, %v", data, err)
	}
}

// An image above base keeps its parent's C library.
func TestBuildKeepsTheLibc(t *testing.T) {
	b, f := testEnv(t)
	f.Built.Libc = "musl"
	if err := (Builds{b}).Build(t.Context(), "foo"); err != nil {
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

// A build says once what base got when the backend fitted it to the host,
// and says nothing about it when it didn't.
func TestBuildSaysFitted(t *testing.T) {
	for _, c := range []struct {
		fitted backend.Size
		want   []string
	}{
		{backend.Size{}, nil},
		{backend.Size{CPUs: 2, MemoryGiB: 4}, []string{"base_fitted cpus=2 memory=4"}},
		{backend.Size{CPUs: 4, MemoryGiB: 3.5}, []string{"base_fitted cpus=4 memory=3.5"}},
	} {
		b, f := testEnv(t)
		f.Built.Fitted = c.fitted
		var got []string
		b.Info = func(m msg.M) {
			if m.ID() == "base_fitted" {
				got = append(got, m.Names())
			}
		}
		if err := (Builds{b}).Build(t.Context(), "base"); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("fitted %+v: said %q, want %q", c.fitted, got, c.want)
		}
	}
}

// A backend that can't build, or not confined, is refused before anything
// is built.
func TestBuildRequires(t *testing.T) {
	b, f := testEnv(t)
	delete(f.Capabilities, backend.FeatureBuildConfinement)
	if err := (Builds{b}).Build(t.Context(), "foo"); !isMsg(err, "backend_lacks") {
		t.Errorf("Build = %v, want backend_lacks", err)
	}
	if slices.Contains(f.Methods(), "BuildImage") {
		t.Error("built anyway")
	}
}

// stored is a fake whose disks are in dirs (backend.Storage).
type stored struct {
	*backendtest.Fake
	dirs []string
}

func (s stored) DiskDirs() []string { return s.dirs }

func (s stored) Floors() backend.Floors {
	return backend.Floors{Base: 12 << 30, Image: 8 << 30, VM: 4 << 30, Tools: 3 << 30, PerTool: 1 << 30}
}

// Below the free space building base wants, or another image, nothing of
// it is made: the backend is only asked which images it has.
func TestBuildRefusesLowSpace(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	b, f := testEnv(t)
	b.Backend = stored{f, []string{"/lima"}}
	free := uint64(12<<30 - 1)
	b.FreeSpace = func(dir string) (uint64, error) {
		if dir != "/lima" {
			t.Errorf("checked %s", dir)
		}
		return free, nil
	}
	if err := (Builds{b}).Build(t.Context(), "foo"); err == nil || err.Error() != "disk_low dir=/lima free=11.9 name=base need=12" {
		t.Errorf("Build with %d free = %v", free, err)
	}
	if ms := f.Methods(); !slices.Equal(ms, []string{"Images", "Images"}) {
		t.Errorf("calls = %q", ms)
	}
	free = 12 << 30
	if err := (Builds{b}).Build(t.Context(), "base"); err != nil {
		t.Fatal(err)
	}
	free = 8<<30 - 1
	n := len(f.Calls())
	if err := (Builds{b}).Build(t.Context(), "foo"); err == nil || err.Error() != "disk_low dir=/lima free=7.9 name=foo need=8" {
		t.Errorf("Build of foo with %d free = %v", free, err)
	}
	if slices.Contains(f.Methods()[n:], "BuildImage") {
		t.Errorf("built anyway: %q", f.Methods()[n:])
	}
	key, err := b.Key("foo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ReadImage(b.Dirs, key); err == nil {
		t.Error("foo has a record")
	}
}

// An image the backend has, with its record, needs nothing of building:
// a backend that can't build it now still uses it. One without its record
// is refused before it's deleted.
func TestBuiltNeedsNoBuild(t *testing.T) {
	b, f := testEnv(t)
	if err := (Builds{b}).Build(t.Context(), "base"); err != nil {
		t.Fatal(err)
	}
	delete(f.Capabilities, backend.FeatureBuildConfinement)
	before := len(f.Calls())
	if err := (Builds{b}).Build(t.Context(), "base"); err != nil {
		t.Errorf("Build of a built image = %v", err)
	}
	key, err := b.Key("base")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.RemoveImage(b.Dirs, key); err != nil {
		t.Fatal(err)
	}
	if err := (Builds{b}).Build(t.Context(), "base"); !isMsg(err, "backend_lacks") {
		t.Errorf("Build of a leftover = %v, want backend_lacks", err)
	}
	if ms := f.Methods()[before:]; slices.Contains(ms, "DeleteImage") || slices.Contains(ms, "BuildImage") {
		t.Errorf("calls = %q", ms)
	}
}

// An image another devvm is building (no record, its lock held) is left
// alone: a build of it waits, saying so, rebuild and Delete don't delete
// it, and it's listed as building. Once its lock is free, it's a
// leftover, deleted and built again.
func TestBuildInProgress(t *testing.T) {
	b, f := testEnv(t)
	ctx := t.Context()
	if err := (Builds{b}).Build(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	key, err := b.Key("foo+bar")
	if err != nil {
		t.Fatal(err)
	}
	f.ImageKeys = append(f.ImageKeys, key) // another devvm's, unfinished
	path, err := state.ImageLock(b.Dirs, key)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := platform.LockFile(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	deleted := func() bool {
		for _, c := range f.Calls() {
			if c.Method == "DeleteImage" && c.Args[0] == key {
				return true
			}
		}
		return false
	}
	var said []string
	b.Info = func(m msg.M) { said = append(said, m.Names()) }
	b.WaitNote = time.Millisecond

	soon, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if err := (Builds{b}).Build(soon, "foo+bar"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Build while another builds it = %v", err)
	}
	if !slices.Contains(said, "image_build_waiting image=foo+bar") {
		t.Errorf("said %q", said)
	}
	ls, err := (Images{b}).Listing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(ls, func(l Listed) bool { return l.Key == key }); i < 0 || ls[i].State != StateBuilding {
		t.Errorf("listed %+v", ls)
	}
	if err := (Images{b}).Delete(ctx, []string{key}); !isMsg(err, "image_building") {
		t.Errorf("Delete while it builds = %v", err)
	}
	b.Force = true
	if err := (Builds{b}).Rebuild(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	if deleted() {
		t.Fatal("deleted while another devvm builds it")
	}

	unlock()
	if ls, err = (Images{b}).Listing(ctx); err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(ls, func(l Listed) bool { return l.Key == key }); i < 0 || ls[i].State != StateUnfinished {
		t.Errorf("listed once let go %+v", ls)
	}
	if err := (Images{b}).Delete(ctx, []string{key}); err != nil || !deleted() {
		t.Errorf("Delete once let go = %v, deleted %v", err, deleted())
	}
}

// Two builds of one image take turns: the second finds it built.
func TestBuildTakesTurns(t *testing.T) {
	b, f := testEnv(t)
	ctx := t.Context()
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- (Builds{b}).Build(ctx, "foo") }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for _, m := range f.Methods() {
		if m == "BuildImage" {
			n++
		}
	}
	if n != 2 { // base and foo, once each
		t.Errorf("built %d images: %q", n, f.Methods())
	}
}

// A failed build writes no record, so the next build starts over.
func TestFailedBuildHasNoRecord(t *testing.T) {
	b, f := testEnv(t)
	f.Errs = map[string]error{"BuildImage": backend.Logged(msg.ScriptFailed("base.sh", "x"))}
	if err := (Builds{b}).Build(t.Context(), "base"); err == nil {
		t.Fatal("Build succeeded")
	}
	if found, err := state.AnyImage(b.Dirs); found || err != nil {
		t.Errorf("records after a failed build: %v, %v", found, err)
	}
}

func TestList(t *testing.T) {
	b, f := testEnv(t)
	ctx := t.Context()
	if err := (Builds{b}).Build(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	f.ImageKeys = append(f.ImageKeys, "0123456789ab") // no record: unfinished
	write(t, filepath.Join(b.Dirs.Layers(), "foo.sh"), "#!/bin/bash\necho edited\n")
	imgs, err := (Images{b}).List(ctx)
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
	b, f := testEnv(t)
	ctx := t.Context()
	if err := (Builds{b}).Build(ctx, "foo+bar"); err != nil {
		t.Fatal(err)
	}
	old, err := b.Chain("foo+bar")
	if err != nil {
		t.Fatal(err)
	}
	// foo+bar is built on foo: without a terminal, only with --force.
	if m, ok := errors.AsType[msg.M]((Builds{b}).Rebuild(ctx, "foo")); !ok || m.Names() != "force_needed command=devvm rebuild" {
		t.Fatalf("rebuild without a terminal or --force: %v", m)
	}
	b.Force = true
	if err := (Builds{b}).Rebuild(ctx, "foo"); err != nil {
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
	b, f := testEnv(t)
	ctx := t.Context()
	if err := (Builds{b}).Build(ctx, "base"); err != nil {
		t.Fatal(err)
	}
	old := f.ImageKeys[0]
	f.Errs = map[string]error{"DeleteImage": msg.ImageInUse(old, "devvm-p-0123456789abcd")}
	if err := (Builds{b}).Rebuild(ctx, "base"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ReadImage(b.Dirs, old); err != nil {
		t.Errorf("the image in use lost its record: %v", err)
	}
}

// fromEnv is testEnv with --from node:24, the conversion's
// scripts, and a registry whose digest for it is *digest; lookups counts
// what it's asked.
func fromEnv(t *testing.T, digest *string, lookups *int) (*Env, *backendtest.Fake) {
	t.Helper()
	b, f := testEnv(t)
	for _, s := range []string{"build.sh", "bootify.sh", "kernel.sh"} {
		write(t, filepath.Join(assetsDir(b), "oci", s), "#!/bin/bash\n")
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
	b, f := fromEnv(t, &digest, &lookups)
	f.Built = backend.Built{Origin: "node@" + digest, Libc: "musl"}
	if err := (Builds{b}).Build(t.Context(), "foo"); err != nil {
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
	ubuntu := &Env{Dirs: b.Dirs, Assets: b.Assets, Backend: b.Backend, Tools: b.Tools}
	if k, err := ubuntu.Key("base"); err != nil || k == chain[0].Key {
		t.Errorf("Ubuntu's base: %s, %v", k, err)
	}
	// Keys, lists and outdated checks use the recorded digest: no lookups.
	if imgs, err := (Images{b}).List(t.Context()); err != nil || len(imgs) != 2 || !imgs[0].Current || !imgs[1].Current || imgs[0].From != "node:24" {
		t.Errorf("List: %+v, %v", imgs, err)
	}
	if err := (Builds{b}).WarnIfOutdated(chain.String()); err != nil || lookups != 1 {
		t.Errorf("WarnIfOutdated: %v, %d lookups", err, lookups)
	}

	// The tag moved: the next build looks it up, and base and foo are new.
	digest = "sha256:" + strings.Repeat("b", 64)
	again := quiet(&Env{Dirs: b.Dirs, Assets: b.Assets, Backend: b.Backend, Tools: b.Tools, From: b.From, Lookup: b.Lookup})
	if err := (Builds{again}).Build(t.Context(), "foo"); err != nil {
		t.Fatal(err)
	}
	moved, err := again.Chain("foo")
	if err != nil {
		t.Fatal(err)
	}
	if moved[0].Key == chain[0].Key || moved[1].Key == chain[1].Key {
		t.Errorf("a moved tag kept the keys: %s, %s", moved, chain)
	}
	if imgs, err := (Images{again}).List(t.Context()); err != nil || len(imgs) != 4 || imgs[0].Current || imgs[1].Current {
		t.Errorf("after the move, the old images are stale: %+v, %v", imgs, err)
	}
}

// A pinned digest is never looked up, and rebuilds count apart from
// other bases'.
func TestBuildFromPinned(t *testing.T) {
	digest, lookups := "", 0
	b, f := fromEnv(t, &digest, &lookups)
	b.From = "node:24@sha256:" + strings.Repeat("c", 64)
	if err := (Builds{b}).Rebuild(t.Context(), "base"); err != nil {
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
	b, f := fromEnv(t, &digest, &lookups)
	delete(f.Capabilities, backend.FeatureFrom)
	if err := (Builds{b}).Build(t.Context(), "foo"); !isMsg(err, "backend_lacks") {
		t.Errorf("Build = %v, want backend_lacks", err)
	}
	if slices.Contains(f.Methods(), "BuildImage") {
		t.Error("built anyway")
	}
}
