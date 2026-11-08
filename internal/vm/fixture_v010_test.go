package vm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/devvmhq/devvm/internal/gitprotect"
	"github.com/devvmhq/devvm/internal/netpolicy"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/ports"
	"github.com/devvmhq/devvm/internal/project"
	"github.com/devvmhq/devvm/internal/secrets"
	"github.com/devvmhq/devvm/internal/settings"
	"github.com/devvmhq/devvm/internal/sshagent"
	"github.com/devvmhq/devvm/internal/state"
	"github.com/devvmhq/devvm/internal/tools"
	"github.com/devvmhq/devvm/internal/vmname"
)

// v010Fixtures are the copies of devvm 0.1.0's state in the repo
// (testdata/v0.1.0/GOOS, which 0.1.0 itself made), each in a folder of
// the test's own, so nothing a reader writes (the layout's lock) lands in
// the repo.
func v010Fixtures(t *testing.T) map[string]paths.Dirs {
	t.Helper()
	found, err := filepath.Glob(filepath.Join("..", "..", "testdata", "v0.1.0", "*", "data"))
	if err != nil || len(found) == 0 {
		t.Fatalf("no fixture of 0.1.0's state: %v", err)
	}
	out := map[string]paths.Dirs{}
	for _, data := range found {
		src := filepath.Dir(data)
		dst := t.TempDir()
		if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(src)] = paths.Dirs{
			Home: filepath.Join(dst, "home"), Data: filepath.Join(dst, "data", "devvm"),
			Config: filepath.Join(dst, "config", "devvm"), Lima: filepath.Join(dst, "lima"),
		}
	}
	return out
}

// v010Sections are the sections 0.1.0 wrote in a VM's state, each with
// what reads it today, kept as it reads and written again as it would be.
func v010Sections() map[string]func(*state.VM) (any, error) {
	return map[string]func(*state.VM) (any, error){
		netpolicy.Section:  read(netpolicy.Of),
		gitprotect.Section: read(gitprotect.Of),
		ports.Section:      read(ports.Of),
		sshagent.Section:   read(sshagent.Of),
		secrets.Section:    read(secrets.Of),
		tools.Section:      read(tools.Of),
		sizeSection:        read(func(st *state.VM) (Size, error) { return state.Section[Size](st, sizeSection) }),
		cloneSection:       read(func(st *state.VM) (cloneInVM, error) { return state.Section[cloneInVM](st, cloneSection) }),
		linksSection:       read(func(st *state.VM) (linksInVM, error) { return state.Section[linksInVM](st, linksSection) }),
	}
}

// read is of, its section as any.
func read[T any](of func(*state.VM) (T, error)) func(*state.VM) (any, error) {
	return func(st *state.VM) (any, error) { return of(st) }
}

// asJSON is v as plain JSON values: maps, slices, strings, numbers.
func asJSON(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Every VM state 0.1.0 wrote reads today with nothing lost: its core
// fields as they are, each section read by its package and written again
// as 0.1.0 wrote it, field for field, and the whole state as the finder
// checks it. A section devvm doesn't read fails, as nothing would keep it.
func TestV010VMStates(t *testing.T) {
	for goos, d := range v010Fixtures(t) {
		names, err := state.VMNames(d)
		if err != nil || len(names) != 4 {
			t.Fatalf("%s: VMs %q, %v", goos, names, err)
		}
		for _, name := range names {
			raw := map[string]json.RawMessage{}
			hash, _ := vmname.HashOf(name)
			b, err := os.ReadFile(d.VM(name).State())
			if err != nil || json.Unmarshal(b, &raw) != nil {
				t.Fatalf("%s: %s: %v", goos, name, err)
			}
			st, err := state.ReadHash(d, hash)
			if err != nil || !st.Finished() || st.Name() != name {
				t.Fatalf("%s: ReadHash(%s) = %v, %v", goos, name, st, err)
			}
			// The folder names the same VM today: its hash is the same.
			if h := project.Hash(st.Dir); h != hash {
				t.Errorf("%s: %s's folder %s hashes to %s today", goos, name, st.Dir, h)
			}
			checkV010Core(t, st, raw)
			checkV010Sections(t, st, raw)
			if err := checkState(st); err != nil {
				t.Errorf("%s: %s: checkState: %v", goos, name, err)
			}
		}
	}
}

// checkV010Core checks st's core fields against the file's.
func checkV010Core(t *testing.T, st *state.VM, raw map[string]json.RawMessage) {
	t.Helper()
	core := map[string]any{
		"version": float64(1), "id": st.ID, "name": st.Name(), "alias": st.Alias, "image": st.Image,
		"chain": st.Chain, "dir": st.Dir, "folder": st.Folder, "backend": st.Backend,
		"backend_at": st.BackendAt, "made_by": st.MadeBy,
	}
	for k, want := range core {
		var got any
		if err := json.Unmarshal(raw[k], &got); err != nil || got != want {
			t.Errorf("%s: %s: the file has %s, read as %v", st.Name(), k, raw[k], want)
		}
	}
	if st.MadeBy != "0.1.0" || st.Created.IsZero() || st.Backend != "lima" {
		t.Errorf("%s: made by %q at %v on %q", st.Name(), st.MadeBy, st.Created, st.Backend)
	}
}

// checkV010Sections checks each of st's sections, read and written again,
// against the file's.
func checkV010Sections(t *testing.T, st *state.VM, raw map[string]json.RawMessage) {
	t.Helper()
	readers := v010Sections()
	for k, b := range raw {
		if slices.Contains([]string{"version", "id", "name", "alias", "image", "chain", "from", "dir", "folder", "backend", "backend_at", "created", "made_by"}, k) {
			continue
		}
		of, ok := readers[k]
		if !ok {
			t.Errorf("%s: section %q: nothing reads it", st.Name(), k)
			continue
		}
		v, err := of(st)
		if err != nil {
			t.Errorf("%s: section %q: %v", st.Name(), k, err)
			continue
		}
		var want any
		if err := json.Unmarshal(b, &want); err != nil {
			t.Fatal(err)
		}
		if got := asJSON(t, v); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: section %q: 0.1.0 wrote\n%s\nread and written again:\n%v", st.Name(), k, b, got)
		}
	}
}

