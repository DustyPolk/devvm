// Package assets embeds the files devvm ships in its binary: base's Lima
// template (base.yaml), base's scripts and policy (layers/), its optional
// features (features/), the shared tools' config, build script and
// policies (shared-tools/), the scripts sent into every VM (guest/),
// --from's conversion (oci/) and the smolvm backend's own scripts
// (smolvm/).
package assets

import "embed"

// FS holds devvm's files, by their paths in assets/.
//
//go:embed base.yaml layers features shared-tools guest oci smolvm
var FS embed.FS
