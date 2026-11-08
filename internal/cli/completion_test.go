package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/devvmhq/devvm/internal/image"
)

func TestJoined(t *testing.T) {
	names := []string{"base", "docker", "nix", "foo", "fop"}
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", names},
		{"d", []string{"docker"}},
		{"docker+", []string{"docker+base", "docker+nix", "docker+foo", "docker+fop"}},
		{"docker+f", []string{"docker+foo", "docker+fop"}},
		{"docker+foo+f", []string{"docker+foo+fop"}},
		{"x", nil},
	} {
		if got := joined(tc.in, names); !slices.Equal(got, tc.want) {
			t.Errorf("joined(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --tools, --agents and --layers complete a comma-separated list of their
// part's names, and none.
func TestCompleteParts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	u, _, _ := testUI()
	a := newApp(u)
	others := slices.DeleteFunc(slices.Clone(image.Toolchains), func(n string) bool { return n == "node" })
	for _, tc := range []struct {
		flag, in string
		want     []string
	}{
		{"tools", "", append(slices.Clone(image.Toolchains), "none")},
		{"tools", "node,", prefixed("node,", others, "")},
		{"tools", "node,p", prefixed("node,", others, "p")},
		{"agents", "c", prefixed("", image.Agents, "c")},
		{"agents", "n", append(prefixed("", image.Agents, "n"), "none")},
		{"layers", "", []string{"desktop", "docker", "nix", "none"}},
		{"layers", "de", []string{"desktop"}},
	} {
		if got, _ := completeParts(a, tc.flag)(nil, nil, tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("--%s %q = %q, want %q", tc.flag, tc.in, got, tc.want)
		}
	}
}

// prefixed is before and each of names that starts with prefix, in their
// order: what a completion of a list offers.
func prefixed(before string, names []string, prefix string) []string {
	var out []string
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			out = append(out, before+n)
		}
	}
	return out
}

// devvm tools add and rm complete separate words: the shared tools not
// given yet.
func TestCompleteToolNames(t *testing.T) {
	all := slices.Concat(image.Toolchains, image.Agents)
	for _, tc := range []struct {
		args []string
		in   string
		want []string
	}{
		{nil, "c", prefixed("", all, "c")},
		{[]string{"claude"}, "c", slices.DeleteFunc(prefixed("", all, "c"), func(n string) bool { return n == "claude" })},
		{[]string{"node"}, "", slices.DeleteFunc(slices.Clone(all), func(n string) bool { return n == "node" })},
		{nil, "node+", nil},
	} {
		if got, _ := completeToolNames(nil, tc.args, tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("completeToolNames(%q, %q) = %q, want %q", tc.args, tc.in, got, tc.want)
		}
	}
}
