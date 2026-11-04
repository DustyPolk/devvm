package msg

import (
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

// Every question AnswerFor answers is a message, and an ID that isn't a
// question has no answer.
func TestAnswers(t *testing.T) {
	questions := []string{
		"net_ask", "use_parent_vm", "use_parent_vm_repo", "size_cpus_prompt", "size_memory_prompt",
		"features_help", "agents_help", "pick_new_help", "pick_vm_help", "network_default_help", "load_key", "restart_now", "add_it_now",
		"token_prompt", "continue", "gc_confirm", "gc_cache_confirm", "adopt_vm", "rm_confirm",
		"images_rm_confirm", "secret_rm_confirm", "remake_confirm", "remake_confirm_clean_home",
	}
	for _, id := range questions {
		if _, ok := specs[id]; !ok {
			t.Errorf("%s: no such message in locales/en.toml", id)
		}
		if AnswerFor(id) == "" {
			t.Errorf("%s: no answer", id)
		}
	}
	for _, id := range []string{"", "no_vm", "delete_parent_vm", "delete_anyway", "widen_parent_vm"} {
		if a := AnswerFor(id); a != "" {
			t.Errorf("AnswerFor(%q) = %q, want none", id, a)
		}
	}
}

// genDoc is a generated function's doc comment: its name and message ID.
var genDoc = regexp.MustCompile(`(?m)^// ([A-Z][A-Za-z0-9]*) is \[([a-z0-9_]+)\]`)

// promptIDs returns the questions devvm's code asks, by message ID, each
// with where: a message function's call given to a method named as one
// of the UI's prompts.
func promptIDs(t *testing.T) map[string]string {
	t.Helper()
	gen, err := os.ReadFile("messages_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, m := range genDoc.FindAllStringSubmatch(string(gen), -1) {
		ids[m[1]] = m[2]
	}
	prompts := []string{"Ask", "Confirm", "Answer", "Secret", "Pick", "PickOne", "PickParts"}
	found := map[string]string{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id := askedID(n, prompts, ids); id != "" {
				found[id] = fset.Position(n.Pos()).String()
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// askedID is the message ID n asks, when n calls one of prompts with a
// message function's call (ids: by function name) first: "" for any
// other node.
func askedID(n ast.Node, prompts []string, ids map[string]string) string {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return ""
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || !slices.Contains(prompts, sel.Sel.Name) {
		return ""
	}
	q, ok := call.Args[0].(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := q.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "msg" {
		return ""
	}
	return ids[sel.Sel.Name]
}

// Every question devvm asks has an answer that --no-input names
// (AnswerFor), but those only a terminal answers, each with why.
func TestPromptsAnswered(t *testing.T) {
	terminalOnly := map[string]string{
		"delete_parent_vm":       "it deletes a VM another folder's project may need",
		"remake_parent_vm_ask":   "it replaces the disk of a VM another folder's project may need",
		"delete_other_folder_vm": "it deletes a VM an earlier folder's project may need",
		"delete_anyway":          "it loses the changes in a clone VM's own repo",
		"fetch_before_delete":    "devvm rm asks it only in a terminal, before a clone VM's commits are lost",
		"widen_parent_vm":        "it gives another folder's VM more access",
		"net_unenforced_restart": "it's asked only in a terminal, and without one devvm stops the VM",
	}
	found := promptIDs(t)
	if len(found) < 10 {
		t.Fatalf("only %d questions found: %v", len(found), found)
	}
	for _, id := range slices.Sorted(maps.Keys(found)) {
		_, only := terminalOnly[id]
		switch answered := AnswerFor(id) != ""; {
		case answered && only:
			t.Errorf("%s (%s) has an answer: take it out of terminalOnly", id, found[id])
		case !answered && !only:
			t.Errorf("%s (%s) has no answer: name what answers it in AnswerFor, or add it to terminalOnly with why", id, found[id])
		}
	}
	for id := range terminalOnly {
		if _, ok := found[id]; !ok {
			t.Errorf("terminalOnly has %s, which nothing asks", id)
		}
	}
}
