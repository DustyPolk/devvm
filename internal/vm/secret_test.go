package vm

import (
	"strings"
	"testing"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/state"
)

// devvm secret: saved and removed tokens, listed with the VMs using them;
// use and unuse on the folder's VM, claude staying with an image that has
// it.
func TestSecrets(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	n := Net{e}
	st := existingVM(t, e, Options{Network: "open"})
	vm := st.Name()
	ctx := t.Context()
	list := func() string {
		t.Helper()
		ss, err := n.Secrets(ctx)
		if err != nil {
			t.Fatal(msg.NamesOf(err))
		}
		var b strings.Builder
		for _, s := range ss {
			saved := "no"
			if s.Saved {
				saved = "yes"
			}
			b.WriteString(s.Name + " " + saved + " " + strings.Join(s.UsedBy, ",") + "\n")
		}
		return b.String()
	}
	if got, want := list(), "claude no \ngithub no \nproxy no \n"; got != want {
		t.Errorf("Secrets = %q, want %q", got, want)
	}
	// The company proxy's password: saved as given, spaces inside kept, and
	// never a VM's.
	if err := n.SecretSave(ctx, "proxy", " pa55 word\n"); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if v, err := e.Proxy.Secrets.Value(ctx, "proxy"); err != nil || v != "pa55 word" {
		t.Errorf("proxy = %q, %v", v, err)
	}
	for _, use := range []bool{true, false} {
		if err := n.SecretUse(ctx, "proxy", use); msg.NamesOf(err) != "secret_host_only secret=proxy" {
			t.Errorf("SecretUse(proxy, %v) = %v", use, msg.NamesOf(err))
		}
	}
	if err := n.SecretRm(ctx, "proxy"); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := n.SecretSave(ctx, "github", " ghp_testfake\n"); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := n.SecretUse(ctx, "github", true); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if got := list(); !strings.Contains(got, "github yes "+vm+"\n") {
		t.Errorf("Secrets = %q", got)
	}
	if err := n.SecretSave(ctx, "claude", "not-a-setup-token"); msg.NamesOf(err) != "not_setup_token" {
		t.Errorf("SecretSave(claude) = %v", msg.NamesOf(err))
	}
	if err := n.SecretRm(ctx, "github"); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := n.SecretRm(ctx, "github"); msg.NamesOf(err) != "no_secret secret=github" {
		t.Errorf("SecretRm again = %v", msg.NamesOf(err))
	}
	if err := n.SecretUse(ctx, "github", false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if st, _ := state.ReadVM(e.Dirs, vm); get(st, keySecrets) != "" {
		t.Errorf("secrets=%q", get(st, keySecrets))
	}
	// A VM with Claude Code always uses the Claude Code token.
	if err := set(t, st, keyToolNames, "claude"); err != nil {
		t.Fatal(err)
	}
	if err := n.SecretUse(ctx, "claude", false); msg.NamesOf(err) != "secret_with_claude vm="+vm {
		t.Errorf("SecretUse(claude, false) = %v", msg.NamesOf(err))
	}
	if got := list(); !strings.Contains(got, "claude no "+vm+"\n") {
		t.Errorf("Secrets = %q", got)
	}
}
