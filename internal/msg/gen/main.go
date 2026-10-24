// Command gen writes internal/msg/messages_gen.go from locales/en.toml: one
// Go function per message, taking the template's fields as parameters. Run
// it with go generate ./internal/msg.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"log"
	"os"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/BurntSushi/toml"
)

// in and out are relative to internal/msg, where go generate runs.
const (
	in  = "../../locales/en.toml"
	out = "messages_gen.go"
)

func main() {
	src, err := os.ReadFile(in)
	if err != nil {
		log.Fatal(err)
	}
	code, err := generate(src)
	if err != nil {
		log.Fatalf("%s: %v", in, err)
	}
	if err := os.WriteFile(out, code, 0o600); err != nil { //nolint:gosec // a fixed path
		log.Fatal(err)
	}
}

// message is one table of the message file.
type message struct {
	id     string
	other  string
	one    string
	doc    string
	fields []string // the template's fields, sorted; "count" when plural
}

func (m message) plural() bool { return m.one != "" }

var (
	idRe    = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	fieldRe = idRe
	docRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_/-]*(#[a-z0-9][a-z0-9_-]*)?$`)
)

// parseMessages reads and checks the message file.
func parseMessages(src []byte) ([]message, error) {
	var raw map[string]map[string]string
	if err := toml.Unmarshal(src, &raw); err != nil {
		return nil, err
	}
	var msgs []message
	for id, t := range raw {
		m, err := parseMessage(id, t)
		if err != nil {
			return nil, fmt.Errorf("[%s]: %w", id, err)
		}
		msgs = append(msgs, m)
	}
	slices.SortFunc(msgs, func(a, b message) int { return strings.Compare(a.id, b.id) })
	return msgs, nil
}

func parseMessage(id string, t map[string]string) (message, error) {
	if !idRe.MatchString(id) {
		return message{}, errors.New("the ID must be snake_case")
	}
	m := message{id: id}
	for k, v := range t {
		switch k {
		case "other":
			m.other = v
		case "one":
			m.one = v
		case "doc":
			m.doc = v
		default:
			return message{}, fmt.Errorf("unknown key %q (want other, one or doc)", k)
		}
	}
	if m.other == "" {
		return message{}, errors.New("no other text")
	}
	if m.doc != "" && !docRe.MatchString(m.doc) {
		return message{}, fmt.Errorf("doc %q is not page#anchor", m.doc)
	}
	fields := map[string]bool{}
	for _, text := range []string{m.other, m.one} {
		if err := templateFields(text, fields); err != nil {
			return message{}, err
		}
	}
	if m.plural() {
		fields["count"] = true
	} else if fields["count"] {
		return message{}, errors.New("{{.count}} is for plural messages: add a one text")
	}
	for f := range fields {
		if !fieldRe.MatchString(f) {
			return message{}, fmt.Errorf("field %q must be snake_case", f)
		}
		if p := goName(f, false); token.IsKeyword(p) || taken[p] {
			return message{}, fmt.Errorf("field %q is taken in Go; rename it", f)
		}
		m.fields = append(m.fields, f)
	}
	slices.Sort(m.fields)
	return m, nil
}

// templateFields adds the fields text uses ({{.dir}}) to fields. Only
// plain fields are allowed: messages hold no logic.
func templateFields(text string, fields map[string]bool) error {
	if text == "" {
		return nil
	}
	tmpl, err := template.New("").Parse(text)
	if err != nil {
		return err
	}
	for _, n := range tmpl.Root.Nodes {
		switch n := n.(type) {
		case *parse.TextNode:
		case *parse.ActionNode:
			f, ok := plainField(n)
			if !ok {
				return fmt.Errorf("%s: only plain fields like {{.dir}} are allowed", n)
			}
			fields[f] = true
		default:
			return fmt.Errorf("%s: only plain fields like {{.dir}} are allowed", n)
		}
	}
	return nil
}

