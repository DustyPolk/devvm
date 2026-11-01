package msg

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// root is the repo's root, from this package's folder.
const root = "../.."

// generatedFuncs returns the functions in messages_gen.go.
func generatedFuncs(t *testing.T) map[string]bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "messages_gen.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]bool{}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			funcs[fd.Name.Name] = true
		}
	}
	return funcs
}

// Every message is called somewhere other than the generated file and the
// tests.
func TestEveryMessageIsUsed(t *testing.T) {
	unused := generatedFuncs(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "docs") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || d.Name() == "messages_gen.go" {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		inMsg := filepath.Dir(path) == filepath.Join(root, "internal", "msg")
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				if x, ok := n.X.(*ast.Ident); ok && x.Name == "msg" {
					delete(unused, n.Sel.Name)
				}
			case *ast.Ident:
				if inMsg {
					delete(unused, n.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name := range unused {
		t.Errorf("msg.%s is never used: call it, or delete its message from locales/en.toml", name)
	}
}

// TestDocLinks checks every doc link in docs.go: a message's key is a
// message in locales/en.toml, a command's is devvm's path, and its page
// and anchor exist in docs/content/docs. mise run lint runs it.
func TestDocLinks(t *testing.T) {
	dir := filepath.Join(root, "docs", "content", "docs")
	for _, err := range checkDocLinks(docs, specs, dir) {
		t.Error(err)
	}
	for _, err := range checkCommandDocs(commandDocs, dir) {
		t.Error(err)
	}
}

var commandRe = regexp.MustCompile(`^devvm( [a-z][a-z-]*)*$`)

// checkCommandDocs checks links (command path to "page#anchor") against
// the pages in dir. Which commands need one is internal/cli's to check.
func checkCommandDocs(links map[string]string, dir string) []error {
	var errs []error
	for _, path := range slices.Sorted(maps.Keys(links)) {
		doc := links[path]
		var err error
		switch {
		case !commandRe.MatchString(path):
			err = errors.New("not a command's path")
		case !docRe.MatchString(doc):
			err = errors.New("not page#anchor")
		case strings.HasPrefix(doc, "internals/"), strings.HasPrefix(doc, "contributing/"):
			err = errors.New("not a user page")
		default:
			err = checkDoc(dir, doc)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("commandDocs[%q] = %q: %w", path, doc, err))
		}
	}
	return errs
}

var docRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_/-]*(#[a-z0-9][a-z0-9_-]*)?$`)

// checkDocLinks checks links (message ID to "page#anchor") against the
// messages in ids and the pages in dir.
func checkDocLinks(links map[string]string, ids map[string]spec, dir string) []error {
	var errs []error
	for _, id := range slices.Sorted(maps.Keys(links)) {
		doc := links[id]
		var err error
		switch _, ok := ids[id]; {
		case !ok:
			err = errors.New("no such message in locales/en.toml")
		case !docRe.MatchString(doc):
			err = errors.New("not page#anchor")
		default:
			err = checkDoc(dir, doc)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("docs[%q] = %q: %w", id, doc, err))
		}
	}
	return errs
}

// checkDoc reports whether doc ("page#anchor") is a page in dir with that
// anchor.
func checkDoc(dir, doc string) error {
	page, anchor, _ := strings.Cut(doc, "#")
	anchors, err := pageAnchors(pageFile(dir, page))
	if err != nil {
		return err
	}
	if anchor != "" && !anchors[anchor] {
		return fmt.Errorf("%s has no heading #%s", page, anchor)
	}
	return nil
}

// pageFile returns the Markdown file of page in dir: page.md, or a
// section's page/_index.md, which Hugo serves at the section's address.
func pageFile(dir, page string) string {
	path := filepath.Join(dir, filepath.FromSlash(page))
	if _, err := os.Stat(path + ".md"); err != nil {
		if index := filepath.Join(path, "_index.md"); fileExists(index) {
			return index
		}
	}
	return path + ".md"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var (
	headingRe  = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*$`)
	customIDRe = regexp.MustCompile(`\s*\{#([^}]+)\}$`)
	notIDRe    = regexp.MustCompile(`[^\p{L}\p{N}_ -]`)
)

// pageAnchors returns the IDs Hugo gives a Markdown page's headings:
// {#custom} when given, else the text lowercased, punctuation dropped and
// spaces as hyphens. Headings in fenced code don't count.
func pageAnchors(path string) (map[string]bool, error) {
	f, err := os.Open(path) //nolint:gosec // a docs page named in docs.go
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no page %s", path)
		}
		return nil, err
	}
	defer f.Close()
	anchors := map[string]bool{}
	fence := ""
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			switch {
			case fence == "":
				fence = t[:3]
			case strings.HasPrefix(t, fence):
				fence = ""
			}
			continue
		}
		m := headingRe.FindStringSubmatch(line)
		if fence != "" || m == nil {
			continue
		}
		text := m[1]
		if c := customIDRe.FindStringSubmatch(text); c != nil {
			anchors[c[1]] = true
			continue
		}
		text = notIDRe.ReplaceAllString(strings.ToLower(text), "")
		anchors[strings.ReplaceAll(text, " ", "-")] = true
	}
	return anchors, s.Err()
}

