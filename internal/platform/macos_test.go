package platform

import (
	"regexp"
	"runtime"
	"testing"
)

func TestMacOSVersion(t *testing.T) {
	v := MacOSVersion()
	if runtime.GOOS != "darwin" {
		if v != "" {
			t.Errorf("MacOSVersion() = %q off macOS", v)
		}
		return
	}
	// Never 10.16, the version older programs are told for 11 and later.
	if !regexp.MustCompile(`^(1[1-9]|[2-9][0-9])\.[0-9]+`).MatchString(v) {
		t.Errorf("MacOSVersion() = %q", v)
	}
}
