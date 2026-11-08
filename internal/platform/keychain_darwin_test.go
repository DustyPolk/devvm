package platform

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/devvmhq/devvm/internal/stubtest"
)

// golden reads a golden fixture of macOS's, as mise run e2e -- --golden
// captured it from the real command.
func golden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "darwin", name)) //nolint:gosec // the repo's own fixture
	if err != nil {
		t.Fatalf("%v: run mise run e2e -- --golden on macOS", err)
	}
	return string(b)
}

// Real security's answer for an item that isn't there, and real tmutil
// isexcluded's, as the golden fixtures have them: the stubs answer
// alike, with the same exit status and the same form.
func TestGoldenSecurityTmutil(t *testing.T) {
	m := regexp.MustCompile(`(?m)^exit (\d+)$`).FindStringSubmatch(golden(t, "security-missing.txt"))
	if m == nil {
		t.Fatal("security-missing.txt has no exit status")
	}
	want, _ := strconv.Atoi(m[1])
	if want != 44 {
		t.Errorf("security exits %d for a missing item, which KeychainGet reads as an error, not ErrNoSecret", want)
	}
	s := stubtest.New(t)
	err := exec.CommandContext(t.Context(), "security", "find-generic-password", "-s", "devvm-test", "-a", "none", "-w").Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != want {
		t.Errorf("the stub's security: %v, want exit %d", err, want)
	}
	form := regexp.MustCompile(`\A\[(Included|Excluded)\] +/\S+\n\z`)
	if tm := golden(t, "tmutil-isexcluded.txt"); !form.MatchString(tm) {
		t.Fatalf("tmutil isexcluded printed %q", tm)
	}
	for _, args := range [][]string{{"isexcluded", "/d"}, {"addexclusion", "/d"}, {"isexcluded", "/d"}} {
		out, err := exec.CommandContext(t.Context(), "tmutil", args...).Output() //nolint:gosec // the tmutil stub
		if err != nil || args[0] == "isexcluded" && !form.Match(out) {
			t.Errorf("the stub's tmutil %q: %q, %v", args, out, err)
		}
	}
	if calls := s.Calls(t); len(calls) != 2 {
		t.Errorf("calls %q", calls)
	}
}

// The stubs come first on PATH in these tests, never the real security
// or tmutil.

func TestKeychainGet(t *testing.T) {
	s := stubtest.New(t)
	if _, err := KeychainGet(t.Context(), "devvm-test", "github"); !errors.Is(err, ErrNoSecret) {
		t.Errorf("no item: %v", err)
	}
	s.SetKeychain(t, "github", "ghp_x")
	if v, err := KeychainGet(t.Context(), "devvm-test", "github"); v != "ghp_x" || err != nil {
		t.Errorf("KeychainGet = %q, %v", v, err)
	}
	if calls := s.Read(t, "calls"); !strings.HasSuffix(calls, "security find-generic-password -s devvm-test -a github -w\n") {
		t.Errorf("calls = %q", calls)
	}
}

// A value is saved through security -i's stdin, never in its arguments;
// one that would end that command line is refused. Deleting a missing
// item is ErrNoSecret.
func TestKeychainSetDelete(t *testing.T) {
	s := stubtest.New(t)
	ctx := t.Context()
	if err := KeychainSet(ctx, "devvm-test", "github", "devvm github", "ghp_testfake"); err != nil {
		t.Fatal(err)
	}
	if v, err := KeychainGet(ctx, "devvm-test", "github"); v != "ghp_testfake" || err != nil {
		t.Errorf("KeychainGet = %q, %v", v, err)
	}
	for _, bad := range []string{`x" -A "`, `x\`, "x\ny"} {
		if err := KeychainSet(ctx, "devvm-test", "github", "devvm github", bad); err == nil {
			t.Errorf("KeychainSet(%q) took it", bad)
		}
	}
	if err := KeychainDelete(ctx, "devvm-test", "github"); err != nil {
		t.Errorf("KeychainDelete: %v", err)
	}
	if err := KeychainDelete(ctx, "devvm-test", "github"); !errors.Is(err, ErrNoSecret) {
		t.Errorf("KeychainDelete again: %v", err)
	}
	if calls := s.Read(t, "calls"); strings.Contains(calls, "ghp_testfake") || !strings.HasPrefix(calls, "security -i\n") {
		t.Errorf("calls = %q", calls)
	}
}

// The CA folder is kept out of Time Machine, also one made before devvm
// did that: only when it isn't already.
func TestExcludeFromBackup(t *testing.T) {
	s := stubtest.New(t)
	for range 2 {
		if err := ExcludeFromBackup(t.Context(), "/data/ca"); err != nil {
			t.Fatal(err)
		}
	}
	if calls := s.Read(t, "calls"); calls != "tmutil addexclusion /data/ca\n" {
		t.Errorf("calls = %q", calls)
	}
}

// security -i's line holds a whole item or none of it: a value that would
// take it past keychainLineMax, or a part that would end it, is refused
// before security runs, so the item saved before stays, and nothing of
// the value runs as a command.
func TestKeychainLine(t *testing.T) {
	for _, tc := range []struct {
		value string
		ok    bool
	}{
		{"ghp_x", true},
		{strings.Repeat("a", 3800), true},
		{strings.Repeat("a", keychainLineMax), false},
		{strings.Repeat("a", 4100) + "\nadd-generic-password -s x -a y -w z", false},
		{`a"b`, false},
		{`a\b`, false},
		{"a\nb", false},
	} {
		line, err := keychainLine("devvm", "custom.stripe", "devvm custom.stripe", tc.value)
		if (err == nil) != tc.ok || tc.ok && (len(line) > keychainLineMax || strings.Count(line, "\n") != 1) {
			t.Errorf("keychainLine(%d bytes): %d bytes, %v; want ok %v", len(tc.value), len(line), err, tc.ok)
		}
	}
	s := stubtest.New(t)
	if err := KeychainSet(t.Context(), "devvm-test", "a", "devvm a", strings.Repeat("a", 5000)); err == nil {
		t.Error("a value past the line was saved")
	}
	if calls := s.Read(t, "calls"); strings.Contains(calls, "security") {
		t.Errorf("security ran: %q", calls)
	}
}
