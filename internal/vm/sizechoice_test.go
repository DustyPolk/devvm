package vm

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/settings"
)

// sizeAsker is a SizeChoice on a temporary config folder whose Answer
// gives answers in turn, recording what it printed and asked.
type sizeAsker struct {
	t       *testing.T
	answers []string
	shown   []string // each message's names, in order
}

func (a *sizeAsker) record(m msg.M) { a.shown = append(a.shown, m.Names()) }

func (a *sizeAsker) answer(m msg.M) string {
	a.record(m)
	if len(a.answers) == 0 {
		a.t.Fatalf("asked %s with no answer left", m.Names())
	}
	v := a.answers[0]
	a.answers = a.answers[1:]
	return v
}

func (a *sizeAsker) choice(d paths.Dirs, h Host) SizeChoice {
	return SizeChoice{Dirs: d, Host: h, Reporter: a.ui()}
}

// ui asks for values with answer, in a terminal, and records what it's
// told.
func (a *sizeAsker) ui() *msg.Fake {
	return &msg.Fake{Terminal: true, OnAnswer: a.answer, OnSay: a.record, OnNote: a.record, OnWarn: a.record, OnInfo: a.record}
}

// saveSize saves s as new VMs' size in d's settings.json.
func saveSize(t *testing.T, d paths.Dirs, s Size) {
	t.Helper()
	if _, err := settings.Save(d, sizeValues(s)); err != nil {
		t.Fatal(err)
	}
}

// savedSize is new VMs' size as d's files give it.
func savedSize(t *testing.T, d paths.Dirs) Size {
	t.Helper()
	def, err := settings.Read(d)
	if err != nil {
		t.Fatal(err)
	}
	return defaultSize(def)
}

// testDirs are devvm's folders in a test's own temporary folder.
func testDirs(t *testing.T) paths.Dirs {
	t.Helper()
	dir := t.TempDir()
	return paths.Dirs{Config: filepath.Join(dir, "config"), Data: filepath.Join(dir, "data")}
}

// Ask starts from the saved size, else base's fitted to the host; Enter
// keeps each, a refused value is said and asked again, and only what's
// typed is saved, with the saved disk kept. What config.toml sets isn't
// asked for.
func TestSizeChoiceAsk(t *testing.T) {
	big, small := Host{CPUs: 8, MemoryGiB: 32}, Host{CPUs: 2, MemoryGiB: 6}
	for _, tc := range []struct {
		name    string
		host    Host
		config  string // config.toml
		saved   Size
		answers []string
		want    *Size // the size saved afterwards; nil for none
		shown   []string
	}{
		{
			name: "Enter keeps base's", host: big, answers: []string{"", ""},
			shown: []string{
				"size_question have_cpus=8 have_memory=32", "size_cpus_prompt cpus=4",
				"size_memory_prompt memory=8", "size_defaults size=cpus=4 memory=8GiB",
			},
		},
		{
			name: "a small host's fitted", host: small, answers: []string{"", ""},
			shown: []string{
				"size_question have_cpus=2 have_memory=6", "size_cpus_prompt cpus=2",
				"size_memory_prompt memory=3", "size_defaults size=cpus=2 memory=3GiB",
			},
		},
		{
			name: "typed", host: big, answers: []string{"2", "4G"}, want: &Size{CPUs: "2", Memory: "4"},
			shown: []string{
				"size_question have_cpus=8 have_memory=32", "size_cpus_prompt cpus=4",
				"size_memory_prompt memory=8", "size_defaults size=cpus=2 memory=4GiB",
			},
		},
		{
			name: "refused, then asked again", host: big, answers: []string{"16", "x", "6", "31", ""},
			want: &Size{CPUs: "6"},
			shown: []string{
				"size_question have_cpus=8 have_memory=32",
				"size_cpus_prompt cpus=4", "too_many_cpus have=8 want=16",
				"size_cpus_prompt cpus=4", `cpus_not_number flag=--cpus value="x"`,
				"size_cpus_prompt cpus=4",
				"size_memory_prompt memory=8", "too_much_memory have=32 max=30 want=31",
				"size_memory_prompt memory=8", "size_defaults size=cpus=6 memory=8GiB",
			},
		},
		{
			name: "from the saved size", host: big, saved: Size{CPUs: "6", Disk: "60"}, answers: []string{"", "16"},
			want: &Size{CPUs: "6", Memory: "16", Disk: "60"},
			shown: []string{
				"size_question have_cpus=8 have_memory=32", "size_cpus_prompt cpus=6",
				"size_memory_prompt memory=8", "size_defaults size=cpus=6 memory=16GiB disk=60GiB",
			},
		},
		{
			name: "config.toml's CPUs", host: big, config: "[size]\ncpus = 2\n", answers: []string{"16"},
			want: &Size{CPUs: "2", Memory: "16"},
			shown: []string{
				"setup_in_config file=$CONFIG key=size.cpus", "size_question have_cpus=8 have_memory=32",
				"size_memory_prompt memory=8", "size_defaults size=cpus=2 memory=16GiB",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDirs(t)
			if tc.config != "" {
				writeFile(t, d.ConfigFile(), tc.config)
			}
			if tc.saved != (Size{}) {
				saveSize(t, d, tc.saved)
			}
			a := &sizeAsker{t: t, answers: tc.answers}
			if err := a.choice(d, tc.host).Ask(); err != nil {
				t.Fatal(msg.NamesOf(err))
			}
			for i, s := range tc.shown {
				tc.shown[i] = strings.ReplaceAll(s, "$CONFIG", d.ConfigFile())
			}
			if !slices.Equal(a.shown, tc.shown) {
				t.Errorf("shown\n%q\nwant\n%q", a.shown, tc.shown)
			}
			_, err := os.Stat(d.Settings())
			if written := err == nil; written != (tc.want != nil || tc.saved != (Size{})) {
				t.Errorf("settings.json written %v", written)
			}
			want := tc.saved
			if tc.want != nil {
				want = *tc.want
			}
			if got := savedSize(t, d); got != want {
				t.Errorf("saved %+v, want %+v", got, want)
			}
		})
	}
}

