package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/paths"
)

// exportName is an export's name in the exports folder, as devvm export
// and devvm rm write it: <vm>-<date>-<time>.tar.gz. Nothing else there
// is one: an export being written (.devvm-export-…) or the user's own.
var exportName = regexp.MustCompile(`^(devvm-.+-[0-9a-f]{14})-(\d{8}-\d{6})\.tar\.gz$`)

// savedExport is one of the exports in devvm's exports folder.
type savedExport struct {
	Path string
	VM   string    // the VM it's of
	At   time.Time // when it was written, from its name
	Size int64
}

// exports lists the exports in devvm's exports folder, oldest first: only
// plain files named as devvm names them.
func exports(d paths.Dirs) ([]savedExport, error) {
	entries, err := os.ReadDir(d.Exports())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []savedExport
	for _, e := range entries {
		m := exportName.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		at, err := time.ParseInLocation("20060102-150405", m[2], time.Local)
		if err != nil {
			continue
		}
		p := filepath.Join(d.Exports(), e.Name())
		out = append(out, savedExport{Path: p, VM: m[1], At: at, Size: du(p)})
	}
	slices.SortFunc(out, func(a, b savedExport) int { return a.At.Compare(b.At) })
	return out, nil
}

// olderThan returns the exports written at least age before now; all of
// them when age is 0.
func olderThan(es []savedExport, age time.Duration, now time.Time) []savedExport {
	if age == 0 {
		return es
	}
	var out []savedExport
	for _, e := range es {
		if now.Sub(e.At) >= age {
			out = append(out, e)
		}
	}
	return out
}

// exportPaths are these exports' files.
func exportPaths(es []savedExport) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Path)
	}
	return out
}

// exportsSize is the disk these exports take.
func exportsSize(es []savedExport) int64 {
	var n int64
	for _, e := range es {
		n += e.Size
	}
	return n
}

// exportDate is how an export's list shows its date.
func exportDate(t time.Time) string { return strings.TrimSpace(t.Format("2006-01-02 15:04")) }
