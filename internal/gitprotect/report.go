package gitprotect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"

	"github.com/devvmhq/devvm/internal/msg"
)

// why is what protect runs for.
type why int

const (
	whyNew   why = iota // a VM just made
	whyOn               // devvm git-protect on or strict
	whyCheck            // a launch of an existing VM
)

// how protect treats the locks.
type how int

const (
	lockNow  how = iota // a stopped VM's: lock
	later               // a running VM's: report, and lock at its next stop
	watching            // a host without locks: report, record .git's identity, lock nothing
)

// run is one protect: what it runs for, how it treats the locks, what
// the last one recorded, what the scan found, and the sum of its
// delegations.
type run struct {
	why why
	how how
	was Made
	s   snapshot
	sum string
}

// report is something protect tells: when says whether a run tells it,
// and report tells it.
type report struct {
	when   func(r *run) bool
	report func(p *Protector, r *run)
}

// reports are what protect tells, in order, once .git is there and isn't
// one that replaced the one locked. A new VM's git dirs are recorded, not
// reported; a check reports what changed since the last one, and the
// hooks path and delegations only when they change. Watching, nothing is
// locked, so the hooks in folders a lock would cover aren't news: the
// watch names their changes.
var reports = []report{
	{ // commondir points git elsewhere for config and hooks, on every run
		when:   func(r *run) bool { return r.s.commondir },
		report: func(p *Protector, r *run) { p.Warn(msg.GitCommondir(r.s.git, r.s.target)) },
	},
	{ // linked worktrees that read config and hooks from elsewhere, on every run
		when: func(*run) bool { return true },
		report: func(p *Protector, r *run) {
			for _, s := range r.s.strays {
				p.Warn(msg.GitWorktreeStray(s))
			}
		},
	},
	{ // a repo made since the VM was
		when:   func(r *run) bool { return r.why == whyCheck && r.was.GitID == "" },
		report: func(p *Protector, _ *run) { p.Warn(msg.GitAppeared()) },
	},
	{ // git dirs that appeared since the last time, once there was one
		when: func(r *run) bool { return r.why != whyNew && r.was.Protected },
		report: func(p *Protector, r *run) {
			seen := p.abs(r.was.GitDirs)
			for _, d := range r.s.dirs {
				if !slices.Contains(seen, d) {
					p.Warn(msg.GitEmbeddedNew(d))
				}
			}
		},
	},
	{ // hooks in a hooks folder that isn't locked appeared since the last
		// lock (there was no folder to lock, or it's a new submodule's), so
		// the VM may have made them
		when:   func(r *run) bool { return r.why == whyCheck && r.how != watching },
		report: (*Protector).warnHooksUnlocked,
	},
	{ // core.hooksPath into the project folder, when it's set up
		when:   func(r *run) bool { return r.why != whyCheck && r.s.hooksPath != "" },
		report: func(p *Protector, r *run) { p.Warn(msg.GitHooksPath(r.s.hooksPath)) },
	},
	{ // what the locked files hand over, when it's set up or changed
		when: func(r *run) bool { return r.why != whyCheck || r.sum != r.was.Delegations },
		report: func(p *Protector, r *run) {
			for _, d := range r.s.ds {
				p.Warn(d.message())
			}
		},
	},
}

// protect locks .git, as h says, and reports what a lock can't stop
// (reports); it returns what it found (g's Made, as it is now), for the
// caller to record with the mode it set. Before anything else, a check
// whose .git replaced the one it locked reports that alone.
func (p *Protector) protect(ctx context.Context, s snapshot, rec InVM, w why, h how) Made {
	was := rec.Made
	if !s.exists {
		return Made{Protected: true, GitDirs: was.GitDirs, Delegations: was.Delegations}
	}
	if w == whyCheck && was.GitID != "" && was.GitID != s.id {
		p.Warn(msg.GitReplaced(s.git))
		if h == watching {
			// Nothing to lock again: the next check compares with this one.
			was.GitID = s.id
		}
		return was
	}
	r := &run{why: w, how: h, was: was, s: s, sum: delegationsSum(s.ds)}
	for _, rep := range reports {
		if rep.when(r) {
			rep.report(p, r)
		}
	}
	made := Made{Protected: true, GitID: was.GitID, GitDirs: p.rel(s.dirs), Delegations: r.sum}
	switch h {
	case lockNow:
		p.lock(ctx, s)
		made.GitID = s.id
	case watching:
		made.GitID = s.id
	case later:
	}
	return made
}

// warnHooksUnlocked names the hooks folders a lock covers that aren't
// locked and hold hooks.
func (p *Protector) warnHooksUnlocked(r *run) {
	for _, path := range r.s.paths {
		if isHooks(path) && !p.Locker.Locked(path) && hasHooks(path) {
			p.Warn(msg.GitHooksUnlocked(path))
		}
	}
}

// delegationKind is what a delegation hands over.
type delegationKind int

const (
	includeInside   delegationKind = iota // an include into the project folder
	hookLinkInside                        // a hook that's a symlink into it
	gitdirOutside                         // a submodule's git dir outside it
	gitdirNotGit                          // a folder a submodule names as its git dir that isn't one
	delegationKinds                       // how many kinds there are
)

// delegation is something the locked files hand over to a file the VM can
// edit, a git dir devvm can't lock, or a folder named as one that isn't.
type delegation struct {
	kind   delegationKind
	where  string // the config, the hook, or the submodule's folder
	target string // the include's path or the hook's link as written, or the git dir
	path   string // what an include leads to, absolute, for the watch
}

// message is the warning that names d.
func (d delegation) message() msg.M {
	switch d.kind {
	case includeInside:
		return msg.GitIncludeInside(d.where, d.target)
	case hookLinkInside:
		return msg.GitHookLinkInside(d.where, d.target)
	case gitdirOutside:
		return msg.GitGitdirOutside(d.target, d.where)
	case gitdirNotGit:
		return msg.GitGitdirNotGit(d.target, d.where)
	case delegationKinds:
	}
	return msg.M{}
}

// name is the kind as delegationsSum has it, which Made records: the
// names it had as strings.
func (k delegationKind) name() string {
	switch k {
	case includeInside:
		return "include"
	case hookLinkInside:
		return "hook"
	case gitdirOutside:
		return "outside"
	case gitdirNotGit:
		return "notgit"
	case delegationKinds:
	}
	return ""
}

func delegationsSum(ds []delegation) string {
	h := sha256.New()
	for _, d := range ds {
		for _, f := range []string{d.kind.name(), d.where, d.target} {
			h.Write([]byte(strconv.Itoa(len(f)) + ":" + f))
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
