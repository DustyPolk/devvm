package vm

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/hostgit"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/shell"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/vmname"
)

// --clone: the VM works on its own clone of the repo instead of mounting
// the folder, so nothing in the VM can reach the working tree or .git.
// The host pushes its branches into the VM once, and the repo gets a git
// remote to fetch the VM's work from, by the VM's full name in devvm's ssh
// config. The clone is at the folder's path in the VM, so shells and
// editors open in the same place.

// cloneShown is how many of the clone's uncommitted changes rm shows, in
// all its worktrees.
const cloneShown = 20

// cloneOutMax is the most of the VM's answer read for its branches, its
// worktrees or a worktree's changes.
const cloneOutMax = 1 << 20

// objectName matches a git object name: what a VM answers is only looked
// up when it's one.
var objectName = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// clones makes a --clone VM's clone, and checks and removes what the repo
// has of it when the VM goes.
type clones struct{ *Env }

// isClone reports whether the VM has its own clone of the repo.
func isClone(st *state.VM) bool { return sectionOf[cloneInVM](st, cloneSection).On }

// cloneURL is the remote URL of vm's clone of dir: the VM's full name in
// devvm's ssh config, which never changes, and the clone's path.
func cloneURL(vm, dir string) string { return vm + ":" + dir }

// hostGit runs git in dir on the host (hostgit) and returns its output,
// trimmed.
func hostGit(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := hostgit.Output(ctx, append([]string{"-C", dir}, args...)...)
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

// gitSSH is host git (hostgit) run in dir with ssh reading devvm's ssh
// config only, for vm. Whatever host the repo's config rewrites vm's URL
// to (url.*.insteadOf, remote.URL.url), ssh trusts only the host key
// devvm pinned for vm itself (HostKeyAlias), forwards nothing (no agent,
// no ports), and never asks on the terminal (BatchMode), so a VM's sshd
// can't prompt for a password.
func (e clones) gitSSH(ctx context.Context, vm, dir string, args ...string) *exec.Cmd {
	cmd := hostgit.Command(ctx, append([]string{"-C", dir}, args...)...)
	cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -F "+shell.Quote(e.Dirs.SSHConfig())+" -o BatchMode=yes"+
		" -o StrictHostKeyChecking=yes -o UserKnownHostsFile="+shell.Quote(e.Dirs.KnownHosts())+" -o GlobalKnownHostsFile=/dev/null"+
		" -o HostKeyAlias="+shell.Quote(vm)+" -o ForwardAgent=no -o ClearAllForwardings=yes")
	return cmd
}

// seedClone makes the running VM's clone: a new repo at the folder's path
// that takes pushes to its checked-out branch, every branch of the
// host's pushed into it, the host's checked out, and a remote for it in
// the host's repo. Then it offers the ~/.ssh/config line that remote
// needs, and warns about uncommitted changes, which aren't copied. It
// needs the VM's ssh config entry.
func (e clones) seedClone(ctx context.Context, b backend.Backend, st *state.VM, f project.Folder) error {
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
	log := e.LogWriter()
	if err := b.Exec(ctx, vm, []string{"sh", "-c", initRepo, "sh", dir, branch}, backend.ExecOpts{Stdout: log, Stderr: log}); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return backend.Logged(msg.CloneSeedFailed(vm))
	}
	e.Info(msg.CopyingBranches())
	if err := e.LoggedCmd(ctx, msg.CopyingBranches(), e.gitSSH(ctx, vm, dir, "push", "-q", "--no-verify", "--no-recurse-submodules", "--no-signed", cloneURL(vm, dir), "--all")); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return backend.Logged(msg.CloneSeedFailed(vm))
	}
	// Recorded first: rm and an interrupt remove it, if it's the VM's.
	if err := update(ctx, st, cloneSection, func(c *cloneInVM) error { c.Made.Remote = remote; return nil }); err != nil {
		return err
	}
	if _, ok := remoteURL(ctx, dir, remote); !ok {
		if _, err := hostGit(ctx, dir, "remote", "add", remote, cloneURL(vm, dir)); err != nil {
			return msg.RemoteAddFailed(remote)
		}
	}
	if err := update(ctx, st, cloneSection, func(c *cloneInVM) error { c.Made.Seeded = true; return nil }); err != nil {
		return err
	}
	if ok, err := e.sshConfig().EnsureInclude(e.Dirs.Home); err != nil {
		return err
	} else if !ok {
		e.Warn(msg.FetchUntil(e.Dirs.SSHConfig(), remote))
	}
	if dirty, _ := hostgit.Status(ctx, dir, "--porcelain", "--untracked-files=no").Output(); len(bytes.TrimSpace(dirty)) > 0 {
		e.Warn(msg.UncommittedNotCopied(branch, dir, remote))
	}
	e.Info(msg.CloneReady(branch, remote))
	return nil
}

