//go:build e2e

package e2e

import (
	"errors"
	"path/filepath"
	"runtime"
)

// projectNestedRepos: a folder of two git repos, without one of its own,
// gets a VM whose .git protection covers both: on macOS and on a Linux
// host that pins, root in the VM can't change either repo's hooks or
// config; on a Linux host that can't pin, devvm stop names the config the
// VM changed. devvm rm takes the locks off both.
func projectNestedRepos(s *S) {
	s.Section("the git repos nested in a VM's folder")
	ws, err := s.NewProject("nested-repos")
	if err != nil {
		s.Check("a folder of repos", func() error { return err })
		return
	}
	for _, r := range []string{"repo-a", "repo-b"} {
		_, _ = s.Run(Cmd{Args: []string{"git", "init", "-q", filepath.Join(ws, r)}})
		_, _ = s.Run(Cmd{Args: []string{"git", "-C", filepath.Join(ws, r), "-c", "user.email=e@e", "-c", "user.name=e", "commit", "-q", "--allow-empty", "-m", "x"}})
	}
	vm, _, _ := s.Launch(ws, "exit", "--network", "open")
	pinned := s.Paths.State(vm)["git.mode"] == "on"
	if runtime.GOOS == "darwin" || pinned {
		nestedReposLocked(s, ws, vm)
	} else {
		nestedReposWatched(s, ws, vm)
	}
	nestedReposPlanted(s, ws, vm, runtime.GOOS == "darwin" || pinned)
	s.Check("devvm rm takes the locks off both repos", func() error {
		if _, err := s.Devvm(ws, "rm", "--force", "--no-export"); err != nil {
			return err
		}
		for _, r := range []string{"repo-a", "repo-b"} {
			if Locked(s, filepath.Join(ws, r, ".git", "config")) {
				return errors.New(r + "'s .git is still locked")
			}
		}
		return nil
	})
}

// nestedReposLocked: root in the VM can't add a hook or change the config
// of either nested repo.
func nestedReposLocked(s *S, ws, vm string) {
	for _, r := range []string{"repo-a", "repo-b"} {
		for _, c := range []struct{ what, script string }{
			{"add a hook", "echo x > .git/hooks/pre-commit"},
			{"change the config", "echo '[core]' >> .git/config && echo '  fsmonitor = ./evil' >> .git/config"},
		} {
			s.Check("root in the VM can't "+c.what+" in "+r, func() error {
				dir := filepath.Join(ws, r)
				if _, err := s.Limactl("shell", "--workdir", "/", vm, "sudo", "sh", "-c", "cd "+dir+" && "+c.script); err == nil {
					return errors.New("root could " + c.what)
				}
				if g, _ := s.Run(Cmd{Args: []string{"git", "config", "--file", ".git/config", "core.fsmonitor"}, Dir: dir}); Trim(g.Stdout) != "" {
					return errors.New("the config changed: " + g.Stdout)
				}
				if Exists(filepath.Join(dir, ".git", "hooks", "pre-commit")) {
					return errors.New("a hook appeared")
				}
				return nil
			})
		}
	}
}

// nestedReposPlanted: what the VM plants to get git on this machine to
// run its fsmonitor is named, or kept protected, at devvm stop: a repo
// whose .git file names a git folder it made inside a protected repo's
// .git; a protected repo whose objects and refs it renamed, with a
// commondir to that git folder; and on macOS, a .git it renamed .GIT.
// Where the host locks, devvm stop also names the repo as one it doesn't
// protect; elsewhere each launch does.
func nestedReposPlanted(s *S, ws, vm string, locks bool) {
	running := func() error {
		if _, err := DevvmIn(s, ws, "exit\n", nil); err != nil || s.Status(vm) != "Running" {
			return errors.New("it didn't start")
		}
		return nil
	}
	s.Check("a repo whose .git names a git folder the VM made in a repo's .git is named at devvm stop", func() error {
		if err := running(); err != nil {
			return err
		}
		if _, err := s.InVM(vm, "cd "+ws+` && mkdir -p repo-a/.git/evil/objects repo-a/.git/evil/refs src &&
      printf 'ref: refs/heads/main\n' > repo-a/.git/evil/HEAD &&
      printf '[core]\n\tfsmonitor = ./evil\n' > repo-a/.git/evil/config &&
      echo 'gitdir: ../repo-a/.git/evil' > src/.git`); err != nil {
			return err
		}
		r, err := s.Devvm(ws, "stop")
		if err != nil {
			return err
		}
		if !grepF(r.Out, FirstLine(s.Msg("git_nested_appeared_runs", "repo=src", "runs=core.fsmonitor", "vm="+vm))) ||
			locks && !grepF(r.Out, FirstLine(s.Msg("git_nested_unprotected", "repos=src", "vm="+vm))) {
			return errors.New("devvm stop didn't name src:\n" + r.Out)
		}
		return nil
	})
	s.Check("a protected repo whose objects and refs the VM renamed, with a commondir, stays protected", func() error {
		if err := running(); err != nil {
			return err
		}
		if _, err := s.InVM(vm, "cd "+ws+"/repo-b/.git && mv objects objects-old && mv refs refs-old && echo ../../repo-a/.git/evil > commondir"); err != nil {
			return err
		}
		r, err := s.Devvm(ws, "stop")
		if err != nil {
			return err
		}
		if grepF(r.Out, FirstLine(s.Msg("git_nested_gone", "repo=repo-b", "vm="+vm))) || s.Paths.State(vm)["git.made.repos.repo-b.git_id"] == "" {
			return errors.New("repo-b went:\n" + r.Out)
		}
		return nil
	})
	if runtime.GOOS != "darwin" {
		return
	}
	s.Check("on macOS, a .git the VM renamed .GIT stays protected", func() error {
		if err := running(); err != nil {
			return err
		}
		// Through another name: the guest finds .GIT already there, the
		// same folder on a file system that folds case, and mv won't
		// rename a file to itself.
		if _, err := s.InVM(vm, "cd "+ws+"/repo-a && mv .git .git-case && mv .git-case .GIT"); err != nil {
			return err
		}
		r, err := s.Devvm(ws, "stop")
		if err != nil {
			return err
		}
		if grepF(r.Out, FirstLine(s.Msg("git_nested_gone", "repo=repo-a", "vm="+vm))) || s.Paths.State(vm)["git.made.repos.repo-a.git_id"] == "" ||
			!Locked(s, filepath.Join(ws, "repo-a", ".GIT", "config")) {
			return errors.New("repo-a went, or isn't locked:\n" + r.Out)
		}
		return nil
	})
}

// nestedReposWatched: on a Linux host that can't pin, the VM changes a
// nested repo's config, and devvm stop names it.
func nestedReposWatched(s *S, ws, vm string) {
	s.Check("on a Linux host that can't pin, devvm stop names a nested repo's config the VM changed", func() error {
		if _, err := s.InVM(vm, "printf '[e2e]\\n\\twatched = vm\\n' >> "+ws+"/repo-a/.git/config"); err != nil {
			return err
		}
		r, err := s.Devvm(ws, "stop")
		if err != nil {
			return err
		}
		if !grepF(r.Out, s.Msg("git_changed", "paths=repo-a/.git/config", "vm="+vm)) {
			return errors.New("devvm stop didn't name repo-a/.git/config:\n" + r.Out)
		}
		return nil
	})
}
