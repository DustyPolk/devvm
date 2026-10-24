package tools

import (
	"bufio"
	"bytes"
	"strings"
)

// Lock is a mise lock file as mise writes it: an entry per tool version,
// [[tools.TOOL]], followed by a [tools.TOOL."platforms.PLATFORM"] table per
// platform with its download's URL and checksum. It's read line by line,
// as mise writes it, so a tool's entry is hashed exactly as written.
type Lock []byte

func lines(data []byte) []string {
	var out []string
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Buffer(nil, 1<<20)
	for s.Scan() {
		out = append(out, s.Text())
	}
	return out
}

func header(tool string) string { return "[[tools." + tool + "]]" }

// Entry returns a tool's entries: from its [[tools.TOOL]] line to the next
// entry of another tool, each line ending in a newline. Empty when the lock
// has none.
func (l Lock) Entry(tool string) []byte {
	var b bytes.Buffer
	in := false
	for _, line := range lines(l) {
		switch {
		case line == header(tool):
			in = true
		case strings.HasPrefix(line, "[["):
			in = false
		}
		if in {
			b.WriteString(line + "\n")
		}
	}
	return b.Bytes()
}

// field returns the first value of a key = "value" line in an entry,
// without its quotes.
func field(entry []byte, key string) string {
	for _, line := range lines(entry) {
		if v, ok := strings.CutPrefix(line, key+" = "); ok {
			return strings.ReplaceAll(v, `"`, "")
		}
	}
	return ""
}

// Version returns a tool's locked version: its first entry's.
func (l Lock) Version(tool string) string { return field(l.Entry(tool), "version") }

// VersionEntry returns the entry for one of a tool's versions, empty when
// the lock has none (a lock can hold several).
func (l Lock) VersionEntry(tool, version string) []byte {
	var b bytes.Buffer
	in, keep := false, false
	want := `version = "` + version + `"`
	for _, line := range lines(l) {
		if strings.HasPrefix(line, "[[") {
			if keep {
				break
			}
			in = line == header(tool)
			b.Reset()
		}
		if in {
			b.WriteString(line + "\n")
			if line == want {
				keep = true
			}
		}
	}
	if !keep {
		return nil
	}
	return b.Bytes()
}

// HasDownload reports whether a tool's first entry has a download (a URL,
// not a build from source) for every one of the platforms. Rust needs
// none: rustup installs it.
func (l Lock) HasDownload(tool string, platforms []string) bool {
	if tool == "rust" {
		return true
	}
	ls := lines(l.Entry(tool))
	for _, p := range platforms {
		table := `[tools.` + tool + `."platforms.` + p + `"]`
		in, url, source, entries := false, false, false, 0
		for _, line := range ls {
			if strings.HasPrefix(line, "[[") {
				if entries++; entries > 1 {
					break
				}
			}
			switch {
			case line == table:
				in = true
				continue
			case strings.HasPrefix(line, "["):
				in = false
			}
			if in && strings.HasPrefix(line, "url = ") {
				url = true
			}
			if in && line == `install = "source"` {
				source = true
			}
		}
		if !url || source {
			return false
		}
	}
	return true
}

// replace returns the lock with a tool's entries replaced by entry, where
// the first of them was.
func (l Lock) replace(tool string, entry []byte) Lock {
	var b bytes.Buffer
	skip, done := false, false
	for _, line := range lines(l) {
		if strings.HasPrefix(line, "[[") {
			skip = line == header(tool)
			if skip && !done {
				b.Write(entry)
				done = true
			}
		}
		if !skip {
			b.WriteString(line + "\n")
		}
	}
	return Lock(b.Bytes())
}

// withSpecifiers returns an entry with its specifiers line set to line.
func withSpecifiers(entry []byte, line string) []byte {
	var b bytes.Buffer
	for _, l := range lines(entry) {
		if strings.HasPrefix(l, "specifiers = ") {
			l = line
		}
		b.WriteString(l + "\n")
	}
	return b.Bytes()
}

// configTools reads the [tools] table of a mise.toml: each tool and its
// version as given ("24", "latest"), in order.
func configTools(data []byte) (names []string, versions map[string]string) {
	versions = map[string]string{}
	in := false
	for _, line := range lines(data) {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "["):
			in = t == "[tools]"
			continue
		case !in || t == "" || strings.HasPrefix(t, "#"):
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		if i := strings.Index(v, "#"); i >= 0 {
			v = v[:i]
		}
		k = strings.TrimSpace(k)
		names = append(names, k)
		versions[k] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return names, versions
}
