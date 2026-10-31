package cli

import (
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// sgrCodes matches the SGR codes a colored table has.
var sgrCodes = regexp.MustCompile("\033\\[[0-9;]*m")

// tables runs write on a plain table and on one FORCE_COLOR colors, and
// returns both; without its codes, the colored one is the plain one, so
// its columns line up the same.
func tables(t *testing.T, write func(*ui.Table) error) (plain, colored string) {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("DEVVM_MESSAGES", "")
	var p, c strings.Builder
	for _, tc := range []struct {
		force string
		out   *strings.Builder
	}{{"", &p}, {"1", &c}} {
		t.Setenv("FORCE_COLOR", tc.force)
		if err := write(ui.New(strings.NewReader(""), tc.out, io.Discard).Table()); err != nil {
			t.Fatal(err)
		}
	}
	if sgrCodes.MatchString(p.String()) {
		t.Errorf("plain table has codes: %q", p.String())
	}
	if got := sgrCodes.ReplaceAllString(c.String(), ""); got != p.String() {
		t.Errorf("colored table without its codes:\n%s\nwant the plain one:\n%s", got, p.String())
	}
	return p.String(), c.String()
}

// wantCodes checks the colored table has each of these, painted.
func wantCodes(t *testing.T, colored string, painted ...string) {
	t.Helper()
	for _, w := range painted {
		if !strings.Contains(colored, w) {
			t.Errorf("no %q in\n%q", w, colored)
		}
	}
}

func TestWriteListColored(t *testing.T) {
	ls := []vm.Listed{
		{Name: "devvm-a", Status: string(backend.StatusRunning), Image: "base", Git: "on", Net: "on", Backend: "lima", Folder: "/a"},
		{Name: "devvm-bb", Status: string(backend.StatusStopped), Image: "node+claude", Git: "off", Net: "unenforced", Backend: "docker", DockerLayer: "sysbox", Folder: "/b"},
		{Name: "devvm-c", Status: "Broken", Git: vm.ModeClone, Backend: "lima", Folder: "/c"},
	}
	plain, colored := tables(t, func(tab *ui.Table) error { return writeList(tab, ls) })
	want := "NAME      STATUS   IMAGE        GIT    NET         BACKEND          FOLDER\n" +
		"devvm-a   Running  base         on     on          lima             /a\n" +
		"devvm-bb  Stopped  node+claude  off    unenforced  docker (sysbox)  /b\n" +
		"devvm-c   Broken   -            clone  -           lima             /c\n"
	if plain != want {
		t.Errorf("got\n%s\nwant\n%s", plain, want)
	}
	wantCodes(t, colored, "\033[1mNAME\033[0m", "\033[32mRunning\033[0m", "\033[33mStopped\033[0m", "\033[31mBroken\033[0m",
		"\033[32mon\033[0m", "\033[31munenforced\033[0m", "\033[32mclone\033[0m", "  off  ", "devvm-a  ")
}

func TestWriteImagesColored(t *testing.T) {
	imgs := []listedImage{
		{Listed: build.Listed{Name: "base", DigestID: "sha256:ab", Key: "k1", State: build.StateCurrent, Sized: true, Size: 1 << 30}, Backend: "lima"},
		{Listed: build.Listed{Name: "node", Key: "k2", State: build.StateStale}, Backend: "lima"},
	}
	_, colored := tables(t, func(tab *ui.Table) error {
		return writeImages(tab, imgs, nil, func(tools.Listed) (string, string) { return "", "" })
	})
	wantCodes(t, colored, "\033[1mIMAGE\033[0m", "\033[2msha256:ab\033[0m", "\033[2mk1\033[0m",
		"\033[32mcurrent\033[0m", "\033[33mstale\033[0m", "\033[1mNAME\033[0m")
}
