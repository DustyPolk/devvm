package platform

import (
	"errors"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/stubtest"
)

// The stubs come first on PATH in these tests, never the real security
// or tmutil.

func TestKeychainGet(t *testing.T) {
	s := stubtest.New(t)
	if _, err := KeychainGet(t.Context(), "devvm-test", "github"); !errors.Is(err, ErrNoSecret) {
		t.Errorf("no item: %v", err)
	}
	s.Write(t, "keychain/github", "ghp_x\n")
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
