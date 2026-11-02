package vmname

import "testing"

func TestIsVM(t *testing.T) {
	for name, want := range map[string]bool{
		"devvm-web-0123456789abcd":        true,
		"devvm-my-app-0123456789abcd":     true,
		"devvm-img-x-0123456789abcd":      true, // a folder named img-x
		"devvm-a-b-0123456789abcd":        true,
		"devvm-tools-build":               false,
		"devvm-web-123456":                false,
		"other-web-12345612345612":        false,
		"devvm-img-0123456789ab":          false, // an image: 12 hex digits
		"devvm-web-0123456789abc":         false,
		"devvm-web-0123456789abcde":       false,
		"devvm-web-0123456789ABCD":        false,
		"devvm--0123456789abcd":           false,
		"devvm-0123456789abcd":            false,
		"devvm-w\neb-0123456789abcd":      false,
		"xdevvm-web-0123456789abcd":       false,
		"devvm-web-0123456789abcd\n":      false,
		"devvm-web-0123456789abcd.tar.gz": false,
		"":                                false,
		"base":                            false,
	} {
		if got := IsVM(name); got != want {
			t.Errorf("IsVM(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSlugOf(t *testing.T) {
	for _, c := range []struct {
		name, slug string
		ok         bool
	}{
		{"devvm-web-0123456789abcd", "web", true},
		{"devvm-my-app-0123456789abcd", "my-app", true},
		{"devvm-img-x", "img", true},
		{"devvm-img-0123456789ab", "img", true},
		{"devvm-web", "", false},
		{"devvm--x", "", false},
		{"devvm-", "", false},
		{"other", "", false},
		{"other-x-0123456789abcd", "", false},
	} {
		if slug, ok := SlugOf(c.name); slug != c.slug || ok != c.ok {
			t.Errorf("SlugOf(%q) = %q, %v; want %q, %v", c.name, slug, ok, c.slug, c.ok)
		}
	}
}
