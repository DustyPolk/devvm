package smolvm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// ToolsVM is the throwaway machine shared tools are built in.
const ToolsVM = "devvm-tools-build"

// BuildTools makes ToolsVM from base's pack, with spec.Out its only mount,
// starts it with the network spec.Net asks for, runs the script with its
// arguments as the VM's user, makes each image it wrote uncompressed
// (plain.sh, ToolFormat), and deletes it. A ToolsVM of devvm's left by an
// earlier run is deleted first, and the machine goes however the run ends
// (a cleanup on the ui stack).
func (s *Smolvm) BuildTools(ctx context.Context, spec backend.ToolsSpec) error {
	if spec.Net != backend.NetInternet && spec.Net != backend.NetNone {
		return fmt.Errorf("smolvm: builds are confined, not started with network %d", spec.Net)
	}
	if spec.Format != s.ToolFormat() {
		return fmt.Errorf("smolvm: tool images are %s, not %q", s.ToolFormat(), spec.Format)
	}
	if !keyPattern.MatchString(spec.Base) {
		return fmt.Errorf("smolvm: invalid image key %q", spec.Base)
	}
	if !filepath.IsAbs(spec.Out) || !filepath.IsAbs(spec.OutAt) {
		return fmt.Errorf("smolvm: the tools build's folders must be absolute: %q at %q", spec.Out, spec.OutAt)
	}
	if _, err := os.Stat(s.packFile(spec.Base)); err != nil {
		return msg.ImageMissing(spec.Base)
	}
	if err := s.removeIfThere(ctx, ToolsVM); err != nil {
		return err
	}
	if err := s.absent(ctx, ToolsVM); err != nil {
		return err
	}
	out, err := volume(spec.Out, spec.OutAt, true)
	if err != nil {
		return err
	}
	cpus, mem, disk, err := s.baseSize()
	if err != nil {
		return err
	}
	// A failure leaves it to the cleanup, which runs once the caller has
	// shown the run log's end.
	done := s.AddCleanup(func(ctx context.Context) { _ = s.removeIfThere(ctx, ToolsVM) })
	// Its steps: create, start, the script, plain.sh, delete.
	ctx = s.WithSteps(ctx, 5)
	if err := s.Runner.Run(ctx, msg.CloningImage("base"), s.smolvm(ctx, "machine", "create", "--name", ToolsVM,
		"--label", LabelKey+"="+s.Dirs.Data, "--from", s.packFile(spec.Base),
		"--cpus", strconv.Itoa(cpus), "--mem", mib(mem), "--storage", gibCeil(disk), "-v", out)); err != nil {
		return backend.Logged(msg.VMCreateFailed(ToolsVM))
	}
	if err := s.writeRecord(ToolsVM, record{Image: spec.Base}); err != nil {
		return err
	}
	if err := s.Start(ctx, ToolsVM, backend.Confinement{Net: spec.Net}); err != nil {
		return err
	}
	plainScript, err := s.script("plain.sh")
	if err != nil {
		return err
	}
	args, err := s.userArgs(ToolsVM, "/", true, false, append([]string{"bash", "-s", "--"}, spec.Args...))
	if err != nil {
		return err
	}
	cmd := s.smolvm(ctx, args...)
	cmd.Stdin = bytes.NewReader(spec.Script.Data)
	if err := s.Runner.Run(ctx, msg.BuildingToolsIn(ToolsVM), cmd); err != nil {
		return backend.Logged(msg.ToolsBuildFailed())
	}
	files := []string{spec.OutAt}
	for i := 2; i < len(spec.Args); i += 3 {
		files = append(files, spec.Args[i])
	}
	plain := s.asRoot(ctx, ToolsVM, files...)
	plain.Stdin = bytes.NewReader(plainScript)
	if err := s.Runner.Run(ctx, msg.RunningScript("plain.sh", ToolsVM), plain); err != nil {
		return backend.Logged(msg.ToolsBuildFailed())
	}
	if err := s.remove(ctx, ToolsVM); err != nil {
		return err
	}
	done()
	return nil
}
