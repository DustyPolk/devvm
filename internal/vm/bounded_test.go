package vm

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/devvmhq/devvm/internal/msg"
)

// bounded keeps, or passes on, at most max bytes. With a cancel, it then
// cancels and fails every write, so a writer that keeps going stops;
// without, it passes on its mark once and takes the rest.
func TestBounded(t *testing.T) {
	writes := []string{"ab", "cd", "ef", "g"}
	t.Run("cancels", func(t *testing.T) {
		var log bytes.Buffer
		cancelled := 0
		b := &bounded{to: &log, max: 5, cancel: func() { cancelled++ }}
		var errs []error
		for _, s := range writes {
			_, err := b.Write([]byte(s))
			errs = append(errs, err)
		}
		if errs[0] != nil || errs[1] != nil || !errors.Is(errs[2], errBounded) || !errors.Is(errs[3], errBounded) {
			t.Errorf("errors %v", errs)
		}
		if log.String() != "abcde" || b.Len() != 0 || !b.over || cancelled != 1 {
			t.Errorf("log %q, kept %q, over %v, cancelled %d", log.String(), b.String(), b.over, cancelled)
		}
	})
	t.Run("marks", func(t *testing.T) {
		var log bytes.Buffer
		b := &bounded{to: &log, max: 5, mark: "|cut"}
		for _, s := range writes {
			if n, err := b.Write([]byte(s)); n != len(s) || err != nil {
				t.Fatal(n, err)
			}
		}
		if log.String() != "abcde|cut" || !b.over {
			t.Errorf("log %q, over %v", log.String(), b.over)
		}
	})
	t.Run("keeps", func(t *testing.T) {
		b := &bounded{max: 4}
		for _, s := range writes {
			if n, err := b.Write([]byte(s)); n != len(s) || err != nil {
				t.Fatal(n, err)
			}
		}
		if b.String() != "abcd" || !b.over {
			t.Errorf("kept %q, over %v", b.String(), b.over)
		}
	})
	// io.Copy, as os/exec copies a command's output, goes through Write:
	// a bounded has no ReadFrom to read past its max.
	t.Run("copied", func(t *testing.T) {
		if _, ok := any(&bounded{}).(io.ReaderFrom); ok {
			t.Fatal("bounded is an io.ReaderFrom")
		}
		b := &bounded{max: 4, cancel: func() {}}
		if _, err := io.Copy(b, strings.NewReader(strings.Repeat("x", 1<<20))); !errors.Is(err, errBounded) || b.Len() != 4 {
			t.Errorf("copied: kept %d, %v", b.Len(), err)
		}
	})
	// Exactly max isn't over.
	b := &bounded{max: 4}
	_, _ = b.Write([]byte("abcd"))
	if b.over {
		t.Error("over at max")
	}
}

// A VM command adds at most vmLogMax bytes to the run log, then the
// truncated mark; each command gets its own.
func TestVMLog(t *testing.T) {
	var log bytes.Buffer
	e := &Env{Reporter: &msg.Fake{Log: &log}}
	w := e.vmLog()
	chunk := []byte(strings.Repeat("x", 1<<16))
	for range (vmLogMax / len(chunk)) + 3 {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if log.Len() != vmLogMax+len(vmLogCut) || !strings.HasSuffix(log.String(), vmLogCut) {
		t.Errorf("log of %d bytes, ending %q", log.Len(), log.String()[log.Len()-40:])
	}
	log.Reset()
	if _, err := e.vmLog().Write([]byte("next\n")); err != nil || log.String() != "next\n" {
		t.Errorf("the next command's log %q, %v", log.String(), err)
	}
}
