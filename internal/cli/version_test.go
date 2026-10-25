package cli

import "testing"

// A release build's version is the one it was given, as it is; root's
// --version prints devvm's.
func TestVersionFromRelease(t *testing.T) {
	t.Parallel()
	if got := versionOf("1.2.3"); got != "1.2.3" {
		t.Errorf("versionOf(1.2.3) = %q, want 1.2.3", got)
	}
	u, _, _ := testUI()
	if got, want := NewRoot(u).Version, devvmVersion(); got != want {
		t.Errorf("root's Version = %q, want %q", got, want)
	}
}

// --version has no -v: Cobra would add one.
func TestVersionFlagHasNoShorthand(t *testing.T) {
	t.Parallel()
	u, _, _ := testUI()
	f := NewRoot(u).Flags().Lookup("version")
	if f == nil || f.Shorthand != "" {
		t.Errorf("--version = %+v, want a flag with no shorthand", f)
	}
}
