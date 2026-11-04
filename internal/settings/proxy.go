package settings

import (
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/proxy"
)

// The keys of config.toml's [proxy], the company proxy.
const (
	KeyProxyHTTPS   = "proxy.https"
	KeyProxyHTTP    = "proxy.http"
	KeyProxyNoProxy = "proxy.no-proxy"
)

// HostProxy is the company proxy devvm's proxies go through: the
// environment's (getenv's HTTPS_PROXY, HTTP_PROXY and NO_PROXY,
// backend.ReadHostProxy), all three of it, when it sets HTTPS_PROXY or
// HTTP_PROXY; else config.toml's [proxy], whose URLs never hold a password:
// a URL that names a user gets the secret proxy.UpstreamSecret's (Secret).
// config.toml is read only when the environment sets none, and checked
// whole then.
func HostProxy(d paths.Dirs, getenv func(string) string) (backend.HostProxy, error) {
	if env := backend.ReadHostProxy(getenv); !env.None() {
		return env, nil
	}
	cf, err := decodeConfig(d.ConfigFile())
	if err != nil {
		return backend.HostProxy{}, err
	}
	p := cf.Proxy
	h := backend.HostProxy{HTTPS: p.HTTPS.v, HTTP: p.HTTP.v, NoProxy: p.NoProxy.v}
	if h.None() {
		return backend.HostProxy{}, nil
	}
	for _, raw := range []string{h.HTTPS, h.HTTP} {
		if u, err := proxy.ParseUpstream(raw); raw != "" && err == nil && u.User != nil {
			h.Secret = proxy.UpstreamSecret
		}
	}
	return h, nil
}

type (
	proxyHTTPSKind struct{}
	proxyHTTPKind  struct{}
	noProxyKind    struct{}
)

func (proxyHTTPSKind) parse(v any) (string, error) { return checkedProxyURL(KeyProxyHTTPS, v) }
func (proxyHTTPKind) parse(v any) (string, error)  { return checkedProxyURL(KeyProxyHTTP, v) }

func (noProxyKind) parse(v any) (string, error) {
	return checked(text(v))(func(s string) error { return backend.CheckNoProxy(KeyProxyNoProxy, s) })
}

// checkedProxyURL checks config.toml's proxy URL at key as the
// environment's is checked (backend.CheckProxyURL), and refuses one that
// holds a password: config.toml is a file people copy between machines
// and keep with their dotfiles, so the password goes in devvm's secrets
// (devvm secret set proxy), never in it.
func checkedProxyURL(key string, v any) (string, error) {
	return checked(text(v))(func(s string) error {
		if err := backend.CheckProxyURL(key, s); err != nil {
			return err
		}
		if u, err := proxy.ParseUpstream(s); err == nil && u.User != nil {
			if _, ok := u.User.Password(); ok {
				return msg.ProxyPasswordInConfig(key)
			}
		}
		return nil
	})
}
