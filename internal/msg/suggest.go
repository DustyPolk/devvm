package msg

import "strings"

// SuggestDistance is how far, in Levenshtein distance, a word can be from
// a name for a did-you-mean to suggest it.
const SuggestDistance = 2

// Closest is the name in names closest to word, ignoring case, within
// SuggestDistance: what a did-you-mean suggests; "" when none is.
func Closest(word string, names []string) string {
	best, bestD := "", SuggestDistance+1
	for _, n := range names {
		if d := levenshtein(strings.ToLower(word), strings.ToLower(n)); d < bestD {
			best, bestD = n, d
		}
	}
	return best
}

// levenshtein is the edit distance between a and b, by runes.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
