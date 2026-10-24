package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/w3cj/devvm/internal/msg"
)

// In a terminal, a status line shows what devvm is waiting for: the step,
// for how long, and as much of the run log's latest line as fits. It's
// redrawn every redrawEvery, at the terminal's width then.
const redrawEvery = 200 * time.Millisecond

// A step that took at least this long leaves a line behind ("✓ step (5s)").
const doneAfter = 3 * time.Second

type statusLine struct {
	label   string
	start   time.Time
	stopped chan struct{}
	ended   chan struct{}
	once    sync.Once
	drawn   int // runes on the line last drawn
	cols    int // the terminal's width when it was drawn
	frame   int
}

var active struct {
	sync.Mutex
	s *statusLine
}

// Status shows label on a status line, around slow work that isn't one
// logged command, until stop is called. Only in a terminal, and not with
// DEVVM_VERBOSE.
func Status(label msg.M) (stop func()) {
	if verbose() || !stderrTTY() {
		return func() {}
	}
	s := startStatus(label.Line())
	return s.stop
}

func startStatus(label string) *statusLine {
	stopStatus()
	s := &statusLine{label: label, start: time.Now(), stopped: make(chan struct{}), ended: make(chan struct{})}
	active.Lock()
	active.s = s
	active.Unlock()
	s.draw()
	go s.run()
	return s
}

// stopStatus stops the status line showing, if any.
func stopStatus() {
	active.Lock()
	s := active.s
	active.Unlock()
	if s != nil {
		s.stop()
	}
}

func (s *statusLine) run() {
	defer close(s.ended)
	t := time.NewTicker(redrawEvery)
	defer t.Stop()
	for {
		select {
		case <-s.stopped:
			s.cols = width()
			write(stderr, clearLine(s.drawn, s.cols))
			return
		case <-t.C:
			s.draw()
		}
	}
}

func (s *statusLine) draw() {
	frames := glyphs(os.Getenv)[1:]
	s.cols = width()
	text := "    " + frames[s.frame%len(frames)] + " " +
		statusText(s.label, int(time.Since(s.start).Seconds()), lastLine(logPath()), s.cols)
	text = truncate(text, s.cols-1)
	s.frame++
	// No line wrapping while it draws: a resize between reading the width
	// and drawing would wrap.
	write(stderr, clearLine(s.drawn, s.cols)+"\033[?7l"+text+"\033[?7h")
	s.drawn = utf8.RuneCountInString(text)
}

func (s *statusLine) stop() {
	s.once.Do(func() {
		close(s.stopped)
		<-s.ended
		active.Lock()
		if active.s == s {
			active.s = nil
		}
		active.Unlock()
	})
}

// width is the terminal's width, 80 when unknown (some terminals say 0).
func width() int {
	if w := termWidth(); w > 0 {
		return w
	}
	return 80
}

// glyphs returns the done mark, then the spinner's frames, in the
// terminal's character set.
func glyphs(getenv func(string) string) []string {
	charset := getenv("LC_ALL")
	if charset == "" {
		charset = getenv("LC_CTYPE")
	}
	if charset == "" {
		charset = getenv("LANG")
	}
	if strings.Contains(strings.ToLower(charset), "utf") {
		return strings.Fields("✓ ⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏")
	}
	return []string{"ok", "|", "/", "-", `\`}
}

// doneLine is what a step that took a while leaves behind.
func doneLine(label string, secs int) string {
	return "    " + glyphs(os.Getenv)[0] + " " + label + " (" + duration(secs) + ")\n"
}

// duration is 5s, or 2m05s from a minute.
func duration(secs int) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
}

// statusText is the label and time, then as much of the log's latest line
// (its last carriage-return segment, without control characters) as fits in
// cols.
func statusText(label string, secs int, logLine string, cols int) string {
	text := label + " " + duration(secs)
	if i := strings.LastIndexByte(logLine, '\r'); i >= 0 {
		logLine = logLine[i+1:]
	}
	logLine = OneLine(logLine)
	room := cols - utf8.RuneCountInString(text) - 9
	if room >= 10 && logLine != "" {
		text += "  " + truncate(logLine, room)
	}
	return text
}

func truncate(s string, n int) string {
	if n < 0 {
		n = 0
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// clearLine clears the status line. A terminal made narrower rewraps the
// line already drawn (most do) onto more rows, so it clears those above too.
func clearLine(drawn, cols int) string {
	if cols <= 0 {
		cols = 80
	}
	up := 0
	if drawn > cols {
		up = (drawn - 1) / cols
	}
	return "\r\033[K" + strings.Repeat("\033[A\033[K", up)
}

// OneLine drops every control character from s, newlines and tabs too: a
// value a VM made, for one line of data output.
func OneLine(s string) string {
	return strings.NewReplacer("\t", "", "\n", "").Replace(printable(s))
}

// printable drops control characters (C0 but newline and tab, DEL, and C1
// in UTF-8) from text a VM or a build printed, which could move the cursor
// or rewrite what's shown.
func printable(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\n' || c == '\t':
		case c < 0x20 || c == 0x7f:
			continue
		case c == 0xc2 && i+1 < len(s) && s[i+1] >= 0x80 && s[i+1] <= 0x9f:
			i++
			continue
		}
		b = append(b, c)
	}
	return string(b)
}

// tail returns up to the last size bytes of the file at path.
func tail(path string, size int64) string {
	f, err := os.Open(path) //nolint:gosec // the run's own log
	if err != nil {
		return ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > size {
		if _, err := f.Seek(-size, io.SeekEnd); err != nil {
			return ""
		}
	}
	b, _ := io.ReadAll(f)
	return string(b)
}

// lastLine is the file's last line, "" when it has none.
func lastLine(path string) string {
	if path == "" {
		return ""
	}
	s := strings.TrimSuffix(tail(path, 4096), "\n")
	return s[strings.LastIndexByte(s, '\n')+1:]
}

// tailLines is the file's last n lines.
func tailLines(path string, n int) string {
	lines := strings.SplitAfter(strings.TrimSuffix(tail(path, 64<<10), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	s := strings.Join(lines, "")
	if s != "" {
		s += "\n"
	}
	return s
}
