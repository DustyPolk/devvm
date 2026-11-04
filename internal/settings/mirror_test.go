package settings

import (
	"errors"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// config.toml's ubuntu-mirror: an http:// or https:// address, else
// refused with ubuntu_mirror_invalid at its line; none is "".
func TestUbuntuMirror(t *testing.T) {
	d := testDirs(t)
	if m, err := UbuntuMirror(d); err != nil || m != "" {
		t.Fatalf("no config.toml: %q, %v", m, err)
	}
	for _, good := range []string{"http://mirror.example/ubuntu", "https://mirror.example:8443/ubuntu-ports/", "http://10.0.0.5/ubuntu"} {
		write(t, d.ConfigFile(), "ubuntu-mirror = \""+good+"\"\n")
		if m, err := UbuntuMirror(d); err != nil || m != good {
			t.Errorf("%s: %q, %v", good, m, err)
		}
	}
	for _, bad := range []string{
		"ftp://mirror.example/ubuntu", "mirror.example/ubuntu", "https://", "https://me:pw@mirror.example/",
		"https://mirror.example/?x=1", "https://mirror.example/#x", "https://mirror.example/a b", `https://mirror.example/\t`,
	} {
		write(t, d.ConfigFile(), "# a mirror\nubuntu-mirror = \""+bad+"\"\n")
		_, err := UbuntuMirror(d)
		var c *state.CorruptError
		if !errors.As(err, &c) || c.Msg.Names() != msg.UbuntuMirrorInvalid(KeyUbuntuMirror).Names() || c.Line != 2 {
			t.Errorf("%s: %v", bad, err)
		}
	}
	write(t, d.ConfigFile(), "ubuntu-mirror = 3\n")
	if _, err := UbuntuMirror(d); err == nil {
		t.Error("a number: read")
	}
}
