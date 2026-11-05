package vm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// printed makes e's warnings and infos a list of their names, and its
// questions' answer yes (no terminal when tty is false).
func printed(t *testing.T, e *Env, tty bool) *[]string {
	t.Helper()
	var got []string
	if e.Reporter == nil {
		e.Reporter = testUI(t)
	}
	f := fake(e)
	f.Terminal = tty
	e.CanAsk = func() bool { return tty }
	f.OnWarn = func(m msg.M) { got = append(got, m.Names()) }
	f.OnInfo = f.OnWarn
	f.OnConfirm = func(m msg.M) bool { got = append(got, "asked "+m.Names()); return true }
	return &got
}

func mkdirs(t *testing.T, home string, rels ...string) {
	t.Helper()
	for _, r := range rels {
		if err := os.MkdirAll(filepath.Join(home, r), 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

// A VM mounts its agents' folders that exist on the host, one per entry,
// names with spaces kept whole; opencode's are its config folder's
// subfolders but node_modules. A path with a comma is left out, with a
// warning.
func TestAgentMounts(t *testing.T) {
	home := t.TempDir()
	e := &Env{Dirs: paths.Dirs{Home: home}}
	got := printed(t, e, false)
	mkdirs(t, home, ".config/opencode/my agents", ".config/opencode/node_modules", ".config/opencode/a\nb", ".claude/skills", ".codex/prompts", ".claude/agents")
	if err := os.WriteFile(filepath.Join(home, ".claude", "commands"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(home, ".claude/skills"), filepath.Join(home, ".claude/agents"), filepath.Join(home, ".config/opencode/my agents"),
	}
	if ms := (Launch{e}).AgentMounts([]string{"node", "claude", "opencode"}); !slices.Equal(ms, want) {
		t.Errorf("AgentMounts = %q, want %q", ms, want)
	}
	if ms := (Launch{e}).AgentMounts([]string{"node"}); ms != nil {
		t.Errorf("AgentMounts without agents = %q", ms)
	}
	comma := filepath.Join(t.TempDir(), "a,b")
	mkdirs(t, comma, ".codex/prompts")
	e.Dirs.Home = comma
	if ms := (Launch{e}).AgentMounts([]string{"codex"}); ms != nil {
		t.Errorf("AgentMounts with a comma = %q", ms)
	}
	if !slices.Equal(*got, []string{"agent_dir_comma path=" + filepath.Join(comma, ".codex/prompts")}) {
		t.Errorf("printed %q", *got)
	}
}

// agentVM is a VM with these shared tools (space-separated) on a fake
// backend, its agent mounts and status as given.
func agentVM(t *testing.T, tools string, status backend.Status, dirs ...string) (*backendtest.Fake, *state.VM) {
	t.Helper()
	root := t.TempDir()
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config")}
	st := vmState(t, d, "devvm-p-00000000000000", withTools(strings.Fields(tools)...))
	f := backendtest.New("fake", allNative())
	f.Instances[st.Name()] = backend.Instance{Name: st.Name(), Status: status, AgentDirs: dirs}
	return f, st
}

func TestUpdateAgentDirs(t *testing.T) {
	home := t.TempDir()
	skills, mine, commands := filepath.Join(home, ".claude/skills"), filepath.Join(home, ".claude/agents"), filepath.Join(home, ".claude/commands")
	mkdirs(t, home, ".claude/skills")
	vm := "devvm-p-00000000000000"
	for _, tc := range []struct {
		name    string
		tools   string
		status  backend.Status
		have    []string
		methods []string
		printed []string
		// later: it changes at its next start, which bringUp offers to
		// make now, and says.
		later bool
	}{
		{
			name: "nothing changed, nothing edited", tools: "node claude", status: backend.StatusStopped,
			have: []string{skills}, methods: []string{"Inspect"},
		},
		{
			name: "a stopped VM gets new folders and loses gone ones", tools: "node claude", status: backend.StatusStopped,
			have: []string{skills, commands}, methods: []string{"Inspect"},
			printed: []string{"agent_dirs_added dirs=" + mine + " vm=" + vm, "agent_dirs_removed dirs=" + commands + " vm=" + vm, "mounts_updated vm=" + vm},
		},
		{
			name: "a running VM changes at its next start, without a question", tools: "node claude", status: backend.StatusRunning,
			have: []string{skills}, methods: []string{"Inspect"}, printed: []string{"agent_dirs_added dirs=" + mine + " vm=" + vm}, later: true,
		},
		{
			name: "a VM without agents drops leftover agent mounts", tools: "node", status: backend.StatusStopped,
			have: []string{filepath.Join(home, ".codex/prompts")}, methods: []string{"Inspect"},
			printed: []string{"agent_dirs_removed dirs=" + filepath.Join(home, ".codex/prompts") + " vm=" + vm, "mounts_updated vm=" + vm},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &Env{Dirs: paths.Dirs{Home: home}}
			got := printed(t, e, true)
			if tc.name != "nothing changed, nothing edited" {
				mkdirs(t, home, ".claude/agents")
			}
			f, st := agentVM(t, tc.tools, tc.status, tc.have...)
			in, err := f.Inspect(t.Context(), st.Name())
			if err != nil {
				t.Fatal(err)
			}
			if later := (Launch{e}).updateAgentDirs(st, in); later != tc.later {
				t.Errorf("later %v", later)
			}
			if ms := f.Methods(); !slices.Equal(ms, tc.methods) {
				t.Errorf("calls = %q, want %q", ms, tc.methods)
			}
			if !slices.Equal(*got, tc.printed) {
				t.Errorf("printed %q, want %q", *got, tc.printed)
			}
			_ = os.Remove(mine)
		})
	}
}

// An import leaves out the agents' logins, ~/.claude.json and what
// devvm's setup writes, and the VM's tar is told to check the paths
// first, keep no owner or mode, and never take absolute paths.
func TestHistoryImportScript(t *testing.T) {
	skips := importSkips()
	for _, p := range []string{
		".claude/.credentials.json", ".codex/auth.json", ".local/share/opencode/auth.json", ".local/share/opencode/account.json",
		".claude.json", ".claude/settings.json", ".claude/CLAUDE.md", ".claude/skills", ".codex/config.toml", ".codex/prompts",
	} {
		if !slices.Contains(skips, p) {
			t.Errorf("%s isn't left out", p)
		}
	}
	if !slices.IsSorted(skips) || len(slices.Compact(slices.Clone(skips))) != len(skips) {
		t.Errorf("skips %q aren't sorted once each", skips)
	}
	s := historyImport()
	for _, want := range []string{
		`tar -tzf "$t/export" --absolute-names --quoting-style=escape`, "grep -vqE '^(\\.claude|\\.claude\\.json|\\.codex|",
		`grep -qE '(^|/)\.\.(/|$)'`, "echo " + historyUnsafe + "; exit 1",
		`tar -xzf "$t/export" -C "$t/x" '--no-same-owner' '--no-same-permissions' '--anchored' '--exclude=`,
		"'--exclude=.claude.json' ", `find "$t/x" -type l`, "/* | .. | ../* | */.. | */../*) ",
		"for p in '.claude' '.claude.json' '.codex' '.local/share/opencode' '.local/state/opencode'; do",
		"tar --extract --file=- --directory ~ --skip-old-files '--no-same-owner' '--no-same-permissions'",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %q:\n%s", want, s)
		}
	}
	// Only the listing takes the paths as they are; the unpacking never.
	unpacks := 0
	for line := range strings.SplitSeq(s, "\n") {
		if !strings.Contains(line, "tar -xzf") && !strings.Contains(line, "tar --extract") {
			continue
		}
		unpacks++
		for _, bad := range []string{" -P", "--absolute-names", "--same-owner", "--preserve-permissions", "--overwrite"} {
			if strings.Contains(line, bad) {
				t.Errorf("tar gets %q: %s", bad, line)
			}
		}
	}
	if unpacks != 2 || strings.Count(s, "--absolute-names") != 1 {
		t.Errorf("%d unpacks, --absolute-names beyond the listing:\n%s", unpacks, s)
	}
}

// The import needs GNU tar, which Debian and Ubuntu always have, and
// base's Alpine branch installs over busybox's: its apk add has tar.
func TestHistoryImportHasGNUTarOnAlpine(t *testing.T) {
	b, err := fs.ReadFile(assets.FS, "layers/base.sh")
	if err != nil {
		t.Fatal(err)
	}
	_, musl, ok := strings.Cut(string(b), "if [[ $devvm_libc == musl ]]; then")
	musl, _, _ = strings.Cut(musl, "\nelse\n")
	add := regexp.MustCompile(`(?m)^\s*sudo apk add [^\n]*(\\\n[^\n]*)*`).FindString(musl)
	if !ok || !slices.Contains(strings.Fields(add), "tar") {
		t.Errorf("base.sh's Alpine branch doesn't install GNU tar: %q", add)
	}
}

// exportOf is a gzipped tarball of these entries: a name ending in / is
// a folder, one with "->" a symlink, else a file whose content is its name.
func exportOf(t *testing.T, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	tw := tar.NewWriter(zw)
	for _, n := range names {
		h := &tar.Header{Name: n, Mode: 0o4755, ModTime: time.Unix(1e9, 0), Typeflag: tar.TypeReg, Size: int64(len(n))}
		if strings.HasSuffix(n, "/") {
			h.Typeflag, h.Size = tar.TypeDir, 0
		} else if name, target, ok := strings.Cut(n, "->"); ok {
			h.Name, h.Linkname, h.Typeflag, h.Size = name, target, tar.TypeSymlink, 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(n)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// The import script, run by GNU tar as the VM's does (skipped where
// there's none): a tarball with a path outside the agents' history,
// absolute or with .., imports nothing; the rest leave out importSkips
// and keep the files the VM has.
func TestHistoryImportRuns(t *testing.T) {
	if out, err := exec.CommandContext(t.Context(), "tar", "--version").Output(); err != nil || !strings.Contains(string(out), "GNU tar") {
		t.Skip("no GNU tar")
	}
	run := func(t *testing.T, home string, export []byte) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "bash", "-c", historyImport()) //nolint:gosec // devvm's own import script
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
		var stderr bytes.Buffer
		cmd.Stdin, cmd.Stderr = bytes.NewReader(export), &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Logf("tar's stderr: %s", stderr.String())
		}
		return string(out), err
	}
	for _, names := range [][]string{
		{".claude/a", "/etc/devvm-import-test"},
		{".claude/a", ".claude/../.bashrc"},
		{".claude/../../devvm-import-test"},
		{".claude/../.claude.json"},
		{".bashrc"},
		{"./.claude/a"},
		{".claudex/a"},
	} {
		t.Run(names[len(names)-1], func(t *testing.T) {
			home := t.TempDir()
			out, err := run(t, home, exportOf(t, names...))
			if err == nil || strings.TrimSpace(out) != historyUnsafe {
				t.Errorf("imported: %q, %v", out, err)
			}
			if es, _ := os.ReadDir(home); len(es) != 1 || es[0].Name() != ".cache" {
				t.Errorf("home has %v", es)
			}
			if es, _ := os.ReadDir(filepath.Join(home, ".cache")); len(es) != 0 {
				t.Errorf("its temp folder stayed: %v", es)
			}
		})
	}
	t.Run("history", func(t *testing.T) {
		home := t.TempDir()
		mkdirs(t, home, ".claude/projects")
		newer := filepath.Join(home, ".claude/projects/kept.jsonl")
		if err := state.WriteFile(newer, []byte("the VM's own\n")); err != nil {
			t.Fatal(err)
		}
		export := exportOf(t, ".claude/", ".claude/projects/", ".claude/projects/p.jsonl", ".claude/projects/kept.jsonl",
			".claude/.credentials.json", ".claude/settings.json", ".claude/skills/", ".claude/skills/x", ".claude.json",
			".claude/link->/etc", ".claude/projects/up->../../.bashrc", ".claude/projects/in->p.jsonl", ".codex/",
			".codex/sessions/s", ".codex/auth.json", ".local/share/opencode/auth.json", ".local/state/opencode/x")
		if out, err := run(t, home, export); err != nil {
			t.Fatalf("%q, %v", out, err)
		}
		for p, want := range map[string]bool{
			".claude/projects/p.jsonl": true, ".codex/sessions/s": true, ".local/state/opencode/x": true, ".claude/projects/in": true,
			".claude/.credentials.json": false, ".claude/settings.json": false, ".claude/skills": false, ".claude.json": false,
			".codex/auth.json": false, ".local/share/opencode/auth.json": false, ".claude/link": false, ".claude/projects/up": false,
		} {
			if _, err := os.Lstat(filepath.Join(home, p)); (err == nil) != want {
				t.Errorf("%s: there %v, want %v", p, err == nil, want)
			}
		}
		if data, _ := os.ReadFile(newer); string(data) != "the VM's own\n" { //nolint:gosec // the test's own file

			t.Errorf("the VM's file was replaced: %q", data)
		}
		if fi, err := os.Stat(filepath.Join(home, ".claude/projects/p.jsonl")); err != nil || fi.Mode()&os.ModeSetuid != 0 {
			t.Errorf("p.jsonl's mode: %v, %v", fi, err)
		}
	})
}
