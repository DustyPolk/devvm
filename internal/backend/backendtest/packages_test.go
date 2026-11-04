package backendtest

import (
	"errors"
	"io"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
)

// A build's packages: Send's stream is kept as PackagesSent and Take
// reads PackagesOut; either failing fails the build. PinOrigin answers
// Pinned, recording how it was asked.
func TestFakePackagesAndPin(t *testing.T) {
	ctx := t.Context()
	f := New("fake", backend.Caps{})
	f.PackagesOut = []byte("out")
	var took string
	spec := backend.ImageSpec{Image: "base", Key: "k1", Packages: &backend.Packages{
		Send: func(w io.Writer) error { _, err := io.WriteString(w, "in"); return err },
		Take: func(r io.Reader) error { b, err := io.ReadAll(r); took = string(b); return err },
	}}
	if _, err := f.BuildImage(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if string(f.PackagesSent) != "in" || took != "out" {
		t.Errorf("sent %q, took %q", f.PackagesSent, took)
	}
	bad := errors.New("bad")
	for name, p := range map[string]*backend.Packages{
		"send": {Send: func(io.Writer) error { return bad }},
		"take": {Take: func(io.Reader) error { return bad }},
	} {
		spec.Packages = p
		if _, err := f.BuildImage(ctx, spec); !errors.Is(err, bad) {
			t.Errorf("%s: %v", name, err)
		}
	}
	f.Pinned = "release-20260101"
	if got, err := f.PinOrigin(ctx, nil, backend.PinNewest); err != nil || got != "release-20260101" {
		t.Errorf("PinOrigin = %q, %v", got, err)
	}
	f.Errs = map[string]error{"PinOrigin": bad}
	if _, err := f.PinOrigin(ctx, nil, backend.PinFirst); !errors.Is(err, bad) {
		t.Errorf("PinOrigin failing: %v", err)
	}
}
