package netpolicy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
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
	for _, l := range proxy.ParsePolicy(strings.Join(lines, "\n")) {
		if strings.EqualFold(l.Text, rule) {
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
	drop := map[int]bool{}
	for _, l := range proxy.ParsePolicy(strings.Join(lines, "\n")) {
		drop[l.N] = strings.EqualFold(l.Text, strings.TrimSpace(rule))
	}
	kept := lines[:0:0]
	for i, l := range lines {
		if !drop[i+1] {
			kept = append(kept, l)
		}
	}
	if len(kept) == len(lines) {
		return false, nil
	}
	return true, writeRules(path, kept)
}

// RuleFile is a policy file and its lines that hold more than a comment,
// each marked whether it's a rule (proxy.ParsePolicy).
type RuleFile struct {
	Path  string
	Lines []proxy.PolicyLine
}

// Rules are the file's rules, as written, comments left out.
func (f RuleFile) Rules() []string {
	var out []string
	for _, l := range f.Lines {
		if l.OK {
			out = append(out, l.Text)
		}
	}
	return out
}

// NotRules are the file's lines that aren't rules, which the proxy skips.
func (f RuleFile) NotRules() []proxy.PolicyLine {
	var out []proxy.PolicyLine
	for _, l := range f.Lines {
		if !l.OK {
			out = append(out, l)
		}
	}
	return out
}

// ReadRules returns each of files that holds more than comments, in
// order, with its lines.
func ReadRules(files []string) ([]RuleFile, error) {
	var out []RuleFile
	for _, f := range files {
		lines, err := readRules(f)
		if err != nil {
			return nil, err
		}
		if pl := proxy.ParsePolicy(strings.Join(lines, "\n")); len(pl) > 0 {
			out = append(out, RuleFile{Path: f, Lines: pl})
		}
	}
	return out, nil
}

// notRuleMark follows a line that isn't a rule in WriteRules' output.
const notRuleMark = "  # not a rule, skipped"

// WriteRules writes each policy file's rules to w: the file's path after
// #, then its lines, indented, each that isn't a rule marked so, with its
// line number.
func WriteRules(w io.Writer, files []RuleFile) error {
	for _, f := range files {
		if _, err := fmt.Fprintf(w, "# %s\n", f.Path); err != nil {
			return err
		}
		for _, l := range f.Lines {
			text, mark := l.Text, ""
			if !l.OK {
				// Anything but a rule may hold control characters: dropped.
				text, mark = msg.OneLine(l.Text), notRuleMark+" (line "+strconv.Itoa(l.N)+")"
			}
			if _, err := fmt.Fprintf(w, "  %s%s\n", text, mark); err != nil {
				return err
			}
		}
	}
	return nil
}

// NotRules returns doctor's warnings (PolicyWarnings) for the policy files
// you edit: every VM's, your layers' and each VM's own.
func NotRules(d paths.Dirs) []msg.M {
	files := []string{d.Policy()}
	for _, pattern := range []string{filepath.Join(d.Layers(), "*.policy"), filepath.Join(d.VMs(), "*.policy")} {
		m, _ := filepath.Glob(pattern)
		files = append(files, m...)
	}
	return PolicyWarnings(files)
}

// PolicyWarnings returns a warning for each line of files that isn't a
// rule: one the proxy skips, or a broken deny (proxy.BrokenDeny), which
// makes the proxies that read it deny everything. A file it can't read is
// left out.
func PolicyWarnings(files []string) []msg.M {
	var out []msg.M
	for _, f := range files {
		rs, err := ReadRules([]string{f})
		if err != nil {
			continue
		}
		for _, rf := range rs {
			for _, l := range rf.NotRules() {
				n := strconv.Itoa(l.N)
				if proxy.BrokenDeny(l) {
					out = append(out, msg.PolicyBrokenDeny(f, n, l.Text))
				} else {
					out = append(out, msg.PolicyNotRule(f, n, l.Text))
				}
			}
		}
	}
	return out
}
