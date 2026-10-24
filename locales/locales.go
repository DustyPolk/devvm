// Package locales embeds devvm's message files, one per language
// (en.toml, ...). internal/msg loads them; go generate ./internal/msg turns
// en.toml into Go functions.
package locales

import "embed"

// FS holds every *.toml message file.
//
//go:embed *.toml
var FS embed.FS
