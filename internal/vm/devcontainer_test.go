package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/devcontainer"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// devcontainerEnv is up on a fake backend with a devcontainer.json in the
// project folder, its warnings and the devcontainer.json commands it runs
// collected.
func devcontainerTestEnv(t *testing.T, src string) (*Env, *backendtest.Fake, *[]string) {
	t.Helper()
	e, f, _ := upEnv(t, allNative())
	if err := os.WriteFile(filepath.Join(e.Dir, ".devcontainer.json"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	var warned []string
	e.Warn = func(m msg.M) { warned = append(warned, m.Names()) }
	e.PublicImage = func(_ context.Context, ref string) (string, error) {
		if ref == "private:1" {
			return "", fmt.Errorf("%w: no", from.ErrNotFound)
		}
		return from.Name(ref) + "@sha256:" + strings.Repeat("a", 64), nil
	}
	return e, f, &warned
}

// commandsRun returns the devcontainer.json commands the fake ran, each
// as its argv after the login shell, and the folders they ran in.
func commandsRun(f *backendtest.Fake) (cmds []string, dirs []string) {
	for _, c := range f.Calls() {
		if c.Method != "Exec" || len(c.Args) < 3 {
			continue
		}
		argv, _ := c.Args[1].([]string)
		if len(argv) < 4 || !slices.Equal(argv[:4], []string{"bash", "-lc", commandScript, "devvm"}) {
			continue
		}
		cmds = append(cmds, strings.Join(argv[4:], " "))
		o, _ := c.Args[2].(backend.ExecOpts)
		dirs = append(dirs, o.Dir)
	}
	return cmds, dirs
}

func TestUpDevcontainer(t *testing.T) {
	e, f, warned := devcontainerTestEnv(t, `{
		"image": "node:24",
		"forwardPorts": [3000],
		"remoteEnv": {"A": "${containerWorkspaceFolderBasename}"},
		"onCreateCommand": ["touch", "x"],
		"postStartCommand": "echo hi",
	}`)
	if err := state.WritePorts(e.Dirs, "5173"); err != nil {
		t.Fatal(err)
	}
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	for k, want := range map[string]string{
		state.KeyFrom: "node@sha256:" + strings.Repeat("a", 64), KeyPorts: "5173",
		KeyDevcontainerCreate: "", KeyDevcontainerEnv: `[{"name":"A","value":"'web'"}]`,
	} {
		if got := st.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if !strings.HasSuffix(st.Get(KeyDevcontainer), " .devcontainer.json") {
		t.Errorf("devcontainer = %q", st.Get(KeyDevcontainer))
	}
	cmds, dirs := commandsRun(f)
	if want := []string{"touch x", "sh -c echo hi"}; !slices.Equal(cmds, want) {
		t.Errorf("ran %q, want %q", cmds, want)
	}
	for _, d := range dirs {
		if d != e.Dir {
			t.Errorf("ran in %q", d)
		}
	}
	s, err := syncer{e}.stage(t.Context(), st, setupInputs{slug: "web"})
	if err != nil {
		t.Fatal(err)
	}
	if env := string(s.files[s.index("env")].data); !strings.HasPrefix(env, "export A='web'\n") {
		t.Errorf("env = %q", env)
	}
	// Its ports beyond the user's are said, not added.
	if want := []string{"devcontainer_ports_not_forwarded file=.devcontainer.json ports=3000"}; !slices.Equal(*warned, want) {
		t.Errorf("warned %q, want %q", *warned, want)
	}
	*warned = nil

	// Launched again, running: nothing runs; changed, it's said.
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if cmds, _ := commandsRun(f); len(cmds) != 2 {
		t.Errorf("ran again: %q", cmds)
	}
	if err := os.WriteFile(filepath.Join(e.Dir, ".devcontainer.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if want := "devcontainer_changed file=.devcontainer.json vm=" + st.Name(); !slices.Equal(*warned, []string{want}) {
		t.Errorf("warned %q, want %q", *warned, want)
	}
	// Started again: postStartCommand runs again.
	if err := f.Stop(t.Context(), st.Name()); err != nil {
		t.Fatal(err)
	}
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if cmds, _ := commandsRun(f); !slices.Equal(cmds, []string{"touch x", "sh -c echo hi", "sh -c echo hi"}) {
		t.Errorf("ran %q", cmds)
	}
}

// A command that fails is said, those after it don't run, and the VM is
// opened all the same.
func TestUpDevcontainerCommandFails(t *testing.T) {
	e, f, warned := devcontainerTestEnv(t, `{"onCreateCommand": "false", "postCreateCommand": "true", "postStartCommand": "echo hi"}`)
	f.ExecFunc = func(_ string, argv []string, _ backend.ExecOpts) error {
		if slices.Contains(argv, "false") {
			return errors.New("exit status 1")
		}
		return nil
	}
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	if want := "devcontainer_command_failed command=onCreateCommand vm=" + st.Name(); !slices.Equal(*warned, []string{want}) {
		t.Errorf("warned %q, want %q", *warned, want)
	}
	if cmds, _ := commandsRun(f); !slices.Equal(cmds, []string{"sh -c false", "sh -c echo hi"}) {
		t.Errorf("ran %q", cmds)
	}
}

// Refused before anything is made.
func TestUpDevcontainerRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"not public", `{"image": "private:1"}`, "devcontainer_image_not_public file=.devcontainer.json from=private:1"},
		{"not a reference", `{"image": "a b"}`, "bad_from from=a b"},
		{"local env", `{"remoteEnv": {"A": "${localEnv:HOME}"}}`, "devcontainer_local_env file=.devcontainer.json variable=${localEnv:HOME}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, f, _ := devcontainerTestEnv(t, tc.src)
			err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true})
			if got := names(err); got != tc.want {
				t.Errorf("error = %s, want %s", got, tc.want)
			}
			for _, c := range f.Calls() {
				if c.Method == "CreateVM" || c.Method == "BuildImage" {
					t.Errorf("made: %s", c.Method)
				}
			}
		})
	}
}

