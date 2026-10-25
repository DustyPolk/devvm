package cli

import "testing"

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
