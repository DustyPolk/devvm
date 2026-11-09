package settings

import (
	"errors"
	"testing"

	"github.com/devvmhq/devvm/internal/state"
)

// Transparent mode: on unless config.toml's transparent-proxy is false;
// it must be true or false, and anything else names its line.
func TestTransparentProxy(t *testing.T) {
	d := testDirs(t)
	for _, c := range []struct {
		config  string
		on, set bool
	}{
		{"", true, false},
		{"transparent-proxy = true\n", true, true},
		{"transparent-proxy = false\n", false, true},
	} {
		write(t, d.ConfigFile(), c.config)
		if on, set, err := TransparentProxy(d); err != nil || on != c.on || set != c.set {
			t.Errorf("%q: %v %v, %v; want %v %v", c.config, on, set, err, c.on, c.set)
		}
	}
	for _, bad := range []string{"\"yes\"", "1", "\"false\"", "[true]"} {
		write(t, d.ConfigFile(), "# transparent mode\ntransparent-proxy = "+bad+"\n")
		_, _, err := TransparentProxy(d)
		if c, ok := errors.AsType[*state.CorruptError](err); !ok || c.Line != 2 {
			t.Errorf("%s: %v", bad, err)
		}
	}
}
