package lima

import (
	"os/exec"
	"strings"
	"testing"
)

var nasty = []string{
	"", "plain", "two words", "it's", `"double"`, `back\slash`, "$HOME", "$(touch /tmp/x)", "`id`",
	"a;b", "a&&b", "a|b", "*", "~", "!x", "new\nline", "tab\there", "'", "''", `'\''`, "-n", "--", "é ü 漢",
	"\x01\x7f",
}

// sh runs script with sh -c and returns its output.
func sh(t *testing.T, script string) (string, error) {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "sh", "-c", script).Output() //nolint:gosec // quoting is what's tested
	return string(out), err
}

// What shq quotes reaches the shell's command as it was.
func TestShq(t *testing.T) {
	for _, s := range nasty {
		if out, err := sh(t, "printf %s "+shq(s)); err != nil || out != s {
			t.Errorf("shq(%q): the shell got %q, %v", s, out, err)
		}
	}
	out, err := sh(t, `printf '%s\0' `+shqJoin(nasty))
	if err != nil || out != strings.Join(nasty, "\x00")+"\x00" {
		t.Errorf("shqJoin: the shell got %q, %v", out, err)
	}
}

func FuzzShq(f *testing.F) {
	for _, s := range nasty {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if strings.ContainsRune(s, 0) {
			return // no argument holds one
		}
		if out, err := sh(t, "printf %s "+shq(s)); err != nil || out != s {
			t.Errorf("shq(%q): the shell got %q, %v", s, out, err)
		}
	})
}