// freeRemote is the name of the repo's remote for the folder's VM: devvm,
// else devvm-<folder>, else the VM's name, whichever is free or already
// the VM's.
func freeRemote(ctx context.Context, f project.Folder) (string, error) {
	url := cloneURL(f.Name, f.Dir)
	for _, r := range []string{"devvm", vmname.Prefix + f.Slug(), f.Name} {
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
// before uncommitted changes in the clone or its worktrees (cloneChanges)
// are lost, starting the VM if it's stopped. Without a terminal, either
// refuses. It reports whether to keep the VM. A VM never seeded has
// nothing of its own.
func (e clones) cloneRmCheck(ctx context.Context, t *found) (bool, error) {
	vm, st, b, dir := t.f.Name, t.st, t.b, t.f.Dir
	c := sectionOf[cloneInVM](st, cloneSection)
	if !c.Made.Seeded {
		return false, nil
	}
	remote := c.Made.Remote
	if err := e.ensureRunning(ctx, t); err != nil {
		return false, e.notStarted(ctx, vm, err)
	}
	refs, err := e.cloneGit(ctx, b, vm, dir, "for-each-ref", "--format=%(objectname) %(refname)", "refs/heads")
	if err != nil {
		return false, err
	}
	if unfetched := unfetchedRefs(ctx, dir, refs); len(unfetched) > 0 {
		e.Say(msg.UnfetchedCommits(quoted(unfetched)))
		if !e.Terminal() {
			return false, msg.NoTerminalFetch(remote)
		}
		if err := e.fetchUnfetched(ctx, vm, dir, remote, unfetched); err != nil {
			return false, err
		}
	}
	if dirty, err := e.cloneChanges(ctx, b, vm, dir); err != nil || !dirty {
		return false, err
	}
	if !e.Terminal() {
		return false, msg.NoTerminalDelete()
	}
	if !e.Ask(msg.DeleteAnyway()) {
		e.Info(msg.Kept(vm))
		return true, nil
	}
	return false, nil
}

// notStarted is rm's error when the VM didn't start for its clone check
// (err): what kept it from starting, said, then the way out, devvm rm
// --no-clone-check, carrying err. An interrupt, or an error that isn't a
// message, is err as it is.
func (e clones) notStarted(ctx context.Context, vm string, err error) error {
	m, ok := errors.AsType[msg.M](err)
	if ctx.Err() != nil || !ok {
		return err
	}
	e.Warn(m)
	out := msg.CloneCheckNotStarted(vm).Because(err)
	if _, logged := errors.AsType[*backend.LoggedError](err); logged {
		return backend.Logged(out)
	}
	return out
}

// cloneChanges shows the uncommitted changes in the VM's clone and in
// its linked worktrees inside it (cloneWorktrees), each under its
// heading, at most cloneShown in all, and reports whether there are any.
// It stops checking once it has shown cloneShown. A clone with more
// worktrees than devvm checks counts as changed, and devvm says so.
func (e clones) cloneChanges(ctx context.Context, b backend.Backend, vm, dir string) (bool, error) {
	list, err := e.cloneGit(ctx, b, vm, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return false, err
	}
	trees, more := cloneWorktrees(dir, list)
	shown, dirty := 0, false
	for _, wt := range append([]string{dir}, trees...) {
		if shown >= cloneShown {
			break
		}
		var out []byte
		if wt == dir {
			out, err = e.cloneGit(ctx, b, vm, dir, "status", "--porcelain")
		} else {
			out, err = e.cloneExec(ctx, b, vm, []string{"sh", "-c", worktreeStatus, "sh", wt})
		}
		if err != nil {
			return false, err
		}
		if len(bytes.TrimSpace(out)) == 0 {
			continue
		}
		dirty = true
		if wt == dir {
			e.Say(msg.CloneDirty())
		} else {
			e.Say(msg.CloneWorktreeDirty(wt))
		}
		shown += e.showChanges(out, cloneShown-shown)
	}
	if more > 0 {
		e.Say(msg.CloneWorktreesUnchecked(strconv.Itoa(cloneWorktreesMax)))
		dirty = true
	}
	return dirty, nil
}

// worktreeStatus is git status in a linked worktree, $1, in the VM: one
// whose folder is gone has nothing to lose.
const worktreeStatus = `[ -d "$1" ] || exit 0; exec git -C "$1" status --porcelain`

// showChanges prints the first n lines of git status's output, each
// through msg.OneLine, and returns how many it printed.
func (e clones) showChanges(status []byte, n int) int {
	var shown strings.Builder
	s := bufio.NewScanner(bytes.NewReader(status))
	i := 0
	for ; i < n && s.Scan(); i++ {
		shown.WriteString(msg.OneLine(s.Text()) + "\n")
	}
	_, _ = e.Stdout.Write([]byte(shown.String()))
	return i
}

// cloneWorktreesMax is how many of the clone's linked worktrees rm checks.
const cloneWorktreesMax = 100

// cloneWorktrees returns, from the VM's git worktree list --porcelain -z
// (records of NUL-ended "key value" lines, each record ended by an empty
// line), the linked worktrees rm checks, at most cloneWorktreesMax, and
// how many more there are. The VM's answer is untrusted: only a clean
// absolute path inside dir is kept, each once; bare and prunable ones are
// skipped.
func cloneWorktrees(dir string, list []byte) (trees []string, more int) {
	seen := map[string]bool{}
	var wt string
	skip := false
	for line := range strings.SplitSeq(string(list), "\x00") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			wt, skip = strings.TrimPrefix(line, "worktree "), false
		case line == "bare" || line == "prunable" || strings.HasPrefix(line, "prunable "):
			skip = true
		case line == "":
			if !skip && insideClone(dir, wt) && !seen[wt] {
				seen[wt] = true
				if len(trees) < cloneWorktreesMax {
					trees = append(trees, wt)
				} else {
					more++
				}
			}
			wt, skip = "", false
		}
	}
	return trees, more
}

