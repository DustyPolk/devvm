// Package assets embeds the files devvm ships in its binary: base's Lima
// template (base.yaml), base's scripts and policy (layers/), its optional
// features (features/), the shared tools' config, build script and
// policies (shared-tools/), the scripts sent into every VM (guest/),
// --from's conversion (oci/), the scripts the backends that run images as
// containers share (container/), and the smolvm and Docker backends' own
// scripts (smolvm/, docker/).
package assets

import "embed"

// FS holds devvm's files, by their paths in assets/.
//
//go:embed base.yaml layers features shared-tools guest oci container smolvm docker
var FS embed.FS
