//go:build e2e

package e2e

import (
	"cmp"
	"encoding/json"
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
	n.atStartRestored()
	s.Check("put back, devvm opens its VM on its own folder, and its start edits nothing of its Lima config", func() error {
		n.restore()
		if s.Status(n.vm) == "Running" {
			if _, err := s.Devvm(n.dir, "stop"); err != nil {
				return err
			}
		}
		if err := n.standIn("none"); err != nil {
			return err
		}
		r, err := DevvmIn(s, n.dir, "ls -A\n", []string{"PATH=" + n.bin + ":" + os.Getenv("PATH")})
		if err != nil || !HasLine(r.Out, "inner-own") {
			return fmt.Errorf("opened: %w\n%s", err, r.Out)
		}
		calls, _ := os.ReadFile(filepath.Join(n.bin, "calls"))
		if !strings.Contains(string(calls), "start") || strings.Contains("\n"+string(calls), "\nedit ") {
			return errors.New("limactl ran:\n" + string(calls))
		}
		if err := n.mountedByInode(); err != nil {
			return err
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

// atStartRestored: replaced as Lima starts its VM and put back once
// limactl start returns, before devvm's last check, as code in the
// parent's VM that times it would. On macOS the VM's mount is by the
// folder's device and inode, so it never sees the decoy: it opened the
// folder where it was moved, and refuses to read it once it's put back
// (Operation not permitted). Linux's 9p has no such mount and follows the
// link, the gap the docs name, which is only noted.
func (n *nested) atStartRestored() {
	n.s.Check("replaced as Lima starts its VM and put back before devvm checks, on macOS the VM never sees the link's folder", func() error {
		defer n.restore()
		if n.s.Status(n.vm) == "Running" {
			if _, err := n.s.Devvm(n.dir, "stop"); err != nil {
				return err
			}
		}
		r, err := n.devvmSwapping("start-restore")
		if !Exists(filepath.Join(n.bin, "swapped")) || Exists(n.dir+".real") {
			return fmt.Errorf("the parent's VM didn't replace the folder and put it back (devvm: %w):\n%s", cmp.Or(err, errors.New("no error")), tailLines(r.Out, 10))
		}
		l, lerr := n.s.InVM(n.vm, "ls -A "+shell.Quote(n.dir))
		seen := strings.ReplaceAll(l.Out, "\n", " ")
		Note(n.s, "the VM's folder, replaced as Lima started it and put back: %s (%v; devvm: %v; %s)", seen, lerr, err, n.ids())
		switch {
		case runtime.GOOS == "linux":
			if HasLine(l.Stdout, "decoy-secret") {
				Note(n.s, "on Linux the VM's 9p mount followed the link, as the docs say")
			}
			return nil
		case HasLine(l.Stdout, "decoy-secret"):
			return errors.New("the VM sees the link's folder: " + seen)
		case lerr == nil && !HasLine(l.Stdout, "inner-own"):
			return errors.New("the VM's virtiofs mount neither keeps nor refuses its folder: " + seen)
		}
		return n.mountedByInode()
	})
}

// mountedByInode checks, on macOS, that the nested VM's Lima config
// mounts the folder by its device and inode at its path, as this machine
// has them now.
func (n *nested) mountedByInode() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	r, err := n.s.Limactl("list", "--json", n.vm)
	if err != nil {
		return err
	}
	var in struct {
		Config struct {
			Mounts []struct {
				Location   string `json:"location"`
				MountPoint string `json:"mountPoint"`
				Writable   bool   `json:"writable"`
			} `json:"mounts"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &in); err != nil {
		return err
	}
	var got []string
	for _, m := range in.Config.Mounts {
		if m.Writable {
			got = append(got, m.Location+" at "+m.MountPoint)
		}
	}
	if want := "/.vol/" + strings.ReplaceAll(strings.TrimSpace(devIno(n.dir)), ":", "/") + " at " + n.dir; len(got) != 1 || got[0] != want {
		return fmt.Errorf("its read-write mounts: %q, want %q", got, want)
	}
	return nil
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

// standIn writes n.bin/limactl, which logs each call's arguments to
// n.bin/calls and runs the real limactl, replacing the folder through
// the parent's VM once, when limactl at of the nested VM starts (start)
// or ends (stop); with start-restore, as it starts, and the folder put
// back once it ends, before devvm's last check. Any other at replaces
// nothing.
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
	_ = os.Remove(filepath.Join(n.bin, "calls"))
	inParent := func(script string) string {
		return q + " shell --workdir / " + shell.Quote(n.parent) + " bash -c " + shell.Quote(script) + " </dev/null >/dev/null 2>&1"
	}
	fns := "swap() { [[ -e " + done + " ]] && return; : > " + done + "; " + inParent(n.swapScript()) + "; }\n" +
		"put() { " + inParent(n.restoreScript()) + "; }\n"
	cmd := strings.TrimSuffix(at, "-restore")
	is := `[[ $1 == ` + cmd + ` && ${@: -1} == ` + shell.Quote(n.vm) + ` ]]`
	run := q + " \"$@\"; rc=$?; "
	var body string
	switch at {
	case "stop":
		body = "if " + is + "; then " + run + "swap; exit $rc; fi\n"
	case "start-restore":
		body = "if " + is + "; then swap; " + run + "put; exit $rc; fi\n"
	default:
		body = "if " + is + "; then swap; fi\n"
	}
	log := "printf '%s\\n' \"$*\" >> " + shell.Quote(filepath.Join(n.bin, "calls")) + "\n"
	script := "#!/bin/bash\n# e2e: the parent folder's VM replaces the nested folder with a link, as limactl " + at + " of its VM runs.\n" +
		fns + log + body + "exec " + q + " \"$@\"\n"
	return os.WriteFile(filepath.Join(n.bin, "limactl"), []byte(script), 0o700) //nolint:gosec // G306: the stand-in runs
}

// swapScript is what the parent's VM runs to replace the folder: moved
// aside, a link to the decoy in its place.
func (n *nested) swapScript() string {
	return "mv " + shell.Quote(n.dir) + " " + shell.Quote(n.dir+".real") + " && ln -s " + shell.Quote(n.decoy) + " " + shell.Quote(n.dir)
}

// restoreScript is what the parent's VM runs to put the folder back: the
// link gone, the folder at its path again.
func (n *nested) restoreScript() string {
	return "rm " + shell.Quote(n.dir) + " && mv " + shell.Quote(n.dir+".real") + " " + shell.Quote(n.dir)
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