// A new VM gets network policy unless it's turned off, asked yes by
// default in a terminal. One that can't have it (made for the docker
// layer, named or from its devcontainer.json's docker-in-docker, or on a
// backend without egress-proxy) is made without it, unasked and warned,
// in a terminal or not; --network-policy on it is refused, never dropped.
func TestChooseNet(t *testing.T) {
	dc := &devcontainer.Plan{File: ".devcontainer.json", Layers: []string{"docker"}}
	noEgress := allNative()
	delete(noEgress, backend.FeatureEgressProxy)
	unreviewed := allNative()
	unreviewed[backend.FeatureEgressProxy] = backend.Emulated(false)
	for _, tc := range []struct {
		name     string
		o        Options
		caps     backend.Caps
		dockerVM bool
		terminal bool
		answer   bool
		want     string // the error's names, else the choice
		asked    bool
		warned   string
	}{
		{"no terminal", Options{}, nil, false, false, false, "true", false, ""},
		{"yes", Options{}, nil, false, true, true, "true", true, ""},
		{"no", Options{}, nil, false, true, false, "false", true, ""},
		{"--network-policy", Options{NetworkPolicy: true}, nil, false, true, false, "true", false, ""},
		{"--no-network-policy", Options{NoNetworkPolicy: true}, nil, false, false, true, "false", false, ""},
		{"named", Options{Image: "docker"}, nil, true, true, true, "false", false, "net_off_docker_layer backend=fake vm=v"},
		{"named, no terminal", Options{Image: "docker"}, nil, true, false, true, "false", false, "net_off_docker_layer backend=fake vm=v"},
		{"named, --network-policy", Options{NetworkPolicy: true, Image: "docker"}, nil, true, true, true, "net_docker_layer backend=fake vm=v", false, ""},
		{"named, --no-network-policy", Options{NoNetworkPolicy: true, Image: "docker"}, nil, true, true, true, "false", false, ""},
		{"the file's", Options{dc: dc}, nil, true, true, true, "false", false, "net_off_docker_layer backend=fake vm=v"},
		{"the file's, no terminal", Options{dc: dc}, nil, true, false, true, "false", false, "net_off_docker_layer backend=fake vm=v"},
		{"the file's, --network-policy", Options{dc: dc, NetworkPolicy: true}, nil, true, true, true, "net_docker_layer backend=fake vm=v", false, ""},
		{"no egress-proxy", Options{}, noEgress, false, true, true, "false", false, "net_off_backend backend=fake vm=v"},
		{"no egress-proxy, no terminal", Options{}, noEgress, false, false, true, "false", false, "net_off_backend backend=fake vm=v"},
		{"unreviewed egress-proxy", Options{}, unreviewed, false, false, true, "false", false, "net_off_backend backend=fake vm=v"},
		// Require refuses it (backend_lacks), as the plan's test shows.
		{"no egress-proxy, --network-policy", Options{NetworkPolicy: true}, noEgress, false, true, true, "true", false, ""},
		{"no egress-proxy, --no-network-policy", Options{NoNetworkPolicy: true}, noEgress, false, true, true, "false", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caps := tc.caps
			if caps == nil {
				caps = allNative()
			}
			e, f, _ := upEnv(t, caps)
			asked, warned := false, ""
			e.Warn = func(m msg.M) { warned += m.Names() }
			e.Proxy.Terminal = func() bool { return tc.terminal }
			e.Proxy.Note = func(msg.M) {}
			e.Proxy.Confirm = func(msg.M) bool { asked = true; return tc.answer }
			o := tc.o
			on, err := creation{Launch{e}}.chooseNet(&o, tc.dockerVM, f, "v")
			got := fmt.Sprint(on)
			if err != nil {
				got = names(err)
			}
			if got != tc.want || asked != tc.asked || warned != tc.warned {
				t.Errorf("chooseNet = %s (asked %v, warned %q), want %s (asked %v, warned %q)", got, asked, warned, tc.want, tc.asked, tc.warned)
			}
		})
	}
}

