package vm

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/shell"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
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

// historyUnsafe is what historyImport prints when it imports nothing
// because a path in the export is outside the agents' history.
const historyUnsafe = "devvm-history-unsafe"

// importSkips are the paths in an export of agents' history, relative to
// the home folder, that an import leaves out, because devvm's setup
// writes them: the agents' logins (historyExcludes), ~/.claude.json,
// whose copy holds the old VM's placeholders, and every agent's folders
// linked to their mounts and files copied from the host. Sorted.
func importSkips() []string {
	skips := append(slices.Clone(historyExcludes), ".claude.json")
	for _, of := range []map[string][]string{agentDirsOf, agentFilesOf} {
		for _, paths := range of {
			skips = append(skips, paths...)
		}
	}
	slices.Sort(skips)
	return slices.Compact(skips)
}

// historyImport is the script that puts an export of agents' history,
// on its stdin, into the VM's home, run there as the VM's user, so the
// host never reads the export. It saves the export in a folder of its
// own and lists its paths as they are in it (--absolute-names, which
// only the listing takes: tar would list them with their leading / and
// .. parts taken off), each on one line (GNU tar's escape quoting). It
// imports nothing when one is outside historyPaths, absolute or has a
// .. part, printing historyUnsafe instead. Every tar here is GNU tar,
// which Debian and Ubuntu have and base's Alpine branch installs over
// busybox's (TestHistoryImportHasGNUTarOnAlpine). GNU tar unpacks it
// in a folder of its own, without its owners or modes (the user's umask
// applies) and without importSkips (anchored, each with what's below
// it); the symlinks there that point outside it (absolute, or with a ..
// part) are taken out, so an agent's write never follows one out of its
// history; then what's left of historyPaths goes into the home folder,
// without replacing a file or folder the VM has (--skip-old-files,
// which implies --no-overwrite-dir).
func historyImport() string {
	allowed := make([]string, len(historyPaths))
	for i, p := range historyPaths {
		allowed[i] = regexp.QuoteMeta(p)
	}
	own := []string{"--no-same-owner", "--no-same-permissions"}
	unpack := append(slices.Clone(own), "--anchored")
	for _, p := range importSkips() {
		unpack = append(unpack, "--exclude="+p)
	}
	return strings.Join([]string{
		"set -o pipefail",
		`mkdir -p ~/.cache && t=$(mktemp -d ~/.cache/devvm-import.XXXXXX) || exit 1`,
		`trap 'rm -rf "$t"' EXIT`,
		`cat > "$t/export" && tar -tzf "$t/export" --absolute-names --quoting-style=escape > "$t/names" || exit 1`,
		"if grep -vqE " + shell.Quote("^("+strings.Join(allowed, "|")+")(/|$)") + ` "$t/names" || ` +
			"grep -qE " + shell.Quote(`(^|/)\.\.(/|$)`) + ` "$t/names"; then echo ` + historyUnsafe + "; exit 1; fi",
		`mkdir "$t/x" && tar -xzf "$t/export" -C "$t/x" ` + shell.QuoteJoin(unpack) + " || exit 1",
		`find "$t/x" -type l -print0 | while IFS= read -r -d '' l; do`,
		`  case $(readlink "$l") in /* | .. | ../* | */.. | */../*) echo "devvm: left out the link $l" >&2; rm -f "$l" ;; esac`,
		"done",
		`cd "$t/x" && set -- && for p in ` + shell.QuoteJoin(historyPaths) + `; do if [ -e "$p" ] || [ -L "$p" ]; then set -- "$@" "$p"; fi; done`,
		`[ "$#" -eq 0 ] || tar -cf - "$@" | tar --extract --file=- --directory ~ --skip-old-files ` + shell.QuoteJoin(own),
	}, "\n")
}

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

// AgentMounts returns the host folders a VM with these shared tools
// mounts: its agents' folders that exist under the home folder. One with a
// comma in its path is left out on every backend, with a warning: Lima
// splits mount options on commas.
func (e Launch) AgentMounts(tools []string) []string {
	dirs, comma := agentMounts(e.Dirs.Home, tools)
	for _, p := range comma {
		e.Warn(msg.AgentDirComma(p))
	}
	return dirs
}

// agentMounts returns the host folders under home a VM with these shared
// tools mounts (AgentMounts), and those it leaves out for a comma.
func agentMounts(home string, tools []string) (dirs, comma []string) {
	for _, a := range image.AgentsOf(tools) {
		for _, rel := range agentDirs(home, a) {
			p := filepath.Join(home, rel)
			if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
				continue
			}
			if strings.Contains(p, ",") {
				comma = append(comma, p)
				continue
			}
			dirs = append(dirs, p)
		}
	}
	return dirs, comma
}

// updateAgentDirs says how an existing VM's agent folder mounts change to
// match the host's folders, which can appear or go after it's made, from
// in, its instance. Each start of a stopped VM mounts the host's
// (starter.start): a stopped VM's change as its launch starts it, a
// running one's at its next start (later), which bringUp offers to make
// now, with whatever else waits for it, in its one question
// (restartForNextStart).
func (e Launch) updateAgentDirs(st *state.VM, in backend.Instance) (later bool) {
	vm := st.Name()
	want := e.AgentMounts(tools.Names(st))
	added := missing(want, in.AgentDirs)
	removed := missing(in.AgentDirs, want)
	if len(added) == 0 && len(removed) == 0 {
		return false
	}
	if len(added) > 0 {
		e.Warn(msg.AgentDirsAdded(shownList(added), vm))
	}
	if len(removed) > 0 {
		e.Warn(msg.AgentDirsRemoved(shownList(removed), vm))
	}
	if in.Status == backend.StatusRunning {
		return true
	}
	e.Info(msg.MountsUpdated(vm))
	return false
}

func shownList(paths []string) string { return strings.Join(paths, ", ") }

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