// What each VM 0.1.0 made reads as today: its tier, .git protection,
// ports, secrets, tools, links and group, clone and agent forwarding. A
// .git watch 0.1.0 never recorded is absent, which the watch takes as its
// first scan.
func TestV010VMsRead(t *testing.T) {
	for goos, d := range v010Fixtures(t) {
		vms := map[string]*state.VM{}
		names, err := state.VMNames(d)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			hash, _ := vmname.HashOf(name)
			st, err := state.ReadHash(d, hash)
			if err != nil {
				t.Fatal(err)
			}
			slug, _ := vmname.SlugOf(name)
			vms[slug] = st
		}
		t.Run(goos, func(t *testing.T) {
			checkV010App(t, vms["app"], vms["db"])
			checkV010Git(t, goos, vms["app"])
			checkV010Others(t, vms)
		})
	}
}

// checkV010App checks app: node and both agents, allowlist, two ports,
// .git protected, github's secret, a link to db, group backend.
func checkV010App(t *testing.T, app, db *state.VM) {
	t.Helper()
	if tier := netpolicy.TierOf(app); tier != netpolicy.TierAllowlist || !tier.Confined() {
		t.Errorf("app's tier: %q", tier)
	}
	if n, _ := netpolicy.Of(app); n.Made.ProxyKey == "" || n.Made.ProxySalt == "" || n.Made.Check != netpolicy.CheckOK {
		t.Errorf("app's net: %+v", n)
	}
	if p, _ := ports.Of(app); p.Forward != "3000,8080" || p.Host.SSH == 0 || p.Host.Proxy == 0 {
		t.Errorf("app's ports: %+v", p)
	}
	if s, _ := secrets.Of(app); !slices.Equal(s.Names, []string{"github"}) {
		t.Errorf("app's secrets: %+v", s)
	}
	if tl, _ := tools.Of(app); !slices.Equal(tl.Names, []string{"node", "claude", "codex"}) || len(tl.Made.Files) != 5 {
		t.Errorf("app's tools: %+v", tl)
	}
	links, err := ReadLinks(app)
	if err != nil || len(links) != 1 || links[0].Name != "db" || links[0].Port != 5432 || links[0].Target != db.Name() ||
		links[0].TargetID != db.ID || !links[0].Addr.IsValid() {
		t.Errorf("app's links: %+v, %v", links, err)
	}
	if l := linksOf(app); l.Group != "backend" {
		t.Errorf("app's group: %+v", l)
	}
	if l := linksOf(db); l.Group != "backend" || l.Publish != "5432" {
		t.Errorf("db's links: %+v", l)
	}
	if a, _ := sshagent.Of(app); sshagent.Mode(app) != sshagent.Off || a.Made.Started == "" {
		t.Errorf("app's ssh agent: %+v", a)
	}
}

