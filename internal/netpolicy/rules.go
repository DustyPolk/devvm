package netpolicy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
)

// Rule is the rule for devvm network allow (deny false) or deny: rule as
// given, with deny's leading ! added. It refuses what the proxy wouldn't
// take as one rule, and an allow of a deny.
func Rule(rule string, deny bool) (string, error) {
	if deny && !strings.HasPrefix(rule, "!") {
		rule = "!" + rule
	}
	if !proxy.ValidRule(rule) || !deny && strings.HasPrefix(rule, "!") {
		return "", msg.NotARule(rule)
	}
	return rule, nil
}

// ruleOf is the rule on a policy file's line, without its comment.
func ruleOf(line string) string {
	r, _, _ := strings.Cut(line, "#")
	return strings.TrimSpace(r)
}

// readRules reads a policy file's lines; a missing file has none.
func readRules(path string) ([]string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // a policy file in devvm's folders
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil || len(b) == 0 {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"), nil
}

// writeRules replaces a policy file whole, so its VMs' proxies never read
// half of it, through a link if it is one.
func writeRules(path string, lines []string) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	return state.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"))
}

// AddRule adds rule to the policy file at path, unless a line has it
// already (in any case); false then.
func AddRule(path, rule string) (bool, error) {
	lines, err := readRules(path)
	if err != nil {
		return false, err
	}
	for _, l := range lines {
		if strings.EqualFold(ruleOf(l), rule) {
			return false, nil
		}
	}
	return true, writeRules(path, append(lines, rule))
}

// RemoveRule removes the lines with rule (in any case, comments aside)
// from the policy file at path; false when none has it.
func RemoveRule(path, rule string) (bool, error) {
	lines, err := readRules(path)
	if err != nil {
		return false, err
	}
	kept := lines[:0:0]
	for _, l := range lines {
		if !strings.EqualFold(ruleOf(l), strings.TrimSpace(rule)) {
			kept = append(kept, l)
		}
	}
	if len(kept) == len(lines) {
		return false, nil
	}
	return true, writeRules(path, kept)
}

// RuleFile is a policy file and its rules, comments left out.
type RuleFile struct {
	Path  string
	Rules []string
}

// ReadRules returns each of files that has rules, in order, with them.
func ReadRules(files []string) ([]RuleFile, error) {
	var out []RuleFile
	for _, f := range files {
		lines, err := readRules(f)
		if err != nil {
			return nil, err
		}
		var rules []string
		for _, l := range lines {
			if r := ruleOf(l); r != "" {
				rules = append(rules, r)
			}
		}
		if len(rules) > 0 {
			out = append(out, RuleFile{Path: f, Rules: rules})
		}
	}
	return out, nil
}

// WriteRules writes each policy file's rules to w: the file's path after
// #, then its rules, indented.
func WriteRules(w io.Writer, files []RuleFile) error {
	for _, f := range files {
		if _, err := fmt.Fprintf(w, "# %s\n", f.Path); err != nil {
			return err
		}
		for _, r := range f.Rules {
			if _, err := fmt.Fprintf(w, "  %s\n", r); err != nil {
				return err
			}
		}
	}
	return nil
}
