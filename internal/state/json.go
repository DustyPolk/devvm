package state

import (
	"bytes"
	"encoding/json"
	"os"
)

// readJSON reads a JSON file only devvm writes into v. Fields v doesn't
// have are ignored, so a newer devvm's additions read; a file that isn't
// JSON is a CorruptError naming it. A missing file is an error that
// matches fs.ErrNotExist.
func readJSON(path string, v any) error {
	data, err := os.ReadFile(path) //nolint:gosec // paths come from paths.Dirs
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return &CorruptError{Path: path, Reason: "not JSON devvm reads: " + err.Error()}
	}
	return nil
}

// writeJSON writes v to path as indented JSON, one field per line, as
// writeFile does: atomically, the user's alone.
func writeJSON(path string, v any) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return writeFile(path, b.Bytes())
}
