package backend

import (
	"errors"
	"net/url"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/proxy"
)

// HostProxy is the proxy the host goes through, as the environment devvm
// runs in sets it: a company's. A VM's network policy proxy reaches allowed
// hosts through it (Upstream), and builds' commands get it (Env); neither
// ever writes it into an image.
type HostProxy struct {
	HTTPS, HTTP, NoProxy string
}

// ReadHostProxy reads HTTPS_PROXY, HTTP_PROXY and NO_PROXY, each in capitals
// first, then lowercase, as Go's own clients do.
func ReadHostProxy(getenv func(string) string) HostProxy {
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(getenv(k)); v != "" {
				return v
			}
		}
		return ""
	}
	return HostProxy{
		HTTPS: first("HTTPS_PROXY", "https_proxy"), HTTP: first("HTTP_PROXY", "http_proxy"),
		NoProxy: first("NO_PROXY", "no_proxy"),
	}
}

// None reports whether the host goes through no proxy.
func (h HostProxy) None() bool { return h.HTTPS == "" && h.HTTP == "" }

// Upstream is the company proxy a VM's proxy or a build's goes through
// (proxy.Config's Upstream): nil for none, or the message for a setting the
// proxy can't go through. It holds the company proxy's password, if it has
// one, so it goes only into the proxy's config, the user's alone.
func (h HostProxy) Upstream() (*proxy.Upstream, error) {
	if h.None() {
		return nil, nil
	}
	for _, s := range []struct{ name, value string }{{"HTTPS_PROXY", h.HTTPS}, {"HTTP_PROXY", h.HTTP}} {
		if s.value == "" {
			continue
		}
		if strings.ContainsAny(s.value, "\r\n") {
			return nil, msg.UpstreamProxyInvalid(s.name)
		}
		if _, err := proxy.ParseUpstream(s.value); errors.Is(err, proxy.ErrUpstreamScheme) {
			scheme := "an unknown"
			if u, err := url.Parse(s.value); err == nil && u.Scheme != "" {
				scheme = strings.ToLower(u.Scheme) + "://"
			}
			return nil, msg.UpstreamProxyScheme(scheme, s.name)
		} else if err != nil {
			return nil, msg.UpstreamProxyInvalid(s.name)
		}
	}
	if strings.ContainsAny(h.NoProxy, "\r\n") {
		return nil, msg.UpstreamProxyInvalid("NO_PROXY")
	}
	return &proxy.Upstream{HTTPS: h.HTTPS, HTTP: h.HTTP, NoProxy: h.NoProxy}, nil
}

// Env is the variables a build's commands get, KEY=value, in capitals and
// lowercase (tools read one or the other): none for a proxy the host
// doesn't set.
func (h HostProxy) Env() []string {
	var out []string
	for _, kv := range [][2]string{{"HTTP_PROXY", h.HTTP}, {"HTTPS_PROXY", h.HTTPS}, {"NO_PROXY", h.NoProxy}} {
		if kv[1] == "" || h.None() {
			continue
		}
		out = append(out, kv[0]+"="+kv[1], strings.ToLower(kv[0])+"="+kv[1])
	}
	return out
}
