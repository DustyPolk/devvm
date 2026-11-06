package packaging

import (
	"io/fs"
	"os"
	"regexp"
	"strings"
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

// The docs that show the release key show minisign.pub's, so you check a
// release against the key that signed it: README.md, SECURITY.md and
// the install page's section have it, and no page has another.
func TestDocsShowTheKey(t *testing.T) {
	key := MinisignKey()
	if key == "" {
		t.Skip("no minisign.pub")
	}
	// A minisign public key as minisign -P takes it: "RW" and the rest of
	// its 42 bytes in base64.
	keyLike := regexp.MustCompile(`RW[A-Za-z0-9+/]{54}`)
	repo := os.DirFS("..")
	files := []string{"README.md", "SECURITY.md"}
	err := fs.WalkDir(repo, "docs/content", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"README.md": false, "SECURITY.md": false, "docs/content/docs/internals/install.md": false}
	for _, f := range files {
		b, err := fs.ReadFile(repo, f)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range keyLike.FindAllString(string(b), -1) {
			if k != key {
				t.Errorf("%s shows the key %s, not minisign.pub's %s", f, k, key)
				continue
			}
			if _, ok := want[f]; ok {
				want[f] = true
			}
		}
	}
	for f, shown := range want {
		if !shown {
			t.Errorf("%s doesn't show the release key %s", f, key)
		}
	}
}
