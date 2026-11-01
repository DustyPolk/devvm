package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// TestScript runs testdata/script/*.txtar against a built devvm, in a sandbox.
func TestScript(t *testing.T) {
	bin := buildDevvm(t)
	release := linuxRelease(t)
	stubs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "stubs"))
	if err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testscript.Params{
		Dir:                 filepath.Join("..", "..", "testdata", "script"),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		Setup: func(env *testscript.Env) error {
			return sandbox(env, bin, stubs)
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"free_port":     freePort,
			"linux_release": release.serve,
			"listen_any":    listenAny,
			"stub_calls":    stubCalls,
			"wait_file":     waitFile,
		},
	})
}

// linuxBuild is a release's Linux archive, for the Linux build devvm
// fetches: the test build of devvm, built for Linux on this machine's
// processor as the test build itself was, and the devvm in it's SHA-256.
type linuxBuild struct {
	archive []byte
	sum     string
}

// linuxRelease builds it.
func linuxRelease(t *testing.T) linuxBuild {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "devvm")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", out, //nolint:gosec // the test's own folder
		"-ldflags", "-X github.com/w3cj/devvm/internal/backend/registry.unreviewed=allowed", "github.com/w3cj/devvm/cmd/devvm")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+runtime.GOARCH)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building devvm for Linux: %v\n%s", err, b)
	}
	data, err := os.ReadFile(out) //nolint:gosec // the test's own build
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "devvm-0.0.0-linux-" + runtime.GOARCH + "/devvm", Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return linuxBuild{archive: b.Bytes(), sum: hex.EncodeToString(sum[:])}
}

// serve serves the archive on a loopback port until the script ends, and
// points the test build at it as a release's Linux build: its address in
// DEVVM_LINUX_BUILD_URL, and in DEVVM_LINUX_BUILD_SHA256 the devvm in it's
// SHA-256 (match), or another's (mismatch).
func (l linuxBuild) serve(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 || args[0] != "match" && args[0] != "mismatch" {
		ts.Fatalf("usage: linux_release match|mismatch")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(l.archive) }))
	ts.Defer(srv.Close)
	sum := l.sum
	if args[0] == "mismatch" {
		sum = strings.Repeat("0", 64)
	}
	ts.Setenv("DEVVM_LINUX_BUILD_URL", srv.URL+"/devvm-0.0.0-linux-"+runtime.GOARCH+".tar.gz")
	ts.Setenv("DEVVM_LINUX_BUILD_SHA256", sum)
	ts.Setenv("LINUX_ARCH", runtime.GOARCH)
}

// buildDevvm builds cmd/devvm, its files embedded, into a temporary folder
// and returns that folder. It's a test build: DEVVM_UNREVIEWED counts.
func buildDevvm(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "go", "install",
		"-ldflags", "-X github.com/w3cj/devvm/internal/backend/registry.unreviewed=allowed", "github.com/w3cj/devvm/cmd/devvm")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOBIN="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building devvm: %v\n%s", err, out)
	}
	return dir
}

// sandbox points every folder devvm uses into a new temporary folder, and puts
// devvm and the stubs first on PATH.
func sandbox(env *testscript.Env, bin, stubs string) error {
	// /tmp keeps Lima's socket paths short.
	root, err := os.MkdirTemp("/tmp", "dvt.")
	if err != nil {
		return err
	}
	env.Defer(func() {
		// VMs' proxies follow a stub hostagent that's gone before they
		// look: stop them with the sandbox.
		_ = exec.CommandContext(context.Background(), "pkill", "-f", "_proxy -listen .* -lifeline "+root+"/").Run() //nolint:gosec,forbidigo // the test's own folder; a cleanup, after the scripts' contexts end
		// And Lima's and smolvm's VMs' port forwarders.
		_ = exec.CommandContext(context.Background(), "pkill", "-f", "_lima-ports -vm .* -lifeline "+root+"/").Run()   //nolint:gosec,forbidigo // the test's own folder; a cleanup, after the scripts' contexts end
		_ = exec.CommandContext(context.Background(), "pkill", "-f", "_smolvm-ports -vm .* -lifeline "+root+"/").Run() //nolint:gosec,forbidigo // the test's own folder; a cleanup, after the scripts' contexts end
		// And Docker VMs'.
		_ = exec.CommandContext(context.Background(), "pkill", "-f", "_docker-ports -vm .* -key "+root+"/").Run() //nolint:gosec,forbidigo // the test's own folder; a cleanup, after the scripts' contexts end
		_ = os.RemoveAll(root)
	})

	dirs := map[string]string{
		"HOME":            "home",
		"XDG_DATA_HOME":   "data",
		"XDG_CONFIG_HOME": "config",
		"DEVVM_LIMA_HOME": "lima",
		"LIMA_HOME":       "lima",
		"STUB_DIR":        "stub",
	}
	for name, dir := range dirs {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o750); err != nil {
			return err
		}
		env.Setenv(name, path)
	}
	log := filepath.Join(root, "stub", "calls")
	if err := os.WriteFile(log, nil, 0o600); err != nil {
		return err
	}
	env.Setenv("STUB_LOG", log)
	// Base's features are chosen (none), so nothing asks.
	if err := os.MkdirAll(filepath.Join(root, "config", "devvm"), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "config", "devvm", "base-features"), nil, 0o600); err != nil {
		return err
	}
	// Ubuntu's cloud images: the pinned dated release in a folder, with the
	// pinned checksums, never the network.
	rel := filepath.Join(root, "ubuntu", ubuntu.CloudRelease)
	if err := os.MkdirAll(rel, 0o750); err != nil {
		return err
	}
	sums := ubuntu.CloudSHA256Arm64 + " *ubuntu-" + ubuntu.Version + "-server-cloudimg-arm64.img\n" +
		ubuntu.CloudSHA256Amd64 + " *ubuntu-" + ubuntu.Version + "-server-cloudimg-amd64.img\n"
	if err := os.WriteFile(filepath.Join(rel, "SHA256SUMS"), []byte(sums), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(rel, "SHA256SUMS.gpg"), []byte("signature\n"), 0o600); err != nil {
		return err
	}
	env.Setenv("DEVVM_UBUNTU_URL", "file://"+filepath.Join(root, "ubuntu"))
	// Linux's /dev/kvm: a file anyone can open, so the host's doesn't
	// matter.
	kvm := filepath.Join(root, "stub", "kvm")
	if err := os.WriteFile(kvm, nil, 0o600); err != nil {
		return err
	}
	env.Setenv("DEVVM_KVM", kvm)
	env.Setenv("DEVVM_SECRET_SERVICE", "devvm-test")
	// A large host, whatever runs the tests: a script that tests a small
	// one sets its own.
	for name, v := range fakeHost() {
		env.Setenv(name, v)
	}
	// The stub smolvm's machines run on this process, which outlives each
	// command, as smolvm's own does.
	env.Setenv("SMOLVM_STUB_PID", strconv.Itoa(os.Getpid()))
	// The shared tools' versions: the stub mise, which resolves the
	// fixture lock.
	env.Setenv("DEVVM_HOST_MISE", filepath.Join(stubs, "mise"))
	env.Setenv("MISE_STUB_LOCK", filepath.Join(filepath.Dir(stubs), "mise.lock"))
	env.Setenv("DEVVM_BIN", bin) // for scripts that set their own PATH
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return err
	}
	env.Setenv("DEVVM_REPO", repo) // for the repo's own scripts (scripts/, assets/)
	env.Setenv("PATH", strings.Join([]string{bin, stubs, env.Getenv("PATH")}, string(os.PathListSeparator)))
	return nil
}

