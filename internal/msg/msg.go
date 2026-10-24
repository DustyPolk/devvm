// Package msg holds every message devvm prints, as typed functions generated
// from locales/en.toml (NoVM(dir) for [no_vm]). It renders them in the
// user's language, but never prints: internal/ui does.
//
// With DEVVM_MESSAGES=names, a message renders as its ID and values in
// sorted key order (no_vm dir=/x), so tests never depend on wording or
// language.
package msg

//go:generate go run ./gen

import (
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github.com/w3cj/devvm/locales"
)

// DocsBase is the docs site's address; doc links are under it.
const DocsBase = "https://w3cj.github.io/devvm/docs/"

// M is a message with its values, made by the generated functions.
type M struct {
	id     string
	doc    string
	plural bool
	count  int
	vals   []val // in sorted key order
}

type val struct{ key, value string }

// spec is what the generated table says about a message.
type spec struct {
	doc    string
	plural bool
	fields []string // sorted; "count" when plural
}

// ID returns the message's ID, its table name in en.toml.
func (m M) ID() string { return m.id }

// String returns the message as devvm prints it: its text, then its doc
// link on a line of its own. With DEVVM_MESSAGES=names, its names instead.
func (m M) String() string {
	if namesMode() {
		return m.Names()
	}
	if u := m.DocURL(); u != "" {
		return m.Text() + "\n" + u
	}
	return m.Text()
}

// Error makes a message usable as an error. It's the same as String.
func (m M) Error() string { return m.String() }

// Names returns the message's ID and values: no_vm dir=/x.
func (m M) Names() string {
	var b strings.Builder
	b.WriteString(m.id)
	for _, v := range m.vals {
		b.WriteString(" " + v.key + "=" + v.value)
	}
	return b.String()
}

// Text returns the message's text in the user's language, falling back to
// English when it has no translation.
func (m M) Text() string {
	return m.textIn(loadBundle(), userLanguage(os.Getenv))
}

func (m M) textIn(b *i18n.Bundle, lang string) string {
	data := make(map[string]string, len(m.vals))
	for _, v := range m.vals {
		data[v.key] = v.value
	}
	lc := &i18n.LocalizeConfig{MessageID: m.id, TemplateData: data}
	if m.plural {
		lc.PluralCount = m.count
	}
	text, err := i18n.NewLocalizer(b, lang).Localize(lc)
	if text == "" && err != nil {
		// Only a message missing from en.toml gets here, which the
		// generated file rules out.
		return m.Names()
	}
	return text
}

// DocURL returns the address of the message's doc page, or "" when it has
// none.
func (m M) DocURL() string { return docURL(m.doc) }

// docURL turns "page#anchor" into the page's address on the docs site.
func docURL(doc string) string {
	if doc == "" {
		return ""
	}
	page, anchor, _ := strings.Cut(doc, "#")
	u := DocsBase + page + "/"
	if anchor != "" {
		u += "#" + anchor
	}
	return u
}

func namesMode() bool { return os.Getenv("DEVVM_MESSAGES") == "names" }

// userLanguage returns the language messages are shown in, from the first
// of LC_ALL, LC_MESSAGES and LANG that is set, as a BCP 47 tag ("de-DE"
// for de_DE.UTF-8). C and POSIX are English.
func userLanguage(getenv func(string) string) string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := getenv(name)
		if v == "" {
			continue
		}
		v, _, _ = strings.Cut(v, ".")
		v, _, _ = strings.Cut(v, "@")
		if v == "C" || v == "POSIX" {
			return "en"
		}
		return strings.ReplaceAll(v, "_", "-")
	}
	return "en"
}

// loadBundle returns every embedded locale, English first.
var loadBundle = sync.OnceValue(func() *i18n.Bundle {
	b, err := newBundle(locales.FS)
	if err != nil {
		// The files are embedded and checked by the tests.
		panic(err)
	}
	return b
})

// localeTable is one message in a locale file. doc is read from the
// generated table, so a translation's is ignored.
type localeTable struct {
	One   string `toml:"one"`
	Other string `toml:"other"`
	Doc   string `toml:"doc"`
}

// newBundle loads every *.toml file in fsys, named by language (en.toml).
// go-i18n's own parser refuses a table that mixes its keys with others
// (doc), so the files are read here.
func newBundle(fsys fs.FS) (*i18n.Bundle, error) {
	b := i18n.NewBundle(language.English)
	files, err := fs.Glob(fsys, "*.toml")
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		tag, err := language.Parse(strings.TrimSuffix(path.Base(file), ".toml"))
		if err != nil {
			return nil, err
		}
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, err
		}
		var tables map[string]localeTable
		if err := toml.Unmarshal(data, &tables); err != nil {
			return nil, &fs.PathError{Op: "parse", Path: file, Err: err}
		}
		msgs := make([]*i18n.Message, 0, len(tables))
		for id, t := range tables {
			msgs = append(msgs, &i18n.Message{ID: id, One: t.One, Other: t.Other})
		}
		if err := b.AddMessages(tag, msgs...); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// FromArgs makes a message from its ID and key=value arguments, checked
// against its fields: what devvm _msg prints. A plural message's count is
// given as count=N.
func FromArgs(id string, args []string) (M, error) {
	s, ok := specs[id]
	if !ok {
		return M{}, MsgUnknown(id)
	}
	given := map[string]string{}
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" {
			return M{}, MsgNotPair(a)
		}
		if _, dup := given[k]; dup {
			return M{}, MsgRepeatedValue(k)
		}
		given[k] = v
	}
	m := M{id: id, doc: s.doc, plural: s.plural}
	for _, f := range s.fields {
		v, ok := given[f]
		if !ok {
			return M{}, MsgMissingValue(f, id)
		}
		delete(given, f)
		if f == "count" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return M{}, MsgBadCount(v)
			}
			m.count, v = n, strconv.Itoa(n)
		}
		m.vals = append(m.vals, val{f, v})
	}
	if len(given) > 0 {
		return M{}, MsgUnexpectedValue(slices.Sorted(maps.Keys(given))[0], id)
	}
	return m, nil
}
