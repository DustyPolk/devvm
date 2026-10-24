package vm

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// --clone: the VM works on its own clone of the repo instead of mounting
// the folder, so nothing in the VM can reach the working tree or .git.
// The host pushes its branches into the VM once, and the repo gets a git
// remote to fetch the VM's work from, by the VM's full name in devvm's ssh
// config. The clone is at the folder's path in the VM, so shells and
// editors open in the same place.

// Keys of a --clone VM's state.
const (
	KeyMode        = "mode"         // clone, for a --clone VM; unset for one that mounts its folder
	KeyCloneRemote = "clone_remote" // the repo's remote for the VM
	KeySeeded      = "seeded"       // yes once the host's branches are in the VM
)

// ModeClone is KeyMode for a --clone VM.
const ModeClone = "clone"

// cloneShown is how many of the clone's uncommitted changes rm shows.
const cloneShown = 20

// cloneOutMax is the most of the VM's answer read for its branches or
// its changes.
const cloneOutMax = 1 << 20

// objectName matches a git object name: what a VM answers is only looked
// up when it's one.
var objectName = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// isClone reports whether the VM has its own clone of the repo.
func isClone(st *state.VM) bool { return st.Get(KeyMode) == ModeClone }

// cloneURL is the remote URL of vm's clone of dir: the VM's full name in
// devvm's ssh config, which never changes, and the clone's path.
func cloneURL(vm, dir string) string { return vm + ":" + dir }

// hostGit runs git in dir on the host and returns its output, trimmed.
func hostGit(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output() //nolint:gosec // devvm's own arguments
	return strings.TrimSpace(string(out)), err
}

// checkCloneRepo refuses --clone for a folder that isn't the top of a git
// repo with a commit and a branch checked out.
func checkCloneRepo(ctx context.Context, dir string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return msg.CloneNeedsGit()
	}
	if top, err := hostGit(ctx, dir, "rev-parse", "--show-toplevel"); err != nil || top != dir {
		return msg.CloneNotTop(dir)
	}
	if _, err := hostGit(ctx, dir, "rev-parse", "-q", "--verify", "HEAD"); err != nil {
		return msg.CloneNoCommits()
	}
	if _, err := hostGit(ctx, dir, "symbolic-ref", "-q", "HEAD"); err != nil {
		return msg.CloneDetached()
	}
	return nil
}

// gitSSH is host git run in dir with ssh reading devvm's ssh config only,
// whose entries check each VM's own host keys.
func (e *Env) gitSSH(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // devvm's own arguments
	cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND=ssh -F "+shellQuote(e.Dirs.SSHConfig()))
	return cmd
}

// seedClone makes the running VM's clone: a new repo at the folder's path
// that takes pushes to its checked-out branch, every branch of the
// host's pushed into it, the host's checked out, and a remote for it in
// the host's repo. Then it offers the ~/.ssh/config line that remote
// needs, and warns about uncommitted changes, which aren't copied. It
// needs the VM's ssh config entry.
func (e *Env) seedClone(ctx context.Context, b backend.Backend, st *state.VM, f project.Folder) error {
	vm, dir := st.Name(), f.Dir
	branch, err := hostGit(ctx, dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return msg.CloneDetached()
	}
	remote, err := freeRemote(ctx, f)
	if err != nil {
		return err
	}
	const initRepo = `sudo mkdir -p -- "$1" && sudo chown -- "$(id -u):$(id -g)" "$1" && cd -- "$1" &&
git init -q && git config receive.denyCurrentBranch updateInstead && git symbolic-ref HEAD "refs/heads/$2"`
	log := ui.LogWriter()
	if err := b.Exec(ctx, vm, []string{"sh", "-c", initRepo, "sh", dir, branch}, backend.ExecOpts{Stdout: log, Stderr: log}); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return backend.Logged(msg.CloneSeedFailed(vm))
	}
	ui.Info(msg.CopyingBranches())
	if err := ui.LoggedCmd(ctx, msg.CopyingBranches(), e.gitSSH(ctx, dir, "push", "-q", cloneURL(vm, dir), "--all")); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return backend.Logged(msg.CloneSeedFailed(vm))
	}
	// Recorded first: rm and an interrupt remove it, if it's the VM's.
	if err := st.Set(KeyCloneRemote, remote); err != nil {
		return err
	}
	if _, ok := remoteURL(ctx, dir, remote); !ok {
		if _, err := hostGit(ctx, dir, "remote", "add", remote, cloneURL(vm, dir)); err != nil {
			return msg.RemoteAddFailed(remote)
		}
	}
	if err := st.Set(KeySeeded, "yes"); err != nil {
		return err
	}
	if ok, err := e.sshConfig().EnsureInclude(e.Home); err != nil {
		return err
	} else if !ok {
		warn(msg.FetchUntil(e.Dirs.SSHConfig(), remote))
	}
	if dirty, _ := hostGit(ctx, dir, "status", "--porcelain", "--untracked-files=no"); dirty != "" {
		warn(msg.UncommittedNotCopied(branch, dir, remote))
	}
	info(msg.CloneReady(branch, remote))
	return nil
}

