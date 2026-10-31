package cli

import (
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/tools"
)

// The tool table: each build's backend and C library, and - for what a
// row lacks, a tool that isn't built included.
func TestWriteImagesTools(t *testing.T) {
	var b strings.Builder
	ts := []tools.Listed{
		{Name: "node", Tool: "node", Version: "24.22.2", File: "a", Built: true, Size: 4096, VMs: 1, State: tools.StateCurrent},
		{Name: "node", Tool: "node", Version: "24.22.2", File: "b", Built: true, Size: 4096, State: tools.StateCurrent},
		{Name: "go", Tool: "go", Version: "1.27.2", State: tools.StateNotBuilt},
	}
	target := func(t tools.Listed) (string, string) {
		if t.File == "a" {
			return "lima", "gnu"
		}
		return "", ""
	}
	if err := writeImages(&b, nil, ts, target); err != nil {
		t.Fatal(err)
	}
	want := "IMAGE  FROM  DIGEST  KEY  STATE  SIZE  BACKEND\n\n" +
		"NAME  TOOL  VERSION  BACKEND  LIBC  SIZE  VMS  STATE\n" +
		"node  node  24.22.2  lima     gnu   4.0K  1    current\n" +
		"node  node  24.22.2  -        -     4.0K  0    current\n" +
		"go    go    1.27.2   -        -     -     -    not built\n"
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{
		0:               "0B",
		1023:            "1023B",
		4096:            "4.0K",
		10 << 20:        "10M",
		2254857830:      "2.1G",
		1 << 40:         "1.0T",
		1<<30 - 1<<20/2: "1.0G",
		10433331:        "9.9M",
		10438000:        "10M",
	} {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}
