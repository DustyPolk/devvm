package vm

import (
	"net/netip"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
)

func TestLinkArgs(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, ok := range []string{"db", "pg-1", "a", "0db", strings.Repeat("a", 63)} {
		if err := CheckLinkName(ok); err != nil {
			t.Errorf("CheckLinkName(%q) = %v", ok, msg.NamesOf(err))
		}
	}
	for _, bad := range []string{"", "DB", "db_1", "-db", "db-", "db.x", "5432", "a b", "db\n", strings.Repeat("a", 64)} {
		if err := CheckLinkName(bad); err == nil {
			t.Errorf("CheckLinkName(%q): want link_name_invalid", bad)
		}
	}
	for in, want := range map[string]int{"5432": 5432, "1": 1, "65535": 65535} {
		if got, err := ParseLinkPort(in); got != want || err != nil {
			t.Errorf("ParseLinkPort(%q) = %d, %v", in, got, msg.NamesOf(err))
		}
	}
	for _, bad := range []string{"0", "65536", "05432", "5432-5433", "all", " 80", "http"} {
		if _, err := ParseLinkPort(bad); msg.NamesOf(err) != `link_port_invalid value="`+bad+`"` {
			t.Errorf("ParseLinkPort(%q) = %v", bad, msg.NamesOf(err))
		}
	}
	if got, err := ParsePublish("8000-8010, 5432,5432"); got != "8000-8010,5432" || err != nil {
		t.Errorf("ParsePublish = %q, %v", got, msg.NamesOf(err))
	}
	for _, bad := range []string{"all", "none", "0", "x"} {
		if _, err := ParsePublish(bad); msg.NamesOf(err) != `link_ports_invalid value="`+bad+`"` {
			t.Errorf("ParsePublish(%q) = %v", bad, msg.NamesOf(err))
		}
	}
}

func TestLinkAddr(t *testing.T) {
	a := netip.MustParseAddr
	links := []Link{
		{Name: "db", Port: 5432, Target: "devvm-db-0123456789abcd", TargetID: "i1", Addr: a("127.77.0.1")},
		{Name: "db", Port: 8000, Target: "devvm-db-0123456789abcd", TargetID: "i1", Addr: a("127.77.0.1")},
		{Name: "api", Port: 80, Target: "devvm-api-0123456789abcd", TargetID: "i2", Addr: a("127.77.0.3")},
	}
	for _, tc := range []struct {
		target, id string
		want       string
	}{
		{"devvm-db-0123456789abcd", "i1", "127.77.0.1"},  // its own, shared by its ports
		{"devvm-api-0123456789abcd", "i2", "127.77.0.3"}, // its own
		{"devvm-web-0123456789abcd", "i3", "127.77.0.2"}, // the lowest free
		{"devvm-db-0123456789abcd", "i9", "127.77.0.2"},  // db made again: another VM
	} {
		if got, err := linkAddr(links, tc.target, tc.id); err != nil || got != a(tc.want) {
			t.Errorf("linkAddr(%s, %s) = %v, %v; want %s", tc.target, tc.id, got, err, tc.want)
		}
	}
	if got, _ := linkAddr(nil, "devvm-db-0123456789abcd", "i1"); got != a("127.77.0.1") {
		t.Errorf("the first address = %v", got)
	}
	var full []Link
	for i := uint16(1); i < 0xffff; i++ {
		full = append(full, Link{Target: "t", TargetID: "x", Addr: proxy.LinkAddr(i)})
	}
	if got, _ := linkAddr(full, "devvm-db-0123456789abcd", "i1"); got.IsValid() {
		t.Errorf("with every address taken: %v", got)
	}
	if got, _ := linkAddr(full[:300], "devvm-db-0123456789abcd", "i1"); got != a("127.77.1.45") {
		t.Errorf("after 300: %v", got)
	}
}

