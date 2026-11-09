package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A rule's methods and path: a host name's or *.name's alone, methods from Methods in their
// canonical order and case, a path pattern (PathOK), in that order, after spaces or tabs; the
// rule's text is the canonical form, which parses back to the same rule.
func TestParseRuleNarrowed(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]string{
		"api.github.com GET":                         "api.github.com GET",
		"API.GitHub.com head,get /repos/":            "api.github.com GET,HEAD /repos/",
		"!github.com\tPOST\t/*/*/git-receive-pack":   "!github.com POST /*/*/git-receive-pack",
		"*.example.com:443 /v1/**":                   "*.example.com:443 /v1/**",
		"example.com:* DELETE":                       "example.com:* DELETE",
		"example.com /":                              "example.com /",
		"example.com GET,QUERY,OPTIONS,TRACE /a*b/c": "example.com GET,OPTIONS,TRACE,QUERY /a*b/c",
		"example.com /Repos/X":                       "example.com /Repos/X",
		// Not rules: an address, a CIDR or **; a path before methods; twice; unknown methods,
		// CONNECT, a method twice; a path that isn't a pattern.
		"10.0.0.5 GET":                             "",
		"10.0.0.0/8:443 /x":                        "",
		"[fd00::1]:443 GET":                        "",
		"** GET":                                   "",
		"!** POST":                                 "",
		"example.com /x GET":                       "",
		"example.com GET HEAD":                     "",
		"example.com /x /y":                        "",
		"example.com GTE":                          "",
		"example.com CONNECT":                      "",
		"example.com GET,GET":                      "",
		"example.com GET,":                         "",
		"example.com x":                            "",
		"example.com repos":                        "",
		"example.com /a//b":                        "",
		"example.com /a/../b":                      "",
		"example.com /a/./b":                       "",
		"example.com /a%2fb":                       "",
		`example.com /a\b`:                         "",
		"example.com /a;b":                         "",
		"example.com /a?b":                         "",
		"example.com /a**/b":                       "",
		"example.com /" + strings.Repeat("a/", 40): "",
		"example.com GET /x extra":                 "",
	} {
		r, ok := parseRule(line)
		if again, back := parseRule(r.text); ok && (!back || again != r) {
			t.Errorf("%q: %q doesn't parse back: %+v, %v", line, r.text, again, back)
		}
		if got := r.text; ok && got != want || !ok && want != "" {
			t.Errorf("parseRule(%q) = %q, %v; want %q", line, got, ok, want)
		}
	}
}

// Path patterns: * within one segment, ** for one or more whole segments, a last / for everything
// below; a path's one trailing / doesn't matter; case matters to an allow and not to a deny.
func TestMatchPath(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		pat, path   string
		exact, fold bool
	}{
		{"/user", "/user", true, true},
		{"/user", "/user/", true, true},
		{"/user", "/users", false, false},
		{"/user", "/User", false, true},
		{"/repos/", "/repos", false, true},
		{"/admin", "/admin.json", false, true},
		{"/admin", "/admin.", false, true},
		{"/admin", "/admin.tar.gz", false, true},
		{"/admin", "/admin.json/", false, true},
		{"/admin", "/.admin", false, false},
		{"/config.json", "/config.json.", false, true},
		{"/config.json", "/config.json..", false, true},
		{"/a.b/c", "/a.b./c", false, true},
		{"/config.json", "/config.jsonx", false, false},
		{"/admin/*", "/admin/x.json", true, true},
		{"/v1.2/x", "/v1.2/x", true, true},
		{"/v1/x", "/v1.2/x", false, false},
		{"/repos/", "/repos/", true, true},
		{"/repos/", "/repos/a/b/c", true, true},
		{"/repos/**", "/repos/a", true, true},
		{"/repos/**", "/repos", false, true},
		{"/repos/*", "/repos/a", true, true},
		{"/repos/*", "/repos/a/b", false, false},
		{"/repos/*/*/issues", "/repos/o/r/issues", true, true},
		{"/repos/*/*/issues", "/repos/o/issues", false, false},
		{"/*/*.git/git-receive-pack", "/o/r.git/git-receive-pack", true, true},
		{"/*/*.git/git-receive-pack", "/o/r/git-receive-pack", false, false},
		{"/a*b*c", "/abc", true, true},
		{"/a*b*c", "/a-b-b-c", true, true},
		{"/a*b*c", "/a-c-b", false, false},
		{"/**/raw/**", "/x/y/raw/z", true, true},
		{"/**/raw/**", "/raw/z", false, false},
		{"/", "/anything/at/all", true, true},
		{"/", "/", true, true},
	} {
		if got := matchPath(tc.pat, tc.path, false); got != tc.exact {
			t.Errorf("matchPath(%q, %q) = %v, want %v", tc.pat, tc.path, got, tc.exact)
		}
		if got := matchPath(tc.pat, tc.path, true); got != tc.fold {
			t.Errorf("matchPath(%q, %q, fold) = %v, want %v", tc.pat, tc.path, got, tc.fold)
		}
	}
	// Several ** against a long path stay quick: the table is linear in the path for each segment.
	long := "/" + strings.Repeat("a/", 20000) + "b"
	start := time.Now()
	if matchPath("/**/x/**/y/**/z", long, false) {
		t.Error("a long path matched")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("a long path took %v", d)
	}
}

