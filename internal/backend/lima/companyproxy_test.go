package lima

import (
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/settings"
)

// Ubuntu's listing is asked for through config.toml's company proxy when
// the environment sets none.
func TestUbuntuThroughCompanyProxy(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.RequestURI)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	d, _ := sandbox(t)
	if err := os.MkdirAll(d.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.ConfigFile(), []byte("[proxy]\nhttp = \""+srv.URL+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cp := settings.NewCompanyProxy(d, func(string) string { return "" }, nil)
	l := New(d, assets.FS, testUI{}, platform.Host{}, "http://ubuntu.example", cp)
	if _, err := l.Ubuntu.Newest(t.Context(), ""); err == nil {
		t.Error("a listing the proxy didn't have")
	}
	want := "GET http://ubuntu.example/"
	if !slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, want) }) {
		t.Errorf("the company proxy got %q, want %s", got, want)
	}
}
