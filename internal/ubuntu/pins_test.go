package ubuntu

import (
	"os"
	"regexp"
	"testing"
)

// repoFile is one of the repo's files, from this package's folder.
func repoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../../" + name) //nolint:gosec // the repo's own files
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestE2EUbuntuPins checks e2e's Ubuntu pin against devvm's release and
// scripts/bump.sh: tests/e2e.sh pins base to a dated release of Series
// (DEVVM_UBUNTU_RELEASE, which a test build pins in place of the newest),
// shaped as Finder.Release takes it, and bump.sh's e2e-ubuntu, which
// moves it, lists Series' releases where devvm does; every Ubuntu release
// either script names is Series (Version).
func TestE2EUbuntuPins(t *testing.T) {
	e2e, bump := repoFile(t, "tests/e2e.sh"), repoFile(t, "scripts/bump.sh")
	pin := regexp.MustCompile(`(?m)^e2e_ubuntu_release=(\S*)$`).FindAllStringSubmatch(e2e, -1)
	if len(pin) != 1 || !releaseName.MatchString(pin[0][1]) {
		t.Errorf("tests/e2e.sh's e2e_ubuntu_release: %q, want one release-YYYYMMDD", pin)
	}
	if !regexp.MustCompile(`(?m)^export DEVVM_UBUNTU_RELEASE=\$e2e_ubuntu_release$`).MatchString(e2e) {
		t.Error("tests/e2e.sh doesn't export e2e_ubuntu_release as DEVVM_UBUNTU_RELEASE")
	}
	if !regexp.MustCompile(`(?m)^bump_e2e-ubuntu\(\) \{\n(?:  .*\n)*  new=\$\(fetch ` + regexp.QuoteMeta(DefaultURL) + `/ \|`).MatchString(bump) {
		t.Errorf("scripts/bump.sh's e2e-ubuntu doesn't list %s/", DefaultURL)
	}
	if !regexp.MustCompile(`set_var \$f e2e_ubuntu_release `).MatchString(bump) ||
		!regexp.MustCompile(`local f=tests/e2e\.sh new\n`).MatchString(bump) {
		t.Error("scripts/bump.sh's e2e-ubuntu doesn't move tests/e2e.sh's e2e_ubuntu_release")
	}
	for file, data := range map[string]string{"tests/e2e.sh": e2e, "scripts/bump.sh": bump} {
		for _, m := range regexp.MustCompile(`cloud-images\.ubuntu\.com/releases/([a-z]+)|/linux/ubuntu/dists/([a-z]+)/`).FindAllStringSubmatch(data, -1) {
			if s := m[1] + m[2]; s != Series {
				t.Errorf("%s names Ubuntu %s, not %s: %s", file, s, Series, m[0])
			}
		}
		for _, m := range regexp.MustCompile(`ubuntu:([0-9.]+)`).FindAllStringSubmatch(data, -1) {
			if m[1] != Version {
				t.Errorf("%s names ubuntu:%s, not %s", file, m[1], Version)
			}
		}
	}
}
