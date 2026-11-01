package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/paths"
)

func TestRetention(t *testing.T) {
	dir := t.TempDir()
	d := paths.Dirs{Config: filepath.Join(dir, "config"), Data: filepath.Join(dir, "data")}
	// No file: every setting off.
	r, err := ReadRetention(d)
	if err != nil || !r.Off() || r.RunLogs != 0 || r.ExportsAge != 0 {
		t.Fatalf("no file: %+v, %v", r, err)
	}
	// Set some; a bad key or value writes nothing.
	if _, err := SetRetention(d, []string{"exports.age=30d", "logs.runs=50", "tools.unused-age=2w"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"nope=1"}, {"exports.age=30"}, {"logs.runs=0"}, {"logs.runs=05"}, {"exports.count=x"}, {"novalue"}, {"exports.age=1d", "orphans.age=-1d"}, {"exports.age="}, {"nope=off"}} {
		if _, err := SetRetention(d, bad); err == nil {
			t.Errorf("SetRetention(%q): want an error", bad)
		}
	}
	r, err = ReadRetention(d)
	if err != nil {
		t.Fatal(err)
	}
	if r.ExportsAge != 30*24*time.Hour || r.RunLogs != 50 || r.ToolsAge != 14*24*time.Hour || r.OrphansAge != 0 || r.ExportsCount != 0 {
		t.Errorf("read back: %+v", r)
	}
	// KEY=off turns one off; all off removes the file.
	if _, err := SetRetention(d, []string{"exports.age=off", "logs.runs=off"}); err != nil {
		t.Fatal(err)
	}
	if r, _ = ReadRetention(d); r.ExportsAge != 0 || r.RunLogs != 0 || r.ToolsAge == 0 {
		t.Errorf("after turning two off: %+v", r)
	}
	if _, err := SetRetention(d, []string{"tools.unused-age=off"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.Retention()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("all off: the file stays: %v", err)
	}
	// A bad line names the file and the line.
	if err := os.WriteFile(d.Retention(), []byte("exports.count=3\n\nexports.age=soon\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ReadRetention(d)
	if c, ok := errors.AsType[*CorruptError](err); !ok || c.Line != 3 || c.Path != d.Retention() {
		t.Errorf("bad line: %v", err)
	}
	if err := os.WriteFile(d.Retention(), []byte("exports.count=3\nexports.count=4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRetention(d); err == nil {
		t.Error("a key set twice: want an error")
	}
	// An empty file: every setting off.
	if err := os.WriteFile(d.Retention(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if r, err := ReadRetention(d); err != nil || !r.Off() {
		t.Errorf("empty file: %+v, %v", r, err)
	}
	// The Nix cache's cap: its default when off, the setting's size when set.
	if r.NixCacheCap() != NixCacheDefault {
		t.Errorf("cap when off: %d", r.NixCacheCap())
	}
	if _, err := SetRetention(d, []string{"nix-cache.size=20g"}); err == nil {
		t.Error("nix-cache.size=20g: want an error")
	}
	r, err = SetRetention(d, []string{"nix-cache.size=20G"})
	if err != nil || r.NixCacheCap() != 20<<30 || r.Raw[RetainNixCacheSize] != "20G" {
		t.Errorf("nix-cache.size=20G: %+v, %v", r, err)
	}
}
