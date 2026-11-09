package proxy

// Requests by method and path: the one matcher for what a network rule narrows to (a rule's
// methods and path, policy.go) and for where a secret's binding goes in (Binding.Methods and
// Binding.Path, secrets.go). Both judge a request as the proxy forwards it, and only in a form
// that every server reads alike (plainPath), so no spelling of a path takes a request somewhere
// the matcher didn't judge.

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
)

// methodList is Methods', joined by commas.
const methodList = "GET,HEAD,POST,PUT,PATCH,DELETE,OPTIONS,TRACE,QUERY"

// Methods are the HTTP methods a rule or a secret may name, in the order their lists are written:
// RFC 9110's, PATCH's and QUERY's, never CONNECT, which inside a decrypted tunnel is refused. A
// method that isn't one of these isn't a rule's, so a typo in a deny is a broken deny (BrokenDeny),
// which denies everything, rather than a deny of nothing.
func Methods() []string { return strings.Split(methodList, ",") }

// overrideLike reports whether a header's name, in any case and with _ for -, as Rack's and CGI's
// servers read it, is one that some frameworks take a request's method from instead of its own:
// X-HTTP-Method-Override, X-HTTP-Method, X-Method-Override, or any other with method-override in
// it.
func overrideLike(name string) bool {
	n := strings.ReplaceAll(strings.ToLower(name), "_", "-")
	return n == "x-http-method" || strings.Contains(n, "method-override")
}

// overrides are the methods r says it has besides its own: each override-like header's value
// (overrideLike) and each _method query parameter's, in any case of its name, which some frameworks
// read the method from too. odd reports one that isn't exactly one of Methods, as written, such as
// a list (DELETE, GET), which a server might read either way, or an empty value; and a query with
// a ; in it, or that Go can't read whole, whose _method Go might not see.
func overrides(r *http.Request) (vals []string, odd bool) {
	add := func(v string) {
		v = strings.TrimSpace(v)
		vals = append(vals, v)
		odd = odd || !slices.Contains(Methods(), v)
	}
	for name, vs := range r.Header {
		if overrideLike(name) {
			for _, v := range vs {
				add(v)
			}
		}
	}
	if r.URL != nil {
		// A query Go reads apart from servers that split it at ; too, or that it can't read
		// whole, which drops the pairs it can't, could carry a _method Go never sees.
		q, err := url.ParseQuery(r.URL.RawQuery)
		odd = odd || err != nil || strings.Contains(r.URL.RawQuery, ";")
		for name, vs := range q {
			if strings.EqualFold(name, "_method") {
				for _, v := range vs {
					add(v)
				}
			}
		}
	}
	return vals, odd
}

// reasonNotDecrypted is why a CONNECT to a host whose rules narrow it is refused where the proxy
// can't decrypt it to judge its requests.
const reasonNotDecrypted = "its rules narrow it to some methods or paths, which the proxy checks only in HTTPS on port 443"

// ruleKey is the context key of the rule that let a request through when rules narrowed its host
// (judgeRequest), for the request's event.
type ruleKey struct{}

// judgeRequest decides r, a request to host:port that the proxy is about to forward, in a
// decrypted tunnel or in absolute form, by the rules that cover it (policy.judge), so every request
// on a kept-alive or HTTP/2 connection is judged, not only the first. A refused one gets 403 and
// a blocked event with its method and path; an allowed one, when rules narrowed the host, carries
// the rule that allowed it for its event. ok is false when r was refused.
func (s *server) judgeRequest(w http.ResponseWriter, r *http.Request, host string, port int) (*http.Request, bool) {
	ok, why, refused, narrowed := s.egress.pol.judge(host, port, r)
	if !ok {
		s.block(w, Event{Host: host, Port: port, Method: r.Method, Path: r.URL.Path, Agent: r.UserAgent(), Reason: why, Refused: refused})
		return r, false
	}
	if narrowed {
		r = r.WithContext(context.WithValue(r.Context(), ruleKey{}, why))
	}
	return r, true
}

// ruleOf is the rule judgeRequest put in ctx, or "".
func ruleOf(ctx context.Context) string {
	s, _ := ctx.Value(ruleKey{}).(string)
	return s
}

