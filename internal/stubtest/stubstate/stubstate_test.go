package stubstate

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

// The state starts empty, keeps what an update leaves, and keeps nothing
// of an update that fails.
func TestUpdate(t *testing.T) {
	dir := t.TempDir()
	st, err := Read(dir)
	if err != nil || len(st.Instances) != 0 || st.Agent != nil {
		t.Fatalf("Read = %+v, %v", st, err)
	}
	err = Update(dir, func(st *State) error {
		st.Instances["a"] = NewInstance("Running")
		st.Keychain["github"] = "v"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if err := Update(dir, func(st *State) error { delete(st.Instances, "a"); return boom }); !errors.Is(err, boom) {
		t.Errorf("a failed update: %v", err)
	}
	st, err = Read(dir)
	if err != nil || st.Instances["a"] == nil || st.Instances["a"].CPUs != DefaultCPUs || st.Keychain["github"] != "v" {
		t.Errorf("Read = %+v, %v", st, err)
	}
}

// Updates at once each keep their change: they take turns under the
// lock.
func TestUpdatesAtOnce(t *testing.T) {
	dir := t.TempDir()
	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			if err := Update(dir, func(st *State) error {
				st.Instances[strconv.Itoa(i)] = NewInstance("Stopped")
				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if st, err := Read(dir); err != nil || len(st.Instances) != n {
		t.Errorf("%d instances, %v", len(st.Instances), err)
	}
}

// A state that isn't JSON, or a folder that isn't there, fails.
func TestBroken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, File), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Error("read a broken state")
	}
	if _, err := Read(filepath.Join(dir, "none")); err == nil {
		t.Error("read a missing folder")
	}
	// A state that can't be replaced: a folder in its new file's place.
	dir = t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, File+".new"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Update(dir, func(*State) error { return nil }); err == nil {
		t.Error("wrote over a folder")
	}
}
