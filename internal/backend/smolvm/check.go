package smolvm

import (
	"cmp"
	"context"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// minMinor is the oldest smolvm 1.x devvm runs: the version it was tested
// with.
const minMinor = 32

// Check reports whether smolvm can run VMs here: the first of Problems.
func (s *Smolvm) Check(ctx context.Context) error {
	if ps := s.Problems(ctx); len(ps) > 0 {
		return ps[0].Msg
	}
	return nil
}

// Problems lists what smolvm needs that this host lacks: an arm64 Mac, or
// Linux on arm64 or x86_64 with /dev/kvm; smolvm 1.32 or newer. A fix is
// Homebrew's for smolvm, when it's installed.
func (s *Smolvm) Problems(ctx context.Context) []backend.Problem {
	host := s.OS + "/" + s.Arch
	switch {
	case s.OS == "darwin" && s.Arch == "arm64":
	case s.OS == "linux" && (s.Arch == "arm64" || s.Arch == "amd64"):
	default:
		return []backend.Problem{{Key: "os", Msg: msg.SmolvmHostUnsupported(host)}}
	}
	var ps []backend.Problem
	if p, ok := s.smolvmProblem(ctx); ok {
		ps = append(ps, p)
	}
	if s.OS == "linux" {
		if p, ok := s.kvmProblem(); ok {
			ps = append(ps, p)
		}
	}
	return ps
}

// smolvmProblem is a missing smolvm, one that doesn't run, or one older
// than 1.32.
func (s *Smolvm) smolvmProblem(ctx context.Context) (backend.Problem, bool) {
	brew := has("brew")
	problem := func(fix string, plain msg.M, fixed func(fix string) msg.M) (backend.Problem, bool) {
		if !brew {
			return backend.Problem{Key: "smolvm", Msg: plain}, true
		}
		return backend.Problem{Key: "smolvm", Fix: fix, Msg: fixed(fix)}, true
	}
	if !has("smolvm") {
		return problem("brew install smol-machines/tap/smolvm", msg.SmolvmMissing(), msg.SmolvmMissingFix)
	}
	out, err := s.Runner.Output(s.smolvm(ctx, "--version"))
	fields := strings.Fields(string(out))
	if err != nil || len(fields) == 0 {
		return problem("brew reinstall smolvm", msg.SmolvmBroken(), msg.SmolvmBrokenFix)
	}
	ver := fields[len(fields)-1]
	if !recent(ver) {
		return problem("brew upgrade smolvm", msg.SmolvmOld(ver), func(fix string) msg.M { return msg.SmolvmOldFix(fix, ver) })
	}
	return backend.Problem{}, false
}

// recent reports whether a version is 1.32 or newer.
func recent(v string) bool {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major > 1 || major == 1 && minor >= minMinor
}

// kvmProblem is a missing /dev/kvm, or one the user can't open.
func (s *Smolvm) kvmProblem() (backend.Problem, bool) {
	if _, err := os.Stat(s.KVM); err != nil {
		return backend.Problem{Key: "kvm", Msg: msg.NoKVM()}, true
	}
	f, err := os.OpenFile(s.KVM, os.O_RDWR, 0)
	if err == nil {
		_ = f.Close()
		return backend.Problem{}, false
	}
	name := s.Getenv("USER")
	if u, uerr := user.Current(); name == "" && uerr == nil {
		name = u.Username
	}
	fix := "sudo usermod -aG kvm " + cmp.Or(name, "$USER")
	return backend.Problem{Key: "kvm-access", Fix: fix, Msg: msg.KVMAccess(fix)}, true
}

// has reports whether name is a program on PATH.
func has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
