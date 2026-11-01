package netpolicy

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/proxy"
)

// Log is a VM's event log, as devvm network log shows it. NewLog sets every
// field; tests change them.
type Log struct {
	Path string
	// Tick is how often a followed log is read for new events.
	Tick time.Duration
}

// NewLog returns vm's event log (EventLog).
func NewLog(d paths.Dirs, vm string) *Log {
	return &Log{Path: EventLog(d, vm), Tick: 250 * time.Millisecond}
}

// Old is the log's previous file, which the proxy renamed it to when it
// rotated it: its older events.
func (l *Log) Old() string { return l.Path + ".1" }

// Write writes the log's last n events to w, one line each (FormatEvent),
// the previous file's (Old) first. With follow it then writes each new
// one, until ctx ends, and goes on in the new log when the proxy rotates
// it. Lines that aren't events are skipped.
func (l *Log) Write(ctx context.Context, w io.Writer, n int, follow bool) error {
	f, err := os.Open(l.Path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var last []string
	keep := func(l string) {
		if s, ok := FormatEvent(l); ok && n > 0 {
			if len(last) == n {
				last = last[1:]
			}
			last = append(last, s)
		}
	}
	// The log is open first, so a rotation now leaves it as Old: read once.
	if err := l.readOld(f, keep); err != nil {
		return err
	}
	r := bufio.NewReader(f)
	var read int64
	partial, err := readLines(r, &read, keep)
	if err != nil {
		return err
	}
	if !follow {
		keep(partial)
	}
	for _, s := range last {
		if _, err := fmt.Fprintln(w, s); err != nil {
			return err
		}
	}
	if !follow {
		return nil
	}
	fl := follower{l: l, f: f, r: r, read: read, partial: partial, w: w}
	defer func() { _ = fl.f.Close() }()
	t := time.NewTicker(l.Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
		if err := fl.next(); err != nil {
			return err
		}
	}
}

// readOld calls line for each line of Old, unless it's cur, the log
// already open.
func (l *Log) readOld(cur *os.File, line func(string)) error {
	old, err := os.Open(l.Old())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	defer func() { _ = old.Close() }()
	if a, err := old.Stat(); err == nil {
		if b, err := cur.Stat(); err == nil && os.SameFile(a, b) {
			return nil
		}
	}
	var read int64
	rest, err := readLines(bufio.NewReader(old), &read, line)
	if err != nil {
		return err
	}
	line(rest)
	return nil
}

// follower writes each new event of a followed log.
type follower struct {
	l       *Log
	f       *os.File
	r       *bufio.Reader
	read    int64
	partial string // a line still being written
	w       io.Writer
}

// next writes the events written since the last call. A log made shorter
// (emptied) is read again from its start. A log the proxy rotated is read
// to its end, then the new one from its start.
func (fl *follower) next() error {
	if fi, err := fl.f.Stat(); err == nil && fi.Size() < fl.read {
		if _, err := fl.f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		fl.r.Reset(fl.f)
		fl.read, fl.partial = 0, ""
	}
	// Whether the log was renamed, checked before its end is read, so no
	// line written to it after is missed.
	nf := fl.rotated()
	if err := fl.drain(); err != nil {
		if nf != nil {
			_ = nf.Close()
		}
		return err
	}
	if nf == nil {
		return nil
	}
	_ = fl.f.Close()
	fl.f, fl.read, fl.partial = nf, 0, ""
	fl.r.Reset(nf)
	return fl.drain()
}

// rotated returns the log's new file, open, when the proxy has renamed
// the one being read.
func (fl *follower) rotated() *os.File {
	cur, err := fl.f.Stat()
	if err != nil {
		return nil
	}
	nf, err := os.Open(fl.l.Path)
	if err != nil {
		return nil
	}
	if fi, err := nf.Stat(); err != nil || os.SameFile(cur, fi) {
		_ = nf.Close()
		return nil
	}
	return nf
}

// drain writes the whole lines the log has after what was read.
func (fl *follower) drain() error {
	var werr error
	rest, err := readLines(fl.r, &fl.read, func(l string) {
		if s, ok := FormatEvent(fl.partial + l); ok && werr == nil {
			_, werr = fmt.Fprintln(fl.w, s)
		}
		fl.partial = ""
	})
	if err != nil {
		return err
	}
	if werr != nil {
		return werr
	}
	fl.partial += rest
	return nil
}

// readLines calls line for each whole line r has (without its newline),
// adding the bytes read to *read, and returns what's after the last
// newline: a line still being written.
func readLines(r *bufio.Reader, read *int64, line func(string)) (string, error) {
	for {
		s, err := r.ReadString('\n')
		*read += int64(len(s))
		if errors.Is(err, io.EOF) {
			return s, nil
		}
		if err != nil {
			return "", err
		}
		line(strings.TrimSuffix(s, "\n"))
	}
}

// FormatEvent turns one line of an event log into what devvm network log shows:
// the time, allowed or BLOCKED, the host and port, and the rule that
// allowed it, the request, the tokens put in, or why it was blocked. The
// VM chose the host and path: control characters are dropped. false for a
// line that isn't an event.
func FormatEvent(line string) (string, bool) {
	var e proxy.Event
	if json.Unmarshal([]byte(line), &e) != nil || e.Event == "" {
		return "", false
	}
	what, d := "allowed", "rule "+e.Rule
	switch e.Event {
	case "blocked":
		what, d = "BLOCKED", e.Reason
	case "forward":
		d = e.Method + " " + e.Path
	case "intercept":
		d += ", decrypted"
	case "nix-cache": // the shared Nix cache: a hit, fetched, or why not
		what, d = "cache", e.Method+" "+e.Path+": "+e.Reason
	}
	if len(e.Secrets) > 0 {
		d += ", token: " + strings.Join(e.Secrets, ",")
	}
	if e.Upstream != "" {
		d += ", through " + e.Upstream
	}
	return fmt.Sprintf("%s  %-7s  %s  %s", oneLine(e.Time), what, hostPort(e), oneLine(d)), true
}

func hostPort(e proxy.Event) string {
	return net.JoinHostPort(oneLine(e.Host), strconv.Itoa(e.Port))
}

// oneLine drops every control character from s (C0, newlines and tabs
// too, DEL and C1): a value the VM's proxy logged, for one line of data
// output.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// WriteBlocked writes the log's blocks (Old's too) to w as a table, one row per host
// and port, by when it was last blocked: the last time, how many times,
// and the devvm network allow command that allows it. A request refused
// whatever the allowlist says, or a host the policy doesn't take, gets -
// instead: the command is meant to be pasted into a shell.
func (l *Log) WriteBlocked(w io.Writer) error {
	f, err := os.Open(l.Path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	type row struct {
		last, host, allow string
		count             int
	}
	type key struct {
		host    string
		port    int
		refused bool
	}
	rows := map[key]*row{}
	var read int64
	add := func(l string) {
		var e proxy.Event
		if json.Unmarshal([]byte(l), &e) != nil || e.Event != "blocked" {
			return
		}
		k := key{e.Host, e.Port, e.Refused}
		r := rows[k]
		if r == nil {
			r = &row{host: hostPort(e), allow: "-"}
			if rule, ok := proxy.AllowRule(e.Host, e.Port); ok && !e.Refused {
				r.allow = "devvm network allow " + rule
			}
			rows[k] = r
		}
		r.count++
		r.last = oneLine(e.Time)
	}
	if err := l.readOld(f, add); err != nil {
		return err
	}
	rest, err := readLines(bufio.NewReader(f), &read, add)
	if err != nil {
		return err
	}
	add(rest)
	sorted := make([]*row, 0, len(rows))
	for _, r := range rows {
		sorted = append(sorted, r)
	}
	slices.SortFunc(sorted, func(a, b *row) int { return cmp.Or(cmp.Compare(a.last, b.last), cmp.Compare(a.host, b.host)) })
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "LAST\tCOUNT\tHOST\tALLOW WITH")
	for _, r := range sorted {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", r.last, r.count, r.host, r.allow)
	}
	return tw.Flush()
}
