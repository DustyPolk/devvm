package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/msg"
)

// about is what a command says of itself that Cobra has no field for, set
// as its Annotations where the command is made, so a new command is
// described in its own file alone.
type about struct {
	// docs is the user page that covers the command, "page#anchor" in
	// docs/content/docs: its help and its CLI reference page link to it.
	docs string
	// group is the command's section of the root's help, one of
	// helpGroups' IDs. Every root command has one, and a subcommand has
	// one only when it belongs in another section than its parent's, as
	// layer add does.
	group string
	// noJSON says why a command users see takes no --json: what it does
	// shows or lists nothing.
	noJSON string
}

// The Annotations keys about sets. docsKey holds the docs page's address,
// which usageTemplate prints.
const (
	docsKey     = "docs"
	docsPageKey = "devvm-docs-page"
	groupKey    = "devvm-group"
	noJSONKey   = "devvm-no-json"
)

// annotations returns a command's Annotations for what about says.
func (m about) annotations() map[string]string {
	out := map[string]string{}
	if m.docs != "" {
		out[docsPageKey] = m.docs
		out[docsKey] = msg.PageURL(m.docs)
	}
	if m.group != "" {
		out[groupKey] = m.group
	}
	if m.noJSON != "" {
		out[noJSONKey] = m.noJSON
	}
	return out
}

// DocsPage returns the "page#anchor" of the user page that covers c, or
// "" when it has none.
func DocsPage(c *cobra.Command) string { return c.Annotations[docsPageKey] }

// The IDs of the root's help's sections, which a command's about names.
const (
	groupVM      = "vm"
	groupChange  = "change"
	groupLinks   = "links"
	groupImages  = "images"
	groupSpace   = "space"
	groupMachine = "machine"
)

// helpGroups are the root's help's sections, in order. help is in Cobra's
// own section, Additional Commands. A group none of whose commands exist
// is left out. A subcommand in a group other than its parent's is listed
// in that one by both names, and its parent's help groups its subcommands
// the same way.
func helpGroups() []cobra.Group {
	return []cobra.Group{
		{ID: groupVM, Title: "Your VM:"},
		{ID: groupChange, Title: "Change your VM:"},
		{ID: groupLinks, Title: "Links between VMs:"},
		{ID: groupImages, Title: "Images:"},
		{ID: groupSpace, Title: "Disk space:"},
		{ID: groupMachine, Title: "This machine:"},
	}
}

// usageTemplate is Cobra's own, with each command's docs page at the end,
// and for the root, devvm's issue tracker.
const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{range $parent := $cmds}}{{if ne $parent.GroupID $group.ID}}{{range $parent.Commands}}{{if (and (eq .GroupID $group.ID) .IsAvailableCommand)}}
  {{rpad (print $parent.Name " " .Name) $parent.NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}{{with index .Annotations "docs"}}

Docs:
  {{.}}{{end}}{{with index .Annotations "issues"}}

Issues:
  {{.}}{{end}}
`

// addGroups puts each command under root whose about names a group into
// that section of its parent's help, in helpGroups' order, so a subcommand
// listed by both names in the root's section is in its parent's section of
// the same group too.
func addGroups(root *cobra.Command) {
	for _, g := range helpGroups() {
		addGroup(root, g)
	}
}

func addGroup(c *cobra.Command, g cobra.Group) {
	for _, sub := range c.Commands() {
		if sub.Annotations[groupKey] == g.ID {
			if !c.ContainsGroup(g.ID) {
				c.AddGroup(&cobra.Group{ID: g.ID, Title: g.Title})
			}
			sub.GroupID = g.ID
		}
		addGroup(sub, g)
	}
}

// addDocs gives root the usage template that ends each help with its
// docs page, from the command's about, and the root's with the issue
// tracker.
func addDocs(root *cobra.Command) {
	root.SetUsageTemplate(usageTemplate)
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations["issues"] = msg.IssuesURL
}

// orList is a list of names in a help's prose: a, b or c.
func orList(names []string) string { return joinLast(names, " or ") }

// andList is a list of names in a help's prose: a, b and c.
func andList(names []string) string { return joinLast(names, " and ") }

// joinLast joins names with commas, last before the last of them.
func joinLast(names []string, last string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + last + names[len(names)-1]
}

// possessive is each name with 's: pi's.
func possessive(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = n + "'s"
	}
	return out
}

// examples is a command's Example: each pair of arguments is a command
// line and a short comment on it, and the comments line up.
func examples(pairs ...string) string {
	if len(pairs)%2 != 0 {
		panic("examples: a command line without its comment")
	}
	width := 0
	for i := 0; i < len(pairs); i += 2 {
		width = max(width, len(pairs[i]))
	}
	lines := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		line, comment := pairs[i], pairs[i+1]
		lines = append(lines, "  "+line+strings.Repeat(" ", width-len(line))+"  # "+comment)
	}
	return strings.Join(lines, "\n")
}