// Path patterns' bounds: a rule's or a binding's path, at most pathPatternMax bytes and
// pathSegmentsMax segments, so matching a request's path against one stays cheap.
const (
	pathPatternMax  = 256
	pathSegmentsMax = 32
)

// ParseMethods reads a list of methods, such as GET,HEAD: names from Methods, in any case, joined
// by commas, each once. It returns the list in its canonical form, upper case in Methods' order,
// and false for anything else, an empty list included.
func ParseMethods(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	known := Methods()
	seen := map[string]bool{}
	for m := range strings.SplitSeq(s, ",") {
		m = strings.ToUpper(m)
		if !slices.Contains(known, m) || seen[m] {
			return "", false
		}
		seen[m] = true
	}
	var out []string
	for _, m := range known {
		if seen[m] {
			out = append(out, m)
		}
	}
	return strings.Join(out, ","), true
}

// PathOK reports whether p is a path pattern, a rule's or a secret's binding's: it starts with /,
// its segments are printable ASCII without a space, %, \, ;, ? or #, none is . or .., none is empty
// but one after a last /, which matches everything below (/api/ is /api/**), and * matches within
// one segment while ** is a whole segment that matches one or more. At most pathPatternMax bytes
// and pathSegmentsMax segments.
func PathOK(p string) bool {
	if p == "" || p[0] != '/' || len(p) > pathPatternMax ||
		strings.ContainsFunc(p, func(c rune) bool { return c <= ' ' || c > '~' || strings.ContainsRune(`%\;?#`, c) }) {
		return false
	}
	segs := strings.Split(p[1:], "/")
	if len(segs) > pathSegmentsMax {
		return false
	}
	for i, s := range segs {
		switch {
		case s == "" && i < len(segs)-1, s == "." || s == "..":
			return false
		case strings.Contains(s, "**") && s != "**":
			return false
		}
	}
	return true
}

// A match narrows requests to some methods and a path: a rule's, or a secret's binding's. Its zero
// value matches every request.
type match struct {
	methods string // canonical (ParseMethods); "": every method
	path    string // a pattern (PathOK); "": every path
}

// newMatch is the match of methods (ParseMethods' form, or "") and a path pattern (or "").
func newMatch(methods, p string) match { return match{methods: methods, path: p} }

// narrows reports whether m matches fewer than every request.
func (m match) narrows() bool { return m.methods != "" || m.path != "" }

// has reports whether method is one of m's, as written.
func (m match) has(method string) bool {
	return slices.Contains(strings.Split(m.methods, ","), method)
}

// allows reports whether r is one m lets through, judged strictly, as an allow and a secret's
// binding are: its method, and every method it says it has besides (overrides), is one of m's as
// written, none of those odd, and its path is plain (plainPath) and matches m's pattern exactly. A
// request it can't judge alike for every server isn't allowed.
func (m match) allows(r *http.Request) bool {
	if m.methods != "" {
		vals, odd := overrides(r)
		if odd || !m.has(r.Method) || slices.ContainsFunc(vals, func(v string) bool { return !m.has(v) }) {
			return false
		}
	}
	return m.path == "" || plainPath(r.URL) && matchPath(m.path, r.URL.Path, false)
}

// denies reports whether r is one m refuses, judged broadly, as a deny is: its method, or any
// method it says it has besides (overrides), every word of a list among them, is one of m's in any
// case, or one of those is odd; and its path matches m's pattern broadly (matchPath with fold). A
// path that isn't plain is denied by any pattern, as no server's reading of it can be ruled out.
func (m match) denies(r *http.Request) bool {
	if m.methods != "" {
		vals, odd := overrides(r)
		hit := odd || m.has(strings.ToUpper(r.Method))
		for _, v := range vals {
			for _, w := range strings.FieldsFunc(v, func(c rune) bool { return c == ',' || c == ' ' || c == '\t' }) {
				hit = hit || m.has(strings.ToUpper(w))
			}
		}
		if !hit {
			return false
		}
	}
	return m.path == "" || !plainPath(r.URL) || matchPath(m.path, r.URL.Path, true)
}

