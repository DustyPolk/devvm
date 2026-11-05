// Package assets embeds the files devvm ships in its binary: base's Lima
// template (base.yaml), the prelude that comes first in its build scripts
// (prelude.sh), base's scripts and policy (layers/), its optional
// features (features/), the shared tools' config, build script and
// policies (shared-tools/), the network tiers' own policies (network/),
// the scripts sent into every VM and its port forwarder's (guest/), and
// --from's conversion (oci/).
package assets

import "embed"

// FS holds devvm's files, by their paths in assets/.
//
//go:embed base.yaml prelude.sh layers features shared-tools network guest oci
var FS embed.FS
