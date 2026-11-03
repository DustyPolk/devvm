package paths

import (
	"path/filepath"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

func TestFromEnv(t *testing.T) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		env  map[string]string
		want Dirs
	}{
		{
			name: "defaults under HOME",
			env:  map[string]string{"HOME": "/h"},
			want: Dirs{Home: "/h", Data: "/h/.local/share/devvm", Config: "/h/.config/devvm", Lima: "/h/.local/share/devvm/lima"},
		},
		{
			name: "HOME cleaned",
			env:  map[string]string{"HOME": "/h//x/../"},
			want: Dirs{Home: "/h", Data: "/h/.local/share/devvm", Config: "/h/.config/devvm", Lima: "/h/.local/share/devvm/lima"},
		},
		{
			name: "XDG folders, LIMA_HOME ignored",
			env:  map[string]string{"HOME": "/h", "XDG_DATA_HOME": "/d", "XDG_CONFIG_HOME": "/c/", "LIMA_HOME": "/elsewhere"},
			want: Dirs{Home: "/h", Data: "/d/devvm", Config: "/c/devvm", Lima: "/d/devvm/lima"},
		},
		{
			name: "DEVVM_LIMA_HOME moves the Lima home",
			env:  map[string]string{"HOME": "/h", "XDG_DATA_HOME": "/d", "DEVVM_LIMA_HOME": "/short", "LIMA_HOME": "/elsewhere"},
			want: Dirs{Home: "/h", Data: "/d/devvm", Config: "/h/.config/devvm", Lima: "/short"},
		},
		{
			name: "a relative DEVVM_LIMA_HOME is made absolute",
			env:  map[string]string{"HOME": "/h", "DEVVM_LIMA_HOME": "lt"},
			want: Dirs{Home: "/h", Data: "/h/.local/share/devvm", Config: "/h/.config/devvm", Lima: filepath.Join(cwd, "lt")},
		},
		{
			name: "relative XDG folders are ignored",
			env:  map[string]string{"HOME": "/h", "XDG_DATA_HOME": "d", "XDG_CONFIG_HOME": "c"},
			want: Dirs{Home: "/h", Data: "/h/.local/share/devvm", Config: "/h/.config/devvm", Lima: "/h/.local/share/devvm/lima"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fromEnv(func(k string) string { return tt.env[k] })
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFromEnvNeedsHome(t *testing.T) {
	for _, home := range []string{"", "rel"} {
		_, err := fromEnv(func(k string) string {
			if k == "HOME" {
				return home
			}
			return ""
		})
		if msg.IDOf(err) != "home_not_absolute" {
			t.Errorf("HOME=%q: got %v, want home_not_absolute", home, err)
		}
	}
}

func TestFiles(t *testing.T) {
	d := Dirs{Data: "/d", Config: "/c", Lima: "/l"}
	tests := []struct{ got, want string }{
		{d.VMState("devvm-p-1"), "/d/vms/devvm-p-1"},
		{d.Image("abc"), "/d/images/abc"},
		{d.ImageRecord("abc"), "/d/images/abc/record.json"},
		{d.ImageLocks(), "/d/images/locks"},
		{d.Rebuilds(), "/d/images/rebuilds.json"},
		{d.RebuildsLock(), "/d/images/locks/rebuilds"},
		{d.Cache(), "/d/cache"},
		{d.FromDigests(), "/d/cache/from-digests.json"},
		{d.NixCache(), "/d/cache/nix-cache"},
		{d.ToolRecord("go-1.2.0-0123456789ab.erofs"), "/d/tools/go-1.2.0-0123456789ab.erofs.json"},
		{d.Run(), "/d/run"},
		{d.Logs(), "/d/logs"},
		{d.RunLogs(), "/d/logs/runs"},
		{d.CA(), "/d/ca"},
		{d.Tools(), "/d/tools"},
		{d.ToolLock(), "/d/mise.lock"},
		{d.Mise(), "/d/cache/mise"},
		{d.Exports(), "/d/exports"},
		{d.AgentSock(), "/d/agent.sock"},
		{d.OCIBuildLock(), "/d/oci-build.lock"},
		{d.LimaVM("devvm-p-1"), "/l/devvm-p-1"},
		{d.BaseFeatures(), "/c/base-features"},
		{d.Size(), "/c/size"},
		{d.Ports(), "/c/ports"},
		{d.Network(), "/c/network"},
		{d.AgentKeys(), "/c/agent-keys"},
		{d.Backend(), "/c/backend"},
		{d.Layers(), "/c/layers"},
		{d.Features(), "/c/features"},
		{d.Secrets(), "/c/secrets"},
		{d.SSHConfig(), "/c/ssh_config"},
		{d.KnownHosts(), "/c/known_hosts"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %s, want %s", tt.got, tt.want)
		}
	}
}