// plainPath reports whether u's path, as the proxy forwards it, is in a form every server reads
// alike: no opaque URL, a path that starts with /, nothing percent-encoded (an encoded slash or
// dot that a server decodes could take the request elsewhere after the check), no backslash or ;
// (which some servers read as a separator, or drop with what follows), and clean already (no . or
// .. segment, no doubled slash). The query is no part of it.
func plainPath(u *url.URL) bool {
	p := u.Path
	if u.Opaque != "" || !strings.HasPrefix(p, "/") || strings.Contains(u.EscapedPath(), "%") || strings.ContainsAny(p, `\;`) {
		return false
	}
	clean := path.Clean(p)
	if strings.HasSuffix(p, "/") && clean != "/" {
		clean += "/"
	}
	return clean == p
}

// matchPath reports whether the plain path p matches the pattern pat (PathOK). A path that ends
// with / matches as itself or without the /, so /user matches /user/, as most servers read them
// alike. With broad, as a deny matches, it also matches in any ASCII case, a pattern that ends with
// / or /** matches the path above it too, so a deny of /gists/ refuses /gists, and the path's last
// segment matches cut at its first dot, so a deny of /admin refuses /admin.json and /admin., as
// frameworks that take a format from a suffix (Rails, Spring) and servers that drop a trailing dot
// route them there, and every segment matches with its trailing dots dropped (trimDots), so a deny
// of /config.json refuses /config.json. too.
func matchPath(pat, p string, broad bool) bool {
	if broad {
		pat, p = strings.ToLower(pat), strings.ToLower(p)
	}
	ps := strings.Split(pat[1:], "/")
	if ps[len(ps)-1] == "" {
		ps[len(ps)-1] = "**" // everything below
	}
	pats := [][]string{ps}
	if broad && len(ps) > 1 && ps[len(ps)-1] == "**" {
		pats = append(pats, ps[:len(ps)-1])
	}
	segs := strings.Split(p[1:], "/")
	paths := [][]string{segs}
	if len(segs) > 1 && segs[len(segs)-1] == "" {
		paths = append(paths, segs[:len(segs)-1])
	}
	if broad {
		for _, s := range paths {
			if trimmed := trimDots(s); !slices.Equal(trimmed, s) {
				paths = append(paths, trimmed)
			}
		}
		for _, s := range paths {
			if last := s[len(s)-1]; strings.Contains(last, ".") {
				if cut, _, _ := strings.Cut(last, "."); cut != "" {
					paths = append(paths, append(slices.Clone(s[:len(s)-1]), cut))
				}
			}
		}
	}
	for _, x := range pats {
		for _, s := range paths {
			if matchSegments(x, s) {
				return true
			}
		}
	}
	return false
}

// trimDots is segs with the trailing dots of each dropped, as servers that drop a name's trailing
// dot read it (/config.json. as /config.json), but a segment of dots alone, which stays.
func trimDots(segs []string) []string {
	out := slices.Clone(segs)
	for i, s := range out {
		if t := strings.TrimRight(s, "."); t != "" {
			out[i] = t
		}
	}
	return out
}

// matchSegments reports whether the path's segments match the pattern's, ** matching one or more
// segments: a table over the two, so a pattern with several ** stays linear in the path's length
// for each of its segments.
func matchSegments(pat, segs []string) bool {
	// at[j]: the pattern's first i segments match the path's first j.
	at := make([]bool, len(segs)+1)
	at[0] = true
	for _, ps := range pat {
		next := make([]bool, len(segs)+1)
		for j := 1; j <= len(segs); j++ {
			if ps == "**" {
				next[j] = at[j-1] || next[j-1]
			} else {
				next[j] = at[j-1] && matchSegment(ps, segs[j-1])
			}
		}
		at = next
	}
	return at[len(segs)]
}

// matchSegment reports whether one segment s matches the pattern's segment ps, where * matches
// any run of characters, none included.
func matchSegment(ps, s string) bool {
	parts := strings.Split(ps, "*")
	if len(parts) == 1 {
		return ps == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(s, part)
		if i < 0 {
			return false
		}
		s = s[i+len(part):]
	}
	return len(s) >= len(last) && strings.HasSuffix(s, last)
}
