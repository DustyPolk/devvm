package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/paths"
)

// The retention settings' keys, in the retention file (key=value lines).
// Every setting is off unless the file sets it.
const (
	RetainRunLogs      = "logs.runs"        // run logs kept, the newest; 20 when off
	RetainExportsAge   = "exports.age"      // exports older than this are deleted
	RetainExportsCount = "exports.count"    // exports kept, the newest
	RetainToolsAge     = "tools.unused-age" // current tool images no VM has for this long are deleted
	RetainOrphansAge   = "orphans.age"      // what devvm keeps of VMs gone from their backend, after this long
)

// RetentionKeys are the retention settings' keys, in the order devvm
// space shows them.
func RetentionKeys() []string {
	return []string{RetainRunLogs, RetainExportsAge, RetainExportsCount, RetainToolsAge, RetainOrphansAge}
}

// Retention is the retention file's settings: 0 is off.
type Retention struct {
	RunLogs, ExportsCount            int
	ExportsAge, ToolsAge, OrphansAge time.Duration
	// Raw is each setting as written, by key.
	Raw map[string]string
}

// Off reports whether no setting is on.
func (r Retention) Off() bool { return len(r.Raw) == 0 }

// ReadRetention reads the retention file. A missing or empty file turns
// every setting off; a line devvm can't read is an error naming the file
// and the line.
func ReadRetention(d paths.Dirs) (Retention, error) {
	r := Retention{Raw: map[string]string{}}
	data, err := os.ReadFile(d.Retention())
	if errors.Is(err, fs.ErrNotExist) {
		return r, nil
	} else if err != nil {
		return r, err
	}
	for i, line := range lines(data) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return r, &CorruptError{d.Retention(), i + 1, "not key=value"}
		}
		if _, dup := r.Raw[key]; dup {
			return r, &CorruptError{d.Retention(), i + 1, key + " is set twice"}
		}
		if err := r.set(key, val); err != nil {
			return r, &CorruptError{d.Retention(), i + 1, err.Error()}
		}
	}
	return r, nil
}

// set sets one setting from its value as written; "" leaves it off.
func (r *Retention) set(key, val string) error {
	if !slices.Contains(RetentionKeys(), key) {
		return fmt.Errorf("unknown key %q", key)
	}
	if val == "" {
		return nil
	}
	var err error
	switch key {
	case RetainRunLogs:
		r.RunLogs, err = count(val)
	case RetainExportsCount:
		r.ExportsCount, err = count(val)
	case RetainExportsAge:
		r.ExportsAge, err = ParseAge(val)
	case RetainToolsAge:
		r.ToolsAge, err = ParseAge(val)
	case RetainOrphansAge:
		r.OrphansAge, err = ParseAge(val)
	}
	if err != nil {
		return fmt.Errorf("%s=%s: %w", key, val, err)
	}
	r.Raw[key] = val
	return nil
}

// errCount is a count that isn't a whole number from 1.
var errCount = errors.New("a count is a whole number from 1 to 10000")

func count(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 10000 || strconv.Itoa(n) != s {
		return 0, errCount
	}
	return n, nil
}

// SetRetention sets retention settings, each KEY=VALUE, KEY= turning one
// off, and writes the file: a bad key or value is refused (an error that
// isn't a CorruptError) before anything is written.
func SetRetention(d paths.Dirs, pairs []string) (Retention, error) {
	r, err := ReadRetention(d)
	if err != nil {
		return r, err
	}
	next := Retention{Raw: map[string]string{}}
	vals := r.Raw
	for _, p := range pairs {
		key, val, ok := strings.Cut(p, "=")
		if !ok {
			return r, fmt.Errorf("%q: not key=value", p)
		}
		if !slices.Contains(RetentionKeys(), key) {
			return r, fmt.Errorf("unknown key %q", key)
		}
		vals[key] = val
	}
	var b strings.Builder
	for _, key := range RetentionKeys() {
		if err := next.set(key, vals[key]); err != nil {
			return r, err
		}
		if v := next.Raw[key]; v != "" {
			b.WriteString(key + "=" + v + "\n")
		}
	}
	if b.Len() == 0 {
		if err := os.Remove(d.Retention()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return r, err
		}
		return next, nil
	}
	return next, writeFile(d.Retention(), []byte(b.String()))
}
