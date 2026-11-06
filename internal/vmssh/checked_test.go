package vmssh

import (
	"errors"
	"slices"
	"testing"

	"github.com/devvmhq/devvm/internal/backend"
)

// Checked refuses a VM whose ssh isn't usable as an *UnusableError: the
// review refuses it, the backend can't reach it, or its target isn't a
// user and a key on a host loopback port. ReadKeys takes only the record
// for the VM's start, never reading the VM; PinKeys reads it first.
func TestChecked(t *testing.T) {
	const vm = "devvm-p-0123456789abcd"
	key := "ssh-ed25519 " + keyED
	ctx := t.Context()
	good := backend.SSHTarget{Host: Loopback, Port: 60022, User: "u", Key: "/k", Options: []backend.SSHOption{{Key: "IdentitiesOnly", Value: "yes"}}}

	unusable := []struct {
		name   string
		caps   backend.Caps
		target backend.SSHTarget
		stop   bool
	}{
		{"refused", backend.Caps{backend.FeatureSSH: backend.Emulated(false)}, good, false},
		{"stopped", nil, good, true},
		{"elsewhere", nil, backend.SSHTarget{Host: "10.0.0.1", Port: 22, User: "u", Key: "/k"}, false},
	}
	for _, c := range unusable {
		f, d := hostKeysFake(t, vm, &key)
		if c.caps != nil {
			f.Capabilities[backend.FeatureSSH] = c.caps[backend.FeatureSSH]
		}
		f.Target = c.target
		if c.stop {
			f.Instances[vm] = backend.Instance{Name: vm, Status: backend.StatusStopped}
		}
		var u *UnusableError
		if _, err := Checked(ctx, d, backend.Review{}, f, vm, PinKeys); !errors.As(err, &u) || u.VM != vm || u.Error() != u.Err.Error() {
			t.Errorf("%s: %v", c.name, err)
		}
		if execs(f) != 0 {
			t.Errorf("%s: read the VM's host keys", c.name)
		}
	}

	f, d := hostKeysFake(t, vm, &key)
	f.Target = good
	if _, err := Checked(ctx, d, backend.Review{}, f, vm, ReadKeys); !errors.Is(err, ErrNoHostKeys) || execs(f) != 0 {
		t.Errorf("read before a pin: %v, %d reads", err, execs(f))
	}
	r, err := Checked(ctx, d, backend.Review{}, f, vm, PinKeys)
	want := SSH{VM: vm, User: "u", Key: "/k", Port: 60022, KnownHosts: d.VM(vm).KnownHosts()}
	if err != nil || r.SSH != want || !slices.Equal(r.Options, good.Options) || r.Start != "start-0" || execs(f) != 1 {
		t.Errorf("pinned: %+v, %v, %d reads", r, err, execs(f))
	}
	if r, err := Checked(ctx, d, backend.Review{}, f, vm, ReadKeys); err != nil || r.SSH != want || r.Start != "start-0" || execs(f) != 1 {
		t.Errorf("read: %+v, %v, %d reads", r, err, execs(f))
	}
	// Another start: reading refuses the old record, pinning reads again.
	f.Restart(vm)
	if _, err := Checked(ctx, d, backend.Review{}, f, vm, ReadKeys); !errors.Is(err, ErrNoHostKeys) || execs(f) != 1 {
		t.Errorf("read after a start: %v", err)
	}
	if r, err := Checked(ctx, d, backend.Review{}, f, vm, PinKeys); err != nil || r.Start != "start-1" || execs(f) != 2 {
		t.Errorf("pinned after a start: %+v, %v", r, err)
	}
}
