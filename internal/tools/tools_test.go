package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

func repo(t *testing.T, parts ...string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join(append([]string{"..", ".."}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // the test's own files
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) []byte { return read(t, repo(t, "testdata", "mise.lock")) }

// lockTools returns the tools a lock has entries for, in its order.
func lockTools(l Lock) []string {
	var out []string
	for _, line := range lines(l) {
		if t, ok := strings.CutPrefix(line, "[[tools."); ok {
			if t, ok = strings.CutSuffix(t, "]]"); ok {
				out = append(out, t)
			}
		}
	}
	return out
}

// env is a sandbox: devvm's folders, a copy of tools/, the stub mise
// resolving the fixture lock, and what the tools printed.
type env struct {
	*Tools
	log    string // the stub's calls
	said   *[]string
	detach *[]string
}

func newEnv(t *testing.T) env {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "xdg")) // the run log
	log := filepath.Join(dir, "calls")
	write(t, log, nil)
	t.Setenv("STUB_LOG", log)
	t.Setenv("MISE_STUB_LOCK", repo(t, "testdata", "mise.lock"))
	t.Setenv("MISE_STUB_FAIL", "")
	root := filepath.Join(dir, "root")
	for _, f := range []string{"mise.toml", "build.sh"} {
		write(t, filepath.Join(root, "tools", f), read(t, repo(t, "tools", f)))
	}
	d := paths.Dirs{Data: filepath.Join(dir, "data"), Config: filepath.Join(dir, "config"), Lima: filepath.Join(dir, "lima")}
	var said, detached []string
	tl := New(d, root)
	tl.Self = "/devvm"
	tl.Arch = "arm64"
	tl.Getenv = func(k string) string {
		if k == "DEVVM_HOST_MISE" {
			return repo(t, "testdata", "stubs", "mise")
		}
		return ""
	}
	tl.ShareWait = time.Millisecond
	tl.Info = func(m msg.M) { said = append(said, "==> "+m.Names()) }
	tl.Warn = func(m msg.M) { said = append(said, "devvm: "+m.Names()) }
	tl.Detach = func(path string, args []string, out *os.File) (int, error) {
		detached = append(detached, path+" "+strings.Join(args, " ")+" > "+out.Name())
		return 1, nil
	}
	return env{Tools: tl, log: log, said: &said, detach: &detached}
}

// calls returns the stub's calls, and forgets them.
func (e env) calls(t *testing.T) string {
	t.Helper()
	c := string(read(t, e.log))
	write(t, e.log, nil)
	return c
}

// locked gives the env a lock already resolved today: the fixture's.
func (e env) locked(t *testing.T, lock []byte) {
	t.Helper()
	write(t, e.Dirs.ToolLock(), lock)
	write(t, e.lockConfig(), read(t, e.configFile()))
	write(t, e.checkedFile(), nil)
}

