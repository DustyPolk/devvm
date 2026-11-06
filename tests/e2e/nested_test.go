//go:build e2e

package e2e

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/devvmhq/devvm/internal/shell"
)

// nested is a project folder inside the base project's folder, a repo of
// its own with a VM of its own, which the base project's VM (parent)
// mounts too, so code in that VM can replace it with a link to a folder
// of this machine's: decoy, which stands in for your home folder.
type nested struct {
	s      *S
	parent string // the base project's VM
	dir    string // the nested project folder
	vm     string // its VM
	decoy  string // the folder the link points to
	bin    string // the folder of limactl's stand-ins
}

// projectNested: the nested folder replaced by a link while its VM runs,
// once its stop ends (so devvm restart's start finds the link) and once
// devvm's start runs limactl start (after devvm's checks, as code in the
// parent's VM that wins the race would): the nested VM never mounts the
// decoy, and devvm refuses, the VM stopped.
func projectNested(s *S, w *webVM) {
	s.Section("a nested project folder replaced by a link")
	n := &nested{s: s, parent: w.vm, dir: filepath.Join(w.p, "inner"), bin: filepath.Join(s.sb.work, "lima-swap")}
	madeFile(s.sb, n.bin)
	n.decoy, _ = s.NewProject("decoy")
	_ = os.MkdirAll(n.dir, 0o755) //nolint:gosec // a project's folder, as its user's mkdir makes it
	_, _ = s.Run(Cmd{Args: []string{"git", "-C", n.dir, "init", "-q"}})
	_ = os.WriteFile(filepath.Join(n.dir, "inner-own"), []byte("inner\n"), 0o600)
	_ = os.WriteFile(filepath.Join(n.decoy, "decoy-secret"), []byte("decoy\n"), 0o600)
	n.vm, _, _ = s.Launch(n.dir, "exit", "--network", "open")
	// The parent's VM runs, to replace the folder.
	_, _ = DevvmIn(s, w.p, "exit\n", nil)
	s.Check("the nested folder, a repo of its own, gets a VM of its own that mounts it", func() error {
		if n.vm == "" || n.vm == w.vm || s.Status(n.vm) != "Running" || s.Status(w.vm) != "Running" {
			return errors.New("no VM of its own running, or the parent's isn't")
		}
		return n.mountsOwn()
	})
	n.whileRunning()
	n.atRestart()
	n.atStart()
	s.Check("put back, devvm opens its VM on its own folder", func() error {
		n.restore()
		if s.Status(n.vm) == "Running" {
			if _, err := s.Devvm(n.dir, "stop"); err != nil {
				return err
			}
		}
		r, err := DevvmIn(s, n.dir, "ls -A\n", nil)
		if err != nil || !HasLine(r.Out, "inner-own") {
			return fmt.Errorf("opened: %w\n%s", err, r.Out)
		}
		return n.mountsOwn()
	})
	_, _ = s.Devvm(n.dir, "rm", "--force")
	unlock(s.r.window().ctx, n.dir)
	_ = os.RemoveAll(n.dir)
}

// whileRunning: replaced while the nested VM runs, its mount never shows
// the decoy: 9p on Linux keeps the folder it opened, and macOS's virtiofs
// refuses to read the folder (Operation not permitted).
func (n *nested) whileRunning() {
	n.s.Check("replaced while its VM runs, the VM never sees the link's folder", func() error {
		defer n.restore()
		if err := n.swap(); err != nil {
			return err
		}
		r, err := n.s.InVM(n.vm, "ls -A "+shell.Quote(n.dir))
		seen := strings.ReplaceAll(r.Out, "\n", " ")
		Note(n.s, "the VM's folder, replaced while it runs: %s (%v; %s)", seen, err, n.ids())
		switch {
		case HasLine(r.Stdout, "decoy-secret"):
			return errors.New("the VM sees the link's folder: " + seen)
		case runtime.GOOS == "linux" && !HasLine(r.Stdout, "inner-own"):
			return errors.New("the VM's 9p mount doesn't keep its folder: " + seen)
		case runtime.GOOS == "darwin" && err == nil && !HasLine(r.Stdout, "inner-own"):
			return errors.New("the VM's virtiofs mount neither keeps nor refuses its folder: " + seen)
		}
		return nil
	})
}

// atRestart: replaced once devvm restart's stop ends, devvm refuses to
// start the VM on the link.
func (n *nested) atRestart() {
	n.s.Check("replaced once devvm restart stopped its VM, devvm refuses to start it, and it stays stopped", func() error {
		defer n.restore()
		r, err := n.devvmSwapping("stop", "restart")
		return n.refused(r, err, "project_folder_changed")
	})
}

// atStart: replaced as devvm's start runs limactl start, once devvm has
// checked the folder, devvm stops the VM and refuses.
func (n *nested) atStart() {
	n.s.Check("replaced as Lima starts its VM, devvm stops the VM and refuses", func() error {
		defer n.restore()
		if n.s.Status(n.vm) == "Running" {
			if _, err := n.s.Devvm(n.dir, "stop"); err != nil {
				return err
			}
		}
		r, err := n.devvmSwapping("start")
		return n.refused(r, err, "project_folder_changed_stopped")
	})
}

