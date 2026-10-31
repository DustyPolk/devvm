package vm

import (
	"slices"
	"strings"
	"testing"
)

// The tool images a VM's report lists: only the VM's own, each once, in
// the report's order; a line that's anything else, or cut short, doesn't
// count. capped keeps the read to its limit.
func TestReported(t *testing.T) {
	node, bun := "node-24.21.0-0123456789ab.erofs", "bun-1.4.2-0123456789ab.erofs"
	files := []string{node, bun}
	cases := []struct {
		name, report string
		want         []string
	}{
		{"none", "", nil},
		{"one", node + "\n", []string{node}},
		{"in its order, once", bun + "\n" + node + "\n" + bun + "\n", []string{bun, node}},
		{"not the VM's", "claude-2.1.295-0123456789ab.erofs\n../x\n\n", nil},
		{"more on the line", node + " x\nx " + node + "\n" + node + "\r\n", nil},
		{"cut short", bun + "\n" + node, []string{bun}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reported(files, []byte(c.report)); !slices.Equal(got, c.want) {
				t.Errorf("reported = %q, want %q", got, c.want)
			}
		})
	}
	out := &capped{max: unmountedMax}
	if _, err := out.Write([]byte(strings.Repeat("x\n", unmountedMax) + node + "\n")); err != nil {
		t.Fatal(err)
	}
	if out.Len() != unmountedMax || reported(files, out.Bytes()) != nil {
		t.Errorf("capped kept %d bytes", out.Len())
	}
}
