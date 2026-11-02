package image

import "testing"

// The value patterns other packages check names and keys with.
func TestValuePatterns(t *testing.T) {
	for _, c := range []struct {
		name string
		f    func(string) bool
		ok   []string
		bad  []string
	}{
		{
			"IsKey", IsKey,
			[]string{"0123456789ab", "abcdefabcdef"},
			[]string{"", "0123456789a", "0123456789abc", "0123456789AB", "0123456789ag", "0123456789ab\n"},
		},
		{
			"ToolName", ToolName,
			[]string{"node", "python3", "claude"},
			[]string{"", "Node", "3node", "node-x", "node_x", "nöde"},
		},
		{
			"ToolVersion", ToolVersion,
			[]string{"24.21.0", "1.2.3-rc.1", "v2", "1.0+build", "2024_01"},
			[]string{"", "-1", ".1", "1 2", "1/2", "1;2", "1\n", "1'2", "$1"},
		},
		{
			"FeatureName", FeatureName,
			[]string{"omb", "cli-tools", "9x"},
			[]string{"", "-x", "X", "a_b", "a.b"},
		},
	} {
		for _, s := range c.ok {
			if !c.f(s) {
				t.Errorf("%s(%q) = false", c.name, s)
			}
		}
		for _, s := range c.bad {
			if c.f(s) {
				t.Errorf("%s(%q) = true", c.name, s)
			}
		}
	}
}
