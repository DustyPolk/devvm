// Package state reads and writes the files devvm keeps its state in: each
// VM's state, image records and the config files.
//
// Most are key=value files: one pair per line, split at the first '=', so
// values may hold '=' but not newlines. Blank lines are skipped. Every write
// replaces the whole file atomically (a temporary file, then a rename), so a
// reader never sees half a file. Files are private to the user (0600), and
// so are the folders devvm creates for them (0700).
package state

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/paths"
)

// CorruptError is a state file devvm can't read: edited by hand, or damaged.
type CorruptError struct {
	Path   string
	Line   int // 1-based; 0 when the problem isn't on one line
	Reason string
}

func (e *CorruptError) Error() string {
	if e.Line == 0 {
		return fmt.Sprintf("%s: %s", e.Path, e.Reason)
	}
	return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Reason)
}

// kv is a key=value file's pairs, in file order.
type kv struct {
	keys []string
	vals map[string]string
}

func newKV() kv { return kv{vals: map[string]string{}} }

func (k *kv) get(key string) string { return k.vals[key] }

func (k *kv) set(key, val string) {
	if _, ok := k.vals[key]; !ok {
		k.keys = append(k.keys, key)
	}
	k.vals[key] = val
}

func (k *kv) bytes() []byte {
	var b bytes.Buffer
	for _, key := range k.keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(k.vals[key])
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// lines splits a file into lines, without their line endings.
func lines(data []byte) []string {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// parseKV reads a key=value file's contents. validKey says which keys the
// file may hold.
func parseKV(path string, data []byte, validKey func(string) bool) (kv, error) {
	k := newKV()
	for i, line := range lines(data) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		switch {
		case !ok:
			return kv{}, &CorruptError{path, i + 1, "not key=value"}
		case !validKey(key):
			return kv{}, &CorruptError{path, i + 1, fmt.Sprintf("unknown key %q", key)}
		}
		if _, dup := k.vals[key]; dup {
			return kv{}, &CorruptError{path, i + 1, fmt.Sprintf("%s is set twice", key)}
		}
		k.set(key, val)
	}
	return k, nil
}

// readKV reads a key=value file. A missing file is an error that matches
// fs.ErrNotExist.
func readKV(path string, validKey func(string) bool) (kv, error) {
	data, err := os.ReadFile(path) //nolint:gosec // paths come from paths.Dirs
	if err != nil {
		return kv{}, err
	}
	return parseKV(path, data, validKey)
}

// checkPair refuses a pair that wouldn't read back as written.
func checkPair(key, val string, validKey func(string) bool) error {
	if !validKey(key) {
		return fmt.Errorf("invalid state key %q", key)
	}
	if strings.ContainsAny(val, "\n\r") {
		return fmt.Errorf("state value for %s has a line break", key)
	}
	return nil
}

// MakePrivate makes devvm's data and config folders, and makes both the
// user's alone (mode 700) whatever mode they had, so nothing beneath them
// is open to others.
func MakePrivate(d paths.Dirs) error {
	for _, dir := range []string{d.Data, d.Config} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
			return err
		}
	}
	return nil
}

// WriteFile replaces path with data atomically, readable only by the
// user, creating its folder (0700) when it's missing.
func WriteFile(path string, data []byte) error { return writeFile(path, data) }

// writeFile replaces path with data atomically, creating its folder (0700)
// when it's missing.
func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// removeFile deletes path; a missing file is not an error.
func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// checkName refuses a VM name or image key that isn't a plain file name.
func checkName(what, name string) error {
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid %s %q", what, name)
	}
	return nil
}

// isWord reports whether s is a lowercase word: a-z first, then a-z, 0-9
// or _.
func isWord(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// oneOf returns a key check accepting only these keys.
func oneOf(keys ...string) func(string) bool {
	return func(k string) bool { return slices.Contains(keys, k) }
}