// insideClone reports whether p, a path the VM gave, is a clean absolute
// path below dir.
func insideClone(dir, p string) bool {
	return path.IsAbs(p) && path.Clean(p) == p && strings.HasPrefix(p, dir+"/")
}

// cloneGit runs git on the VM's clone and returns what it printed, at
// most cloneOutMax bytes.
func (e clones) cloneGit(ctx context.Context, b backend.Backend, vm, dir string, args ...string) ([]byte, error) {
	return e.cloneExec(ctx, b, vm, append([]string{"git", "-C", dir}, args...))
}

// cloneExec runs argv in the VM and returns what it printed, at most
// cloneOutMax bytes.
func (e clones) cloneExec(ctx context.Context, b backend.Backend, vm string, argv []string) ([]byte, error) {
	out := &capped{max: cloneOutMax}
	if err := b.Exec(ctx, vm, argv, backend.ExecOpts{Stdout: out, Stderr: e.LogWriter()}); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, msg.CloneCheckFailed(vm)
	}
	return out.Bytes(), nil
}

// unfetchedRefs returns, from the VM's for-each-ref lines ("SHA
// refs/heads/BRANCH"), the branches with commits no ref in dir's repo
// has, as the VM named them: untrusted, so checked (validBranch) before
// use and quoted when shown.
func unfetchedRefs(ctx context.Context, dir string, refs []byte) []string {
	var out []string
	s := bufio.NewScanner(bytes.NewReader(refs))
	for s.Scan() {
		sha, ref, _ := strings.Cut(strings.TrimSpace(s.Text()), " ")
		if sha == "" {
			continue
		}
		if objectName.MatchString(sha) {
			if got, err := hostGit(ctx, dir, "rev-list", "-n1", sha, "--not", "--all"); err == nil && got == "" {
				continue
			}
		}
		out = append(out, strings.TrimPrefix(ref, "refs/heads/"))
	}
	return out
}

// fetchUnfetched offers to fetch the VM's unfetched branches into new
// local branches, saved/<remote>/<branch> (savedPrefix), never over one
// that exists. Branches whose names git refuses are skipped.
func (e clones) fetchUnfetched(ctx context.Context, vm, dir, remote string, unfetched []string) error {
	var good, bad []string
	for _, br := range unfetched {
		if validBranch(ctx, dir, br) {
			good = append(good, br)
		} else {
			bad = append(bad, br)
		}
	}
	if len(bad) > 0 {
		e.Warn(msg.BranchNamesInvalid(quoted(bad)))
	}
	if len(good) == 0 {
		return nil
	}
	prefix, ok := savedPrefix(ctx, dir, remote)
	if !ok {
		return msg.SavedNamesTaken(remote, vm)
	}
	if !e.Confirm(msg.FetchBeforeDelete(prefix)) {
		return nil
	}
	if err := e.sshConfig().Write(ctx); err != nil {
		return err
	}
	args := []string{"fetch", "--no-tags", "--no-recurse-submodules", cloneURL(vm, dir)}
	saved := make([]string, len(good))
	for i, br := range good {
		saved[i] = prefix + "/" + br
		args = append(args, "refs/heads/"+br+":refs/heads/"+saved[i])
	}
	if err := e.LoggedCmd(ctx, msg.Fetching(prefix), e.gitSSH(ctx, vm, dir, args...)); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return backend.Logged(msg.FetchFailed(vm))
	}
	e.Info(msg.SavedBranches(quoted(saved)))
	return nil
}

