// Package from looks up the container images base can start from
// (--from): references as Docker reads them, their digests from their
// registries or from Docker, and the digest devvm last recorded for each.
//
// base is keyed by an image's digest, not its tag, so a tag that moves (a
// new node:24) makes base stale. Digests are looked up when building, and
// recorded per reference, so listing images and opening VMs never look
// anything up. A registry image's digest is a reference
// (node@sha256:…); an image only Docker has (built and never pushed, or
// private) is keyed by Docker's ID (sha256:…), and only Docker can hand
// it over.
package from

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// Env is the environment variable that sets --from's default.
const Env = "DEVVM_FROM"

var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@+-]*$`)

// Valid reports whether s can be an image reference. References go into
// URLs, docker's arguments, the conversion's arguments and state files.
func Valid(s string) bool { return refPattern.MatchString(s) }

// Check refuses a --from that isn't an image reference; "" is none.
func Check(s string) error {
	if s != "" && !Valid(s) {
		return msg.BadFrom(msg.Shown(s))
	}
	return nil
}

// Pinned reports whether a reference names its digest (…@sha256:…): it
// needs no lookup.
func Pinned(ref string) bool { return strings.Contains(ref, "@sha256:") }

// Name is a reference without its tag or digest: ghcr.io/o/r for
// ghcr.io/o/r:1.
func Name(ref string) string {
	if i := strings.LastIndex(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	last := ref[strings.LastIndex(ref, "/")+1:]
	if i := strings.LastIndex(last, ":"); i >= 0 {
		ref = ref[:len(ref)-len(last)+i]
	}
	return ref
}

// Recorded returns the digest base's key has for ref: ref itself when
// it's pinned, else the one last recorded, "" when there is none.
func Recorded(d paths.Dirs, ref string) (string, error) {
	if ref == "" || Pinned(ref) {
		return ref, nil
	}
	return state.FromDigest(d, ref)
}

// Lookup looks up digests and records them. NewLookup sets every field;
// tests change them.
type Lookup struct {
	Dirs     paths.Dirs
	Registry *Registry
	// DockerID returns Docker's ID for an image, when Docker runs and has
	// it.
	DockerID func(ctx context.Context, ref string) (string, bool)
	Warn     func(msg.M)
}

// NewLookup returns a Lookup on the internet and this machine's Docker.
func NewLookup(d paths.Dirs) *Lookup {
	return &Lookup{Dirs: d, Registry: NewRegistry(), DockerID: DockerID, Warn: ui.Warn}
}

// Refresh looks up ref's digest and records it: its registry's, else
// Docker's ID for an image the registry doesn't have and Docker does.
// Registry digests can't tell: Docker's containerd store gives its own
// builds one too. When neither has it, the digest recorded before is kept,
// with a warning, before Docker's ID, so an offline build doesn't rebuild
// base. A pinned reference needs nothing.
func (l *Lookup) Refresh(ctx context.Context, ref string) error {
	if ref == "" || Pinned(ref) {
		return nil
	}
	host := ParseRef(ref).Host
	d, err := l.Registry.Digest(ctx, ref)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return state.SetFromDigest(l.Dirs, ref, Name(ref)+"@"+d)
	}
	notFound := !errors.Is(err, ErrUnreachable)
	if notFound {
		if id, ok := l.DockerID(ctx, ref); ok {
			return state.SetFromDigest(l.Dirs, ref, id)
		}
	}
	cached, err := Recorded(l.Dirs, ref)
	if err != nil {
		return err
	}
	switch {
	case cached != "" && notFound:
		l.Warn(msg.FromCachedNotFound(ref, host))
		return nil
	case cached != "":
		l.Warn(msg.FromCachedUnreachable(ref, host))
		return nil
	case notFound:
		return msg.FromNotFound(ref, host)
	}
	if id, ok := l.DockerID(ctx, ref); ok {
		return state.SetFromDigest(l.Dirs, ref, id)
	}
	return msg.FromUnreachable(ref, host)
}

// DockerID returns Docker's ID for an image (sha256:…), when Docker is
// installed and running and has it. It only asks.
func DockerID(ctx context.Context, ref string) (string, bool) {
	if !Valid(ref) {
		return "", false
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return "", false
	}
	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", ref).Output() //nolint:gosec // a checked reference
	id := strings.TrimSpace(string(out))
	return id, err == nil && digestPattern.MatchString(id)
}

// DockerRunning reports whether Docker is installed and running.
func DockerRunning(ctx context.Context) bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}
