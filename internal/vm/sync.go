package vm

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
)

// syncCmd unpacks what the host sends into a folder of its own in the VM,
// runs its sync.sh, and removes it.
const syncCmd = `d=$(mktemp -d); tar -xf - -C "$d" --warning=no-unknown-keyword --warning=no-timestamp && bash "$d/sync.sh"; rc=$?; rm -rf "$d"; exit $rc`

// sync sets up a running VM on every launch: guest/sync.sh runs in it,
// with what it needs sent over as a tar stream. For now that's the shared
// tools: guest/tools.sh, which mounts them (at boot too), and the VM's
// tool images.
func (e *Env) sync(ctx context.Context, b backend.Backend, vm string, files []string) error {
	stage, err := e.stage(files)
	if err != nil {
		return err
	}
	out := ui.LogWriter()
	err = b.Exec(ctx, vm, []string{"bash", "-c", syncCmd}, backend.ExecOpts{Stdin: bytes.NewReader(stage), Stdout: out, Stderr: out})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return backend.Logged(msg.SyncFailed(vm))
	}
	return nil
}

// stage returns the tar stream sync sends.
func (e *Env) stage(files []string) ([]byte, error) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	add := func(name string, mode int64, data []byte) error {
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := w.Write(data)
		return err
	}
	for _, s := range []string{"sync.sh", "tools.sh"} {
		data, err := os.ReadFile(filepath.Join(e.Root, "guest", s)) //nolint:gosec // devvm's own files
		if err != nil {
			return nil, err
		}
		if err := add(s, 0o755, data); err != nil {
			return nil, err
		}
	}
	if err := add("tools", 0o644, tools.Mounts(files)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