// validBranch reports whether git takes name as a branch name.
func validBranch(ctx context.Context, dir, name string) bool {
	_, err := hostGit(ctx, dir, "check-ref-format", "refs/heads/"+name)
	return err == nil
}

// savedPrefixMax is how many names savedPrefix tries.
const savedPrefixMax = 99

// savedPrefix is where a deleted VM's branches are kept, as local
// branches: saved/<remote>, else saved/<remote>-2 and on to -99, the first
// that no local branch is, is in or holds. A remote devvm picks starts
// with devvm, so these never look like a later VM's remote-tracking
// branches.
func savedPrefix(ctx context.Context, dir, remote string) (string, bool) {
	out, err := hostGit(ctx, dir, "for-each-ref", "--format=%(refname)", "refs/heads")
	if err != nil {
		return "", false
	}
	heads := strings.Fields(out)
	for n := 1; n <= savedPrefixMax; n++ {
		p := "refs/heads/saved/" + remote
		if n > 1 {
			p += "-" + strconv.Itoa(n)
		}
		free := true
		for _, h := range heads {
			if h == p || strings.HasPrefix(h, p+"/") || strings.HasPrefix(p, h+"/") {
				free = false
				break
			}
		}
		if free {
			return strings.TrimPrefix(p, "refs/heads/"), true
		}
	}
	return "", false
}

// quoted is names, each quoted, space-separated.
func quoted(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = strconv.Quote(n)
	}
	return strings.Join(q, " ")
}

// removeRemote removes the repo's remote for the deleted VM, if it still
// is the VM's, with its remote-tracking branches and upstream settings.
// Fetched branches holding commits no other ref has are kept as local
// branches first (saveFetched); if that fails, the remote stays.
func (e clones) removeRemote(ctx context.Context, dir, vm, remote string) {
	if remote == "" {
		return
	}
	if url, ok := remoteURL(ctx, dir, remote); !ok || url != cloneURL(vm, dir) {
		return
	}
	if err := e.saveFetched(ctx, dir, remote); err != nil {
		e.Warn(msg.RemoteKept(remote))
		return
	}
	if _, err := hostGit(ctx, dir, "remote", "remove", remote); err == nil {
		e.Info(msg.RemovedRemote(remote))
	}
}

// saveFetched makes a new local branch, under savedPrefix, of each of
// remote's remote-tracking branches with commits no other ref has, all at
// once and never over a branch that exists.
func (e clones) saveFetched(ctx context.Context, dir, remote string) error {
	tracking := "refs/remotes/" + remote + "/"
	out, err := hostGit(ctx, dir, "for-each-ref", "--format=%(objectname) %(refname)", tracking)
	if err != nil {
		return err
	}
	var input strings.Builder
	var saved []string
	prefix := ""
	for line := range strings.Lines(out) {
		sha, ref, _ := strings.Cut(strings.TrimSpace(line), " ")
		br := strings.TrimPrefix(ref, tracking)
		if sha == "" || br == "HEAD" || !validBranch(ctx, dir, br) {
			continue
		}
		if only, err := hostGit(ctx, dir, "rev-list", "-n1", sha, "--not", "--exclude="+tracking+"*", "--all"); err != nil || only == "" {
			continue
		}
		if prefix == "" {
			var ok bool
			if prefix, ok = savedPrefix(ctx, dir, remote); !ok {
				return errors.New("no free saved/ branch names")
			}
		}
		saved = append(saved, prefix+"/"+br)
		input.WriteString("create refs/heads/" + prefix + "/" + br + " " + sha + "\n")
	}
	if len(saved) == 0 {
		return nil
	}
	cmd := hostgit.Command(ctx, "-C", dir, "update-ref", "--stdin")
	cmd.Stdin = strings.NewReader(input.String())
	if err := cmd.Run(); err != nil {
		return err
	}
	e.Info(msg.SavedBranches(quoted(saved)))
	return nil
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