// What devvm network -g or devvm setup saved: off makes a new VM without network policy,
// unasked and unwarned even where it couldn't have it; on gives it
// unasked, and a VM that can't have it is still made without it and
// warned; either flag beats it, and with one a file that can't be read
// isn't read.
func TestChooseNetSaved(t *testing.T) {
	noEgress := allNative()
	delete(noEgress, backend.FeatureEgressProxy)
	for _, tc := range []struct {
		name     string
		saved    string
		o        Options
		caps     backend.Caps
		dockerVM bool
		want     string
		warned   string
	}{
		{"off", "off", Options{}, nil, false, "false", ""},
		{"off, docker layer", "off", Options{}, nil, true, "false", ""},
		{"off, no egress-proxy", "off", Options{}, noEgress, false, "false", ""},
		{"off, --network-policy", "off", Options{NetworkPolicy: true}, nil, false, "true", ""},
		{"on", "on", Options{}, nil, false, "true", ""},
		{"on, --no-network-policy", "on", Options{NoNetworkPolicy: true}, nil, false, "false", ""},
		{"on, docker layer", "on", Options{}, nil, true, "false", "net_off_docker_layer backend=fake vm=v"},
		{"on, no egress-proxy", "on", Options{}, noEgress, false, "false", "net_off_backend backend=fake vm=v"},
		{"ask", "ask", Options{}, nil, false, "true", ""},
		{"unknown", "maybe", Options{}, nil, false, "error", ""},
		{"unknown, --network-policy", "maybe", Options{NetworkPolicy: true}, nil, false, "true", ""},
		{"unknown, --no-network-policy", "maybe", Options{NoNetworkPolicy: true}, nil, false, "false", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caps := tc.caps
			if caps == nil {
				caps = allNative()
			}
			e, f, _ := upEnv(t, caps)
			if err := os.MkdirAll(e.Dirs.Config, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(e.Dirs.Network(), []byte(tc.saved+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			asked, warned := false, ""
			e.Warn = func(m msg.M) { warned += m.Names() }
			e.Proxy.Terminal = func() bool { return true }
			e.Proxy.Note = func(msg.M) {}
			e.Proxy.Confirm = func(msg.M) bool { asked = true; return true }
			o := tc.o
			on, err := creation{Launch{e}}.chooseNet(&o, tc.dockerVM, f, "v")
			got := fmt.Sprint(on)
			if err != nil {
				got = "error"
			}
			if got != tc.want || warned != tc.warned || asked != (tc.saved == state.NetworkAsk) {
				t.Errorf("chooseNet = %s (asked %v, warned %q), want %s (warned %q)", got, asked, warned, tc.want, tc.warned)
			}
		})
	}
}

func TestDevcontainerEnvFromState(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	st, err := state.CreateVM(e.Dirs, "devvm-x-0123456789abcd", state.NewVM{})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Set(KeyDevcontainerEnv, `[{"name":"A","value":"'x'"},{"name":"B;rm","value":"'y'"}]`); err != nil {
		t.Fatal(err)
	}
	got, err := devcontainerEnv(st)
	if err != nil || got != "export A='x'\n" {
		t.Errorf("devcontainerEnv = %q, %v", got, err)
	}
}
