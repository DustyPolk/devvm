package msg

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/w3cj/devvm/locales"
)

// sample makes message id with a value for each field: the field's name,
// and count for a plural message.
func sample(t *testing.T, id string, count int) M {
	t.Helper()
	var args []string
	for _, f := range specs[id].fields {
		v := "v_" + f
		if f == "count" {
			v = strconv.Itoa(count)
		}
		args = append(args, f+"="+v)
	}
	m, err := FromArgs(id, args)
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return m
}

func TestLocalesLoad(t *testing.T) {
	b, err := newBundle(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	if tags := b.LanguageTags(); len(tags) == 0 || tags[0].String() != "en" {
		t.Errorf("languages %v: want English first", tags)
	}
}

// Every message renders in every language, with every value used.
func TestEveryMessageRenders(t *testing.T) {
	b := loadBundle()
	for id, s := range specs {
		for _, count := range []int{1, 2} {
			m := sample(t, id, count)
			for _, tag := range b.LanguageTags() {
				text := m.textIn(b, tag.String())
				if text == "" || strings.Contains(text, "<no value>") {
					t.Errorf("%s in %s: %q", id, tag, text)
				}
				for _, f := range s.fields {
					if f != "count" && !strings.Contains(text, "v_"+f) {
						t.Errorf("%s in %s: %q doesn't show %s", id, tag, text, f)
					}
				}
			}
			if !s.plural {
				break
			}
		}
	}
}

func TestValuesAsGiven(t *testing.T) {
	v := `x y $HOME "q" <b> {{.name}}`
	if got, want := MsgUnknown(v).Text(), "no message "+v; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNames(t *testing.T) {
	m := MsgMissingValue("dir", "no_vm")
	if got, want := m.Names(), "msg_missing_value key=dir name=no_vm"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := HomeNotAbsolute().Names(), "home_not_absolute"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	t.Setenv("DEVVM_MESSAGES", "names")
	if got, want := m.String(), m.Names(); got != want {
		t.Errorf("String in names mode: got %q, want %q", got, want)
	}
	if got, want := m.Line(), m.Names(); got != want {
		t.Errorf("Line in names mode: got %q, want %q", got, want)
	}
	if got, want := m.Error(), m.Names(); got != want {
		t.Errorf("Error in names mode: got %q, want %q", got, want)
	}
}

func TestStringAddsDocLink(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "")
	m := MsgUnknown("x")
	if got := m.String(); got != m.Text() {
		t.Errorf("no doc: got %q, want the text", got)
	}
	docs["msg_unknown"] = "images#where-state-lives"
	defer delete(docs, "msg_unknown")
	want := m.Text() + "\nhttps://w3cj.github.io/devvm/docs/images/#where-state-lives"
	if got := m.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := m.Line(); got != m.Text() {
		t.Errorf("Line: got %q, want the text without the link", got)
	}
}

func TestDocURL(t *testing.T) {
	for doc, want := range map[string]string{
		"":                         "",
		"images":                   "https://w3cj.github.io/devvm/docs/images/",
		"images#where-state-lives": "https://w3cj.github.io/devvm/docs/images/#where-state-lives",
		"cli/devvm_ls":             "https://w3cj.github.io/devvm/docs/cli/devvm_ls/",
	} {
		if got := docURL(doc); got != want {
			t.Errorf("docURL(%q) = %q, want %q", doc, got, want)
		}
	}
}

func TestUserLanguage(t *testing.T) {
	for _, tt := range []struct {
		env  map[string]string
		want string
	}{
		{nil, "en"},
		{map[string]string{"LANG": "de_DE.UTF-8"}, "de-DE"},
		{map[string]string{"LANG": "de_DE.UTF-8", "LC_MESSAGES": "fr_FR"}, "fr-FR"},
		{map[string]string{"LANG": "de_DE", "LC_MESSAGES": "fr_FR", "LC_ALL": "pt_BR.UTF-8"}, "pt-BR"},
		{map[string]string{"LANG": "sr_RS@latin"}, "sr-RS"},
		{map[string]string{"LANG": "de_DE", "LC_ALL": "C"}, "en"},
		{map[string]string{"LC_ALL": "POSIX"}, "en"},
		{map[string]string{"LC_ALL": "", "LANG": "ja_JP.UTF-8"}, "ja-JP"},
	} {
		if got := userLanguage(func(k string) string { return tt.env[k] }); got != tt.want {
			t.Errorf("%v: got %q, want %q", tt.env, got, tt.want)
		}
	}
}

// A translation is used for the messages it has; English for the rest.
// Plural forms follow the count.
func TestTranslationsAndPlurals(t *testing.T) {
	b, err := newBundle(fstest.MapFS{
		"en.toml": {Data: []byte(`
[t_vms]
one = "{{.count}} VM in {{.dir}}"
other = "{{.count}} VMs in {{.dir}}"
[t_plain]
other = "plain"
`)},
		"de.toml": {Data: []byte(`
[t_vms]
one = "{{.count}} VM in {{.dir}} (de)"
other = "{{.count}} VMs in {{.dir}} (de)"
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	vms := func(n int) M {
		return M{id: "t_vms", plural: true, count: n, vals: []val{{"count", strconv.Itoa(n)}, {"dir", "/x"}}}
	}
	for _, tt := range []struct {
		m          M
		lang, want string
	}{
		{vms(1), "en", "1 VM in /x"},
		{vms(2), "en", "2 VMs in /x"},
		{vms(1), "de-DE", "1 VM in /x (de)"},
		{vms(2), "de", "2 VMs in /x (de)"},
		{vms(2), "fr", "2 VMs in /x"},
		{M{id: "t_plain"}, "de", "plain"},
		{M{id: "t_missing"}, "en", "t_missing"},
	} {
		if got := tt.m.textIn(b, tt.lang); got != tt.want {
			t.Errorf("%s in %s: got %q, want %q", tt.m.Names(), tt.lang, got, tt.want)
		}
	}
}

func TestBundleErrors(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"bad TOML":      {"en.toml": {Data: []byte("[x\n")}},
		"bad file name": {"not a tag.toml": {Data: []byte("")}},
		"unknown key":   {"en.toml": {Data: []byte("[x]\nother = \"x\"\ndoc = \"images\"\n")}},
	} {
		if _, err := newBundle(fsys); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestFromArgs(t *testing.T) {
	specs["t_plural"] = spec{plural: true, fields: []string{"count", "dir"}}
	defer delete(specs, "t_plural")
	for _, tt := range []struct {
		id   string
		args []string
		want string // names of the message, or of the error
	}{
		{"msg_unknown", []string{"name=a b=c"}, "msg_unknown name=a b=c"},
		{"home_not_absolute", nil, "home_not_absolute"},
		{"msg_missing_value", []string{"name=n", "key=k"}, "msg_missing_value key=k name=n"},
		{"t_plural", []string{"dir=/x", "count=02"}, "t_plural count=2 dir=/x"},
		{"nope", nil, "msg_unknown name=nope"},
		{"msg_unknown", nil, "msg_missing_value key=name name=msg_unknown"},
		{"msg_unknown", []string{"name=a", "vm=b"}, "msg_unexpected_value key=vm name=msg_unknown"},
		{"msg_unknown", []string{"name=a", "name=b"}, "msg_repeated_value key=name"},
		{"msg_unknown", []string{"a"}, "msg_not_pair arg=a"},
		{"msg_unknown", []string{"=a"}, "msg_not_pair arg==a"},
		{"t_plural", []string{"dir=/x", "count=two"}, "msg_bad_count value=two"},
	} {
		m, err := FromArgs(tt.id, tt.args)
		got := m.Names()
		if err != nil {
			e, ok := errors.AsType[M](err)
			if !ok {
				t.Fatalf("%s %q: %v is not a message", tt.id, tt.args, err)
			}
			got = e.Names()
		}
		if got != tt.want {
			t.Errorf("%s %q: got %q, want %q", tt.id, tt.args, got, tt.want)
		}
	}
}
