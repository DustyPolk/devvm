package vm

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// What each agent shares from the host, as paths relative to the home
// folder. Folders are mounted read-only at their host path and linked into
// the VM's home; files are copied in on every launch. Host history
// (transcripts, sessions, databases) is never shared.
var (
	agentDirsOf = map[string][]string{
		"claude": {".claude/skills", ".claude/agents", ".claude/commands", ".claude/output-styles", ".claude/plugins"},
		"codex":  {".codex/prompts", ".codex/skills"},
	}
	agentFilesOf = map[string][]string{
		"claude":   {".claude/settings.json", ".claude/CLAUDE.md"},
		"codex":    {".codex/config.toml", ".codex/AGENTS.md", ".codex/auth.json"},
		"opencode": {".local/share/opencode/auth.json"},
	}
)

// The agents' history in a VM, relative to its home: what devvm rm
// exports, without their logins.
var (
	historyPaths    = []string{".claude", ".claude.json", ".codex", ".local/share/opencode", ".local/state/opencode"}
	historyExcludes = []string{
		".claude/.credentials.json", ".codex/auth.json",
		".local/share/opencode/auth.json", ".local/share/opencode/account.json", ".local/share/opencode/bin",
	}
)

// opencodeDir is opencode's config folder, whose subfolders are mounted
// and whose files are copied.
const opencodeDir = ".config/opencode"

// agentDirs returns the folders an agent shares, relative to home. For
// opencode, its config folder's subfolders as they are now.
func agentDirs(home, agent string) []string {
	if agent != "opencode" {
		return agentDirsOf[agent]
	}
	var dirs []string
	for _, e := range readDir(filepath.Join(home, opencodeDir)) {
		if e.IsDir() && e.Name() != "node_modules" {
			dirs = append(dirs, opencodeDir+"/"+e.Name())
		}
	}
	return dirs
}

// agentFiles returns the files an agent shares, relative to home. For
// opencode, its config folder's files too, but lock files.
func agentFiles(home, agent string) []string {
	files := slices.Clone(agentFilesOf[agent])
	if agent != "opencode" {
		return files
	}
	for _, e := range readDir(filepath.Join(home, opencodeDir)) {
		n := e.Name()
		if e.Type().IsRegular() && !strings.HasSuffix(n, ".lock") && n != "package-lock.json" && n != ".gitignore" && n != ".DS_Store" {
			files = append(files, opencodeDir+"/"+n)
		}
	}
	return files
}

// readDir returns a folder's entries, but those whose names have a
// control character: the VM's setup takes paths one per line.
func readDir(dir string) []os.DirEntry {
	es, _ := os.ReadDir(dir)
	return slices.DeleteFunc(es, func(e os.DirEntry) bool { return strings.IndexFunc(e.Name(), unicode.IsControl) >= 0 })
}

// agentsOf returns the agents an image name holds, in its order.
func agentsOf(img string) []string {
	var as []string
	for p := range strings.SplitSeq(img, "+") {
		if image.IsAgent(p) {
			as = append(as, p)
		}
	}
	return as
}

// AgentMounts returns the host folders a VM with that image mounts: its
// agents' folders that exist under the home folder. One with a comma in
// its path is left out, with a warning: Lima splits mount options on
// commas.
func (e *Env) AgentMounts(img string) []string {
	var dirs []string
	for _, a := range agentsOf(img) {
		for _, rel := range agentDirs(e.Home, a) {
			p := filepath.Join(e.Home, rel)
			if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
				continue
			}
			if strings.Contains(p, ",") {
				e.Warn(msg.AgentDirComma(msg.Shown(p)))
				continue
			}
			dirs = append(dirs, p)
		}
	}
	return dirs
}

// UpdateAgentDirs makes an existing VM's agent folder mounts match the
// host's folders, which can appear or go after it's made. A backend
// changes mounts only while a VM is stopped: a stopped VM is changed at
// once, a running one after the user agrees to a restart (devvm stops it,
// and the caller starts it again), else at its next start.
func (e *Env) UpdateAgentDirs(ctx context.Context, b backend.Backend, st *state.VM) error {
	vm := st.Name()
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	want := e.AgentMounts(st.Get(state.KeyImage))
	added := missing(want, in.AgentDirs)
	removed := missing(in.AgentDirs, want)
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	if len(added) > 0 {
		e.Warn(msg.AgentDirsAdded(shownList(added), vm))
	}
	if len(removed) > 0 {
		e.Warn(msg.AgentDirsRemoved(shownList(removed), vm))
	}
	if in.Status == backend.StatusRunning {
		if !e.Terminal() || !e.Confirm(msg.UpdateMounts(vm)) {
			e.Warn(msg.MountsNextStart(vm))
			return nil
		}
		if err := b.Stop(ctx, vm); err != nil {
			return err
		}
	}
	if err := b.SetAgentDirs(ctx, vm, want); err != nil {
		return err
	}
	e.Info(msg.MountsUpdated(vm))
	return nil
}

func shownList(paths []string) string {
	shown := make([]string, len(paths))
	for i, p := range paths {
		shown[i] = msg.Shown(p)
	}
	return strings.Join(shown, ", ")
}

// missing returns what of a isn't in b.
func missing(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}
