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

// Log is a VM's event log, as devvm log shows it. NewLog sets every
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

// Write writes the log's last n events to w, one line each (FormatEvent).
// With follow it then writes each new one, until ctx ends. Lines that
// aren't events are skipped.
func (l *Log) Write(ctx context.Context, w io.Writer, n int, follow bool) error {
	f, err := os.Open(l.Path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	r := bufio.NewReader(f)
	var last []string
	keep := func(l string) {
		if s, ok := FormatEvent(l); ok && n > 0 {
			if len(last) == n {
				last = last[1:]
			}
			last = append(last, s)
		}
	}
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
	t := time.NewTicker(l.Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
		// A log made shorter (emptied) is read again from its start.
		if fi, err := f.Stat(); err == nil && fi.Size() < read {
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			r.Reset(f)
			read, partial = 0, ""
		}
		var werr error
		rest, err := readLines(r, &read, func(l string) {
			if s, ok := FormatEvent(partial + l); ok && werr == nil {
				_, werr = fmt.Fprintln(w, s)
			}
			partial = ""
		})
		if err != nil {
			return err
		}
		if werr != nil {
			return werr
		}
		partial += rest
	}
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

// FormatEvent turns one line of an event log into what devvm log shows:
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
	}
	if len(e.Secrets) > 0 {
		d += ", token: " + strings.Join(e.Secrets, ",")
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

// WriteBlocked writes the log's blocks to w as a table, one row per host
// and port, by when it was last blocked: the last time, how many times,
// and the devvm policy allow command that allows it. A request refused
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
				r.allow = "devvm policy allow " + rule
			}
			rows[k] = r
		}
		r.count++
		r.last = oneLine(e.Time)
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
