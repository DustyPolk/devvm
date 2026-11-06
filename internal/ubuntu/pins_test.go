package ubuntu

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
// scripts/bump.sh: tests/e2e pins base to a dated release of Series
// (DEVVM_UBUNTU_RELEASE, which a test build pins in place of the newest),
// shaped as Finder.Release takes it, and bump.sh's e2e-ubuntu, which
// moves it, lists Series' releases where devvm does; every Ubuntu release
// either names is Series (Version).
func TestE2EUbuntuPins(t *testing.T) {
	const sandbox = "tests/e2e/sandbox_test.go"
	e2e, bump := repoFile(t, sandbox), repoFile(t, "scripts/bump.sh")
	pin := regexp.MustCompile(`(?m)^const e2eUbuntuRelease = "(.*)"$`).FindAllStringSubmatch(e2e, -1)
	if len(pin) != 1 || !releaseName.MatchString(pin[0][1]) {
		t.Errorf("%s's e2eUbuntuRelease: %q, want one release-YYYYMMDD", sandbox, pin)
	}
	if !strings.Contains(e2e, `set["DEVVM_UBUNTU_RELEASE"] = e2eUbuntuRelease`) {
		t.Errorf("%s doesn't set DEVVM_UBUNTU_RELEASE to e2eUbuntuRelease", sandbox)
	}
	if !regexp.MustCompile(`(?m)^bump_e2e-ubuntu\(\) \{\n(?:  .*\n)*  new=\$\(fetch ` + regexp.QuoteMeta(DefaultURL) + `/ \|`).MatchString(bump) {
		t.Errorf("scripts/bump.sh's e2e-ubuntu doesn't list %s/", DefaultURL)
	}
	if !regexp.MustCompile(`set_var \$f e2eUbuntuRelease `).MatchString(bump) ||
		!strings.Contains(bump, "local f="+sandbox+" new\n") {
		t.Errorf("scripts/bump.sh's e2e-ubuntu doesn't move %s's e2eUbuntuRelease", sandbox)
	}
	files := map[string]string{"scripts/bump.sh": bump}
	e2eFiles, err := filepath.Glob("../../tests/e2e/*.go")
	if err != nil || len(e2eFiles) == 0 {
		t.Fatalf("tests/e2e's files: %v", err)
	}
	for _, f := range e2eFiles {
		name := strings.TrimPrefix(f, "../../")
		files[name] = repoFile(t, name)
	}
	for file, data := range files {
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
