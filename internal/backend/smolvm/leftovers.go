package smolvm

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/w3cj/devvm/internal/backend"
)

var (
	_ backend.Locator     = (*Smolvm)(nil)
	_ backend.Forgetter   = (*Smolvm)(nil)
	_ backend.StrayFinder = (*Smolvm)(nil)
)

// Location is where smolvm keeps this run's machines: on macOS the user's
// own smolvm state, under HOME (a devvm run with another HOME sees other
// machines); on Linux smolvm's state in devvm's data folder.
func (s *Smolvm) Location() (string, bool) {
	if s.OS == "darwin" {
		return s.Dirs.Home, true
	}
	return s.stateDir(), true
}

// Forget deletes what devvm keeps for a machine smolvm no longer has (or
// has as someone else's): its record, proxy file, ssh key and port
// forwarder's files. One of devvm's machines that smolvm has is refused.
func (s *Smolvm) Forget(ctx context.Context, vm string) error {
	if _, err := s.inspect(ctx, vm); err == nil {
		return fmt.Errorf("smolvm: %s exists", vm)
	} else if !errors.Is(err, backend.ErrNotFound) && !errors.Is(err, errForeign) {
		return err
	}
	if err := s.Forwards.Remove(vm); err != nil {
		return err
	}
	for _, f := range []string{s.Dirs.SmolvmVM(vm), s.proxyFile(vm), s.sshKey(vm)} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Strays are the project machines with devvm's label that this devvm has
// no state for: another devvm's (its label names another data folder),
// or this devvm's whose state is gone. Images' and the tools build's
// machines aren't project machines.
func (s *Smolvm) Strays(ctx context.Context) ([]backend.Stray, error) {
	ms, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var out []backend.Stray
	for _, m := range ms {
		data, labelled := m.Labels[LabelKey]
		if _, isImage := imageKey(m.Name); !labelled || isImage || m.Name == ToolsVM {
			continue
		}
		if data != s.Dirs.Data {
			out = append(out, backend.Stray{Name: m.Name, Data: data})
		} else if _, err := os.Stat(s.Dirs.VMState(m.Name)); errors.Is(err, os.ErrNotExist) {
			out = append(out, backend.Stray{Name: m.Name, Data: data})
		}
	}
	return out, nil
}
