package lima

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/w3cj/devvm/internal/backend"
)

var (
	_ backend.Locator     = (*Lima)(nil)
	_ backend.Forgetter   = (*Lima)(nil)
	_ backend.StrayFinder = (*Lima)(nil)
)

// Location is devvm's Lima home, which DEVVM_LIMA_HOME moves.
func (l *Lima) Location() (string, bool) {
	return l.Dirs.Lima, l.Dirs.Lima == l.Dirs.DefaultLima()
}

// Forget deletes what devvm keeps for a VM its Lima home no longer has:
// its backend folder, its port record and forwarder's files, its
// forwarder stopped first. A VM the home has is refused.
func (l *Lima) Forget(ctx context.Context, vm string) error {
	if _, err := l.inspect(ctx, vm); err == nil {
		return fmt.Errorf("lima: %s exists", vm)
	} else if !errors.Is(err, backend.ErrNotFound) {
		return err
	}
	return l.Forwards.Forget(vm)
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
