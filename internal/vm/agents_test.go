package vm

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
	t.Setenv("DEVVM_MESSAGES", "names")
	var got []string
	e.Terminal = func() bool { return tty }
	e.Warn = func(m msg.M) { got = append(got, m.Names()) }
	e.Info = e.Warn
	e.Confirm = func(m msg.M) bool { got = append(got, "asked "+m.Names()); return true }
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
	st, err := state.CreateVM(d, "devvm-p-00000000000000", state.NewVM{Image: "base", Tools: strings.Fields(tools)})
	if err != nil {
		t.Fatal(err)
	}
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
		tty     bool
		have    []string
		methods []string
		set     []string
		printed []string
	}{
		{
			name: "nothing changed, nothing edited", tools: "node claude", status: backend.StatusStopped,
			have: []string{skills}, methods: []string{"Inspect"},
		},
		{
			name: "a stopped VM gets new folders and loses gone ones", tools: "node claude", status: backend.StatusStopped,
			have: []string{skills, commands}, methods: []string{"Inspect", "SetAgentDirs"}, set: []string{skills, mine},
			printed: []string{"agent_dirs_added dirs=" + mine + " vm=" + vm, "agent_dirs_removed dirs=" + commands + " vm=" + vm, "mounts_updated vm=" + vm},
		},
		{
			name: "a running VM without a terminal changes at its next start", tools: "node claude", status: backend.StatusRunning,
			methods: []string{"Inspect"}, printed: []string{"agent_dirs_added dirs=" + skills + ", " + mine + " vm=" + vm, "mounts_next_start vm=" + vm},
		},
		{
			name: "a running VM restarts once the user agrees", tools: "node claude", status: backend.StatusRunning, tty: true,
			have: []string{skills}, methods: []string{"Inspect", "Stop", "SetAgentDirs"}, set: []string{skills, mine},
			printed: []string{"agent_dirs_added dirs=" + mine + " vm=" + vm, "asked update_mounts vm=" + vm, "mounts_updated vm=" + vm},
		},
		{
			name: "a VM without agents drops leftover agent mounts", tools: "node", status: backend.StatusStopped,
			have: []string{filepath.Join(home, ".codex/prompts")}, methods: []string{"Inspect", "SetAgentDirs"},
			printed: []string{"agent_dirs_removed dirs=" + filepath.Join(home, ".codex/prompts") + " vm=" + vm, "mounts_updated vm=" + vm},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &Env{Dirs: paths.Dirs{Home: home}}
			got := printed(t, e, tc.tty)
			if tc.name != "nothing changed, nothing edited" {
				mkdirs(t, home, ".claude/agents")
			}
			f, st := agentVM(t, tc.tools, tc.status, tc.have...)
			if err := (Launch{e}).UpdateAgentDirs(t.Context(), f, st); err != nil {
				t.Fatal(msg.NamesOf(err))
			}
			if ms := f.Methods(); !slices.Equal(ms, tc.methods) {
				t.Errorf("calls = %q, want %q", ms, tc.methods)
			}
			if c := call(f, "SetAgentDirs"); c.Method != "" {
				if dirs, _ := c.Args[1].([]string); !slices.Equal(dirs, tc.set) {
					t.Errorf("SetAgentDirs %q, want %q", dirs, tc.set)
				}
			}
			if !slices.Equal(*got, tc.printed) {
				t.Errorf("printed %q, want %q", *got, tc.printed)
			}
			_ = os.Remove(mine)
		})
	}
}
