package packaging

import (
	"os"
	"testing"

	"github.com/devvmhq/devvm/internal/upgrade"
)

// The release key is minisign.pub's, a minisign public key, once the
// file is there, and none before.
func TestMinisignKey(t *testing.T) {
	_, err := os.Stat("minisign.pub")
	switch k := MinisignKey(); {
	case err != nil && k != "":
		t.Errorf("no minisign.pub, but the key %q", k)
	case err == nil:
		if _, err := upgrade.ParsePublicKey(k); err != nil {
			t.Errorf("minisign.pub's key %q: %v", k, err)
		}
	}
}
