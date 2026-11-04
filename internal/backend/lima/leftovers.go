package lima

import (
	"context"
	"errors"
	"os"

	"github.com/w3cj/devvm/internal/backend"
)

var (
	_ backend.Locator     = (*Lima)(nil)
	_ backend.StrayFinder = (*Lima)(nil)
)

// Location is devvm's Lima home, which DEVVM_LIMA_HOME moves.
func (l *Lima) Location() (string, bool) {
	return l.Dirs.Lima, l.Dirs.Lima == l.Dirs.DefaultLima()
}

// Strays are the VMs in devvm's Lima home it has no state for.
func (l *Lima) Strays(ctx context.Context) ([]backend.Stray, error) {
	vms, err := l.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []backend.Stray
	for _, in := range vms {
		if _, err := os.Stat(l.Dirs.VM(in.Name).State()); errors.Is(err, os.ErrNotExist) {
			out = append(out, backend.Stray{Name: in.Name, Data: l.Dirs.Data})
		}
	}
	return out, nil
}
