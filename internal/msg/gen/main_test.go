package main

import (
	"os"
	"strings"
	"testing"
)

func TestGeneratedFileIsCurrent(t *testing.T) {
	src, err := os.ReadFile("../../../locales/en.toml")
	if err != nil {
		t.Fatal(err)
	}
	want, err := generate(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../messages_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Error("internal/msg/messages_gen.go is stale: run go generate ./internal/msg")
	}
}

func TestGenerate(t *testing.T) {
	code, err := generate([]byte(`
[no_vm]
other = "no VM for {{.dir}}: run devvm there to create one"
doc = "images#where-state-lives"

[vm_count]
one = "{{.vm_name}} has {{.count}} VM"
other = "{{.vm_name}} has {{.count}} VMs"

[ssh_key_id]
other = "key {{.key_id}} for {{.url}} on {{.vms}}"

[ready]
other = "ready"
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`func NoVM(dir string) M {`,
		`return M{id: "no_vm", doc: "images#where-state-lives", vals: []val{{"dir", dir}}}`,
		`func VMCount(count int, vmName string) M {`,
		`return M{id: "vm_count", plural: true, count: count, vals: []val{{"count", strconv.Itoa(count)}, {"vm_name", vmName}}}`,
		`func SSHKeyID(keyID string, url string, vms string) M {`,
		`func Ready() M {`,
		`return M{id: "ready"}`,
		`import "strconv"`,
		`"no_vm":      {doc: "images#where-state-lives", fields: []string{"dir"}},`,
		`"vm_count":   {plural: true, fields: []string{"count", "vm_name"}},`,
		`"ready":      {},`,
	} {
		if !strings.Contains(string(code), want) {
			t.Errorf("no %s in:\n%s", want, code)
		}
	}

	code, err = generate([]byte("[ready]\nother = \"ready\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(code), "strconv") {
		t.Errorf("strconv imported without plurals:\n%s", code)
	}
}

func TestGenerateRefuses(t *testing.T) {
	for _, tt := range []struct{ name, src, want string }{
		{"bad TOML", `[x`, "expected"},
		{"camelCase ID", "[noVM]\nother = \"x\"", "snake_case"},
		{"no other", "[x]\none = \"x\"", "no other text"},
		{"unknown key", "[x]\nother = \"x\"\ndescription = \"y\"", `unknown key "description"`},
		{"camelCase field", "[x]\nother = \"{{.vmName}}\"", `field "vmName" must be snake_case`},
		{"nested field", "[x]\nother = \"{{.vm.name}}\"", "only plain fields"},
		{"logic", "[x]\nother = \"{{if .a}}a{{end}}\"", "only plain fields"},
		{"a function", "[x]\nother = \"{{printf .a}}\"", "only plain fields"},
		{"a variable", "[x]\nother = \"{{$a := .a}}\"", "only plain fields"},
		{"bad template", "[x]\nother = \"{{.a\"", "unclosed action"},
		{"a Go keyword", "[x]\nother = \"{{.type}}\"", `field "type" is taken`},
		{"a Go type", "[x]\nother = \"{{.string}}\"", `field "string" is taken`},
		{"count without plurals", "[x]\nother = \"{{.count}}\"", "add a one text"},
		{"bad doc", "[x]\nother = \"x\"\ndoc = \"https://example.com\"", "not page#anchor"},
		{"same Go name", "[no_vm]\nother = \"x\"\n[no_v_m]\nother = \"y\"", "both NoVM"},
	} {
		_, err := generate([]byte(tt.src))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: got %v, want %q", tt.name, err, tt.want)
		}
	}
}
