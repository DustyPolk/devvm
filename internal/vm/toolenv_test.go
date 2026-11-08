package vm

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/devvmhq/devvm/assets"
	"github.com/devvmhq/devvm/internal/image"
	"github.com/devvmhq/devvm/internal/tools"
)

// between is the text of file in devvm's assets after its first line that is start, up to the
// next line that is end.
func between(t *testing.T, file, start, end string) string {
	t.Helper()
	b, err := fs.ReadFile(assets.FS, file)
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut("\n"+string(b), "\n"+start+"\n")
	block, _, closed := strings.Cut(rest, "\n"+end+"\n")
	if !ok || !closed {
		t.Fatalf("%s has no %q", file, start)
	}
	return block
}

// guestEnvFiles writes what a launch's setup installs in /etc/devvm into dir/etc/devvm, and
// base's devvm-tools.sh, reading them there, as dir/devvm-tools.sh, whose path it returns.
func guestEnvFiles(t *testing.T, dir string) string {
	t.Helper()
	etc := filepath.Join(dir, "etc", "devvm")
	if err := os.MkdirAll(etc, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"toolchains.sh": toolchainEnv(), "agents.sh": agentEnv()} {
		if err := os.WriteFile(filepath.Join(etc, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	block := between(t, "layers/base.sh", "sudo tee /etc/profile.d/devvm-tools.sh >/dev/null <<'SH'", "SH")
	script := filepath.Join(dir, "devvm-tools.sh")
	if err := os.WriteFile(script, []byte(strings.ReplaceAll(block, "/etc/devvm/", etc+"/")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return script
}

// shellEnv runs a shell with only HOME and a PATH, and returns the environment it exports, sorted,
// without the variables a shell sets of its own.
func shellEnv(t *testing.T, env []string, name string, args ...string) string {
	t.Helper()
	shellNoise := []string{"PWD", "OLDPWD", "SHLVL", "_", "HOME", "BASH_ENV", "__CF_USER_TEXT_ENCODING"}
	cmd := exec.CommandContext(t.Context(), name, args...) //nolint:gosec // the test's own shells, on its own files
	cmd.Env = append([]string{"HOME=/home/u", "PATH=/usr/bin:/bin"}, env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %q: %v", name, args, err)
	}
	var lines []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if k, _, _ := strings.Cut(line, "="); !slices.Contains(shellNoise, k) {
			lines = append(lines, line)
		}
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n") + "\n"
}

// Every shell in a VM gets the shared tools' environment and PATH from base's devvm-tools.sh and
// what it reads: a login shell, which sources it from profile.d; a non-login bash -c, which
// BASH_ENV points at it; and a shell inside one of them, whose PATH it leaves as it is.
func TestToolsShellGolden(t *testing.T) {
	for _, c := range []string{"sh", "bash", "env"} {
		if _, err := exec.LookPath(c); err != nil {
			t.Skip("no " + c)
		}
	}
	script := guestEnvFiles(t, t.TempDir())
	login := shellEnv(t, nil, "sh", "-c", `. "$0"; env`, script)
	if got := shellEnv(t, []string{"BASH_ENV=" + script}, "bash", "-c", "env"); got != login {
		t.Errorf("a non-login bash -c gets\n%s\na login shell\n%s", got, login)
	}
	if got := shellEnv(t, nil, "sh", "-c", `. "$0"; . "$0"; env`, script); got != login {
		t.Errorf("a shell in a shell gets\n%s\none shell\n%s", got, login)
	}
	golden(t, "tools-shell.txt", []byte(login))
	golden(t, "toolchain-env", toolchainEnv())
	golden(t, "version-files.toml", versionFiles())
}

// The toolchains' environment, PATH and version files have one owner, image.SharedTools: base's
// devvm-tools.sh and guest/sync.sh hold none of them, so a new toolchain changes no image and no
// script.
func TestToolchainsOwnTheirEnvironment(t *testing.T) {
	tools := between(t, "layers/base.sh", "sudo tee /etc/profile.d/devvm-tools.sh >/dev/null <<'SH'", "SH")
	for _, v := range image.GuestEnv() {
		if strings.Contains(tools, v.Name) {
			t.Errorf("base's devvm-tools.sh sets %s, a toolchain's, which the setup's toolchains.sh sets", v.Name)
		}
	}
	for _, d := range image.GuestPath() {
		if strings.Contains(tools, "$HOME/"+d) {
			t.Errorf("base's devvm-tools.sh puts ~/%s on the PATH, a toolchain's, which the setup's toolchains.sh does", d)
		}
	}
	sync, err := fs.ReadFile(assets.FS, "guest/sync.sh")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sync), "idiomatic_version_file_enable_tools =") {
		t.Error("guest/sync.sh lists the version files' tools, which the setup's version-files.toml holds")
	}
}

// The settings of the VM's mise, as sync.sh writes them into /etc/mise/config.toml from what a
// launch stages: the version files mise reads among them.
func TestMiseSettingsGolden(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	dir := t.TempDir()
	aliases, err := tools.Aliases(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	var list strings.Builder
	for _, d := range image.SharedTools {
		for _, m := range d.Members {
			list.WriteString(m + "-1-0123456789ab.erofs " + m + " 1\n")
		}
	}
	staged := map[string][]byte{"tool-alias.toml": aliases, "tools": []byte(list.String()), "version-files.toml": versionFiles()}
	for name, data := range staged {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	block := "{\n" + between(t, "guest/sync.sh", "{", `} > "$s/mise.toml"`) + "\n} > \"$s/mise.toml\"\n"
	cmd := exec.CommandContext(t.Context(), "bash", "-c", "set -euo pipefail; s=$1\n"+block, "bash", dir) //nolint:gosec // sync.sh's own block, on the test's files
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sync.sh's mise.toml: %v\n%s", err, out)
	}
	var config struct {
		Settings map[string]any `toml:"settings"`
	}
	if _, err := toml.DecodeFile(filepath.Join(dir, "mise.toml"), &config); err != nil {
		t.Fatal(err)
	}
	// The version files' tools are a set, which mise looks a tool up in.
	if v, ok := config.Settings["idiomatic_version_file_enable_tools"].([]any); ok {
		slices.SortFunc(v, func(a, b any) int { return strings.Compare(a.(string), b.(string)) })
	}
	got, err := json.MarshalIndent(config.Settings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "mise-settings.json", append(got, '\n'))
}