func names(err error) string {
	if m, ok := errors.AsType[msg.M](err); ok {
		return m.Names()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestMiseArch(t *testing.T) {
	for in, want := range map[string]string{"amd64": "x64", "arm64": "arm64"} {
		if got := MiseArch(in); got != want {
			t.Errorf("MiseArch(%s) = %s, want %s", in, got, want)
		}
	}
}

// The first use resolves the versions, for every VM platform, once.
func TestLockResolvesOnce(t *testing.T) {
	e := newEnv(t)
	lock, err := e.Lock(t.Context())
	if err != nil {
		t.Fatal(names(err))
	}
	if !bytes.Equal(lock, fixture(t)) || !bytes.Equal(read(t, e.Dirs.ToolLock()), fixture(t)) {
		t.Error("the lock isn't what mise wrote")
	}
	if !bytes.Equal(read(t, e.lockConfig()), read(t, e.configFile())) {
		t.Error("the lock's mise.toml isn't tools/mise.toml")
	}
	if _, err := os.Stat(e.checkedFile()); err != nil {
		t.Error("not marked checked")
	}
	if c := e.calls(t); !strings.Contains(c, "mise lock --platform linux-arm64,linux-arm64-musl\n") {
		t.Errorf("calls = %q", c)
	}
	if !slices.Contains(*e.said, "==> resolving_tools") {
		t.Errorf("said %q", *e.said)
	}
	*e.said = nil
	if _, err := e.Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := e.calls(t); c != "" || len(*e.said) > 0 {
		t.Errorf("resolved again: %q, said %q", c, *e.said)
	}
}

// A changed tools/mise.toml (a devvm upgrade) resolves again: every tool,
// or nothing.
func TestLockResolvesAgainWhenConfigChanges(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	config := read(t, e.configFile())
	write(t, e.configFile(), bytes.Replace(config, []byte("[tools]\n"), []byte("[tools]\nzig = \"latest\"\n"), 1))
	if _, err := e.Lock(t.Context()); names(err) != "tools_need_resolve" {
		t.Errorf("Lock with a tool mise didn't lock = %v", names(err))
	}
	if !bytes.Equal(read(t, e.Dirs.ToolLock()), fixture(t)) || !bytes.Equal(read(t, e.lockConfig()), config) {
		t.Error("the lock changed")
	}
	write(t, e.configFile(), append(config, "# a comment\n"...))
	if _, err := e.Lock(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if !bytes.Equal(read(t, e.lockConfig()), read(t, e.configFile())) || !strings.Contains(e.calls(t), "mise lock") {
		t.Error("not resolved again")
	}
}

// Offline with no lock yet, it says what's needed, with the details in
// the run log (tools.txtar shows them).
func TestLockOffline(t *testing.T) {
	e := newEnv(t)
	t.Setenv("MISE_STUB_FAIL", "1")
	_, err := e.Lock(t.Context())
	var logged *backend.LoggedError
	if !errors.As(err, &logged) || logged.Msg.ID() != "tools_need_resolve" {
		t.Errorf("Lock offline = %v", err)
	}
	if _, err := os.Stat(e.Dirs.ToolLock()); !errors.Is(err, os.ErrNotExist) {
		t.Error("a lock was written")
	}
}

// mise reads no config above its folder: MISE_CEILING_PATHS stops it,
// with the physical path, and its state stays in the folder.
func TestMiseCmdCeiling(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "shared", "t"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "shared"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	cmd, err := miseCmd(t.Context(), "/bin/sh", filepath.Join(dir, "link", "t"), "-c",
		`echo "ceiling=$MISE_CEILING_PATHS trusted=$MISE_TRUSTED_CONFIG_PATHS data=$MISE_DATA_DIR pwd=$(pwd -P)"`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	p, err := filepath.EvalSymlinks(filepath.Join(dir, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("ceiling=%[1]s trusted=%[1]s/t data=%[1]s/t/data pwd=%[1]s/t\n", p); string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The daily check: a lock not checked in the last day; never with
// DEVVM_NO_TOOL_CHECK, or without a lock (the first use resolves it).
func TestCheckDue(t *testing.T) {
	e := newEnv(t)
	if e.CheckDue() {
		t.Error("due without a lock")
	}
	e.locked(t, fixture(t))
	if e.CheckDue() {
		t.Error("due when checked now")
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(e.checkedFile(), old, old); err != nil {
		t.Fatal(err)
	}
	if !e.CheckDue() {
		t.Error("not due a day later")
	}
	getenv := e.Getenv
	e.Getenv = func(k string) string {
		if k == "DEVVM_NO_TOOL_CHECK" {
			return "1"
		}
		return getenv(k)
	}
	if e.CheckDue() {
		t.Error("due with DEVVM_NO_TOOL_CHECK")
	}
	e.Getenv = getenv
	if err := os.Remove(e.checkedFile()); err != nil {
		t.Fatal(err)
	}
	if !e.CheckDue() {
		t.Error("not due when never checked")
	}
}

// At most once a day, in the background: devvm _refresh-tools, its output
// in the logs folder.
func TestRefreshDaily(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	if err := e.RefreshDaily(); err != nil || len(*e.detach) > 0 {
		t.Fatalf("checked today, yet %v %q", err, *e.detach)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(e.checkedFile(), old, old); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshDaily(); err != nil {
		t.Fatal(err)
	}
	want := []string{"/devvm _refresh-tools > " + filepath.Join(e.Dirs.Logs(), "tools-check.log")}
	if !slices.Equal(*e.detach, want) {
		t.Errorf("started %q, want %q", *e.detach, want)
	}
	if err := e.RefreshDaily(); err != nil || len(*e.detach) != 1 {
		t.Errorf("started again: %v %q", err, *e.detach)
	}
	if fi, err := os.Stat(e.Dirs.Logs()); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("logs folder: %v %v", fi, err)
	}
}

// newerLock is the fixture with every tool newer, and node built from
// source on musl (its musl builds not out yet, as for days after a node
// release).
func newerLock(t *testing.T) []byte {
	t.Helper()
	out := regexp.MustCompile(`(?m)^version = "(.*)"$`).ReplaceAll(fixture(t), []byte(`version = "$1.1"`))
	return regexp.MustCompile(`(?m)^(\[tools\.node\."platforms\.linux-.*-musl"\])$`).ReplaceAll(out, []byte("$1\ninstall = \"source\""))
}

// olderLock is what mise locks for node's older versions: 24.20.1 also
// built from source on musl, 24.20.0 with every download, pinned.
func olderLock(t *testing.T) []byte {
	t.Helper()
	ver := regexp.MustCompile(`(?m)^version = .*$`)
	spec := regexp.MustCompile(`(?m)^specifiers = .*$`)
	a := ver.ReplaceAll(Lock(newerLock(t)).Entry("node"), []byte(`version = "24.20.1"`))
	b := ver.ReplaceAll(Lock(fixture(t)).Entry("node"), []byte(`version = "24.20.0"`))
	return spec.ReplaceAll(append(a, b...), []byte(`specifiers = ["pinned"]`))
}

func (e env) stubLocks(t *testing.T, newer, older []byte, versions string) {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "newer.lock"), newer)
	write(t, filepath.Join(dir, "older.lock"), older)
	t.Setenv("MISE_STUB_LOCK", filepath.Join(dir, "newer.lock"))
	t.Setenv("MISE_STUB_OLDER_LOCK", filepath.Join(dir, "older.lock"))
	t.Setenv("MISE_STUB_VERSIONS", versions)
}

// A tool whose newest version has no download for some platform steps back
// to the newest older one that has; the others move on.
func TestRefreshStepsBack(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	newer := newerLock(t)
	e.stubLocks(t, newer, olderLock(t), "24.19.0 24.20.0 24.20.1 24.21.0.1 24.22.0")
	if err := e.Refresh(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if !slices.Contains(*e.said, "devvm: tool_stepped_back older=24.20.0 tool=node version=24.21.0.1") {
		t.Errorf("said %q", *e.said)
	}
	lock := Lock(read(t, e.Dirs.ToolLock()))
	if v := lock.Version("node"); v != "24.20.0" {
		t.Errorf("node = %s", v)
	}
	if !bytes.Contains(lock.Entry("node"), []byte("specifiers = [\"24\"]\n")) {
		t.Errorf("node's specifiers aren't mise.toml's anymore:\n%s", lock.Entry("node"))
	}
	if !lock.HasDownload("node", e.platforms()) {
		t.Error("node has no download for every platform")
	}
	if !bytes.Equal(lock.Entry("bun"), Lock(newer).Entry("bun")) {
		t.Error("bun didn't move on")
	}
	if !slices.Equal(lockTools(lock), lockTools(newer)) {
		t.Errorf("tools = %q", lockTools(lock))
	}
	if c := e.calls(t); !strings.Contains(c, "mise ls-remote core:node 24\n") {
		t.Errorf("not from the backend: %q", c)
	}
}

// With no older version that has every download, the newest stays, and
// says so.
func TestRefreshStepsBackToNothing(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	newer := newerLock(t)
	e.stubLocks(t, newer, newer, "")
	if err := e.Refresh(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if !slices.Contains(*e.said, "devvm: tool_no_download tool=node version=24.21.0.1") {
		t.Errorf("said %q", *e.said)
	}
	if !bytes.Equal(read(t, e.Dirs.ToolLock()), newer) {
		t.Error("the lock isn't the newest")
	}
}

// Newer versions replace the lock; a failure leaves it.
func TestRefresh(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	newer := regexp.MustCompile(`(?m)^version = "(.*)"$`).ReplaceAll(fixture(t), []byte(`version = "$1.1"`))
	e.stubLocks(t, newer, newer, "")
	t.Setenv("MISE_STUB_FAIL", "1")
	if err := e.Refresh(t.Context()); err == nil {
		t.Fatal("Refresh offline succeeded")
	}
	if !bytes.Equal(read(t, e.Dirs.ToolLock()), fixture(t)) {
		t.Error("a failure changed the lock")
	}
	if _, err := os.Stat(e.Dirs.ToolLock() + ".next"); !errors.Is(err, os.ErrNotExist) {
		t.Error("a failure left the next lock")
	}
	t.Setenv("MISE_STUB_FAIL", "")
	if err := e.Refresh(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if !bytes.Equal(read(t, e.Dirs.ToolLock()), newer) {
		t.Error("the lock isn't the newer one")
	}
	if _, err := os.Stat(e.shareLock()); !errors.Is(err, os.ErrNotExist) {
		t.Error("the share's lock is still held")
	}
	// Check says a failure, and carries on.
	t.Setenv("MISE_STUB_FAIL", "1")
	*e.said = nil
	e.Check(t.Context())
	if want := []string{"==> checking_tools", "devvm: tools_check_failed"}; !slices.Equal(*e.said, want) {
		t.Errorf("Check offline said %q, want %q", *e.said, want)
	}
}

// A download for every platform, not a build from source; rust needs none.
func TestHasDownload(t *testing.T) {
	e := newEnv(t)
	lock := Lock(fixture(t))
	if !lock.HasDownload("node", e.platforms()) || !lock.HasDownload("rust", e.platforms()) {
		t.Error("the fixture's node or rust has no download")
	}
	newer := Lock(newerLock(t))
	if newer.HasDownload("node", e.platforms()) {
		t.Error("a source build on musl counts")
	}
	if !newer.HasDownload("node", []string{"linux-arm64"}) {
		t.Error("glibc's download doesn't count")
	}
	// opencode without its platforms' tables.
	var bare bytes.Buffer
	skip := false
	for _, line := range lines(fixture(t)) {
		if strings.HasPrefix(line, "[") {
			skip = strings.HasPrefix(line, "[tools.opencode.")
		}
		if !skip {
			bare.WriteString(line + "\n")
		}
	}
	if Lock(bare.Bytes()).HasDownload("opencode", e.platforms()) {
		t.Error("a tag with no release (no platforms) counts")
	}
	// Only the VM's own platform counts.
	if e.noDownload(newer, "node", "gnu") || !e.noDownload(newer, "node", "musl") || e.noDownload(newer, "bun", "musl") {
		t.Error("noDownload looks at other platforms")
	}
}

func TestVersionEntry(t *testing.T) {
	lock := Lock(olderLock(t))
	if e := lock.VersionEntry("node", "24.20.0"); !bytes.HasPrefix(e, []byte("[[tools.node]]\nversion = \"24.20.0\"\n")) || bytes.Count(e, []byte("[[")) != 1 {
		t.Errorf("entry = %s", e)
	}
	if e := lock.VersionEntry("node", "1.0.0"); e != nil {
		t.Errorf("entry for a version it hasn't = %s", e)
	}
}

// Tool files: the lock's version, the groups' tools, and a locked version
// that isn't shaped like one refused; musl bases get their own images.
func TestFiles(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	files, err := e.Files(t.Context(), []string{"node", "claude"}, "gnu")
	if err != nil {
		t.Fatal(err)
	}
	var tools []string
	for _, f := range files {
		tool, v, ok := image.ParseToolFile(f)
		if !ok || v != Lock(fixture(t)).Version(tool) {
			t.Errorf("%s isn't at the lock's version", f)
		}
		tools = append(tools, tool)
	}
	if !slices.Equal(tools, []string{"node", "pnpm", "bun", "claude"}) {
		t.Errorf("tools = %q", tools)
	}
	musl, err := e.Files(t.Context(), []string{"node"}, "musl")
	if err != nil || musl[0] == files[0] {
		t.Errorf("musl's node = %q, %v", musl, err)
	}
	bad := regexp.MustCompile(`(?m)(\[\[tools\.go\]\]\n)version = ".*"`).ReplaceAll(fixture(t), []byte(`${1}version = "1$$(id)"`))
	e.locked(t, bad)
	if _, err := e.Files(t.Context(), []string{"go"}, "gnu"); names(err) != `tool_bad_version tool=go version="1$(id)"` {
		t.Errorf("a bad version: %v", names(err))
	}
	if _, err := e.File(Lock(fixture(t)), "zig", "gnu"); names(err) != "tool_not_locked lock="+e.Dirs.ToolLock()+" tool=zig" {
		t.Errorf("a tool the lock hasn't: %v", names(err))
	}
}

func fake() *backendtest.Fake {
	caps := backend.Caps{}
	for _, f := range backend.Features {
		caps[f] = backend.Native
	}
	return backendtest.New("fake", caps)
}

const baseKey = "0123456789ab"

func (e env) files(t *testing.T, tools ...string) []string {
	t.Helper()
	var out []string
	for _, tool := range tools {
		f, err := e.File(Lock(read(t, e.Dirs.ToolLock())), tool, "gnu")
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, f)
	}
	return out
}

func (e env) checkHashes(t *testing.T, files []string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(e.Dirs.Tools(), f)
		sum, err := image.SHA256File(p)
		if err != nil {
			t.Fatal(err)
		}
		fi, err := os.Lstat(p + ".sha256")
		if err != nil || !fi.Mode().IsRegular() || string(read(t, p+".sha256")) != sum+"\n" {
			t.Errorf("%s's hash isn't recorded", f)
		}
	}
}

func (e env) noLeftovers(t *testing.T) {
	t.Helper()
	if left, _ := filepath.Glob(filepath.Join(e.Dirs.Data, "tools-build.*")); len(left) > 0 {
		t.Errorf("left %q", left)
	}
	if _, err := os.Stat(e.shareLock()); !errors.Is(err, os.ErrNotExist) {
		t.Error("the share's lock is still held")
	}
}

// The missing tool images build in one throwaway VM, with a folder of its
// own (never the share); each moves into the share with its hash.
func TestBuild(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	f := fake()
	all := e.files(t, "node", "pnpm", "bun", "claude")
	write(t, filepath.Join(e.Dirs.Tools(), all[1]), []byte("built"))
	write(t, filepath.Join(e.Dirs.Tools(), all[1]+".sha256"), []byte("x\n"))
	if err := e.Build(t.Context(), f, baseKey, "gnu", all); err != nil {
		t.Fatal(names(err))
	}
	if ms := f.Methods(); !slices.Equal(ms, []string{"BuildTools"}) {
		t.Fatalf("calls = %q", ms)
	}
	spec, _ := f.Calls()[0].Args[0].(backend.ToolsSpec)
	lock := Lock(fixture(t))
	wantArgs := []string{
		"node", lock.Version("node"), all[0], "bun", lock.Version("bun"), all[2], "claude", lock.Version("claude"), all[3],
	}
	if spec.Base != baseKey || spec.OutAt != ShareAt || spec.Net != backend.NetInternet || !slices.Equal(spec.Args, wantArgs) ||
		spec.Script.Path != filepath.Join(e.Root, "tools", "build.sh") {
		t.Errorf("spec = %+v", spec)
	}
	if filepath.Dir(spec.Out) != e.Dirs.Data || !strings.HasPrefix(filepath.Base(spec.Out), "tools-build.") {
		t.Errorf("the build's folder is %s", spec.Out)
	}
	want := "==> building_tools tools=" + label([]string{all[0], all[2], all[3]})
	if !slices.Contains(*e.said, want) {
		t.Errorf("said %q, want %q", *e.said, want)
	}
	e.checkHashes(t, []string{all[0], all[2], all[3]})
	e.noLeftovers(t)
	// Nothing to do when the share has them all.
	if err := e.Build(t.Context(), f, baseKey, "gnu", all); err != nil || len(f.Calls()) != 1 {
		t.Errorf("built again: %v %q", err, f.Methods())
	}
}

// The build sees the lock and mise.toml it was given, in its folder.
func TestBuildFolder(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	f := fake()
	f.ToolsBuild = func(spec backend.ToolsSpec) error {
		if !bytes.Equal(read(t, filepath.Join(spec.Out, ".build", "mise.lock")), fixture(t)) ||
			!bytes.Equal(read(t, filepath.Join(spec.Out, ".build", "mise.toml")), read(t, e.configFile())) {
			t.Error("the build's .build folder isn't the lock and mise.toml")
		}
		if _, err := os.Stat(e.shareLock()); err != nil {
			t.Error("the share isn't locked during the build")
		}
		return nil
	}
	if err := e.Build(t.Context(), f, baseKey, "gnu", e.files(t, "uv")); names(err) != "tool_bad_output file="+e.files(t, "uv")[0] {
		t.Errorf("Build with no image written = %v", names(err))
	}
	e.noLeftovers(t)
}

// The tools build together, in one VM and folder.
func TestBuildOneBatch(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	f := fake()
	files := e.files(t, "node", "go", "bun")
	if err := e.Build(t.Context(), f, baseKey, "gnu", files); err != nil {
		t.Fatal(names(err))
	}
	var batches [][]string
	outs := map[string]bool{}
	for _, c := range f.Calls() {
		spec, _ := c.Args[0].(backend.ToolsSpec)
		batches = append(batches, spec.Args)
		outs[spec.Out] = true
	}
	lock := Lock(fixture(t))
	want := [][]string{
		{"node", lock.Version("node"), files[0], "go", lock.Version("go"), files[1], "bun", lock.Version("bun"), files[2]},
	}
	if !slices.EqualFunc(batches, want, slices.Equal) || len(outs) != 1 {
		t.Errorf("batches = %q in %d folders", batches, len(outs))
	}
	e.checkHashes(t, files)
	e.noLeftovers(t)
}

// A build that leaves a symlink instead of an image is refused; one that
// plants FILE.sha256 as a symlink can't make devvm write through it.
func TestBuildHostileOutput(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	victim := filepath.Join(t.TempDir(), "victim")
	write(t, victim, []byte("victim\n"))
	f := fake()
	f.ToolsBuild = func(spec backend.ToolsSpec) error { return os.Symlink(victim, filepath.Join(spec.Out, spec.Args[2])) }
	node := e.files(t, "node")
	if err := e.Build(t.Context(), f, baseKey, "gnu", node); names(err) != "tool_bad_output file="+node[0] {
		t.Errorf("Build = %v", names(err))
	}
	if _, err := os.Lstat(filepath.Join(e.Dirs.Tools(), node[0])); !errors.Is(err, os.ErrNotExist) {
		t.Error("the symlink reached the share")
	}
	e.noLeftovers(t)

	f.ToolsBuild = func(spec backend.ToolsSpec) error {
		file := spec.Args[2]
		write(t, filepath.Join(spec.Out, file), []byte("image"))
		// The share is the VM's to write in this scenario, through
		// whatever it planted.
		return os.Symlink(victim, filepath.Join(e.Dirs.Tools(), file+".sha256"))
	}
	if err := e.Build(t.Context(), f, baseKey, "gnu", node); err != nil {
		t.Fatal(names(err))
	}
	if string(read(t, victim)) != "victim\n" {
		t.Error("devvm wrote through the planted link")
	}
	e.checkHashes(t, node)
}

// A tool the lock would build from source for the VM's platform says so,
// and builds nothing; so does a lock that moved on while devvm waited.
func TestBuildRefuses(t *testing.T) {
	e := newEnv(t)
	lock := regexp.MustCompile(`(?m)^(\[tools\.node\."platforms\.linux-.*-musl"\])$`).ReplaceAll(fixture(t), []byte("$1\ninstall = \"source\""))
	e.locked(t, lock)
	f := fake()
	musl, err := e.File(Lock(lock), "node", "musl")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Build(t.Context(), f, baseKey, "musl", []string{musl}); names(err) != "tool_unbuilt tool=node version="+Lock(lock).Version("node") {
		t.Errorf("Build = %v", names(err))
	}
	if err := e.Build(t.Context(), f, baseKey, "gnu", []string{"node-1.0.0-0123456789ab.erofs"}); names(err) != "tools_changed" {
		t.Errorf("Build of a version the lock moved on from = %v", names(err))
	}
	if len(f.Calls()) > 0 {
		t.Errorf("calls = %q", f.Methods())
	}
	e.noLeftovers(t)
}

// Another devvm holding the share is waited for, then given up on.
func TestLockShare(t *testing.T) {
	e := newEnv(t)
	unlock, err := e.LockShare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.LockShare(t.Context())
	if names(err) != "share_lock_held dir="+e.Dirs.Tools() {
		t.Errorf("LockShare while held = %v", names(err))
	}
	if !slices.Contains(*e.said, "==> share_waiting dir="+e.Dirs.Tools()) {
		t.Errorf("said %q", *e.said)
	}
	unlock()
	unlock, err = e.LockShare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	write(t, filepath.Join(e.shareLock(), "x"), nil)
	if _, err := e.LockShare(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("LockShare canceled = %v", err)
	}
}

// gc's list: the tool images List calls unused, neither current nor
// listed by a VM; none without a lock. Delete takes them and their
// hashes, and nothing but tool images.
func TestUnusedAndDelete(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	cur := e.files(t, "node")[0]
	old, used := "node-24.1.0-0123456789ab.erofs", "node-24.2.0-0123456789ab.erofs"
	for _, f := range []string{cur, old, old + ".sha256", used, "x.erofs.tmp", ".lock"} {
		write(t, filepath.Join(e.Dirs.Tools(), f), []byte("x"))
	}
	if err := os.Remove(e.Dirs.ToolLock()); err != nil {
		t.Fatal(err)
	}
	unused := func() []string {
		ls, err := e.List("gnu", map[string]int{used: 1})
		if err != nil {
			t.Fatal(err)
		}
		var files []string
		for _, l := range Unused(ls) {
			files = append(files, l.File)
		}
		return files
	}
	if got := unused(); len(got) != 0 {
		t.Errorf("Unused without a lock = %q", got)
	}
	e.locked(t, fixture(t))
	got := unused()
	if want := []string{old}; !slices.Equal(got, want) {
		t.Fatalf("Unused = %q, want %q", got, want)
	}
	if err := e.Delete(got); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{old, old + ".sha256"} {
		if _, err := os.Stat(filepath.Join(e.Dirs.Tools(), f)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: %v", f, err)
		}
	}
	for _, f := range []string{cur, used, "x.erofs.tmp", ".lock"} {
		if _, err := os.Stat(filepath.Join(e.Dirs.Tools(), f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	for _, bad := range []string{".lock", "../" + cur, "x.erofs.tmp"} {
		if err := e.Delete([]string{bad}); err == nil {
			t.Errorf("Delete(%q) took it", bad)
		}
	}
}

// List: by name, newest first, with how many VMs use each image and
// whether it's current; the lock's version when the share lacks it; a
// name with nothing in the share as not installed. It never resolves.
func TestList(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	goCur, claude := e.files(t, "go")[0], e.files(t, "claude")[0]
	old, used := "go-1.9.0-0123456789ab.erofs", "go-1.20.0-0123456789ab.erofs"
	for _, f := range []string{old, used, goCur, claude, "x.erofs.tmp"} {
		write(t, filepath.Join(e.Dirs.Tools(), f), []byte("x"))
	}
	ls, err := e.List("gnu", map[string]int{used: 2, claude: 1})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range ls {
		got = append(got, fmt.Sprintf("%s %s %s %v %d %s", l.Name, l.Tool, l.Version, l.Built, l.VMs, l.State))
		if l.Built && l.Size <= 0 {
			t.Errorf("%s %s: size %d", l.Tool, l.Version, l.Size)
		}
	}
	want := []string{
		"node   false 0 not installed",
		"python   false 0 not installed",
		"rust   false 0 not installed",
		"go go 1.27.2 true 0 current",
		"go go 1.20.0 true 2 in use",
		"go go 1.9.0 true 0 unused",
		"claude claude 2.1.295 true 1 current",
		"codex   false 0 not installed",
		"opencode   false 0 not installed",
	}
	if !slices.Equal(got, want) {
		t.Errorf("List:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// The lock's newer go isn't built; without a lock, nothing is current
	// or unused.
	if err := os.Remove(filepath.Join(e.Dirs.Tools(), goCur)); err != nil {
		t.Fatal(err)
	}
	ls, _ = e.List("gnu", nil)
	if len(ls) < 4 || ls[3].State != StateNotBuilt || ls[3].Version != "1.27.2" || ls[3].Built || ls[4].State != StateUnused {
		t.Errorf("List without the lock's go: %+v", ls)
	}
	if err := os.Remove(e.Dirs.ToolLock()); err != nil {
		t.Fatal(err)
	}
	ls, _ = e.List("gnu", map[string]int{used: 1})
	if len(ls) < 5 || ls[3].State != StateInUse || ls[4].State != "" {
		t.Errorf("List without a lock: %+v", ls)
	}
	if c := e.calls(t); c != "" {
		t.Errorf("List ran %s", c)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.10.0", "1.9.2", 1},
		{"24.21.0", "24.21.0", 0},
		{"1.2", "1.2.1", -1},
		{"0.162.0", "0.99.9", 1},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// devvm remove: the tools leave the VM's name and list, the rest stays;
// what it lacks, and custom layers, are refused.
func TestRemove(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	t.Setenv("DEVVM_MESSAGES", "names")
	vm := "devvm-p-abc123abc123ab"
	all := e.files(t, "node", "pnpm", "bun", "python", "uv", "claude")
	img, files, err := Remove(vm, "node+python+claude", all, "python+claude")
	if err != nil || img != "node" || !slices.Equal(files, all[:3]) {
		t.Errorf("Remove = %q, %q, %v", img, files, err)
	}
	img, files, err = Remove(vm, img, files, "node")
	if err != nil || img != "base" || len(files) != 0 {
		t.Errorf("Remove the last = %q, %q, %v", img, files, err)
	}
	if _, _, err := Remove(vm, "node+foo", nil, "python"); names(err) != "vm_lacks has=node foo layer=python vm="+vm {
		t.Errorf("Remove what it lacks = %v", names(err))
	}
	if _, _, err := Remove(vm, "node+foo", nil, "foo"); names(err) != "custom_layer_remove layer=foo vm="+vm {
		t.Errorf("Remove a layer = %v", names(err))
	}
}

// Newer shared tools than a VM's are named.
func TestWarnNewer(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	have := append(e.files(t, "node", "pnpm", "bun"), "claude-2.0.0-0123456789ab.erofs")
	if err := e.WarnNewer(t.Context(), "node+claude", have, "gnu"); err != nil {
		t.Fatal(err)
	}
	want := []string{"devvm: newer_tools tools=claude " + Lock(fixture(t)).Version("claude")}
	if !slices.Equal(*e.said, want) {
		t.Errorf("said %q, want %q", *e.said, want)
	}
	*e.said = nil
	if err := e.WarnNewer(t.Context(), "node+claude", e.files(t, "node", "pnpm", "bun", "claude"), "gnu"); err != nil || len(*e.said) > 0 {
		t.Errorf("said %q, %v", *e.said, err)
	}
}

// devvm update: resolves the newest versions, builds them, and says which
// the VM gets; offline, it says so and keeps the versions it has.
func TestUpdate(t *testing.T) {
	e := newEnv(t)
	e.locked(t, fixture(t))
	newer := regexp.MustCompile(`(?m)(\[\[tools\.go\]\]\n)version = ".*"`).ReplaceAll(fixture(t), []byte(`${1}version = "9.9.9"`))
	e.stubLocks(t, newer, newer, "")
	f := fake()
	build := func(ctx context.Context, files []string) error { return e.Build(ctx, f, baseKey, "gnu", files) }
	vm := "devvm-p-abc123abc123ab"
	before := e.files(t, "go")
	files, changed, err := e.Update(t.Context(), build, vm, "go", before, "gnu")
	if err != nil {
		t.Fatal(names(err))
	}
	if Lock(read(t, e.Dirs.ToolLock())).Version("go") != "9.9.9" || !changed || slices.Equal(files, before) {
		t.Errorf("Update = %q, %v", files, changed)
	}
	if !slices.Equal(*e.said, []string{"==> checking_tools", "==> building_tools tools=go 9.9.9", "==> tools_switch vm=" + vm}) {
		t.Errorf("said %q", *e.said)
	}
	*e.said = nil
	if again, changed, err := e.Update(t.Context(), build, vm, "go", files, "gnu"); err != nil || changed || !slices.Equal(again, files) {
		t.Errorf("Update again = %q, %v, %v", again, changed, err)
	}
	if !slices.Contains(*e.said, "==> tools_newest vm="+vm) {
		t.Errorf("said %q", *e.said)
	}
	t.Setenv("MISE_STUB_FAIL", "1")
	*e.said = nil
	if _, changed, err := e.Update(t.Context(), build, vm, "go", before, "gnu"); err != nil || !changed {
		t.Errorf("Update offline = %v, %v", changed, err)
	}
	if !slices.Contains(*e.said, "devvm: tools_check_failed") {
		t.Errorf("said %q", *e.said)
	}
}

// The host's mise is downloaded once and checked against its pin.
func TestHostMise(t *testing.T) {
	body := []byte("#!/bin/sh\necho mise\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v"+miseVersion+"/mise-v"+miseVersion+"-linux-x64" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	e := newEnv(t)
	e.Getenv = func(string) string { return "" }
	e.OS, e.Arch = "linux", "amd64"
	e.miseURL = srv.URL + "/v%[1]s/mise-v%[1]s-%[2]s"
	e.miseSums = map[string]string{"linux-x64": strings.Repeat("0", 64)}
	if _, err := e.HostMise(t.Context()); names(err) != "mise_checksum version="+miseVersion {
		t.Errorf("a download that doesn't match = %v", names(err))
	}
	path := filepath.Join(e.Dirs.Mise(), miseVersion, "mise")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("kept a download that doesn't match")
	}
	sum, err := image.SHA256File(writeTemp(t, body))
	if err != nil {
		t.Fatal(err)
	}
	e.miseSums["linux-x64"] = sum
	got, err := e.HostMise(t.Context())
	if err != nil || got != path {
		t.Fatalf("HostMise = %q, %v", got, err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("mise: %v %v", fi, err)
	}
	srv.Close()
	if again, err := e.HostMise(t.Context()); err != nil || again != path {
		t.Errorf("downloaded again: %q, %v", again, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := e.HostMise(t.Context()); names(err) != "mise_download_failed version="+miseVersion {
		t.Errorf("offline = %v", names(err))
	}
}

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	write(t, p, data)
	return p
}

// The real pins: one per host devvm runs on, 64 hex digits each.
func TestMisePins(t *testing.T) {
	for _, p := range []string{"macos-arm64", "macos-x64", "linux-arm64", "linux-x64"} {
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(miseSHA256[p]) {
			t.Errorf("%s's checksum = %q", p, miseSHA256[p])
		}
	}
}

// What a VM reads to mount its tools.
func TestMounts(t *testing.T) {
	got := string(Mounts([]string{"node-24.21.0-0123456789ab.erofs", "bad", "go-1.27.2-0123456789ab.erofs"}))
	if want := "node-24.21.0-0123456789ab.erofs node 24.21.0\ngo-1.27.2-0123456789ab.erofs go 1.27.2\n"; got != want {
		t.Errorf("Mounts = %q, want %q", got, want)
	}
}

// tools/mise.toml's tools, and the fixture lock has each.
func TestConfigTools(t *testing.T) {
	got, _ := configTools(read(t, repo(t, "tools", "mise.toml")))
	if want := []string{"node", "pnpm", "bun", "python", "uv", "go", "rust", "claude", "codex", "opencode"}; !slices.Equal(got, want) {
		t.Errorf("configTools = %q, want %q", got, want)
	}
	if locked := lockTools(fixture(t)); !slices.Equal(slices.Sorted(slices.Values(got)), locked) {
		t.Errorf("the fixture lock has %q", locked)
	}
}
