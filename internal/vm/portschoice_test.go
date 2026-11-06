package vm

import (
	"slices"
	"strings"
	"testing"

	"github.com/devvmhq/devvm/internal/msg"
)

// The ports picker's rows are every port, none and a list, the summary's
// on: a list on for a list.
func TestPortsRows(t *testing.T) {
	for shown, want := range map[string]string{
		"all":  "*all:new_vm_ports_all none:new_vm_ports_none list:new_vm_ports_list",
		"none": "all:new_vm_ports_all *none:new_vm_ports_none list:new_vm_ports_list",
		"3000": "all:new_vm_ports_all none:new_vm_ports_none *list:new_vm_ports_list",
	} {
		var got []string
		for _, r := range portsRows(shown) {
			s := r.Name + ":" + r.Desc.ID()
			if r.On {
				s = "*" + s
			}
			got = append(got, s)
		}
		if strings.Join(got, " ") != want {
			t.Errorf("rows for %s: %v, want %s", shown, got, want)
		}
	}
}

// Each choice gives its ports; a list is asked for, Enter keeping the
// summary's and one that isn't a list asked again; where it can't ask,
// or without a picker, the summary's stand.
func TestChoosePorts(t *testing.T) {
	prompt := "new_vm_ports_prompt ports=3000"
	for _, tc := range []struct {
		name     string
		pick     int // -1: no picker
		answers  []string
		terminal bool
		want     string
		shown    []string
	}{
		{name: "every port", pick: 0, terminal: true, want: "all", shown: []string{"new_vm_ports_help vm=devvm-x"}},
		{name: "none", pick: 1, terminal: true, want: "none", shown: []string{"new_vm_ports_help vm=devvm-x"}},
		{
			name: "a list", pick: 2, terminal: true, answers: []string{"5173, 8000-8010"}, want: "5173,8000-8010",
			shown: []string{"new_vm_ports_help vm=devvm-x", prompt},
		},
		{
			name: "not a list, then a list", pick: 2, terminal: true, answers: []string{"5173-", "5173"}, want: "5173",
			shown: []string{"new_vm_ports_help vm=devvm-x", prompt, `port_invalid value="5173-"`, prompt},
		},
		{
			name: "Enter keeps the summary's", pick: 2, terminal: true, answers: []string{""}, want: "3000",
			shown: []string{"new_vm_ports_help vm=devvm-x", prompt},
		},
		{name: "can't ask", pick: 2, want: "3000", shown: []string{"new_vm_ports_help vm=devvm-x"}},
		{name: "no picker", pick: -1, want: "3000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &sizeAsker{t: t, answers: tc.answers}
			fk := a.ui()
			fk.Terminal = tc.terminal
			e := creation{Launch{&Env{Reporter: fk}}}
			if tc.pick >= 0 {
				e.PickOne = func(help msg.M, _ []PickRow) int { a.record(help); return tc.pick }
			}
			if got := e.choosePorts("devvm-x", "3000"); got != tc.want {
				t.Errorf("choosePorts = %q, want %q", got, tc.want)
			}
			if !slices.Equal(a.shown, tc.shown) {
				t.Errorf("shown\n%q\nwant\n%q", a.shown, tc.shown)
			}
		})
	}
}
