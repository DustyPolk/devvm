package msg

import "testing"

func TestLevenshtein(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"stop", "stop", 0}, {"sotp", "stop", 2}, {"nodee", "node", 1}, {"", "ab", 2}, {"kitten", "sitting", 3},
	} {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// The closest name within SuggestDistance, ignoring case; none further.
func TestClosest(t *testing.T) {
	t.Parallel()
	for word, want := range map[string]string{"Pyhton": "python", "nod": "node", "zzz": ""} {
		if got := Closest(word, []string{"node", "python"}); got != want {
			t.Errorf("Closest(%q) = %q, want %q", word, got, want)
		}
	}
}
