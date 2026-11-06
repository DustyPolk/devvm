package backend

import (
	"slices"
	"testing"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/proxy"
)

// corpProxy is a fake company proxy, with a fake password.
const corpProxy = "http://u:p@proxy.corp:8080" //nolint:gosec // a test's fake credential

func TestHostProxy(t *testing.T) {
	env := map[string]string{
		"HTTPS_PROXY": corpProxy, "https_proxy": "http://other:1",
		"http_proxy": "proxy.corp:3128", "no_proxy": "localhost,.corp",
	}
	h := ReadHostProxy(func(k string) string { return env[k] })
	if h != (HostProxy{HTTPS: corpProxy, HTTP: "proxy.corp:3128", NoProxy: "localhost,.corp"}) {
		t.Errorf("read %+v (capitals first, then lowercase)", h)
	}
	up, err := h.Upstream()
	if want := (proxy.Upstream{HTTPS: corpProxy, HTTP: "proxy.corp:3128", NoProxy: "localhost,.corp"}); err != nil || up == nil || *up != want {
		t.Errorf("Upstream = %+v, %v", up, err)
	}
	if got, want := h.Env(), []string{
		"HTTP_PROXY=proxy.corp:3128", "http_proxy=proxy.corp:3128", "HTTPS_PROXY=" + corpProxy,
		"https_proxy=" + corpProxy, "NO_PROXY=localhost,.corp", "no_proxy=localhost,.corp",
	}; !slices.Equal(got, want) {
		t.Errorf("Env = %q", got)
	}
	// config.toml's password is a secret's, which the proxy looks up.
	withSecret := HostProxy{HTTPS: "http://u@proxy.corp:8080", Secret: proxy.UpstreamSecret}
	if up, err := withSecret.Upstream(); err != nil || up == nil || up.Secret != proxy.UpstreamSecret || up.HTTPS != withSecret.HTTPS {
		t.Errorf("with a secret: %+v, %v", up, err)
	}
	if err := CheckProxyURL("proxy.https", ""); err != nil {
		t.Errorf("none: %v", err)
	}
	// NO_PROXY alone is no proxy.
	none := HostProxy{NoProxy: "*"}
	if up, err := none.Upstream(); !none.None() || up != nil || err != nil || none.Env() != nil {
		t.Errorf("none: %+v %v %q", up, err, none.Env())
	}
	for _, c := range []struct {
		h    HostProxy
		want string
	}{
		{HostProxy{HTTPS: "https://proxy.corp"}, "upstream_proxy_scheme scheme=https:// setting=HTTPS_PROXY"},
		{HostProxy{HTTP: "SOCKS5://proxy.corp:1080"}, "upstream_proxy_scheme scheme=socks5:// setting=HTTP_PROXY"},
		{HostProxy{HTTPS: "http://proxy.corp:x"}, "upstream_proxy_invalid setting=HTTPS_PROXY"},
		{HostProxy{HTTPS: "http://proxy.corp\nX=1"}, "upstream_proxy_invalid setting=HTTPS_PROXY"},
		{HostProxy{HTTPS: "proxy.corp:8080", NoProxy: "a\nHTTPS_PROXY=x"}, "upstream_proxy_invalid setting=NO_PROXY"},
	} {
		_, err := c.h.Upstream()
		if msg.NamesOf(err) != c.want {
			t.Errorf("%+v: %v, want %s", c.h, err, c.want)
		}
	}
}