// refused checks that devvm, which printed r and returned err, said name
// and left the VM stopped, saying what the VM mounted otherwise.
func (n *nested) refused(r Res, err error, name string) error {
	if !Exists(n.dir + ".real") {
		return errors.New("the parent's VM didn't replace the folder:\n" + tailLines(r.Out, 10))
	}
	status := n.s.Status(n.vm)
	if err == nil || !grepF(r.Out, n.s.Msg(name, "dir="+n.dir, "vm="+n.vm)) || status != "Stopped" {
		seen := ""
		if status == "Running" {
			seen = "; the VM's folder has: " + n.listing() + " (" + n.ids() + ")"
		}
		return fmt.Errorf("devvm: %w, the VM %s%s\n%s", cmp.Or(err, errors.New("no error")), status, seen, tailLines(r.Out, 10))
	}
	return nil
}

// devvmSwapping runs devvm args in the nested folder, its limactl a
// stand-in that replaces the folder with the link when devvm's limactl
// at (start or stop) of the nested VM starts (start) or ends (stop).
func (n *nested) devvmSwapping(at string, args ...string) (Res, error) {
	if err := n.standIn(at); err != nil {
		return Res{}, err
	}
	env := []string{"PATH=" + n.bin + ":" + os.Getenv("PATH")}
	return DevvmIn(n.s, n.dir, "exit\n", env, args...)
}

// standIn writes n.bin/limactl, which runs the real limactl, replacing
// the folder through the parent's VM once, when limactl at of the nested
// VM starts (start) or ends (stop).
func (n *nested) standIn(at string) error {
	lima, err := exec.LookPath("limactl")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(n.bin, 0o700); err != nil {
		return err
	}
	q, done := shell.Quote(lima), shell.Quote(filepath.Join(n.bin, "swapped"))
	_ = os.Remove(filepath.Join(n.bin, "swapped"))
	swap := "swap() { [[ -e " + done + " ]] && return; : > " + done + "; " + q + " shell --workdir / " + shell.Quote(n.parent) +
		" bash -c " + shell.Quote(n.swapScript()) + " </dev/null >/dev/null 2>&1; }\n"
	is := `[[ $1 == ` + at + ` && ${@: -1} == ` + shell.Quote(n.vm) + ` ]]`
	body := "if " + is + "; then swap; fi\nexec " + q + " \"$@\"\n"
	if at == "stop" {
		body = "if " + is + "; then " + q + " \"$@\"; rc=$?; swap; exit $rc; fi\nexec " + q + " \"$@\"\n"
	}
	script := "#!/bin/bash\n# e2e: the parent folder's VM replaces the nested folder with a link, as limactl " + at + " of its VM runs.\n" + swap + body
	return os.WriteFile(filepath.Join(n.bin, "limactl"), []byte(script), 0o700) //nolint:gosec // G306: the stand-in runs
}

// swapScript is what the parent's VM runs to replace the folder: moved
// aside, a link to the decoy in its place.
func (n *nested) swapScript() string {
	return "mv " + shell.Quote(n.dir) + " " + shell.Quote(n.dir+".real") + " && ln -s " + shell.Quote(n.decoy) + " " + shell.Quote(n.dir)
}

// swap replaces the folder through the parent's VM, and checks that this
// machine sees the link.
func (n *nested) swap() error {
	if _, err := n.s.InVM(n.parent, n.swapScript()); err != nil {
		return err
	}
	if fi, err := os.Lstat(n.dir); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("the parent's VM didn't replace the folder with a link: %w", cmp.Or(err, errors.New("not a link")))
	}
	return nil
}

// restore puts the folder back where the link was.
func (n *nested) restore() {
	if fi, err := os.Lstat(n.dir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(n.dir)
	}
	if Exists(n.dir + ".real") {
		_ = os.Rename(n.dir+".real", n.dir)
	}
}

// mountsOwn checks that the running nested VM sees its own folder at its
// path, never the decoy's files.
func (n *nested) mountsOwn() error {
	l := n.listing()
	if !HasLine(l, "inner-own") || HasLine(l, "decoy-secret") {
		return errors.New("the VM's folder has: " + strings.ReplaceAll(l, "\n", " ") + " (" + n.ids() + ")")
	}
	Note(n.s, "the nested folder: %s", n.ids())
	return nil
}

// listing is what the nested VM sees in its folder.
func (n *nested) listing() string {
	r, _ := n.s.InVM(n.vm, "ls -A "+shell.Quote(n.dir))
	return r.Stdout
}

// ids is the device and inode of the folder at its path in the nested VM,
// and of the folder and the decoy on this machine.
func (n *nested) ids() string {
	r, _ := n.s.InVM(n.vm, "stat -c '%d:%i' "+shell.Quote(n.dir))
	return "in the VM " + strings.TrimSpace(r.Stdout) + "; here, the folder " + devIno(n.dir) + devIno(n.dir+".real") + ", the decoy " + devIno(n.decoy)
}

// devIno is the device and inode of what's at path, following a link.
func devIno(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d:%d ", st.Dev, st.Ino)
}