func plainField(n *parse.ActionNode) (string, bool) {
	if len(n.Pipe.Decl) > 0 || len(n.Pipe.Cmds) != 1 || len(n.Pipe.Cmds[0].Args) != 1 {
		return "", false
	}
	f, ok := n.Pipe.Cmds[0].Args[0].(*parse.FieldNode)
	if !ok || len(f.Ident) != 1 {
		return "", false
	}
	return f.Ident[0], true
}

// taken are names a parameter would shadow in the generated functions.
var taken = map[string]bool{
	"any": true, "bool": true, "byte": true, "error": true, "false": true, "int": true,
	"nil": true, "string": true, "true": true, "len": true, "new": true, "make": true,
	"strconv": true, "val": true, "spec": true,
}

// initialisms are written in capitals in Go names: vm_name is vmName,
// no_vm is NoVM.
var initialisms = map[string]bool{
	"api": true, "ca": true, "cli": true, "cpu": true, "dns": true, "gc": true, "http": true,
	"https": true, "id": true, "ip": true, "json": true, "oci": true, "os": true, "pid": true,
	"sha": true, "ssh": true, "tcp": true, "tls": true, "tty": true, "ui": true, "uid": true,
	"url": true, "vm": true, "vms": true, "xdg": true,
}

// goName turns snake_case into a Go name, exported or not.
func goName(s string, exported bool) string {
	var b strings.Builder
	for i, w := range strings.Split(s, "_") {
		switch {
		case i == 0 && !exported:
			b.WriteString(w)
		case w == "vms":
			b.WriteString("VMs")
		case initialisms[w]:
			b.WriteString(strings.ToUpper(w))
		default:
			b.WriteString(strings.ToUpper(w[:1]) + w[1:])
		}
	}
	return b.String()
}

// generate returns messages_gen.go for the message file src.
func generate(src []byte) ([]byte, error) {
	msgs, err := parseMessages(src)
	if err != nil {
		return nil, err
	}
	funcs := map[string]string{}
	for _, m := range msgs {
		name := goName(m.id, true)
		if other, ok := funcs[name]; ok {
			return nil, fmt.Errorf("[%s] and [%s] are both %s in Go", other, m.id, name)
		}
		funcs[name] = m.id
	}

	var b bytes.Buffer
	b.WriteString("// Code generated by go generate ./internal/msg from locales/en.toml. DO NOT EDIT.\n\n")
	b.WriteString("package msg\n\n")
	if slices.ContainsFunc(msgs, message.plural) {
		b.WriteString("import \"strconv\"\n\n")
	}
	for _, m := range msgs {
		writeFunc(&b, m)
	}
	b.WriteString("// specs describes every message, by ID.\nvar specs = map[string]spec{\n")
	for _, m := range msgs {
		var s []string
		if m.doc != "" {
			s = append(s, fmt.Sprintf("doc: %q", m.doc))
		}
		if m.plural() {
			s = append(s, "plural: true")
		}
		if len(m.fields) > 0 {
			s = append(s, fmt.Sprintf("fields: %#v", m.fields))
		}
		fmt.Fprintf(&b, "\t%q: {%s},\n", m.id, strings.Join(s, ", "))
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

func writeFunc(b *bytes.Buffer, m message) {
	var params, vals []string
	for _, f := range m.fields {
		p := goName(f, false)
		if f == "count" {
			params = append(params, p+" int")
			vals = append(vals, fmt.Sprintf("{%q, strconv.Itoa(%s)}", f, p))
			continue
		}
		params = append(params, p+" string")
		vals = append(vals, fmt.Sprintf("{%q, %s}", f, p))
	}
	fmt.Fprintf(b, "// %s is [%s]: %q\n", goName(m.id, true), m.id, m.other)
	fmt.Fprintf(b, "func %s(%s) M {\n", goName(m.id, true), strings.Join(params, ", "))
	fmt.Fprintf(b, "\treturn M{id: %q", m.id)
	if m.doc != "" {
		fmt.Fprintf(b, ", doc: %q", m.doc)
	}
	if m.plural() {
		b.WriteString(", plural: true, count: count")
	}
	if len(vals) > 0 {
		fmt.Fprintf(b, ", vals: []val{%s}", strings.Join(vals, ", "))
	}
	b.WriteString("}\n}\n\n")
}