func TestCheckDoc(t *testing.T) {
	dir := t.TempDir()
	page := "---\ntitle: T\n---\n\n## Where state lives\n\n### Sizes, ports & more\n\n## Custom {#short}\n\n" +
		"```sh\n# Not a heading\n```\n"
	for _, d := range []string{"cli", "backends"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"images.md", "cli/devvm.md", "backends/_index.md"} {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(page), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		doc string
		ok  bool
	}{
		{"images", true},
		{"images#where-state-lives", true},
		{"images#sizes-ports--more", true},
		{"images#short", true},
		{"cli/devvm#where-state-lives", true},
		{"backends", true},
		{"backends#where-state-lives", true},
		{"backends#nope", false},
		{"cli", false},
		{"images#custom", false},
		{"images#not-a-heading", false},
		{"images#nope", false},
		{"nope", false},
		{"nope#where-state-lives", false},
	} {
		if err := checkDoc(dir, tt.doc); (err == nil) != tt.ok {
			t.Errorf("checkDoc(%q) = %v, want ok %t", tt.doc, err, tt.ok)
		}
	}
}

func TestCheckDocLinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "images.md"), []byte("## Where state lives\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ids := map[string]spec{"no_vm": {}, "ready": {}, "gone": {}, "url": {}}
	links := map[string]string{
		"no_vm":   "images#where-state-lives",
		"ready":   "images",
		"gone":    "images#nope",
		"url":     "https://example.com",
		"missing": "images",
	}
	var got []string
	for _, err := range checkDocLinks(links, ids, dir) {
		got = append(got, err.Error())
	}
	want := []string{
		`docs["gone"] = "images#nope": images has no heading #nope`,
		`docs["missing"] = "images": no such message in locales/en.toml`,
		`docs["url"] = "https://example.com": not page#anchor`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheckCommandDocs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "images.md"), []byte("## Where state lives\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "internals"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internals", "x.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		"devvm":           "images",
		"devvm images rm": "images#where-state-lives",
		"devvm gc":        "images#nope",
		"images":          "images",
		"devvm stop":      "internals/x",
		"devvm ls":        "https://example.com",
	}
	var got []string
	for _, err := range checkCommandDocs(links, dir) {
		got = append(got, err.Error())
	}
	want := []string{
		`commandDocs["devvm gc"] = "images#nope": images has no heading #nope`,
		`commandDocs["devvm ls"] = "https://example.com": not page#anchor`,
		`commandDocs["devvm stop"] = "internals/x": not a user page`,
		`commandDocs["images"] = "images": not a command's path`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