// request is a request to example.com as the proxy would forward it: method, and target in origin
// form, as it would arrive, with headers given as name, value pairs.
func request(t *testing.T, method, target string, headers ...string) *http.Request {
	t.Helper()
	u, err := url.ParseRequestURI(target)
	if err != nil {
		t.Fatalf("%q: %v", target, err)
	}
	r := &http.Request{Method: method, URL: u, Header: http.Header{}, Host: "example.com"}
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Add(headers[i], headers[i+1])
	}
	return r
}

// A deny that narrows refuses every spelling of what it names: another case of the method or the
// path, a trailing slash, a method-override header, and any path that isn't plain, whichever way a
// server would read it. Other requests pass, by the host's whole allow.
func TestDenyBypasses(t *testing.T) {
	t.Parallel()
	p := writePolicy(t, "example.com\n!example.com POST,PUT,PATCH,DELETE\n!example.com /gists/\n!example.com /admin\n")
	for _, tc := range []struct {
		method, target string
		headers        []string
		ok             bool
	}{
		{"GET", "/repos/o/r", nil, true},
		{"HEAD", "/repos/o/r?x=/gists/", nil, true},
		{"GET", "/gistsx", nil, true},
		{"POST", "/repos/o/r/issues", nil, false},
		{"post", "/repos/o/r/issues", nil, false},
		{"Delete", "/x", nil, false},
		{"GET", "/x", []string{"X-HTTP-Method-Override", "POST"}, false},
		{"GET", "/x", []string{"X-Http-Method", " delete "}, false},
		{"GET", "/x", []string{"X-Method-Override", "patch"}, false},
		{"POST", "/x", []string{"X-HTTP-Method-Override", "DELETE, GET"}, false},
		{"GET", "/x", []string{"X-HTTP-Method-Override", "GET DELETE"}, false},
		{"POST", "/x", []string{"X_HTTP_METHOD_OVERRIDE", "DELETE"}, false},
		{"GET", "/x", []string{"x-custom-method-override", "PUT"}, false},
		{"GET", "/x", []string{"X-HTTP-Method-Override", ""}, false},
		{"GET", "/x", []string{"X-HTTP-Method-Override", "GET"}, true},
		{"GET", "/x?_method=DELETE", nil, false},
		{"GET", "/x?_METHOD=delete", nil, false},
		{"GET", "/x?_method=GET", nil, true},
		{"GET", "/gists", nil, false},
		{"POST", "/gists", nil, false},
		{"GET", "/gists.json", nil, false},
		{"GET", "/admin.json", nil, false},
		{"GET", "/admin.", nil, false},
		{"GET", "/administrator", nil, true},
		{"GET", "/x?a=1;_method=DELETE", nil, false},
		{"GET", "/x?_method=DELETE;a=1", nil, false},
		{"GET", "/x?a=%zz&_method=DELETE", nil, false},
		{"GET", "/x?a=1&b=2", nil, true},
		{"GET", "/gists/1", nil, false},
		{"GET", "/GISTS/1", nil, false},
		{"GET", "/gists/", nil, false},
		{"GET", "/admin", nil, false},
		{"GET", "/admin/", nil, false},
		{"GET", "/Admin", nil, false},
		{"GET", "/%67ists/1", nil, false},
		{"GET", "/x/../gists/1", nil, false},
		{"GET", "/./gists/1", nil, false},
		{"GET", "//gists/1", nil, false},
		{"GET", "/gists;x/1", nil, false},
		{"GET", "/gists%2F1", nil, false},
		{"GET", `/x\..\gists`, nil, false},
		{"GET", "/repos/%2e%2e/gists/1", nil, false},
		{"GET", "/repos/a%20b", nil, false},
	} {
		r := request(t, tc.method, tc.target, tc.headers...)
		ok, why, refused, narrowed := p.judge("example.com", 443, r)
		if ok != tc.ok || !narrowed || !ok && !refused {
			t.Errorf("%s %s %v: %v (%s), refused %v, narrowed %v; want %v", tc.method, tc.target, tc.headers, ok, why, refused, narrowed, tc.ok)
		}
	}
	if !p.byRequest("example.com", 443) {
		t.Error("a deny that narrows didn't make the host's requests judged")
	}
}

