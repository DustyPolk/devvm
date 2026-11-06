package assets

import (
	"io/fs"
	"strings"
	"testing"
)

// Every VM's shells get the tools' environment from base's
// /etc/profile.d/devvm-tools.sh, which turns off the agents'
// self-updates: their installs are read-only shared tools, and mise pins
// their versions.
func TestToolsEnvironment(t *testing.T) {
	b, err := fs.ReadFile(FS, "layers/base.sh")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(b), "/etc/profile.d/devvm-tools.sh >/dev/null <<'SH'\n")
	block, _, closed := strings.Cut(rest, "\nSH\n")
	if !ok || !closed {
		t.Fatal("base.sh writes no /etc/profile.d/devvm-tools.sh")
	}
	for _, line := range []string{"export DISABLE_AUTOUPDATER=1", "export OPENCODE_DISABLE_AUTOUPDATE=1"} {
		if !strings.Contains("\n"+block+"\n", "\n"+line+"\n") {
			t.Errorf("devvm-tools.sh lacks %q", line)
		}
	}
}