// checkV010Git checks app's .git protection: on and locked on a Mac,
// off on Linux, where 0.1.0 had no locks. Neither has a watch yet, which
// a host without locks takes as its first scan.
func checkV010Git(t *testing.T, goos string, app *state.VM) {
	t.Helper()
	g, err := gitprotect.Of(app)
	switch {
	case err != nil:
		t.Errorf("app's git: %v", err)
	case goos == "darwin" && (g.Mode != gitprotect.On || !g.Made.Protected || g.Made.GitID == "" || g.Made.Delegations == ""):
		t.Errorf("app's git on a Mac: %+v", g)
	case goos != "darwin" && (g.Mode != gitprotect.Off || !reflect.ValueOf(g.Made).IsZero()):
		t.Errorf("app's git on %s: %+v", goos, g)
	}
	if !reflect.ValueOf(g.Watched).IsZero() {
		t.Errorf("app's git watch: %+v, not none yet", g.Watched)
	}
}

// checkV010Others checks web (--clone, open) and scratch (no network, no
// .git protection), and that every VM's size is config.toml's.
func checkV010Others(t *testing.T, vms map[string]*state.VM) {
	t.Helper()
	web, scratch := vms["web"], vms["scratch"]
	if c := sectionOf[cloneInVM](web, cloneSection); !c.On || c.Made.Remote != "devvm" || !c.Made.Seeded {
		t.Errorf("web's clone: %+v", c)
	}
	if g, _ := gitprotect.Of(web); g.Mode != gitprotect.Clone {
		t.Errorf("web's git: %+v", g)
	}
	if tier := netpolicy.TierOf(web); tier != netpolicy.TierOpen {
		t.Errorf("web's tier: %q", tier)
	}
	if g, _ := gitprotect.Of(scratch); g.Mode != gitprotect.Off {
		t.Errorf("scratch's git: %+v", g)
	}
	if tier := netpolicy.TierOf(scratch); tier != netpolicy.TierNone {
		t.Errorf("scratch's tier: %q", tier)
	}
	for slug, st := range vms {
		if s := sectionOf[Size](st, sizeSection); s != (Size{CPUs: "4", Memory: "8"}) {
			t.Errorf("%s's size: %+v", slug, s)
		}
	}
}

// 0.1.0's defaults read today as it saved them: settings.json's tier,
// ports, agents, layers and features, config.toml's size and retention.
// Its layout version is today's, so checking it changes nothing.
func TestV010Settings(t *testing.T) {
	for goos, d := range v010Fixtures(t) {
		def, err := settings.Read(d)
		if err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		saved := func(v settings.Value, want string, from settings.Source) {
			t.Helper()
			if v.Value != want || v.Source != from {
				t.Errorf("%s: %+v, want %q from %s", goos, v, want, from)
			}
		}
		saved(def.Network, "allowlist", settings.FromSettings)
		saved(def.Ports, "3000,5173", settings.FromSettings)
		saved(def.CPUs, "4", settings.FromConfig)
		saved(def.Memory, "8", settings.FromConfig)
		if !slices.Equal(def.Agents.Names, []string{"claude", "codex"}) || !def.Agents.Chosen ||
			len(def.Layers.Names) != 0 || !def.Layers.Chosen || len(def.Features.Names) != 0 || !def.Features.Chosen {
			t.Errorf("%s: agents %+v, layers %+v, features %+v", goos, def.Agents, def.Layers, def.Features)
		}
		if def.Retention.ExportsCount != 20 || def.Retention.Sources[settings.RetainHistoryCount] != settings.FromConfig {
			t.Errorf("%s: retention %+v", goos, def.Retention)
		}
		before, _ := os.ReadFile(d.Version())
		if err := state.CheckLayout(t.Context(), d); err != nil {
			t.Errorf("%s: CheckLayout: %v", goos, err)
		}
		if after, _ := os.ReadFile(d.Version()); string(after) != string(before) || string(before) != "1\n" {
			t.Errorf("%s: layout version %q, then %q", goos, before, after)
		}
	}
}

// 0.1.0's agent store: the settings it imported are what a VM mounts, and
// the logins are where devvm reads them.
func TestV010AgentStore(t *testing.T) {
	for goos, d := range v010Fixtures(t) {
		dirs, comma := agentMounts(d, []string{"node", "claude", "codex"})
		if want := []string{filepath.Join(d.AgentSettings("claude"), ".claude", "skills")}; !slices.Equal(dirs, want) || comma != nil {
			t.Errorf("%s: mounts %q (comma %q), want %q", goos, dirs, comma, want)
		}
		for _, f := range []string{
			filepath.Join(d.AgentSettings("claude"), ".claude", "settings.json"),
			filepath.Join(d.AgentSettings("codex"), ".codex", "config.toml"),
			secrets.LoginFile(d, "claude"),
			secrets.LoginFile(d, "codex"),
		} {
			if _, err := os.Stat(f); err != nil {
				t.Errorf("%s: %v", goos, err)
			}
		}
	}
}