// freeRemote is the name of the repo's remote for the folder's VM: devvm,
// else devvm-<folder>, else the VM's name, whichever is free or already
// the VM's.
func freeRemote(ctx context.Context, f project.Folder) (string, error) {
	url := cloneURL(f.Name, f.Dir)
	for _, r := range []string{"devvm", project.Prefix + f.Slug(), f.Name} {
		if got, ok := remoteURL(ctx, f.Dir, r); !ok || got == url {
			return r, nil
		}
	}
	return "", msg.RemotesTaken(f.Slug(), f.Name)
}

// remoteURL returns the URL of dir's remote by that name, if it has one.
func remoteURL(ctx context.Context, dir, name string) (string, bool) {
	url, err := hostGit(ctx, dir, "remote", "get-url", name)
	return url, err == nil
}

// cloneRmCheck is rm's check of a --clone VM before deleting it: it offers
// to fetch the branches whose commits the host's repo lacks, and asks
// before uncommitted changes in the clone are lost, starting the VM if
// it's stopped. Without a terminal, either refuses. It reports whether to
// keep the VM. A VM never seeded has nothing of its own.
func (e *Env) cloneRmCheck(ctx context.Context, t *found) (bool, error) {
	vm, st, b, dir := t.f.Name, t.st, t.b, t.f.Dir
	if st.Get(KeySeeded) == "" {
		return false, nil
	}
	remote := st.Get(KeyCloneRemote)
	if err := e.ensureRunning(ctx, t); err != nil {
		return false, err
	}
	refs, err := e.cloneGit(ctx, b, vm, dir, "for-each-ref", "--format=%(objectname) %(refname:short)", "refs/heads")
	if err != nil {
		return false, err
	}
	if unfetched := unfetchedRefs(ctx, dir, refs); len(unfetched) > 0 {
		ui.Say(msg.UnfetchedCommits(strings.Join(unfetched, " ")))
		if !terminal() {
			return false, msg.NoTerminalFetch(remote)
		}
		if confirm(msg.FetchBeforeDelete(remote)) {
			if err := e.sshConfig().Write(ctx); err != nil {
				return false, err
			}
			fetch := e.gitSSH(ctx, dir, "fetch", cloneURL(vm, dir), "+refs/heads/*:refs/remotes/"+remote+"/*")
			if err := ui.LoggedCmd(ctx, msg.Fetching(remote), fetch); err != nil {
				if ctx.Err() != nil {
					return false, ctx.Err()
				}
				return false, backend.Logged(msg.FetchFailed(vm))
			}
		}
	}
	dirty, err := e.cloneGit(ctx, b, vm, dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	if len(bytes.TrimSpace(dirty)) == 0 {
		return false, nil
	}
	ui.Say(msg.CloneDirty())
	var shown strings.Builder
	s := bufio.NewScanner(bytes.NewReader(dirty))
	for n := 0; n < cloneShown && s.Scan(); n++ {
		shown.WriteString(msg.Shown(s.Text()) + "\n")
	}
	_, _ = ui.Stdout().Write([]byte(shown.String()))
	if !terminal() {
		return false, msg.NoTerminalDelete()
	}
	if !ask(msg.DeleteAnyway()) {
		info(msg.Kept(vm))
		return true, nil
	}
	return false, nil
}

// cloneGit runs git on the VM's clone and returns what it printed, at
// most cloneOutMax bytes.
func (e *Env) cloneGit(ctx context.Context, b backend.Backend, vm, dir string, args ...string) ([]byte, error) {
	out := &capped{max: cloneOutMax}
	argv := append([]string{"git", "-C", dir}, args...)
	if err := b.Exec(ctx, vm, argv, backend.ExecOpts{Stdout: out, Stderr: ui.LogWriter()}); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, msg.CloneCheckFailed(vm)
	}
	return out.Bytes(), nil
}

// unfetchedRefs returns, from the VM's for-each-ref lines ("SHA REF"),
// the branches whose commits dir's repo lacks, as they can be shown.
func unfetchedRefs(ctx context.Context, dir string, refs []byte) []string {
	var out []string
	s := bufio.NewScanner(bytes.NewReader(refs))
	for s.Scan() {
		sha, ref, _ := strings.Cut(strings.TrimSpace(s.Text()), " ")
		if sha == "" {
			continue
		}
		if objectName.MatchString(sha) {
			if _, err := hostGit(ctx, dir, "cat-file", "-e", sha+"^{commit}"); err == nil {
				continue
			}
		}
		out = append(out, msg.Shown(ref))
	}
	return out
}

// removeRemote removes the repo's remote for the deleted VM, if it still
// is the VM's. The branches fetched from it stay (git remote remove would
// delete them, rm's fetch included).
func removeRemote(ctx context.Context, dir, vm, remote string) {
	if remote == "" {
		return
	}
	if url, ok := remoteURL(ctx, dir, remote); !ok || url != cloneURL(vm, dir) {
		return
	}
	if _, err := hostGit(ctx, dir, "config", "--remove-section", "remote."+remote); err == nil {
		info(msg.RemovedRemote(remote))
	}
}

// capped keeps the first max bytes written to it, and takes the rest
// without keeping it.
type capped struct {
	bytes.Buffer
	max int
}

func (w *capped) Write(p []byte) (int, error) {
	if room := w.max - w.Len(); room > 0 {
		w.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
