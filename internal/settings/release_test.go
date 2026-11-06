package settings

import (
	"errors"
	"testing"

	"github.com/devvmhq/devvm/internal/state"
)

// The daily release check: on unless config.toml's release-check is false
// or DEVVM_NO_RELEASE_CHECK is set; release-check must be true or false.
func TestReleaseCheck(t *testing.T) {
	d := testDirs(t)
	none := func(string) string { return "" }
	ci := func(k string) string {
		if k == "DEVVM_NO_RELEASE_CHECK" {
			return "1"
		}
		return ""
	}
	for _, c := range []struct {
		config string
		getenv func(string) string
		want   bool
	}{
		{"", none, true},
		{"release-check = true\n", none, true},
		{"release-check = false\n", none, false},
		{"", ci, false},
		{"release-check = true\n", ci, false},
	} {
		write(t, d.ConfigFile(), c.config)
		if on, err := ReleaseCheck(d, c.getenv); err != nil || on != c.want {
			t.Errorf("%q: %v, %v; want %v", c.config, on, err, c.want)
		}
	}
	write(t, d.ConfigFile(), "# off\nrelease-check = \"no\"\n")
	_, err := ReleaseCheck(d, none)
	if c, ok := errors.AsType[*state.CorruptError](err); !ok || c.Line != 2 {
		t.Errorf("a string: %v", err)
	}
}
