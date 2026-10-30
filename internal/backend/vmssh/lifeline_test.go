package vmssh

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A forwarder follows the VM's lifeline: it holds while its contents stay
// and its process runs, and not once a new start rewrites it.
func TestLifeline(t *testing.T) {
	life := filepath.Join(t.TempDir(), "agent.pid")
	if got, err := ReadLifeline(life); got != "" || err != nil {
		t.Errorf("none: %q, %v", got, err)
	}
	if err := os.WriteFile(life, []byte(strconv.Itoa(os.Getpid())+"\n123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cur, err := ReadLifeline(life)
	if err != nil || !LifelineHolds(life, cur) {
		t.Errorf("holds: %q, %v", cur, err)
	}
	if err := os.WriteFile(life, []byte(strconv.Itoa(os.Getpid())+"\n456\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if LifelineHolds(life, cur) {
		t.Error("holds after a new start")
	}
	reserved := func(string) (map[int]bool, error) { return nil, nil }
	base := []string{"-vm", "v", "-user", "u", "-key", "/k", "-known-hosts", "/kh", "-control", "/c", "-port", "1"}
	if err := LifelineMain(t.Context(), "_x-ports", base, nil, reserved, &strings.Builder{}); err == nil {
		t.Error("no lifeline: ran")
	}
	gone := filepath.Join(t.TempDir(), "ha.pid")
	if err := LifelineMain(t.Context(), "_x-ports", append(base, "-lifeline", gone), nil, reserved, &strings.Builder{}); err == nil {
		t.Error("an empty lifeline: ran")
	}
}