// Where its question is refused, Ask returns the refusal, naming devvm
// size -g, having said and saved nothing.
func TestSizeChoiceRefused(t *testing.T) {
	d := testDirs(t)
	a := &sizeAsker{t: t}
	c := a.choice(d, Host{CPUs: 8, MemoryGiB: 32})
	c.Reporter.(*msg.Fake).Terminal = false
	if err := c.Ask(); msg.NamesOf(err) != "no_input answer=devvm size -g" {
		t.Errorf("Ask: %v", msg.NamesOf(err))
	}
	if _, err := os.Stat(d.Settings()); err == nil || len(a.shown) != 0 {
		t.Errorf("settings.json written, or said %q", a.shown)
	}
}

// Set saves what the flags give, checked, keeping the rest as saved; a
// size config.toml sets is refused.
func TestSizeChoiceSet(t *testing.T) {
	d := testDirs(t)
	saveSize(t, d, Size{Disk: "60"})
	a := &sizeAsker{t: t}
	c := a.choice(d, Host{CPUs: 8, MemoryGiB: 32})
	if err := c.Set(Size{CPUs: "2", Memory: "4GiB"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	for _, s := range []Size{{CPUs: "9"}, {Memory: "31"}, {Memory: "x"}} {
		if err := c.Set(s); err == nil {
			t.Errorf("Set(%+v) took it", s)
		}
	}
	if got := savedSize(t, d); got != (Size{CPUs: "2", Memory: "4", Disk: "60"}) {
		t.Errorf("saved %+v", got)
	}
	if !slices.Equal(a.shown, []string{"size_defaults size=cpus=2 memory=4GiB disk=60GiB"}) {
		t.Errorf("shown %q", a.shown)
	}
	writeFile(t, d.ConfigFile(), "[size]\nmemory = \"8G\"\n")
	if err := c.Set(Size{Memory: "16"}); msg.NamesOf(err) != "set_in_config file="+d.ConfigFile()+" key=size.memory" {
		t.Errorf("Set over config.toml's: %v", msg.NamesOf(err))
	}
	if err := c.Set(Size{CPUs: "4"}); err != nil {
		t.Errorf("Set what config.toml leaves: %v", msg.NamesOf(err))
	}
	if got := savedSize(t, d); got != (Size{CPUs: "4", Memory: "8", Disk: "60"}) {
		t.Errorf("in effect %+v", got)
	}
}

// writeFile writes a test's file, its folder first.
func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The first run asks for new VMs' size, before base's features are
// chosen, when no size is set and none was given; the new VM gets what's
// typed, and the next run doesn't ask. Refused (no terminal), it goes on
// with the size new VMs get.
func TestFirstRunSize(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	a := &sizeAsker{t: t}
	ui := a.ui()
	ui.OnAsk, ui.OnConfirm = unasked(t), unasked(t)
	e.Reporter = ui
	// terminals has questions asked, or refused.
	terminals := func(on bool) { ui.Terminal = on }
	terminals(true)

	asked := func(given Size) bool {
		t.Helper()
		a.shown, a.answers = nil, []string{"", ""}
		def, err := settings.Read(e.Dirs)
		if err != nil {
			t.Fatal(err)
		}
		if err := (creation{Launch{e}}).firstRunSize(given, def); err != nil {
			t.Fatal(msg.NamesOf(err))
		}
		return len(a.shown) > 0
	}
	if asked(Size{CPUs: "2"}) || asked(Size{Memory: "4"}) {
		t.Error("asked with --cpus or --memory")
	}
	terminals(false)
	if asked(Size{}) {
		t.Error("asked without a terminal")
	}
	terminals(true)
	writeFile(t, e.Dirs.ConfigFile(), "[size]\ndisk = 120\n")
	if asked(Size{}) {
		t.Error("asked with a size set")
	}
	if err := os.Remove(e.Dirs.ConfigFile()); err != nil {
		t.Fatal(err)
	}

	a.shown, a.answers = nil, []string{"2", "4"}
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	if got := f.Instances[st.Name()].Size; got.CPUs != 2 || got.MemoryGiB != 4 {
		t.Errorf("the new VM got %+v", got)
	}
	if len(a.shown) == 0 || a.shown[0] != "size_question have_cpus=8 have_memory=32" {
		t.Errorf("shown %q", a.shown)
	}
	if def, err := settings.Read(e.Dirs); err != nil || !def.Features.Chosen {
		t.Errorf("base's features unchosen: %+v, %v", def.Features, err)
	}
	if asked(Size{}) {
		t.Error("asked again after the first run")
	}
}
