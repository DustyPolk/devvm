package vm

import (
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/state"
)

// devvm secret: saved and removed tokens, listed with the VMs using them;
// use and unuse on the folder's VM, claude staying with an image that has
// it.
func TestSecrets(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	vm := st.Name()
	ctx := t.Context()
	list := func() string {
		t.Helper()
		var b strings.Builder
		if err := e.Secrets(ctx, &b); err != nil {
			t.Fatal(names(err))
		}
		return b.String()
	}
	if got, want := list(), "NAME    SAVED  USED BY\nclaude  no     -\ngithub  no     -\n"; got != want {
		t.Errorf("Secrets = %q, want %q", got, want)
	}
	if err := e.SecretSave(ctx, "github", " ghp_testfake\n"); err != nil {
		t.Fatal(names(err))
	}
	if err := e.SecretUse(ctx, "github", true); err != nil {
		t.Fatal(names(err))
	}
	if got := list(); !strings.Contains(got, "github  yes    "+vm+"\n") {
		t.Errorf("Secrets = %q", got)
	}
	if err := e.SecretSave(ctx, "claude", "not-a-setup-token"); names(err) != "not_setup_token" {
		t.Errorf("SecretSave(claude) = %v", names(err))
	}
	if err := e.SecretRm(ctx, "github"); err != nil {
		t.Fatal(names(err))
	}
	if err := e.SecretRm(ctx, "github"); names(err) != "no_secret secret=github" {
		t.Errorf("SecretRm again = %v", names(err))
	}
	if err := e.SecretUse(ctx, "github", false); err != nil {
		t.Fatal(names(err))
	}
	if st, _ := state.ReadVM(e.Dirs, vm); st.Get(secrets.KeySecrets) != "" {
		t.Errorf("secrets=%q", st.Get(secrets.KeySecrets))
	}
	// An image with claude always uses the Claude token.
	if err := st.Set(state.KeyImage, "base+claude"); err != nil {
		t.Fatal(err)
	}
	if err := e.SecretUse(ctx, "claude", false); names(err) != "secret_with_image vm="+vm {
		t.Errorf("SecretUse(claude, false) = %v", names(err))
	}
	if got := list(); !strings.Contains(got, "claude  no     "+vm+"\n") {
		t.Errorf("Secrets = %q", got)
	}
}
