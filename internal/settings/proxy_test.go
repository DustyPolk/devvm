package settings

import (
	"errors"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
)

// The company proxy: the environment's, all of it, when it sets
// HTTPS_PROXY or HTTP_PROXY; else config.toml's, whose URL that names a
// user gets its password from the secret proxy.
func TestHostProxy(t *testing.T) {
	d := testDirs(t)
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	host := func() backend.HostProxy {
		t.Helper()
		h, err := HostProxy(d, getenv)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if h := host(); h != (backend.HostProxy{}) {
		t.Errorf("no file, no environment: %+v", h)
	}
	write(t, d.ConfigFile(), fullConfig)
	want := backend.HostProxy{
		HTTPS: "http://alice@proxy.corp.example:3128", HTTP: "proxy.corp.example:3128",
		NoProxy: "localhost,.corp.example", Secret: proxy.UpstreamSecret,
	}
	if h := host(); h != want {
		t.Errorf("config.toml's: %+v", h)
	}
	// No user: no secret.
	write(t, d.ConfigFile(), "[proxy]\nhttp = \"proxy.corp.example:3128\"\n")
	if h := host(); h != (backend.HostProxy{HTTP: "proxy.corp.example:3128"}) {
		t.Errorf("no user: %+v", h)
	}
	// NO_PROXY alone is no proxy.
	write(t, d.ConfigFile(), "[proxy]\nno-proxy = \"*\"\n")
	if h := host(); h != (backend.HostProxy{}) {
		t.Errorf("no-proxy alone: %+v", h)
	}
	// The environment's wins whole: config.toml's no-proxy isn't mixed in,
	// and a broken config.toml isn't read.
	write(t, d.ConfigFile(), fullConfig)
	env["https_proxy"] = "http://env.corp:8080"
	if h := host(); h != (backend.HostProxy{HTTPS: "http://env.corp:8080"}) {
		t.Errorf("the environment's: %+v", h)
	}
	write(t, d.ConfigFile(), "[proxy]\nbogus = 1\n")
	if h := host(); h.HTTPS != "http://env.corp:8080" {
		t.Errorf("the environment's, config.toml broken: %+v", h)
	}
	// NO_PROXY alone in the environment leaves config.toml's, which a
	// broken file fails.
	env = map[string]string{"NO_PROXY": "localhost"}
	if _, err := HostProxy(d, getenv); !errors.As(err, new(*state.CorruptError)) {
		t.Errorf("broken config.toml: %v", err)
	}
}
