package daemon

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// A lifeline is the file of the process that runs a VM (Lima's hostagent's
// ha.pid, smolvm's agent.pid), its pid on the first line, which each start
// writes anew. A process that follows it runs while the file holds what it
// held when the process first saw it running, and that process runs.

// ReadLifeline returns what a lifeline holds, "" when it's gone.
func ReadLifeline(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the backend's own file for the VM
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(b)), err
}

// LifeRuns reports whether the process a lifeline's contents name (its
// first line) runs.
func LifeRuns(life string) bool {
	first, _, _ := strings.Cut(life, "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(first))
	return err == nil && Alive(pid)
}

// A Lifeline is one start's lifeline: its file, and what it held then.
type Lifeline struct {
	Path, Life string
}

// FollowLifeline returns the lifeline at path once it names a process that
// runs, looking every tick for up to wait (none: once), so a process
// started before its VM's finds it. What the file held when it first
// looked is an earlier start's, waited past; anything else whose process
// is gone (a VM that stopped between two looks) ends the wait. ok is false
// when the wait ends without one.
func FollowLifeline(path string, wait, tick time.Duration) (l Lifeline, ok bool) {
	deadline := time.Now().Add(wait)
	stale, _ := ReadLifeline(path)
	cur := stale
	for {
		switch {
		case LifeRuns(cur):
			return Lifeline{Path: path, Life: cur}, true
		case cur != "" && cur != stale, !time.Now().Add(tick).Before(deadline):
			return Lifeline{}, false
		}
		time.Sleep(tick)
		cur, _ = ReadLifeline(path)
	}
}

// Holds reports whether the lifeline still holds what it held, and its
// process runs.
func (l Lifeline) Holds() bool {
	cur, err := ReadLifeline(l.Path)
	return err == nil && cur == l.Life && LifeRuns(cur)
}

// Wait returns once the lifeline no longer holds, looking every tick.
func (l Lifeline) Wait(tick time.Duration) {
	for l.Holds() {
		time.Sleep(tick)
	}
}
