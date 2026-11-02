package structure

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
)

// consts are a module's string constants, by "folder.Name".
type consts map[string]string

// stringConsts finds every string constant m declares with a literal.
func stringConsts(m *module) consts {
	c := consts{}
	for _, f := range m.files {
		for _, d := range f.ast.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, s := range gd.Specs {
				spec, ok := s.(*ast.ValueSpec)
				if !ok || len(spec.Values) != len(spec.Names) {
					continue
				}
				for i, name := range spec.Names {
					if v, ok := literal(spec.Values[i]); ok {
						c[f.dir+"."+name.Name] = v
					}
				}
			}
		}
	}
	return c
}

// literal is a string literal's value.
func literal(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	return v, err == nil
}

// importName is the name f imports the package p as, or "" when it
// doesn't.
func importName(f srcFile, p string) string {
	for _, imp := range f.ast.Imports {
		if v, err := strconv.Unquote(imp.Path.Value); err != nil || v != p {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return path.Base(p)
	}
	return ""
}

// value is a string expression's value in f: a literal, a constant of f's
// package, or one of a package of the module's that f imports.
func (c consts) value(m *module, f srcFile, e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		v, ok := c[f.dir+"."+x.Name]
		return v, ok
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		for _, imp := range f.ast.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			dir, inModule := strings.CutPrefix(p, m.path+"/")
			if err != nil || !inModule {
				continue
			}
			name := path.Base(p)
			if imp.Name != nil {
				name = imp.Name.Name
			}
			if name == pkg.Name {
				v, ok := c[dir+"."+x.Sel.Name]
				return v, ok
			}
		}
		return "", false
	}
	return literal(e)
}

// calls calls fn with every call in f.
func calls(f srcFile, fn func(*ast.CallExpr)) {
	ast.Inspect(f.ast, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			fn(call)
		}
		return true
	})
}

// checkCommands: outside tests, a host command that commands lists runs
// only in its package. An exec.Command or exec.CommandContext naming it,
// as a literal or a constant, runs it.
func checkCommands(m *module, r rules) []violation {
	var vs []violation
	c := stringConsts(m)
	for _, f := range m.files {
		exec := importName(f, "os/exec")
		if f.test || exec == "" {
			continue
		}
		at := map[string]int{exec + ".Command": 0, exec + ".CommandContext": 1} // the command's argument
		calls(f, func(call *ast.CallExpr) {
			i, ok := at[callName(call)]
			if !ok || len(call.Args) <= i {
				return
			}
			name, ok := c.value(m, f, call.Args[i])
			if owner, guarded := r.commands[name]; ok && guarded && f.dir != owner {
				vs = append(vs, violation{ruleOwners, m.pos(call), fmt.Sprintf("runs %s: run it through %s, the only package that does", name, owner)})
			}
		})
	}
	return vs
}

// funcName is the name of the function a call calls: F for F(), x.F() and
// x.y.F().
func funcName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

// checkEnv: outside tests, a DEVVM_ variable is read only in envOwner, or
// in the files envSettings lists for it, and a variable envOwners lists
// only in its file. A call to a function or a field
// named Getenv or LookupEnv, in any case, reads its argument, a literal
// or a constant. A listed file that no longer reads its variable fails.
func checkEnv(m *module, r rules) []violation {
	var vs []violation
	c := stringConsts(m)
	read := map[string]bool{} // "NAME file"
	for _, f := range m.files {
		if f.test {
			continue
		}
		calls(f, func(call *ast.CallExpr) {
			fn := strings.ToLower(funcName(call))
			if (fn != "getenv" && fn != "lookupenv") || len(call.Args) != 1 {
				return
			}
			name, ok := c.value(m, f, call.Args[0])
			if owner, owned := r.envOwners[name]; ok && owned {
				if f.name != owner {
					vs = append(vs, violation{ruleOwners, m.pos(call), fmt.Sprintf("reads %s: only %s reads it; take it from there", name, owner)})
				}
				return
			}
			if !ok || !strings.HasPrefix(name, "DEVVM_") || f.name == r.envOwner {
				return
			}
			if s, listed := r.envSettings[name]; listed && slices.Contains(s.files, f.name) {
				read[name+" "+f.name] = true
				return
			}
			vs = append(vs, violation{ruleOwners, m.pos(call), fmt.Sprintf("reads %s: a test build's stand-in is read in %s, a setting is listed in envSettings with why", name, r.envOwner)})
		})
	}
	for name, s := range r.envSettings {
		for _, file := range s.files {
			if !read[name+" "+file] {
				vs = append(vs, violation{ruleOwners, "envSettings", name + " isn't read in " + file + ": take it out"})
			}
		}
	}
	return vs
}

// checkLiterals: outside tests, a string literal holding a text that
// literals lists is in its owner's package: one anywhere else is a copy
// of what that package owns.
func checkLiterals(m *module, r rules) []violation {
	var vs []violation
	for _, f := range m.files {
		if f.test {
			continue
		}
		ast.Inspect(f.ast, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, ok := literal(lit)
			if !ok {
				return true
			}
			for _, text := range slices.Sorted(maps.Keys(r.literals)) {
				if o := r.literals[text]; strings.Contains(v, text) && f.dir != o.dir {
					vs = append(vs, violation{ruleOwners, m.pos(lit), fmt.Sprintf("holds %s: a copy of what %s owns, so use %s", text, o.dir, o.use)})
				}
			}
			return true
		})
	}
	return vs
}
