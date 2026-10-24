package state

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
)

func TestSize(t *testing.T) {
	d := testDirs(t)
	got, err := ReadSize(d)
	if err != nil || got != (Size{}) {
		t.Fatalf("no file: got %+v, %v", got, err)
	}
	want := Size{CPUs: "2", Disk: "60"}
	if err := WriteSize(d, want); err != nil {
		t.Fatal(err)
	}
	if s := read(t, d.Size()); s != "cpus=2\ndisk=60\n" {
		t.Errorf("file: %q", s)
	}
	if got, err = ReadSize(d); err != nil || got != want {
		t.Errorf("got %+v, %v; want %+v", got, err, want)
	}
	if err := WriteSize(d, Size{Memory: "8\n"}); err == nil {
		t.Error("a value with a line break: want an error")
	}
	write(t, d.Size(), "cpus=2\nswap=1\n")
	_, err = ReadSize(d)
	wantCorrupt(t, err, d.Size(), 2)
}

func TestPorts(t *testing.T) {
	d := testDirs(t)
	if got, err := ReadPorts(d); err != nil || got != "" {
		t.Fatalf("no file: got %q, %v", got, err)
	}
	for _, p := range []string{"none", "5173,8000-8010", "all"} {
		if err := WritePorts(d, p); err != nil {
			t.Fatal(err)
		}
		if got, err := ReadPorts(d); err != nil || got != p {
			t.Errorf("got %q, %v; want %q", got, err, p)
		}
	}
	for _, p := range []string{"", "a\nb"} {
		if err := WritePorts(d, p); err == nil {
			t.Errorf("WritePorts(%q): want an error", p)
		}
	}
	write(t, d.Ports(), " 5173 \n")
	if got, err := ReadPorts(d); err != nil || got != "5173" {
		t.Errorf("edited by hand: got %q, %v", got, err)
	}
	write(t, d.Ports(), "all\nnone\n")
	_, err := ReadPorts(d)
	wantCorrupt(t, err, d.Ports(), 2)
}

func TestBaseFeatures(t *testing.T) {
	d := testDirs(t)
	if _, err := ReadBaseFeatures(d); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("not chosen yet: got %v, want fs.ErrNotExist", err)
	}
	for _, want := range [][]string{{"fzf", "omb"}, {}} {
		if err := WriteBaseFeatures(d, want); err != nil {
			t.Fatal(err)
		}
		got, err := ReadBaseFeatures(d)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	for _, bad := range [][]string{{"fzf", "fzf"}, {"My_DB"}, {""}} {
		if err := WriteBaseFeatures(d, bad); err == nil {
			t.Errorf("WriteBaseFeatures(%q): want an error", bad)
		}
	}
	tests := []struct {
		name, data string
		line       int
	}{
		{"invalid name", "fzf\n../evil\n", 2},
		{"listed twice", "fzf\nomb\nfzf\n", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			write(t, d.BaseFeatures(), tt.data)
			_, err := ReadBaseFeatures(d)
			wantCorrupt(t, err, d.BaseFeatures(), tt.line)
		})
	}
}

func TestAgentKeys(t *testing.T) {
	d := testDirs(t)
	if got, err := ReadAgentKeys(d); err != nil || got != nil {
		t.Fatalf("no file: got %v, %v", got, err)
	}
	want := []AgentKey{
		{File: "/k/deploy key", Fingerprint: "SHA256:fp-deploy"},
		{File: "/k/other", Fingerprint: "SHA256:fp-other", Options: "confirm lifetime=30m"},
	}
	if err := WriteAgentKeys(d, want); err != nil {
		t.Fatal(err)
	}
	if s := read(t, d.AgentKeys()); s != "/k/deploy key\tSHA256:fp-deploy\t\n/k/other\tSHA256:fp-other\tconfirm lifetime=30m\n" {
		t.Errorf("file: %q", s)
	}
	got, err := ReadAgentKeys(d)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	write(t, d.AgentKeys(), "/k/main\t\n/k/bare\n")
	got, err = ReadAgentKeys(d)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []AgentKey{{File: "/k/main"}, {File: "/k/bare"}}) {
		t.Errorf("short lines: got %+v", got)
	}

	if err := WriteAgentKeys(d, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadAgentKeys(d); err != nil || len(got) != 0 {
		t.Errorf("none: got %v, %v", got, err)
	}

	for _, bad := range []AgentKey{{File: "rel"}, {File: "/k\tx"}, {File: "/k", Options: "a\nb"}} {
		if err := WriteAgentKeys(d, []AgentKey{bad}); err == nil {
			t.Errorf("WriteAgentKeys(%+v): want an error", bad)
		}
	}
	tests := []struct {
		name, data string
		line       int
	}{
		{"relative file", "/k/a\t\t\nkey\t\t\n", 2},
		{"too many fields", "/k/a\tfp\topts\textra\n", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			write(t, d.AgentKeys(), tt.data)
			_, err := ReadAgentKeys(d)
			wantCorrupt(t, err, d.AgentKeys(), tt.line)
		})
	}
}
