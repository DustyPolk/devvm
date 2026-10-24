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
