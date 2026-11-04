package ubuntu

import "testing"

// CompareVersions orders versions as dpkg --compare-versions does: Debian
// policy's examples (5.6.12), and the cases dpkg's own tests check.
func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"0:1.0", "1.0", 0},
		{"1.0-0", "1.0", 0},
		{"1.0-1", "1.0-01", 0},
		{"1.0", "1.0-1", -1},
		{"1.0-1", "1.0-2", -1},
		{"1.0-1ubuntu1", "1.0-1ubuntu2", -1},
		{"1.0-1ubuntu1", "1.0-1ubuntu1.1", -1},
		{"1.0-1ubuntu1.1", "1.0-1ubuntu2", -1},
		{"2.39-0ubuntu8", "2.39-0ubuntu8.3", -1},
		{"1.9", "1.10", -1},
		{"1.0", "1.0.1", -1},
		{"1.0", "1.0a", -1},
		{"1.0a", "1.0b", -1},
		{"1.0a", "1.0+", -1},  // letters sort before everything else but ~
		{"1.0+", "1.0.", -1},  // then by ASCII
		{"1.0", "1.0+b1", -1}, // a binary rebuild is newer
		{"1.0~rc1", "1.0", -1},
		{"1.0~rc1", "1.0~rc2", -1},
		{"1.0~~", "1.0~~a", -1}, // policy: ~~ < ~~a < ~ < (nothing) < a
		{"1.0~~a", "1.0~", -1},
		{"1.0~", "1.0", -1},
		{"1.0", "1.0a", -1},
		{"1.0-1~bpo1", "1.0-1", -1},
		{"2.0", "1:0.1", -1}, // the epoch first
		{"1:2.0", "2:0.1", -1},
		{"10:1.0", "9:1.0", 1},
		{"1.0-1-1", "1.0-1-2", -1}, // the revision is after the last hyphen
		{"1.0-a", "1.0-1", 1},
		{"1:1.0:1-1", "1:1.0:2-1", -1}, // colons after the epoch's are upstream's
		{"001.000", "1.0", 0},
		{"1.0.0", "1.0", 1},
		{"a", "b", -1},
		{"1.2.3", "1.2.3", 0},
		{"7.88.1-10+deb12u5", "7.88.1-10+deb12u12", -1},
	} {
		got, err := CompareVersions(c.a, c.b)
		if err != nil || got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, %v; want %d", c.a, c.b, got, err, c.want)
		}
		if back, _ := CompareVersions(c.b, c.a); back != -c.want {
			t.Errorf("CompareVersions(%q, %q) = %d; want %d", c.b, c.a, back, -c.want)
		}
	}
	for _, bad := range []string{"", ":1.0", "x:1.0", "-1:1.0", "1:", "1.0-", "-1", "1.0 2", "1.0_1", "1.0-1:2", "1.0-é", "99999999999999999999:1"} {
		if _, err := CompareVersions(bad, "1.0"); err == nil {
			t.Errorf("CompareVersions(%q): no error", bad)
		}
		if _, err := CompareVersions("1.0", bad); err == nil {
			t.Errorf("CompareVersions(1.0, %q): no error", bad)
		}
	}
}
