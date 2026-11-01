package cli

import (
	"slices"
	"testing"
)

func TestJoined(t *testing.T) {
	names := []string{"base", "node", "python", "foo", "claude", "codex"}
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", names},
		{"n", []string{"node"}},
		{"node+", []string{"node+base", "node+python", "node+foo", "node+claude", "node+codex"}},
		{"node+c", []string{"node+claude", "node+codex"}},
		{"node+claude+c", []string{"node+claude+codex"}},
		{"x", nil},
	} {
		if got := joined(tc.in, names); !slices.Equal(got, tc.want) {
			t.Errorf("joined(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// devvm tools add and rm complete separate words: the shared tools not
// given yet.
func TestCompleteToolNames(t *testing.T) {
	for _, tc := range []struct {
		args []string
		in   string
		want []string
	}{
		{nil, "c", []string{"claude", "codex"}},
		{[]string{"claude"}, "c", []string{"codex"}},
		{[]string{"node"}, "", []string{"python", "rust", "go", "claude", "codex", "opencode"}},
		{nil, "node+", nil},
	} {
		if got, _ := completeToolNames(nil, tc.args, tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("completeToolNames(%q, %q) = %q, want %q", tc.args, tc.in, got, tc.want)
		}
	}
}
