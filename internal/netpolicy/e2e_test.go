package netpolicy

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// TestE2ESelfCheck runs the self-check on a real VM with network policy,
// without a terminal: tests/e2e.sh runs it in its sandbox, with the VM in
// DEVVM_E2E_VM and the result it expects in DEVVM_E2E_WANT (the message's
// names; empty for enforced). A failed check must leave the VM stopped.
func TestE2ESelfCheck(t *testing.T) {
	vm := os.Getenv("DEVVM_E2E_VM")
	if vm == "" {
		t.Skip("tests/e2e.sh runs it, on a VM of its own")
	}
	t.Setenv("DEVVM_MESSAGES", "names")
	d, err := paths.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	b := lima.New(d)
	st, err := state.ReadVM(d, vm)
	if err != nil {
		t.Fatal(err)
	}
	want := os.Getenv("DEVVM_E2E_WANT")
	p := &Proxy{Warn: func(m msg.M) { t.Log(m.Names()) }, Terminal: func() bool { return false }}
	err = p.SelfCheck(t.Context(), b, st, func(context.Context) error { return errors.New("restarted without a terminal") })
	if names(err) != want {
		t.Fatalf("SelfCheck = %v, want %q", names(err), want)
	}
	in, err := b.Inspect(t.Context(), vm)
	if err != nil {
		t.Fatal(err)
	}
	if running := in.Status == backend.StatusRunning; running != (want == "") {
		t.Errorf("status %s after %q", in.Status, want)
	}
}
