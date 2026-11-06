package vm

import (
	"bytes"
	"errors"
	"io"
)

// bounded is what devvm reads from a VM through, at most max bytes: it
// keeps the first max bytes written to it, or passes them on to to.
// Past max, it keeps and passes on nothing more and says so (over).
// With cancel, it then cancels the command writing and fails every write
// after (errBounded), so a command that keeps writing ends; without, it
// passes on mark once, when there's one, and takes the rest. Its buffer
// is a field, not embedded, so it has no ReadFrom: io.Copy, as os/exec
// copies a command's output, would read through that without end.
type bounded struct {
	buf    bytes.Buffer
	to     io.Writer
	max    int
	n      int
	over   bool
	cancel func()
	mark   string
}

// errBounded is a bounded writer's error once it's over max, with a
// cancel.
var errBounded = errors.New("more output than devvm reads")

func (b *bounded) Write(p []byte) (int, error) {
	if b.over {
		if b.cancel != nil {
			return 0, errBounded
		}
		return len(p), nil
	}
	keep := p
	if room := b.max - b.n; len(p) > room {
		keep, b.over = p[:room], true
	}
	b.n += len(keep)
	var err error
	if b.to != nil {
		_, err = b.to.Write(keep)
	} else {
		b.buf.Write(keep)
	}
	switch {
	case err != nil:
		return len(keep), err
	case !b.over:
		return len(p), nil
	case b.cancel != nil:
		b.cancel()
		return len(keep), errBounded
	}
	if b.to != nil && b.mark != "" {
		if _, err := io.WriteString(b.to, b.mark); err != nil {
			return len(keep), err
		}
	}
	return len(p), nil
}

// Bytes, String and Len are what it kept.
func (b *bounded) Bytes() []byte  { return b.buf.Bytes() }
func (b *bounded) String() string { return b.buf.String() }
func (b *bounded) Len() int       { return b.buf.Len() }

// vmLogMax is the most of one VM command's output that goes into the run
// log; vmLogCut marks the end of what went in when there was more.
const (
	vmLogMax = 4 << 20
	vmLogCut = "\ndevvm: truncated: the command wrote more than 4 MiB, and the rest isn't in this log\n"
)

// vmLog is where one command in a VM writes the run log (the output of a
// backend's Exec): the run log, at most vmLogMax bytes of it, then
// vmLogCut. Each command gets its own.
func (e *Env) vmLog() io.Writer {
	return &bounded{to: e.LogWriter(), max: vmLogMax, mark: vmLogCut}
}
