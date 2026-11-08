package netpolicy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/proxy"
	"github.com/devvmhq/devvm/internal/state"
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

// readRules reads a policy file's lines; a missing file has none. It reads
// it as the proxy does (proxy.ReadPolicy), so a FIFO or a device there
// can't hang devvm network or doctor.
func readRules(path string) ([]string, error) {
	b, err := proxy.ReadPolicy(path)
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

// RuleFile is a policy and its lines that hold more than a comment, each
// marked whether it's a rule (proxy.ParsePolicy): a built-in one, by its
// name in devvm's files, or one of the user's files, by its path.
type RuleFile struct {
	Path    string // the file's path, or the built-in policy's name
	Builtin bool
	// DeniesOnly is a file whose denies alone apply (PolicySet.Denies):
	// its Lines are its denies and its lines that aren't rules.
	DeniesOnly bool
	Lines      []proxy.PolicyLine
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

// ReadRules returns each of the policies (Policies) that holds more than
// comments, in the order its proxy reads them, with its lines: the
// built-in ones from devvm's files, own, then the user's files from disk.
func ReadRules(own fs.FS, builtin, files []string) ([]RuleFile, error) {
	var out []RuleFile
	for _, n := range builtin {
		b, err := fs.ReadFile(own, n)
		if err != nil {
			return nil, err
		}
		if pl := proxy.ParsePolicy(string(b)); len(pl) > 0 {
			out = append(out, RuleFile{Path: n, Builtin: true, Lines: pl})
		}
	}
	for _, f := range files {
		rf, err := readPolicyFile(f, false)
		if err != nil {
			return nil, err
		}
		if len(rf.Lines) > 0 {
			out = append(out, rf)
		}
	}
	return out, nil
}

// ReadPolicies is ReadRules for a VM's policies (Policies): the built-in
// ones and its files, then the files whose denies alone apply, each with
// only its denies and its lines that aren't rules.
func ReadPolicies(own fs.FS, s PolicySet) ([]RuleFile, error) {
	out, err := ReadRules(own, s.Builtin, s.Files)
	if err != nil {
		return nil, err
	}
	for _, f := range s.Denies {
		rf, err := readPolicyFile(f, true)
		if err != nil {
			return nil, err
		}
		if len(rf.Lines) > 0 {
			out = append(out, rf)
		}
	}
	return out, nil
}

// readPolicyFile is the user's policy file f with its lines; with
// deniesOnly, those of them that are denies or aren't rules.
func readPolicyFile(f string, deniesOnly bool) (RuleFile, error) {
	lines, err := readRules(f)
	if err != nil {
		return RuleFile{}, err
	}
	pl := proxy.ParsePolicy(strings.Join(lines, "\n"))
	if deniesOnly {
		pl = slices.DeleteFunc(pl, func(l proxy.PolicyLine) bool { return l.OK && !strings.HasPrefix(l.Text, "!") })
	}
	return RuleFile{Path: f, DeniesOnly: deniesOnly, Lines: pl}, nil
}

// notRuleMark follows a line that isn't a rule in WriteRules' output.
const notRuleMark = "  # not a rule, skipped"

// builtinMark labels a built-in policy in WriteRules' output, before its
// name: it comes with devvm, so it has no path.
const builtinMark = "devvm: "

// deniesOnlyMark follows the path of a file whose denies alone apply in
// WriteRules' output.
const deniesOnlyMark = " (denies only)"

// WriteRules writes each policy's rules to w: after #, a file's path or
// a built-in policy's name, labelled so, then its lines, indented, each
// that isn't a rule marked so, with its line number.
func WriteRules(w io.Writer, files []RuleFile) error {
	for _, f := range files {
		label := f.Path
		if f.Builtin {
			label = builtinMark + f.Path
		}
		if f.DeniesOnly {
			label += deniesOnlyMark
		}
		if _, err := fmt.Fprintf(w, "# %s\n", label); err != nil {
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
	layers, _ := filepath.Glob(filepath.Join(d.Layers(), "*.policy"))
	files = append(files, layers...)
	vms, _ := state.VMNames(d)
	for _, vm := range vms {
		files = append(files, d.VM(vm).Policy())
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
		rs, err := ReadRules(nil, nil, []string{f})
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
