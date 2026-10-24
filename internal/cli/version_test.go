package cli

import "testing"

// A release build's version is the one it was given, as it is.
func TestVersionFromRelease(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	version = "1.2.3"
	if got := devvmVersion(); got != "1.2.3" {
		t.Errorf("devvmVersion() = %q, want 1.2.3", got)
	}
	if got := NewRoot().Version; got != "1.2.3" {
		t.Errorf("root's Version = %q, want 1.2.3", got)
	}
}

// --version has no -v: Cobra would add one.
func TestVersionFlagHasNoShorthand(t *testing.T) {
	f := NewRoot().Flags().Lookup("version")
	if f == nil || f.Shorthand != "" {
		t.Errorf("--version = %+v, want a flag with no shorthand", f)
	}
}
