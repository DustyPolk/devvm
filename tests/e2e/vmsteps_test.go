//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The helpers the link and from sections share with the others: a line
// of a check's own in its log, a VM's state set by hand, a docker that
// isn't running and the newest run log.

// Note adds a line to the check's log, as a bash check's echo did: a
// failure shows it among the check's last lines.
func Note(s *S, format string, a ...any) {
	f, err := os.OpenFile(s.sb.logFile(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, format+"\n", a...)
}

// SetState sets key (dotted: net.tier) of the VM's vm.json to the JSON
// value js, keeping the rest, and writes the file back whole, mode 600:
// a change by hand, as nothing devvm runs would make it.
func SetState(p Paths, vm, key, js string) error {
	f := filepath.Join(p.VMDir(vm), "vm.json")
	b, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v map[string]any
	if err := d.Decode(&v); err != nil {
		return fmt.Errorf("%s: %w", f, err)
	}
	var val any
	d = json.NewDecoder(strings.NewReader(js))
	d.UseNumber()
	if err := d.Decode(&val); err != nil {
		return err
	}
	parts := strings.Split(key, ".")
	o := v
	for _, k := range parts[:len(parts)-1] {
		next, ok := o[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			o[k] = next
		}
		o = next
	}
	o[parts[len(parts)-1]] = val
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(f+".tmp", append(out, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(f+".tmp", f)
}

// NoDocker is a PATH with a docker first on it that's installed but not
// running: it logs what devvm asked it to DockerCalls, then fails as
// Docker's client does without its daemon. Registry images need no
// Docker, which the from section checks.
func NoDocker(s *S) ([]string, error) {
	dir := filepath.Join(s.sb.work, "nodocker")
	calls := DockerCalls(s)
	if !Exists(calls) {
		madeFile(s.sb, dir)
		madeFile(s.sb, calls)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		script := "#!/bin/sh\necho \"docker $*\" >> '" + calls + "'\necho \"Cannot connect to the Docker daemon\" >&2; exit 1\n"
		if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o700); err != nil { //nolint:gosec // G306: the stand-in docker runs
			return nil, err
		}
		if err := os.WriteFile(calls, nil, 0o600); err != nil {
			return nil, err
		}
	}
	path := ""
	for _, kv := range s.sb.env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = v
		}
	}
	return []string{"PATH=" + dir + ":" + path}, nil
}

// DockerCalls is the file NoDocker's docker logs its calls to.
func DockerCalls(s *S) string { return filepath.Join(s.sb.work, "docker-calls") }

// NewestLog is devvm's newest run log, the last devvm's: nothing when
// there's none.
func NewestLog(p Paths) string {
	m, _ := filepath.Glob(filepath.Join(p.Devvm, "logs", "runs", "*.log"))
	newest, at := "", int64(0)
	for _, f := range m {
		if fi, err := os.Stat(f); err == nil && fi.ModTime().UnixNano() > at {
			newest, at = f, fi.ModTime().UnixNano()
		}
	}
	return newest
}
