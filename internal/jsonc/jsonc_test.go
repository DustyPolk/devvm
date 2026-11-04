package jsonc

import "testing"

func TestStrip(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"line comment", "{\"a\": 1 // c\n}", "{\"a\": 1     \n}"},
		{"block comment", "{/* x\ny */\"a\": 1}", "{    \n    \"a\": 1}"},
		{"trailing commas", `{"a": [1, 2,], "b": {"c": 3,},}`, `{"a": [1, 2 ], "b": {"c": 3 } }`},
		{"comma before a comment", "{\"a\": 1, // c\n}", "{\"a\": 1      \n}"},
		{"slashes in strings", `{"a": "http://x/*y*/", "b": "q\"//"}`, `{"a": "http://x/*y*/", "b": "q\"//"}`},
		{"comma in a string", `{"a": ",}"}`, `{"a": ",}"}`},
		{"escaped backslash", `{"a": "\\", "b": 1,}`, `{"a": "\\", "b": 1 }`},
		{"bom", "\xef\xbb\xbf{}", "   {}"},
		{"unterminated block", "{} /* x", "{}     "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := string(Strip([]byte(tc.in)))
			if got != tc.want {
				t.Errorf("Strip(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) != len(tc.in) {
				t.Errorf("length changed: %d, want %d", len(got), len(tc.in))
			}
		})
	}
}
