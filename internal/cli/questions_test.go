package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// A question's declared answer, as AnsweredBy found it in the code.
type declared struct{ answer, where string }

// declaredAnswers finds every question devvm's code makes, outside tests:
// each AnsweredBy's answer, which must be a string that isn't empty, and
// each RefusedWith's refusal, which must be a message. A msg.Question
// made any other way declares no answer, which it refuses too.
func declaredAnswers(t *testing.T) (answers []declared, refusals int) {
	t.Helper()
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			where := fset.Position(n.Pos()).String()
			if lit, ok := n.(*ast.CompositeLit); ok && isSel(lit.Type, "msg", "Question") {
				t.Errorf("%s: a msg.Question made without AnsweredBy or RefusedWith declares no answer", where)
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			switch sel.Sel.Name {
			case "AnsweredBy":
				s := ""
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					s, _ = strconv.Unquote(lit.Value)
				}
				if s == "" {
					t.Errorf("%s: AnsweredBy's answer isn't a string that names it", where)
					return true
				}
				answers = append(answers, declared{s, where})
			case "RefusedWith":
				if c, ok := call.Args[0].(*ast.CallExpr); !ok || !isSelOf(c.Fun, "msg") {
					t.Errorf("%s: RefusedWith's refusal isn't a message", where)
				}
				refusals++
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return answers, refusals
}

// isSel reports whether e is pkg.name.
func isSel(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name && isSelOf(sel, pkg)
}

// isSelOf reports whether e is a selector on the package pkg.
func isSelOf(e ast.Expr, pkg string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg
}

// answerParts splits an answer into what each part names: a flag, or a
// command with its flags and placeholders.
var answerParts = regexp.MustCompile(`, then |, and | and | or |, `)

// placeholder is an argument an answer names by what it stands for
// (KEY_FILE).
var placeholder = regexp.MustCompile(`^[A-Z][A-Z_]*$`)

// Every question devvm asks declares what answers it where devvm can't
// ask: a flag or command, which its refusal names, or a refusal of its
// own. Each flag an answer names is one of devvm's, on the command it's
// given with, and each command is one devvm has.
func TestQuestionsDeclareAnswers(t *testing.T) {
	answers, refusals := declaredAnswers(t)
	if len(answers) < 15 || refusals < 8 {
		t.Fatalf("found %d answers and %d refusals: the walk missed the code", len(answers), refusals)
	}
	u, _, _ := testUI()
	root := NewRoot(u)
	for _, a := range answers {
		for part := range strings.SplitSeq(answerParts.ReplaceAllString(a.answer, "\x00"), "\x00") {
			if err := checkAnswer(root, part); err != "" {
				t.Errorf("%s: %q: %s", a.where, a.answer, err)
			}
		}
	}
}

// checkAnswer is what's wrong with part of an answer, "" for nothing: a
// flag no command has, a command devvm doesn't have, or a flag its
// command doesn't take.
func checkAnswer(root *cobra.Command, part string) string {
	words := strings.Fields(part)
	switch {
	case len(words) == 0:
		return "an empty part"
	case strings.HasPrefix(words[0], "-") && len(words) == 1:
		if !anyHasFlag(root, words[0]) {
			return words[0] + " is no command's flag"
		}
		return ""
	case words[0] != "devvm":
		return "neither a flag nor a devvm command"
	}
	cmd, rest, err := root.Find(words[1:])
	if err != nil {
		return err.Error()
	}
	for _, w := range rest {
		switch {
		case strings.HasPrefix(w, "-"):
			if !hasFlag(cmd, w) {
				return cmd.CommandPath() + " takes no " + w
			}
		case !placeholder.MatchString(w):
			return cmd.CommandPath() + " has no command " + w
		}
	}
	return ""
}

// hasFlag reports whether cmd takes flag, as --name or -n.
func hasFlag(cmd *cobra.Command, flag string) bool {
	if name, ok := strings.CutPrefix(flag, "--"); ok {
		return cmd.Flags().Lookup(name) != nil || cmd.InheritedFlags().Lookup(name) != nil
	}
	short := strings.TrimPrefix(flag, "-")
	return cmd.Flags().ShorthandLookup(short) != nil || cmd.InheritedFlags().ShorthandLookup(short) != nil
}

// anyHasFlag reports whether cmd or a command under it takes flag.
func anyHasFlag(cmd *cobra.Command, flag string) bool {
	if hasFlag(cmd, flag) {
		return true
	}
	for _, c := range cmd.Commands() {
		if anyHasFlag(c, flag) {
			return true
		}
	}
	return false
}

// The check of an answer finds what isn't devvm's.
func TestCheckAnswer(t *testing.T) {
	u, _, _ := testUI()
	root := NewRoot(u)
	// d keeps the wrong ones out of TestCommandLines.
	d := "devvm "
	for part, bad := range map[string]bool{
		"--force": false, "devvm size -g": false, "devvm ssh-agent add KEY_FILE": false, "devvm": false,
		"--no-such-flag": true, d + "rm --no-such-flag": true, d + "size --stdin": true,
		d + "no-such-command": true, d + "tools frob": true, "git fetch": true, "": true,
	} {
		if got := checkAnswer(root, part); (got != "") != bad {
			t.Errorf("checkAnswer(%q) = %q, want bad %v", part, got, bad)
		}
	}
}