// An allow that narrows lets through only what it names, as written: the method in its case, the
// path in its case and plain, and no method-override header naming another method. A host's whole
// allow, beside it, allows the rest.
func TestAllowNarrowed(t *testing.T) {
	t.Parallel()
	p := writePolicy(t, "api.test GET,HEAD /repos/\napi.test POST /graphql\n")
	for _, tc := range []struct {
		method, target string
		headers        []string
		ok             bool
		why            string
	}{
		{"GET", "/repos/o/r", nil, true, "api.test GET,HEAD /repos/"},
		{"HEAD", "/repos/o/r/", nil, true, "api.test GET,HEAD /repos/"},
		{"POST", "/graphql", nil, true, "api.test POST /graphql"},
		{"POST", "/graphql/", nil, true, "api.test POST /graphql"},
		{"POST", "/repos/o/r", nil, false, reasonNoRequest},
		{"get", "/repos/o/r", nil, false, reasonNoRequest},
		{"GET", "/Repos/o/r", nil, false, reasonNoRequest},
		{"GET", "/repos", nil, false, reasonNoRequest},
		{"GET", "/user", nil, false, reasonNoRequest},
		{"GET", "/repos/o/r", []string{"X-HTTP-Method-Override", "DELETE"}, false, reasonNoRequest},
		{"GET", "/repos/o/r", []string{"X-HTTP-Method-Override", "GET"}, true, "api.test GET,HEAD /repos/"},
		{"GET", "/repos/o/r", []string{"X-HTTP-Method-Override", "GET, DELETE"}, false, reasonOverride},
		{"GET", "/repos/o/r", []string{"X-HTTP-Method-Override", "get"}, false, reasonOverride},
		{"GET", "/repos/o/r", []string{"X_HTTP_METHOD_OVERRIDE", "DELETE"}, false, reasonNoRequest},
		{"GET", "/repos/o/r?_method=DELETE", nil, false, reasonNoRequest},
		{"GET", "/repos/o/r?_method=HEAD", nil, true, "api.test GET,HEAD /repos/"},
		{"GET", "/repos/o/r?a=1;_method=DELETE", nil, false, reasonOverride},
		{"GET", "/repos/o/r?a=1&b=2", nil, true, "api.test GET,HEAD /repos/"},
		{"GET", "/repos/%6f/r", nil, false, reasonPathNotPlain},
		{"GET", "/repos/o/../../user", nil, false, reasonPathNotPlain},
		{"CONNECT", "/repos/o/r", nil, false, reasonConnectIn},
		{"GET", "/repos/o/r", []string{"Upgrade", "h2c"}, false, reasonUpgrade},
		{"GET", "/repos/o/r", []string{"Upgrade", "WebSocket"}, true, "api.test GET,HEAD /repos/"},
	} {
		r := request(t, tc.method, tc.target, tc.headers...)
		ok, why, _, narrowed := p.judge("api.test", 443, r)
		if ok != tc.ok || why != tc.why || !narrowed {
			t.Errorf("%s %s %v: %v (%s), narrowed %v; want %v (%s)", tc.method, tc.target, tc.headers, ok, why, narrowed, tc.ok, tc.why)
		}
	}
	// The host as a whole: allowed, its requests judged one by one.
	if ok, _ := p.decide("api.test", 443); !ok || !p.byRequest("api.test", 443) {
		t.Error("the host isn't allowed, or its requests aren't judged")
	}
	// Another host, or another port, is no rule's.
	if ok, _ := p.decide("api.test", 8443); ok || p.byRequest("other.test", 443) {
		t.Error("another port allowed, or another host judged")
	}
	// With the host's whole allow beside it, every request goes, and the tunnel needn't be decrypted.
	whole := writePolicy(t, "api.test GET /repos/\napi.test\n")
	if ok, _, _, _ := whole.judge("api.test", 443, request(t, "DELETE", "/user/%2e%2e")); !ok || whole.byRequest("api.test", 443) {
		t.Error("the host's whole allow didn't allow every request")
	}
	// A deny of the whole host beats every allow, and an open policy judges nothing.
	denied := writePolicy(t, "api.test GET\n!api.test\n")
	if ok, why, refused, _ := denied.judge("api.test", 443, request(t, "GET", "/")); ok || why != "denied by !api.test" || refused {
		t.Errorf("a whole deny: %v %q %v", ok, why, refused)
	}
	open := newPolicy(nil, nil, nil, quiet())
	open.open = true
	if ok, _, _, narrowed := open.judge("api.test", 443, request(t, "DELETE", "/x")); !ok || narrowed || open.byRequest("api.test", 443) {
		t.Error("an open policy judged a request")
	}
}

