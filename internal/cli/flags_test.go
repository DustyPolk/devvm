package cli

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// eachCommand calls do on c and every command under it.
func eachCommand(c *cobra.Command, do func(*cobra.Command)) {
	do(c)
	for _, sub := range c.Commands() {
		eachCommand(sub, do)
	}
}

// Flags are local unless truly global: no command has a persistent flag
// but --help, --no-input and -v/--verbose, so no subcommand takes the
// root's flags for a new VM.
func TestPersistentFlags(t *testing.T) {
	persistentFlags := []string{"help", "no-input", "verbose"}
	u, _, _ := testUI()
	root := NewRoot(u)
	eachCommand(root, func(c *cobra.Command) {
		c.InitDefaultHelpFlag()
		c.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			if !slices.Contains(persistentFlags, f.Name) {
				t.Errorf("%s: --%s is persistent", c.CommandPath(), f.Name)
			}
		})
	})
}

// flagUse is what a flag name must keep the same on every command: one
// name, one meaning.
type flagUse struct {
	shorthand, kind, value string
	where                  string
}

// A flag name has one meaning across the tree: the same shorthand, type
// and value it takes wherever it appears, and a shorthand names one flag.
func TestFlagNamesHaveOneMeaning(t *testing.T) {
	u, _, _ := testUI()
	root := NewRoot(u)
	byName := map[string]flagUse{}
	byShort := map[string]string{}
	eachCommand(root, func(c *cobra.Command) {
		if c.Hidden {
			return
		}
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			value, _ := pflag.UnquoteUsage(f)
			got := flagUse{shorthand: f.Shorthand, kind: f.Value.Type(), value: value, where: c.CommandPath()}
			if was, ok := byName[f.Name]; ok {
				if was.shorthand != got.shorthand || was.kind != got.kind || was.value != got.value {
					t.Errorf("--%s: %+v on %s, %+v on %s", f.Name, got, got.where, was, was.where)
				}
			} else {
				byName[f.Name] = got
			}
			if f.Shorthand == "" {
				return
			}
			if name, ok := byShort[f.Shorthand]; ok && name != f.Name {
				t.Errorf("-%s is --%s on %s and --%s elsewhere", f.Shorthand, f.Name, c.CommandPath(), name)
			}
			byShort[f.Shorthand] = f.Name
		})
	})
}

// devvm [IMAGE]'s flags are its own, and --cpus, --memory and --disk
// are only its and devvm size's.
func TestUpFlagsAreUpsAlone(t *testing.T) {
	u, _, _ := testUI()
	root := NewRoot(u)
	sizeFlags := []string{"cpus", "memory", "disk"}
	upOnly := []string{"network-policy", "no-network-policy", "ports", "clone", "here", "unsafe-folder", "no-git-protect", "ssh-agent", "no-devcontainer"}
	eachCommand(root, func(c *cobra.Command) {
		for _, name := range slices.Concat(sizeFlags, upOnly) {
			if c.Flags().Lookup(name) == nil {
				continue
			}
			ok := c == root || (c.Name() == "size" && slices.Contains(sizeFlags, name))
			if !ok {
				t.Errorf("%s takes --%s", c.CommandPath(), name)
			}
		}
	})
}
