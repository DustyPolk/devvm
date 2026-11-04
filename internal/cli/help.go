package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
)

// commandGroup is a section of the root's help: its title and the
// commands in it, by name.
type commandGroup struct {
	id, title string
	commands  []string
}

// commandGroups are the root's help's sections, in order. help is in
// Cobra's own section, Additional Commands. A group none of whose commands
// exist is left out.
func commandGroups() []commandGroup {
	return []commandGroup{
		{"vm", "Your VM:", []string{"restart", "stop", "update", "rm", "adopt", "ls", "code", "desktop", "host", "history"}},
		{"change", "Change your VM:", []string{"tools", "size", "ports", "network", "ssh-agent", "git-protect", "secret"}},
		{"links", "Links between VMs:", []string{"link", "group"}},
		{"images", "Images:", []string{"images", "build", "rebuild", "layer"}},
		{"space", "Disk space:", []string{"space", "gc", "retention"}},
		{"machine", "This machine:", []string{"setup", "doctor", "completion", "limactl"}},
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
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

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

// addGroups puts root's commands into commandGroups' sections.
func addGroups(root *cobra.Command) {
	byName := map[string]*cobra.Command{}
	for _, c := range root.Commands() {
		byName[c.Name()] = c
	}
	for _, g := range commandGroups() {
		var cmds []*cobra.Command
		for _, name := range g.commands {
			if c, ok := byName[name]; ok {
				cmds = append(cmds, c)
			}
		}
		if len(cmds) == 0 {
			continue
		}
		root.AddGroup(&cobra.Group{ID: g.id, Title: g.title})
		for _, c := range cmds {
			c.GroupID = g.id
		}
	}
}

// addDocs ends the help of root and every command under it with its docs
// page, from msg's links, and the root's with the issue tracker.
func addDocs(root *cobra.Command) {
	root.SetUsageTemplate(usageTemplate)
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations["issues"] = msg.IssuesURL
	setDocs(root)
}

func setDocs(c *cobra.Command) {
	if u := msg.CommandDocURL(c.CommandPath()); u != "" {
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations["docs"] = u
	}
	for _, sub := range c.Commands() {
		setDocs(sub)
	}
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
