//go:build e2e

package e2e

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// SaidLine is whether one of out's lines is one of msg's lines (grep -qxF).
func SaidLine(out, msg string) bool {
	for l := range strings.SplitSeq(msg, "\n") {
		if l != "" && HasLine(out, l) {
			return true
		}
	}
	return false
}

// FirstLine and LastLine are a message's first and last lines (m … | head -1).
func FirstLine(msg string) string { l, _, _ := strings.Cut(msg, "\n"); return l }

func LastLine(msg string) string {
	ls := strings.Split(strings.TrimRight(msg, "\n"), "\n")
	return ls[len(ls)-1]
}

// Trim is a command's output as $(…) has it: its trailing newlines gone.
func Trim(out string) string { return strings.TrimRight(out, "\n") }

// FilesHave is whether a regular file under one of paths has text in
// it (grep -rqsF): a missing path has none.
func FilesHave(text string, paths ...string) bool {
	found := false
	for _, p := range paths {
		_ = filepath.WalkDir(p, func(f string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // what it can't read has none, as grep -s skips it
			}
			if found || !d.Type().IsRegular() {
				return nil
			}
			if strings.Contains(readText(f), text) {
				found = true
			}
			return nil
		})
	}
	return found
}

// Listed is whether limactl lists the VM (limactl list -q).
func Listed(s *S, vm string) bool {
	r, _ := s.Limactl("list", "-q")
	return HasLine(r.Stdout, vm)
}

// Locked is whether the file has macOS's user immutable flag, as .git
// protection sets it (ls -lO shows uchg).
func Locked(s *S, f string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	r, err := s.Run(Cmd{Args: []string{"ls", "-lOd", f}})
	return err == nil && strings.Contains(r.Stdout, "uchg")
}

// Procs is every host process's command line, one a line (ps -axww).
func Procs(s *S) []string {
	r, _ := s.Run(Cmd{Args: []string{"ps", "-axww", "-o", "command="}})
	return strings.Split(r.Stdout, "\n")
}

// Forwarders is the command lines of the VM's port forwarders here
// (devvm _ports -vm VM).
func Forwarders(s *S, vm string) []string {
	var out []string
	for _, l := range Procs(s) {
		if strings.Contains(l, "_ports -vm "+vm+" ") {
			out = append(out, l)
		}
	}
	return out
}

// DevvmIn runs devvm args in dir with input (none when empty), env on
// top of the sandbox's (devvmCmd's command).
func DevvmIn(s *S, dir, input string, env []string, args ...string) (Res, error) {
	return s.Run(devvmCmd(s, dir, input, env, args...))
}

// OwnVM makes the VM this run's: deleted at its end.
func OwnVM(s *S, vm string) {
	if vm != "" && !slices.Contains(s.sb.created, vm) {
		s.sb.created = append(s.sb.created, vm)
	}
}
