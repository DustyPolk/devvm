// Package packaging embeds what devvm's releases are packaged with, for
// devvm upgrade: minisign.pub, the release key's public half, which
// signs each release's SHA256SUMS (scripts/release.sh). It's embedded
// with the rest of this folder, so a build without it still builds, and
// has no key.
package packaging

import (
	"embed"
	"strings"
)

// files is this folder, minisign.pub among it once the release key
// exists.
//
//go:embed *
var files embed.FS

// MinisignKey is the release key's public half, as minisign -P takes it
// (minisign.pub's second line), or "" when this build has none.
func MinisignKey() string {
	b, err := files.ReadFile("minisign.pub")
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		return ""
	}
	return strings.TrimSpace(lines[1])
}
