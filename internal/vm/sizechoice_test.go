package vm

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
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
	return SizeChoice{Dirs: d, Assets: assets.FS, Host: h, Answer: a.answer, Say: a.record, Warn: a.record, Info: a.record}
}

// Ask starts from the saved size, else base's fitted to the host; Enter
// keeps each, a refused value is said and asked again, and only what's
// typed is saved, with the saved disk kept.
func TestSizeChoiceAsk(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	big, small := Host{CPUs: 8, MemoryGiB: 32}, Host{CPUs: 2, MemoryGiB: 6}
	for _, tc := range []struct {
		name    string
		host    Host
		saved   state.Size
		answers []string
		want    *state.Size // the size file afterwards; nil for none
		shown   []string
	}{
		{
			name: "Enter keeps base's", host: big, answers: []string{"", " "},
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
			name: "typed", host: big, answers: []string{"2", "4G"}, want: &state.Size{CPUs: "2", Memory: "4"},
			shown: []string{
				"size_question have_cpus=8 have_memory=32", "size_cpus_prompt cpus=4",
				"size_memory_prompt memory=8", "size_defaults size=cpus=2 memory=4GiB",
			},
		},
		{
			name: "refused, then asked again", host: big, answers: []string{"16", "x", "6", "31", ""},
			want: &state.Size{CPUs: "6"},
			shown: []string{
				"size_question have_cpus=8 have_memory=32",
				"size_cpus_prompt cpus=4", "too_many_cpus have=8 want=16",
				"size_cpus_prompt cpus=4", `cpus_not_number value="x"`,
				"size_cpus_prompt cpus=4",
				"size_memory_prompt memory=8", "too_much_memory have=32 max=30 want=31",
				"size_memory_prompt memory=8", "size_defaults size=cpus=6 memory=8GiB",
			},
		},
		{
			name: "from the saved size", host: big, saved: state.Size{CPUs: "6", Disk: "60"}, answers: []string{"", "16"},
			want: &state.Size{CPUs: "6", Memory: "16", Disk: "60"},
			shown: []string{
				"size_question have_cpus=8 have_memory=32", "size_cpus_prompt cpus=6",
				"size_memory_prompt memory=8", "size_defaults size=cpus=6 memory=16GiB disk=60GiB",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := paths.Dirs{Config: t.TempDir()}
			if tc.saved != (state.Size{}) {
				if err := state.WriteSize(d, tc.saved); err != nil {
					t.Fatal(err)
				}
			}
			a := &sizeAsker{t: t, answers: tc.answers}
			if err := a.choice(d, tc.host).Ask(); err != nil {
				t.Fatal(msg.NamesOf(err))
			}
			if !slices.Equal(a.shown, tc.shown) {
				t.Errorf("shown\n%q\nwant\n%q", a.shown, tc.shown)
			}
			_, err := os.Stat(d.Size())
			if written := err == nil; written != (tc.want != nil || tc.saved != (state.Size{})) {
				t.Errorf("size file written %v", written)
			}
			got, err := state.ReadSize(d)
			if err != nil {
				t.Fatal(err)
			}
			want := tc.saved
			if tc.want != nil {
				want = *tc.want
			}
			if got != want {
				t.Errorf("saved %+v, want %+v", got, want)
			}
		})
	}
}

// Set saves what the flags give, checked, keeping the rest as saved.
func TestSizeChoiceSet(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	d := paths.Dirs{Config: t.TempDir()}
	if err := state.WriteSize(d, state.Size{Disk: "60"}); err != nil {
		t.Fatal(err)
	}
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
	if got, err := state.ReadSize(d); err != nil || got != (state.Size{CPUs: "2", Memory: "4", Disk: "60"}) {
		t.Errorf("saved %+v, %v", got, err)
	}
	if !slices.Equal(a.shown, []string{"size_defaults size=cpus=2 memory=4GiB disk=60GiB"}) {
		t.Errorf("shown %q", a.shown)
	}
}

// The first run asks for new VMs' size in a terminal, before base's
// features are chosen, when no size is saved and none was given; the new
// VM gets what's typed, and the next run doesn't ask.
func TestFirstRunSize(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	a := &sizeAsker{t: t}
	e.Interactive = func() bool { return true }
	e.Answer = a.answer
	e.Say, e.Warn, e.Info = a.record, a.record, a.record

	asked := func(given Size) bool {
		t.Helper()
		a.shown, a.answers = nil, []string{"", ""}
		if err := (creation{Launch{e}}).firstRunSize(given); err != nil {
			t.Fatal(msg.NamesOf(err))
		}
		return len(a.shown) > 0
	}
	if asked(Size{CPUs: "2"}) || asked(Size{Memory: "4"}) {
		t.Error("asked with --cpus or --memory")
	}
	e.Interactive = func() bool { return false }
	if asked(Size{}) {
		t.Error("asked without a terminal")
	}
	e.Interactive = func() bool { return true }
	if err := state.WriteSize(e.Dirs, state.Size{Disk: "120"}); err != nil {
		t.Fatal(err)
	}
	if asked(Size{}) {
		t.Error("asked with a size saved")
	}
	if err := os.Remove(e.Dirs.Size()); err != nil {
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
	if _, err := os.Stat(filepath.Join(e.Dirs.Config, "base-features")); err != nil {
		t.Errorf("base's features unchosen: %v", err)
	}
	if asked(Size{}) {
		t.Error("asked again after the first run")
	}
}