// Links read back as written, sorted, and a state edited into something
// else is refused.
func TestReadLinks(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	st, err := state.CreateVM(d, "devvm-app-0123456789abcd", state.NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if ls, err := ReadLinks(st); err != nil || ls != nil {
		t.Errorf("no links: %v, %v", ls, err)
	}
	want := []Link{
		{Name: "api", Port: 80, Target: "devvm-api-0123456789abcd", TargetID: "i2", Addr: netip.MustParseAddr("127.77.0.2")},
		{Name: "db", Port: 5432, Target: "devvm-db-0123456789abcd", TargetID: "i1", Addr: netip.MustParseAddr("127.77.0.1")},
		{Name: "db", Port: 6379, Target: "devvm-db-0123456789abcd", TargetID: "i1", Addr: netip.MustParseAddr("127.77.0.1")},
	}
	if err := writeLinks(st, []Link{want[2], want[0], want[1]}); err != nil {
		t.Fatal(err)
	}
	if v := st.Get(KeyLinks); v != "api,80,devvm-api-0123456789abcd,i2,127.77.0.2 db,5432,devvm-db-0123456789abcd,i1,127.77.0.1 db,6379,devvm-db-0123456789abcd,i1,127.77.0.1" {
		t.Errorf("links = %q", v)
	}
	if got, err := ReadLinks(st); err != nil || !slices.Equal(got, want) {
		t.Errorf("ReadLinks = %v, %v", got, err)
	}
	for _, bad := range []string{
		"db,5432,devvm-db-0123456789abcd,i1",
		"DB,5432,devvm-db-0123456789abcd,i1,127.77.0.1",
		"db,0,devvm-db-0123456789abcd,i1,127.77.0.1",
		"db,5432,base,i1,127.77.0.1",
		"db,5432,devvm-db-0123456789abcd,,127.77.0.1",
		"db,5432,devvm-db-0123456789abcd,i1,127.0.0.1",
		"db,5432,devvm-db-0123456789abcd,i1,::ffff:127.77.0.1",
	} {
		if err := st.Set(KeyLinks, bad); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadLinks(st); err == nil {
			t.Errorf("ReadLinks(%q): want an error", bad)
		}
	}
}

// Whether a link applies, from the states as they are: its target's ID,
// its published ports and the two VMs' network policy.
func TestCheckLink(t *testing.T) {
	d := paths.Dirs{Data: filepath.Join(t.TempDir(), "data")}
	client, err := state.CreateVM(d, "devvm-app-0123456789abcd", state.NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := state.CreateVM(d, "devvm-db-0123456789abcd", state.NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	l := Link{Name: "db", Port: 5432, Target: target.Name(), TargetID: target.Get(state.KeyID)}
	check := func(want string) {
		t.Helper()
		if got, _, err := CheckLink(d, client, l); err != nil || got != want {
			t.Errorf("CheckLink = %q, %v; want %q", got, err, want)
		}
	}
	check(LinkUnpublished)
	for _, ports := range []string{"5000-6000", "80,5432"} {
		if err := target.Set(KeyPublish, ports); err != nil {
			t.Fatal(err)
		}
		check("")
	}
	if err := client.Set(KeyNet, "on"); err != nil {
		t.Fatal(err)
	}
	check(LinkPolicy)
	if err := target.Set(netpolicy.KeyNet, "on"); err != nil {
		t.Fatal(err)
	}
	check("")
	// A client without network policy may link to a target with it.
	if err := client.Set(KeyNet, "off"); err != nil {
		t.Fatal(err)
	}
	check("")
	if err := target.Set(KeyPublish, ""); err != nil {
		t.Fatal(err)
	}
	check(LinkUnpublished)
	if _, err := state.CreateVM(d, target.Name(), state.NewVM{Image: "base"}); err != nil {
		t.Fatal(err)
	}
	check(LinkReplaced)
	if err := target.Remove(); err != nil {
		t.Fatal(err)
	}
	check(LinkGone)
}

// A new link's name: --as, else the one the target has, else its folder
// part; one name per target, one target per name, never localhost or
// the client's hostname.
func TestLinkNameFor(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	client := &found{f: project.Folder{Name: "devvm-app-0123456789abcd"}}
	db, api := "devvm-db-0123456789abcd", "devvm-api-0123456789abcd"
	links := []Link{{Name: "pg", Port: 5432, Target: db, TargetID: "i1"}}
	for _, tc := range []struct {
		target, id, as string
		want, err      string
	}{
		{target: db, id: "i1", want: "pg"},
		{target: db, id: "i1", as: "pg", want: "pg"},
		{target: db, id: "i1", as: "db", err: "link_target_named name=pg target=" + db + " vm=devvm-app-0123456789abcd"},
		{target: api, id: "i2", want: "api"},
		{target: api, id: "i2", as: "pg", err: "link_name_taken name=pg target=" + db + " vm=devvm-app-0123456789abcd"},
		{target: api, id: "i2", as: "app", err: "link_name_host name=app vm=devvm-app-0123456789abcd"},
		{target: api, id: "i2", as: "localhost", err: "link_name_host name=localhost vm=devvm-app-0123456789abcd"},
		// db made again: another VM, which the old link's name doesn't name.
		{target: db, id: "i9", want: "db"},
		{target: db, id: "i9", as: "pg", err: "link_name_taken name=pg target=" + db + " vm=devvm-app-0123456789abcd"},
		// A client whose folder part is another's: its own hostname.
		{target: "devvm-app-fedcba98765432", id: "i3", err: "link_name_host name=app vm=devvm-app-0123456789abcd"},
	} {
		got, err := linkNameFor(client, links, tc.target, tc.id, tc.as)
		if got != tc.want || msg.NamesOf(err) != tc.err {
			t.Errorf("linkNameFor(%s, %s, %q) = %q, %v; want %q, %s", tc.target, tc.id, tc.as, got, msg.NamesOf(err), tc.want, tc.err)
		}
	}
}
