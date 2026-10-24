// Package backends registers every backend devvm has. It's apart from
// package backend, which the backends themselves import.
package backends

import (
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/paths"
)

// Registry returns the registry of devvm's backends, on devvm's folders.
// lima is the default (backend.Default).
func Registry(d paths.Dirs) (*backend.Registry, error) {
	return backend.NewRegistry(lima.New(d))
}