// A deny holds however a request spells what it names: whatever path a server could read as
// /gists or below it, decoding it, reading \ as / or dropping ;'s parameters, then cleaning it, in
// any case, with a format's suffix dropped, is refused by a deny of /gists/ alone, and so is any
// spelling of POST, in the method, in any override-like header's value, a list's word included,
// or in a _method parameter.
func FuzzDenyBypass(f *testing.F) {
	for _, s := range []string{"/gists/1", "/%67ists", "/x/../gists", "//gists", "/gists;a/b", `/a\..\gists`, "/GISTS/", "/repos/%2e%2e/gists", "/gists.json", "/gists"} {
		f.Add("GET", s, "", "")
	}
	f.Add("post", "/x", "", "")
	f.Add("GET", "/x", "X-HTTP-Method-Override", "POST, GET")
	f.Add("GET", "/x", "X_HTTP_METHOD_OVERRIDE", "post")
	f.Add("GET", "/x?_METHOD=post", "", "")
	f.Add("GET", "/x?a=1;_method=POST", "", "")
	file := filepath.Join(f.TempDir(), "policy")
	if err := os.WriteFile(file, []byte("example.com\n!example.com /gists/\n!example.com POST\n"), 0o600); err != nil {
		f.Fatal(err)
	}
	p := newPolicy(nil, []string{file}, nil, quiet())
	f.Fuzz(func(t *testing.T, method, target, header, value string) {
		u, err := url.ParseRequestURI(target)
		// A method is a token, as Go's server parses a request's, which is all the proxy judges.
		if err != nil || method == "" || strings.ContainsFunc(method, func(c rune) bool {
			return c <= ' ' || c > '~' || strings.ContainsRune(`"(),/:;<=>?@[\]{}`, c)
		}) {
			return
		}
		r := &http.Request{Method: method, URL: u, Header: http.Header{}}
		if header != "" {
			r.Header[header] = []string{value}
		}
		if ok, _, _, _ := p.judge("example.com", 443, r); !ok {
			return
		}
		for _, seen := range serverReadings(u) {
			if seen == "/gists" || strings.HasPrefix(seen, "/gists/") {
				t.Fatalf("%s %q passed, which a server could read as %q", method, target, seen)
			}
		}
		says := []string{method}
		if n := strings.ReplaceAll(strings.ToLower(header), "_", "-"); n == "x-http-method" || strings.Contains(n, "method-override") {
			says = append(says, strings.FieldsFunc(value, func(c rune) bool { return c == ',' || c == ' ' || c == '\t' })...)
		}
		// The query as servers split it, at & and at ;, each part decoded as far as it can be.
		for part := range strings.FieldsFuncSeq(u.RawQuery, func(c rune) bool { return c == '&' || c == ';' }) {
			k, v, _ := strings.Cut(part, "=")
			if dk, err := url.QueryUnescape(k); err == nil {
				k = dk
			}
			if dv, err := url.QueryUnescape(v); err == nil {
				v = dv
			}
			if strings.EqualFold(strings.TrimSpace(k), "_method") {
				says = append(says, strings.FieldsFunc(v, func(c rune) bool { return c == ',' || c == ' ' || c == '\t' })...)
			}
		}
		for _, m := range says {
			if strings.EqualFold(strings.TrimSpace(m), "POST") {
				t.Fatalf("%s %q with %s: %q passed, which a server could read as POST", method, target, header, value)
			}
		}
	})
}

