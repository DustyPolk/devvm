package backend

import (
	"errors"
	"net/url"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/proxy"
)

// HostProxy is the proxy the host goes through: a company's, as the
// environment devvm runs in sets it, or else config.toml
// (settings.HostProxy). A VM's proxy reaches allowed hosts
// through it (Upstream), and builds' commands get it (Env); neither ever
// writes it into an image.
type HostProxy struct {
	HTTPS, HTTP, NoProxy string
	// Secret is the devvm secret (proxy.UpstreamSecret) whose value is the
	// password of a proxy URL that names a user and no password:
	// config.toml's, which never holds one. The proxy looks it up as it
	// starts. "" for none, as the environment's URLs carry their own.
	Secret string
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

// CheckProxyURL refuses a company proxy's URL that the proxy can't go
// through, naming setting (HTTPS_PROXY, HTTP_PROXY, or config.toml's key):
// a line break, a URL it can't read (proxy.ParseUpstream), or a scheme but
// http://. "" is none, and passes. Its messages never hold the URL, which
// may hold a password.
func CheckProxyURL(setting, value string) error {
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "\r\n") {
		return msg.UpstreamProxyInvalid(setting)
	}
	if _, err := proxy.ParseUpstream(value); errors.Is(err, proxy.ErrUpstreamScheme) {
		scheme := "an unknown"
		if u, err := url.Parse(value); err == nil && u.Scheme != "" {
			scheme = strings.ToLower(u.Scheme) + "://"
		}
		return msg.UpstreamProxyScheme(scheme, setting)
	} else if err != nil {
		return msg.UpstreamProxyInvalid(setting)
	}
	return nil
}

// CheckNoProxy refuses a list of hosts that go directly that holds a line
// break, naming setting (NO_PROXY, or config.toml's key).
func CheckNoProxy(setting, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return msg.UpstreamProxyInvalid(setting)
	}
	return nil
}

// Upstream is the company proxy a VM's proxy or a build's goes through
// (proxy.Config's Upstream): nil for none, or the message for a setting the
// proxy can't go through. It holds the company proxy's password when the
// environment's URL has one, so it goes only into the proxy's config, the
// user's alone; config.toml's password stays in devvm's secrets (Secret).
func (h HostProxy) Upstream() (*proxy.Upstream, error) {
	if h.None() {
		return nil, nil
	}
	for _, s := range []struct{ name, value string }{{"HTTPS_PROXY", h.HTTPS}, {"HTTP_PROXY", h.HTTP}} {
		if err := CheckProxyURL(s.name, s.value); err != nil {
			return nil, err
		}
	}
	if err := CheckNoProxy("NO_PROXY", h.NoProxy); err != nil {
		return nil, err
	}
	return &proxy.Upstream{HTTPS: h.HTTPS, HTTP: h.HTTP, NoProxy: h.NoProxy, Secret: h.Secret}, nil
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
