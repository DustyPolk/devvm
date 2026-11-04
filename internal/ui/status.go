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
	u       *UI
	label   []part // its values bold
	start   time.Time
	stopped chan struct{}
	ended   chan struct{}
	once    sync.Once
	drawn   int // runes on the line last drawn
	cols    int // the terminal's width when it was drawn
	frame   int
}

// Status shows label on a status line, around slow work that isn't one
// logged command, until stop is called. Only in a terminal, and not
// while verbose.
func (u *UI) Status(label msg.M) (stop func()) {
	if verbose(u) || !u.stderrTTY {
		return func() {}
	}
	s := u.startStatus(u.errStyle.valueParts(label))
	return s.stop
}

func (u *UI) startStatus(label []part) *statusLine {
	u.stopStatus()
	s := &statusLine{u: u, label: label, start: time.Now(), stopped: make(chan struct{}), ended: make(chan struct{})}
	u.status.Lock()
	u.status.s = s
	u.status.Unlock()
	s.draw()
	go s.run()
	return s
}

// stopStatus stops the status line showing, if any.
func (u *UI) stopStatus() {
	u.status.Lock()
	s := u.status.s
	u.status.Unlock()
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
			s.cols = s.u.width()
			s.u.write(s.u.stderr, clearLine(s.drawn, s.cols))
			return
		case <-t.C:
			s.draw()
		}
	}
}

func (s *statusLine) draw() {
	frames := glyphs(s.u.getenv)[1:]
	s.cols = s.u.width()
	secs := int(time.Since(s.start).Seconds())
	_, log := statusText(plainText(s.label), secs, lastLine(s.u.logPath()), s.cols)
	parts := append([]part{{"    " + frames[s.frame%len(frames)] + " ", ""}}, s.label...)
	parts = append(parts, part{" " + duration(secs), ""}, part{log, sgrDim})
	line, drawn := s.u.errStyle.fit(parts, s.cols-1)
	s.frame++
	// No line wrapping while it draws: a resize between reading the width
	// and drawing would wrap.
	s.u.write(s.u.stderr, clearLine(s.drawn, s.cols)+"\033[?7l"+line+"\033[?7h")
	s.drawn = drawn
}

func (s *statusLine) stop() {
	s.once.Do(func() {
		close(s.stopped)
		<-s.ended
		s.u.status.Lock()
		if s.u.status.s == s {
			s.u.status.s = nil
		}
		s.u.status.Unlock()
	})
}

// width is the terminal's width, 80 when unknown (some terminals say 0).
func (u *UI) width() int {
	if w := u.termWidth(); w > 0 {
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

// doneLine is what a step that took a while leaves behind, its mark green
// and its label's values bold in a terminal.
func (u *UI) doneLine(label []part, secs int) string {
	l, _ := u.errStyle.fit(label, len(plainText(label)))
	return "    " + u.errStyle.paint(sgrDone, glyphs(u.getenv)[0]) + " " + l + " (" + duration(secs) + ")\n"
}

// duration is 5s, or 2m05s from a minute.
func duration(secs int) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
}

// statusText is the label and time, then apart, so it can be dimmed, as
// much of the log's latest line (its last carriage-return segment, without
// control characters) as fits in cols, after two spaces.
func statusText(label string, secs int, logLine string, cols int) (text, log string) {
	text = label + " " + duration(secs)
	if i := strings.LastIndexByte(logLine, '\r'); i >= 0 {
		logLine = logLine[i+1:]
	}
	logLine = msg.OneLine(logLine)
	room := cols - utf8.RuneCountInString(text) - 9
	if room >= 10 && logLine != "" {
		log = "  " + truncate(logLine, room)
	}
	return text, log
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
