package vm

import (
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

func TestCheckGroupName(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, ok := range []string{"web", "team-2", "a"} {
		if err := CheckGroupName(ok); err != nil {
			t.Errorf("CheckGroupName(%q) = %v", ok, names(err))
		}
	}
	for _, bad := range []string{"", "Web", "a_b", "-a", "a.b", "42", strings.Repeat("a", 64)} {
		if got := names(CheckGroupName(bad)); !strings.HasPrefix(got, "group_name_invalid ") {
			t.Errorf("CheckGroupName(%q) = %s", bad, got)
		}
	}
}

// A link its group made reads back as one, with the user's beside it.
func TestReadGroupLinks(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	st, err := state.CreateVM(d, "devvm-app-0123456789abcd", state.NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	a := netip.MustParseAddr("127.77.0.1")
	want := []Link{
		{Name: "db", Port: 5432, Target: "devvm-db-0123456789abcd", TargetID: "i1", Addr: a},
		{Name: "db", Port: 8000, Target: "devvm-db-0123456789abcd", TargetID: "i1", Addr: a, Group: true},
	}
	if err := writeLinks(st, want); err != nil {
		t.Fatal(err)
	}
	if v := st.Get(KeyLinks); v != "db,5432,devvm-db-0123456789abcd,i1,127.77.0.1 db,8000,devvm-db-0123456789abcd,i1,127.77.0.1,group" {
		t.Errorf("links = %q", v)
	}
	if got, err := ReadLinks(st); err != nil || !slices.Equal(got, want) {
		t.Errorf("ReadLinks = %v, %v", got, err)
	}
	for _, bad := range []string{"db,8000,devvm-db-0123456789abcd,i1,127.77.0.1,other", "db,8000,devvm-db-0123456789abcd,i1,127.77.0.1,group,x"} {
		if err := st.Set(KeyLinks, bad); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadLinks(st); err == nil {
			t.Errorf("ReadLinks(%q): want an error", bad)
		}
	}
}

// member makes a VM's state with its published ports and the name it
// joined as.
func member(t *testing.T, d paths.Dirs, name, publish, as string) *state.VM {
	t.Helper()
	st, err := state.CreateVM(d, name, state.NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{KeyPublish: publish, KeyGroupAs: as, KeyGroup: "web"} {
		if err := st.Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

// The group's links in a client: a link to each port each other member
// publishes, at one address per member, named as the user's links to it
// are, else by its name in the group; the user's links stay and take
// precedence.
func TestGroupLinks(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	a := netip.MustParseAddr
	app := member(t, d, "devvm-app-0123456789abcd", "", "")
	db := member(t, d, "devvm-db-0123456789abcd", "5432,8000-8002", "")
	api := member(t, d, "devvm-api-0123456789abcd", "80", "web2")
	none := member(t, d, "devvm-cache-0123456789abcd", "", "")
	dbID, apiID := db.Get(state.KeyID), api.Get(state.KeyID)
	others := []*state.VM{api, db, none}

	links, clashes := groupLinks(app, nil, nil, others)
	want := []Link{
		{Name: "db", Port: 5432, Target: db.Name(), TargetID: dbID, Addr: a("127.77.0.2"), Group: true},
		{Name: "db", Port: 8000, Target: db.Name(), TargetID: dbID, Addr: a("127.77.0.2"), Group: true},
		{Name: "db", Port: 8001, Target: db.Name(), TargetID: dbID, Addr: a("127.77.0.2"), Group: true},
		{Name: "db", Port: 8002, Target: db.Name(), TargetID: dbID, Addr: a("127.77.0.2"), Group: true},
		{Name: "web2", Port: 80, Target: api.Name(), TargetID: apiID, Addr: a("127.77.0.1"), Group: true},
	}
	if !slices.Equal(links, want) || clashes != nil {
		t.Errorf("groupLinks = %v, %v; want %v", links, clashes, want)
	}
	if got := shownLinks(links); got != "db:5432,8000-8002, web2:80" {
		t.Errorf("shownLinks = %q", got)
	}

	// The addresses stay as they were, whatever the order.
	again, _ := groupLinks(app, nil, links, []*state.VM{db, api})
	if !slices.Equal(again, want) {
		t.Errorf("again = %v", again)
	}

	// The user's link to db names it and has its port; one called web2 to
	// another VM takes the name from api, whose links go.
	own := []Link{
		{Name: "pg", Port: 5432, Target: db.Name(), TargetID: dbID, Addr: a("127.77.0.2")},
		{Name: "web2", Port: 9000, Target: "devvm-x-0123456789abcd", TargetID: "i9", Addr: a("127.77.0.3")},
	}
	links, clashes = groupLinks(app, own, slices.Concat(want, own), others)
	got := []string{}
	for _, l := range links {
		got = append(got, formatLink(l))
	}
	if strings.Join(got, " ") != "pg,5432,"+db.Name()+","+dbID+",127.77.0.2 pg,8000,"+db.Name()+","+dbID+",127.77.0.2,group "+
		"pg,8001,"+db.Name()+","+dbID+",127.77.0.2,group pg,8002,"+db.Name()+","+dbID+",127.77.0.2,group "+
		"web2,9000,devvm-x-0123456789abcd,i9,127.77.0.3" {
		t.Errorf("with the user's links: %v", got)
	}
	if !slices.Equal(clashes, []groupClash{{Name: "web2", Target: api.Name(), Own: true}}) {
		t.Errorf("clashes = %v", clashes)
	}
	added, removed := groupChanged(want, links)
	if shownLinks(added) != "pg:8000-8002" || shownLinks(removed) != "db:5432,8000-8002, web2:80" {
		t.Errorf("changed: %s; %s", shownLinks(added), shownLinks(removed))
	}

	// A member whose name is the client's own hostname, or localhost.
	twin := member(t, d, "devvm-app-fedcba98765432", "3000", "")
	local := member(t, d, "devvm-local-0123456789abcd", "3000", "localhost")
	links, clashes = groupLinks(app, nil, nil, []*state.VM{twin, local})
	if links != nil || !slices.Equal(clashes, []groupClash{{Name: "app", Target: twin.Name()}, {Name: "localhost", Target: local.Name()}}) {
		t.Errorf("own hostname: %v, %v", links, clashes)
	}
}

// A link a group made applies only while the two are in that group, so
// one left behind carries nothing.
func TestCheckGroupLink(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	app := member(t, d, "devvm-app-0123456789abcd", "", "")
	db := member(t, d, "devvm-db-0123456789abcd", "5432", "")
	l := Link{Name: "db", Port: 5432, Target: db.Name(), TargetID: db.Get(state.KeyID), Group: true}
	check := func(want string) {
		t.Helper()
		if got, _, err := CheckLink(d, app, l); err != nil || got != want {
			t.Errorf("CheckLink = %q, %v; want %q", got, err, want)
		}
	}
	check("")
	for _, g := range []string{"other", ""} {
		if err := db.Set(KeyGroup, g); err != nil {
			t.Fatal(err)
		}
		check(LinkUngrouped)
	}
	if err := app.Set(KeyGroup, ""); err != nil {
		t.Fatal(err)
	}
	check(LinkUngrouped)
	// The user's own link doesn't need a group.
	l.Group = false
	check("")
}

// A group's members are the VMs whose state names it.
func TestGroupMembers(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	member(t, d, "devvm-db-0123456789abcd", "", "")
	member(t, d, "devvm-app-0123456789abcd", "", "pg")
	other := member(t, d, "devvm-api-0123456789abcd", "", "")
	if err := other.Set(KeyGroup, "other"); err != nil {
		t.Fatal(err)
	}
	ms, err := groupMembers(d, "web")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range ms {
		got = append(got, m.Name()+"="+memberName(m))
	}
	if strings.Join(got, " ") != "devvm-app-0123456789abcd=pg devvm-db-0123456789abcd=db" {
		t.Errorf("members = %v", got)
	}
	if ms, err := groupMembers(d, ""); ms != nil || err != nil {
		t.Errorf("no group: %v, %v", ms, err)
	}
}