// serverReadings are the paths servers could read u's as: decoded once or twice, \ read as /,
// each segment's ; parameters dropped, cleaned, in lower case, and each with its last segment's
// format suffix dropped too.
func serverReadings(u *url.URL) []string {
	read := func(s string) string {
		s = strings.ReplaceAll(s, `\`, "/")
		var segs []string
		for seg := range strings.SplitSeq(s, "/") {
			seg, _, _ = strings.Cut(seg, ";")
			segs = append(segs, seg)
		}
		return strings.ToLower(path.Clean("/" + strings.Join(segs, "/")))
	}
	out := []string{read(u.EscapedPath()), read(u.Path)}
	if dec, err := url.PathUnescape(u.Path); err == nil {
		out = append(out, read(dec))
	}
	for _, s := range out {
		dir, last := path.Split(s)
		if cut, _, ok := strings.Cut(last, "."); ok && cut != "" {
			out = append(out, dir+cut)
		}
	}
	return out
}

// An allow by method and path whose hosts a whole allow already covers on 443 narrows nothing
// (Allowlist.CoversWhole); one that only an allow by method and path, another port or a deny
// covers does narrow.
func TestCoversWhole(t *testing.T) {
	t.Parallel()
	files := policyFiles(t, "api.github.com\n*.corp.test\nother.test:8443\n**:22\nnarrow.test GET\n!denied.test\nhttps.test:443\n")
	al, err := ReadAllowlist(nil, nil, files, nil)
	must(t, err)
	nothing, httpOnly, narrows := [2]bool{true, false}, [2]bool{false, true}, [2]bool{}
	for rule, want := range map[string][2]bool{
		"api.github.com GET,HEAD":   nothing,
		"dev.corp.test GET":         nothing,
		"*.dev.corp.test /api/":     nothing,
		"*.corp.test GET":           nothing,
		"api.github.com:443 /user/": nothing, // it covers no port 80
		"https.test GET":            httpOnly,
		"https.test:443 GET":        nothing,
		"https.test:80 GET":         narrows,
		"*.github.com GET":          narrows,
		"other.test GET":            narrows,
		"narrow.test POST":          narrows,
		"denied.test GET":           narrows,
		"elsewhere.test GET":        narrows,
		"!api.github.com POST":      narrows,
		"api.github.com":            narrows,
		"api.github.com:22 GET":     narrows,
	} {
		if n, h := al.CoversWhole(rule); [2]bool{n, h} != want {
			t.Errorf("CoversWhole(%q) = %v, %v; want %v", rule, n, h, want)
		}
	}
}

// A secret's binding narrowed to methods goes in only for them, judged as an allow: another
// method, another case or a method-override header naming another keeps the placeholder.
func TestForRequestMethods(t *testing.T) {
	t.Parallel()
	b := Binding{Name: "k", Kind: KindSecret, Hosts: []string{"api.test"}, Methods: "GET,HEAD", Path: "/v1/", Header: "Authorization", Placeholder: "PH"}
	for _, tc := range []struct {
		method, target string
		headers        []string
		in             bool
	}{
		{"GET", "/v1/charges", nil, true},
		{"HEAD", "/v1/charges", nil, true},
		{"POST", "/v1/charges", nil, false},
		{"get", "/v1/charges", nil, false},
		{"GET", "/v1/charges", []string{"X-HTTP-Method-Override", "POST"}, false},
		{"GET", "/v1/charges", []string{"X_HTTP_METHOD_OVERRIDE", "POST"}, false},
		{"GET", "/v1/charges", []string{"X-HTTP-Method-Override", "GET, POST"}, false},
		{"GET", "/v1/charges?_method=POST", nil, false},
		{"GET", "/v1/charges?_method=HEAD", nil, true},
		{"GET", "/v2/charges", nil, false},
	} {
		if got := b.match().allows(request(t, tc.method, tc.target, tc.headers...)); got != tc.in {
			t.Errorf("%s %s %v: %v, want %v", tc.method, tc.target, tc.headers, got, tc.in)
		}
	}
}

// rulesSetup is interceptSetup with no binding, whose policy is rules for its upstream's host and
// port (PORT in rules) and the address that the floor needs, and a client that keeps one
// connection, HTTP/2 with h2. The upstream echoes each request's method, path and Authorization,
// and counts them.
func rulesSetup(t *testing.T, rules string, h2 bool) (*server, int, *http.Client, *syncBuffer, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	s, port, file, c := interceptSetup(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		fmt.Fprintf(w, "%s %s", r.Method, r.Header.Get("Authorization")) //nolint:gosec // a test echo server
	})
	writeBindings(t, file)
	rules = strings.ReplaceAll(rules, "PORT", fmt.Sprint(port))
	s.egress.pol = writePolicy(t, rules+fmt.Sprintf("127.0.0.1:%d\n", port))
	tr := c.Transport.(*http.Transport)
	tr.ForceAttemptHTTP2 = h2
	if !h2 {
		tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}
	ev := logs(s)
	return s, port, c, ev, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), seen...) }
}

// send makes a request through c and returns its status, its body and the protocol the proxy
// answered with.
func send(t *testing.T, c *http.Client, method, u string) (int, string, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), method, u, nil)
	req.Header.Set("Authorization", "Bearer PH")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Proto
}

// A host whose rules narrow it is decrypted with no secret bound to it, and every request on the
// connection meets the rules, over HTTP/1.1 kept alive and over HTTP/2 alike: allowed ones reach
// the host, logged with the rule that allowed them, and the rest get 403 and a blocked event with
// their method and path, without reaching it.
func TestInterceptRules(t *testing.T) {
	t.Parallel()
	for _, h2 := range []bool{false, true} {
		t.Run(fmt.Sprint("h2=", h2), func(t *testing.T) {
			t.Parallel()
			_, port, c, ev, seen := rulesSetup(t, "localhost:PORT GET /ok/\n", h2)
			base := fmt.Sprintf("https://localhost:%d", port)
			proto := "HTTP/1.1"
			if h2 {
				proto = "HTTP/2.0"
			}
			for i, tc := range []struct {
				method, path string
				code         int
			}{
				{"GET", "/ok/1", 200},
				{"POST", "/ok/1", 403},
				{"GET", "/other", 403},
				{"GET", "/ok/2", 200},
				{"DELETE", "/ok/3", 403},
				{"GET", "/ok/4", 200},
			} {
				code, body, got := send(t, c, tc.method, base+tc.path)
				if code != tc.code || got != proto {
					t.Errorf("request %d, %s %s: %d %q over %s; want %d over %s", i, tc.method, tc.path, code, body, got, tc.code, proto)
				}
			}
			if got, want := seen(), []string{"GET /ok/1 Bearer PH", "GET /ok/2 Bearer PH", "GET /ok/4 Bearer PH"}; strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("the host got %q, want %q", got, want)
			}
			log := ev.String()
			if n := strings.Count(log, `"event":"intercept"`); n != 1 {
				t.Errorf("%d tunnels, want the one kept:\n%s", n, log)
			}
			for _, want := range []string{
				`"event":"forward","host":"localhost","port":` + fmt.Sprint(port) + `,"method":"GET","path":"/ok/2","rule":"localhost:` + fmt.Sprint(port) + ` GET /ok/"`,
				`"event":"blocked","host":"localhost","port":` + fmt.Sprint(port) + `,"method":"POST","path":"/ok/1","reason":"` + reasonNoRequest + `"`,
				`"event":"blocked","host":"localhost","port":` + fmt.Sprint(port) + `,"method":"DELETE","path":"/ok/3","reason":"` + reasonNoRequest + `"`,
			} {
				if !strings.Contains(log, want) {
					t.Errorf("no %s in:\n%s", want, log)
				}
			}
		})
	}
}

// A secret narrowed to some methods goes into each request of a kept-alive or HTTP/2 connection
// that has one of them, and only those: the rest carry the placeholder.
func TestInterceptSecretMethods(t *testing.T) {
	t.Parallel()
	for _, h2 := range []bool{false, true} {
		t.Run(fmt.Sprint("h2=", h2), func(t *testing.T) {
			t.Parallel()
			s, port, c, _, seen := rulesSetup(t, "localhost:PORT\n", h2)
			b := bound("localhost")
			b.Methods = "GET"
			writeBindings(t, s.secrets.file.path, b)
			fakeSecret(s.secrets, "real")
			base := fmt.Sprintf("https://localhost:%d/x", port)
			for _, m := range []string{"GET", "POST", "GET", "PUT"} {
				if code, body, _ := send(t, c, m, base); code != 200 {
					t.Errorf("%s: %d %q", m, code, body)
				}
			}
			want := []string{"GET /x Bearer real", "POST /x Bearer PH", "GET /x Bearer real", "PUT /x Bearer PH"}
			if got := seen(); strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("the host got %q, want %q", got, want)
			}
		})
	}
}

// A host whose rules narrow it is refused where the proxy can't decrypt it, rather than tunneled
// unjudged: on a port it doesn't intercept, and without a CA. Absolute-form plain HTTP is judged
// request by request, on any port.
func TestNarrowedNotDecrypted(t *testing.T) {
	t.Parallel()
	s := testServer(t, Config{}, "localhost:* GET\n127.0.0.1:*\n")
	ev := logs(s)
	addr := listen(t, s)
	for _, port := range []int{443, 8443} {
		code, body := roundTrip(t, addr, fmt.Sprintf("CONNECT localhost:%d HTTP/1.1\r\nHost: localhost:%d\r\n\r\n", port, port))
		if code != http.StatusForbidden || !strings.Contains(body, reasonNotDecrypted) {
			t.Errorf("CONNECT :%d: %d %q", port, code, body)
		}
	}
	if !strings.Contains(ev.String(), `"refused":true`) {
		t.Errorf("not logged as refused:\n%s", ev)
	}
	code, body := roundTrip(t, addr, "POST http://localhost:8080/x HTTP/1.1\r\nHost: localhost:8080\r\nContent-Length: 0\r\n\r\n")
	if code != http.StatusForbidden || !strings.Contains(body, reasonNoRequest) {
		t.Errorf("a plain POST: %d %q", code, body)
	}
	if !strings.Contains(ev.String(), `"method":"POST","path":"/x","reason":"`+reasonNoRequest) {
		t.Errorf("the plain POST isn't logged with its method and path:\n%s", ev)
	}
}