// fakeHost is the host the test build reads in every script
// (platform.Host's overrides): twice the CPUs and memory base asks for,
// and more free space than any build checks for.
func fakeHost() map[string]string {
	return map[string]string{
		platform.TestCPUs:      "8",
		platform.TestMemoryGiB: "16",
		platform.TestFreeGiB:   "500",
		// Backends' listings wait long, as the stubs are slow under load:
		// a script that tests the timeout sets its own.
		platform.TestListWait: "10m",
	}
}

// waitFileTimeout is how long wait_file waits. A background devvm takes
// a dozen stub calls to get to the one that writes the file: about a
// second alone, but over 10 under the full suite's load, when each stub
// call (bash and its own commands) can take a second.
const waitFileTimeout = 60 * time.Second

// waitFile waits for a file to exist, for a background command to get
// somewhere. It fails at once when every background command has ended
// (nothing is left to write it), else after waitFileTimeout, for one that
// hangs on the way.
func waitFile(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: wait_file FILE")
	}
	deadline := time.Now().Add(waitFileTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(ts.MkAbs(args[0])); err == nil {
			return
		}
		if bg := ts.BackgroundCmds(); len(bg) > 0 && !slices.ContainsFunc(bg, running) {
			ts.Fatalf("%s didn't appear: the background command ended", args[0])
		}
		time.Sleep(50 * time.Millisecond)
	}
	ts.Fatalf("%s didn't appear in %s", args[0], waitFileTimeout)
}

// running reports whether a background command's process hasn't been
// waited for yet. Signal 0 asks without reading the state testscript's
// own goroutine writes when it waits.
func running(cmd *exec.Cmd) bool {
	return cmd.Process != nil && !errors.Is(cmd.Process.Signal(syscall.Signal(0)), os.ErrProcessDone)
}

// listenAny listens on a free port on every address, as a program on the
// host does, until the script ends, and sets VAR to the port.
func listenAny(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: listen_any VAR")
	}
	l, err := net.Listen("tcp", ":0") //nolint:noctx,gosec // every address, as the program it stands for; the script's whole life
	ts.Check(err)
	ts.Defer(func() { _ = l.Close() })
	ts.Setenv(args[0], strconv.Itoa(l.Addr().(*net.TCPAddr).Port))
}

// freePort sets each VAR to a different port nothing on the host listens
// on now, for a script that lists ports a start or a change finds free: a
// fixed one (5173) may be a program's here.
func freePort(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) == 0 {
		ts.Fatalf("usage: free_port VAR...")
	}
	var held []net.Listener
	defer func() {
		for _, l := range held {
			_ = l.Close()
		}
	}()
	for _, v := range args {
		l, err := net.Listen("tcp", ":0") //nolint:noctx,gosec // every address, so the port is free on each; closed at once
		ts.Check(err)
		held = append(held, l)
		ts.Setenv(v, strconv.Itoa(l.Addr().(*net.TCPAddr).Port))
	}
}

// stubCalls writes the calls the stubs logged to stdout, one per line.
// Negated, it fails if any were logged.
func stubCalls(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) > 0 {
		ts.Fatalf("usage: stub_calls")
	}
	calls := ts.ReadFile(ts.Getenv("STUB_LOG"))
	if neg {
		if calls != "" {
			ts.Fatalf("unexpected stub calls:\n%s", calls)
		}
		return
	}
	_, err := ts.Stdout().Write([]byte(calls))
	ts.Check(err)
}
