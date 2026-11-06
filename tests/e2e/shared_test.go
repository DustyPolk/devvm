//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Golden captures a golden fixture with -e2e.golden: what args printed,
// output and errors, as testdata/golden/<os>/name in the checkout, this
// machine's home, user name, full name and timezone replaced and a VM's
// environment made up, as tests/e2e.sh's golden does. A capture isn't a
// check, so a run without -e2e.golden runs as many checks; one that fails
// says so and keeps the fixture it had.
func (s *S) Golden(name string, args ...string) {
	if !s.sb.o.golden {
		return
	}
	dir := filepath.Join(s.sb.root, "testdata", "golden", runtime.GOOS)
	r, err := s.Run(Cmd{Args: args})
	if err == nil {
		err = os.MkdirAll(dir, 0o700)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, name), []byte(anonymous(r.Out, os.Getenv("HOME"))), 0o600)
	}
	if err != nil {
		s.r.out.line("golden: capturing %s failed", name)
		return
	}
	s.r.out.line("golden: %s", name)
}

// anonymous is out with home as /home/me, this user's name as me, and
// Lima's comment, timezone and a VM's environment made up.
func anonymous(out, home string) string {
	if home != "" {
		out = strings.ReplaceAll(out, home, "/home/me")
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		out = regexp.MustCompile(`\b`+regexp.QuoteMeta(u.Username)+`\b`).ReplaceAllString(out, "me")
	}
	out = regexp.MustCompile(`"comment":"[^"]*"`).ReplaceAllString(out, `"comment":"me"`)
	out = regexp.MustCompile(`"timezone":"[^"]*"`).ReplaceAllString(out, `"timezone":"UTC"`)
	return regexp.MustCompile(`"env":\{[^{}]*\}`).ReplaceAllString(out, `"env":{"NAME":"value"}`)
}

// GoE2E runs the Go test test in internal/pkg from the checkout, with
// vars (VAR=VALUE) in its environment, so no value is in a process's
// arguments: the tests that need a real VM or desktop, which skip unless
// their DEVVM_E2E_* variables are set. It fails unless test passed.
func (s *S) GoE2E(test, pkg string, vars ...string) error {
	args := []string{"go", "test", "-count=1", "-v", "-run", "^" + test + "$", "./internal/" + pkg}
	if _, err := exec.LookPath("go"); err != nil {
		args = append([]string{"mise", "exec", "--"}, args...)
	}
	r, err := s.Run(Cmd{Args: args, Dir: s.sb.root, Env: append([]string{"CGO_ENABLED=0"}, vars...)})
	switch {
	case err != nil:
		return err
	case !strings.Contains(r.Out, "--- PASS: "+test+" ("):
		return fmt.Errorf("go test %s didn't pass (did it skip?)", test)
	}
	return nil
}
