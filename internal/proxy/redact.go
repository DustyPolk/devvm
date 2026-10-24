package proxy

// Agents' settings files copied into VMs with network policy (Codex's config.toml, opencode's
// opencode.json, Claude Code's settings.json) can hold keys: an MCP server's headers or env, a
// provider's apiKey. The proxy can't put those in (it only knows the logins), so the VM's copy gets
// "devvm-redacted" for every value whose key looks like a secret's.
//
//   devvm _proxy -redact json|toml < FILE > COPY   (the redacted keys' paths on stderr, one per line)
//
// What's kept although its key matches: values that point elsewhere rather than hold a secret (an
// env var's name, {env:X} or ${X}, a path), and non-strings.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

var (
	// The key names: *key*, *token*, *secret*, *password*, Authorization (any case).
	redactName = regexp.MustCompile(`(?i)(key|token|secret|passw|^authorization$|^proxy-authorization$|^cookie$)`)
	envName    = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	envRef     = regexp.MustCompile(`^(\{(env|file):[^}]*\}|\$\{?[A-Za-z_][A-Za-z0-9_]*\}?)$`)
)

// redactKeep: a value that names where a secret is rather than holding it.
func redactKeep(v string) bool {
	switch {
	case v == "" || v == "devvm-redacted":
		return true
	case envName.MatchString(v): // an env var's name (Codex's env_key)
		return true
	case envRef.MatchString(v):
		return true
	case strings.HasPrefix(v, "/") || strings.HasPrefix(v, "~/") || strings.HasPrefix(v, "./"):
		return true // a path: a helper command or a key file the VM doesn't have
	}
	return false
}

func redactCmd(format string, in io.Reader, out, names io.Writer) error {
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	var paths []string
	switch format {
	case "json":
		var v any
		if err := json.Unmarshal(stripJSONC(data), &v); err != nil {
			return fmt.Errorf("not JSON: %w", err)
		}
		v = redactJSON(v, "", &paths)
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		if _, err := out.Write(append(b, '\n')); err != nil {
			return err
		}
	case "toml":
		s, err := redactTOML(string(data), &paths)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(out, s); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown format %q", format)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintln(names, p)
	}
	return nil
}

func redactJSON(v any, path string, paths *[]string) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			p := k
			if path != "" {
				p = path + "." + k
			}
			if s, ok := x.(string); ok && redactName.MatchString(k) && !redactKeep(s) {
				t[k] = "devvm-redacted"
				*paths = append(*paths, p)
				continue
			}
			t[k] = redactJSON(x, p, paths)
		}
	case []any:
		for i, x := range t {
			t[i] = redactJSON(x, fmt.Sprintf("%s[%d]", path, i), paths)
		}
	}
	return v
}

// stripJSONC: opencode's files may be JSONC: // and /* */ comments, and trailing commas.
func stripJSONC(b []byte) []byte {
	var out []byte
	inStr, esc := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inStr {
			out = append(out, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i+1 < len(b) && (b[i] != '*' || b[i+1] != '/') {
				i++
			}
			i++
		default:
			out = append(out, c)
		}
	}
	// Trailing commas: a comma followed (after whitespace) by } or ].
	return regexp.MustCompile(`,(\s*[}\]])`).ReplaceAll(out, []byte("$1"))
}

// TOML, line by line (Codex's config.toml): `key = "value"` lines, and `{ key = "value", ... }` inline
// tables, under [table] headers. Multi-line strings are refused rather than copied unread.
var (
	tomlTable   = regexp.MustCompile(`^\s*\[\[?\s*([^\]]+?)\s*\]\]?\s*(#.*)?$`)
	tomlKV      = regexp.MustCompile(`^(\s*)((?:"[^"]*"|'[^']*'|[A-Za-z0-9_.\-]+))(\s*=\s*)(.*)$`)
	tomlString  = regexp.MustCompile(`^("(?:[^"\\]|\\.)*"|'[^']*')(.*)$`)
	tomlStrings = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'[^']*'`)
	tomlInline  = regexp.MustCompile(`((?:"[^"]*"|'[^']*'|[A-Za-z0-9_\-]+))(\s*=\s*)("(?:[^"\\]|\\.)*"|'[^']*')`)
)

func tomlUnquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

func redactTOML(s string, paths *[]string) (string, error) {
	var b strings.Builder
	table := ""
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, `"""`) || strings.Contains(line, `'''`) {
			return "", fmt.Errorf("multi-line strings aren't supported (line %q)", line)
		}
		if m := tomlTable.FindStringSubmatch(line); m != nil && !strings.Contains(line, "=") {
			table = m[1]
			b.WriteString(line + "\n")
			continue
		}
		m := tomlKV.FindStringSubmatch(line)
		if m == nil {
			b.WriteString(line + "\n")
			continue
		}
		key, path := tomlUnquote(m[2]), tomlUnquote(m[2])
		if table != "" {
			path = table + "." + key
		}
		val := m[4]
		if sm := tomlString.FindStringSubmatch(val); sm != nil {
			if last := key[strings.LastIndex(key, ".")+1:]; redactName.MatchString(last) && !redactKeep(tomlUnquote(sm[1])) {
				val = `"devvm-redacted"` + sm[2]
				*paths = append(*paths, path)
			}
		} else if last := key[strings.LastIndex(key, ".")+1:]; last == "args" && strings.HasPrefix(strings.TrimSpace(val), "[") {
			// An MCP server's arguments: as in ~/.claude.json (scrubArgs).
			var as []any
			for _, q := range tomlStrings.FindAllString(val, -1) {
				as = append(as, tomlUnquote(q))
			}
			if s, changed := scrubArgs(as); changed {
				b, _ := json.Marshal(s)
				val = string(b)
				*paths = append(*paths, path)
			}
		} else if strings.HasPrefix(strings.TrimSpace(val), "{") {
			val = tomlInline.ReplaceAllStringFunc(val, func(kv string) string {
				p := tomlInline.FindStringSubmatch(kv)
				if k := tomlUnquote(p[1]); redactName.MatchString(k) && !redactKeep(tomlUnquote(p[3])) {
					*paths = append(*paths, path+"."+k)
					return p[1] + p[2] + `"devvm-redacted"`
				}
				return kv
			})
		}
		b.WriteString(m[1] + m[2] + m[3] + val + "\n")
	}
	return b.String(), sc.Err()
}
