//go:build e2e

package e2e

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// watchPL is the watcher, perl (every base has it) calling inotify
// itself, so the test VM needs nothing added. It prints its events, and
// writes them to a file too (its first argument), as 9p shows this
// machine a file the VM writes only later.
const watchPL = `use strict; use IO::Handle;
my %nr = (x86_64 => [294, 254], aarch64 => [26, 27]);
chomp(my $m = qx(uname -m));
my ($init, $add) = @{$nr{$m} or die "no inotify syscalls for $m\n"};
my $fd = syscall($init, 0); $fd >= 0 or die "inotify_init1: $!\n";
my $log = shift @ARGV; open(my $lf, ">>", $log) or die "$log: $!\n";
sub out { print STDOUT @_; print $lf @_ }
my ($top, %rel) = ($ARGV[0]);
for my $dir (@ARGV) {
  my $wd = syscall($add, $fd, $dir, 0x2 | 0x4 | 0x8 | 0x40 | 0x80 | 0x100 | 0x200);
  $wd >= 0 or die "inotify_add_watch $dir: $!\n";
  $rel{$wd} = $dir eq $top ? "" : substr($dir, length($top) + 1) . "/";
}
open(my $in, "<&=", $fd) or die "inotify fd: $!\n";
STDOUT->autoflush(1); $lf->autoflush(1); alarm 120;
my %name = (0x2 => "MODIFY", 0x4 => "ATTRIB", 0x8 => "CLOSE_WRITE", 0x40 => "MOVED_FROM", 0x80 => "MOVED_TO", 0x100 => "CREATE", 0x200 => "DELETE");
out("ready\n");
while (sysread($in, my $buf, 65536)) {
  for (my $i = 0; $i + 16 <= length $buf; ) {
    my ($wd, $mask, undef, $len) = unpack("iIII", substr($buf, $i, 16));
    my $f = $rel{$wd} . unpack("Z*", substr($buf, $i + 16, $len));
    for my $bit (sort { $a <=> $b } keys %name) { out("$name{$bit} $f\n") if $mask & $bit }
    $i += 16 + $len;
  }
}
`

// watch is one watcher's run in a VM, on a project folder and a
// subfolder of it, as a dev server's.
type watch struct {
	s       *S
	vm, dir string
	wd, sub string // its own folder (the script and its log) and the subfolder, names of their own
	proc    *Proc
	missed  []string
}

// WatchSees checks that a watcher in the VM on dir and a subfolder sees
// a file changed in each, one made and one deleted, by this machine (by
// "host") or the VM (by "vm"), each within 5 s; without nested, never
// the subfolder's (9p's limit). It's what the watcher saw, each line
// once, and a failure lists that and what it missed.
//
// Each call's folders have names of their own: a folder deleted and made
// again under the same name can stay gone in the VM for longer than any
// wait (Lima's virtiofs keeps the deleted folder's entry), while a name
// the VM never looked up shows as soon as this machine makes it.
func WatchSees(s *S, vm, dir, by string, nested bool) ([]string, error) {
	n := strconv.FormatInt(time.Now().UnixNano(), 36)
	w := &watch{s: s, vm: vm, dir: dir, wd: filepath.Join(dir, ".e2e-watch-"+n), sub: "e2e-sub-" + n}
	defer func() {
		_ = os.RemoveAll(w.wd)
		_ = os.Remove(filepath.Join(dir, "watched.txt"))
		_ = os.Remove(filepath.Join(dir, "made.txt"))
		_ = os.RemoveAll(filepath.Join(dir, w.sub))
	}()
	if err := w.prepare(); err != nil {
		return nil, err
	}
	// A bound, not the fix: the VM waits up to 20 s for the script to
	// show, then runs it, which fails naming the file if it's still missing.
	script := fmt.Sprintf(`for _ in $(seq 100); do [ -s '%[1]s/watch.pl' ] && break; sleep 0.2; done
    LC_ALL=C exec perl '%[1]s/watch.pl' '%[1]s/events' '%[2]s' '%[2]s/%[3]s'`, w.wd, dir, w.sub)
	p, err := s.Start(Cmd{Args: []string{"limactl", "shell", "--workdir", "/", vm, "bash", "-lc", script}})
	if err != nil {
		return nil, err
	}
	w.proc = p
	ok := false
	for range 60 {
		if slices.Contains(w.events(), "ready") {
			ok = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if ok {
		ok = w.exercise(by, nested)
	}
	_, _ = s.InVM(vm, "pkill -f '[.]e2e-watch-"+n+"/watch[.]pl'")
	p.Stop()
	ev := w.events()
	if !ok {
		return ev, fmt.Errorf("the watcher saw:\n%s\n%s", strings.Join(ev, "\n"), strings.Join(w.missed, "\n"))
	}
	return ev, nil
}

func (w *watch) prepare() error {
	if err := os.MkdirAll(filepath.Join(w.dir, w.sub), 0o755); err != nil { //nolint:gosec // a project's folder, as its user's mkdir makes it
		return err
	}
	if err := os.MkdirAll(w.wd, 0o755); err != nil { //nolint:gosec // a project's folder, as its user's mkdir makes it
		return err
	}
	for f, body := range map[string]string{
		filepath.Join(w.wd, "watch.pl"):           watchPL,
		filepath.Join(w.wd, "events"):             "",
		filepath.Join(w.dir, "watched.txt"):       "before\n",
		filepath.Join(w.dir, w.sub, "nested.txt"): "before\n",
	} {
		if err := os.WriteFile(f, []byte(body), 0o644); err != nil { //nolint:gosec // the VM's user reads them
			return err
		}
	}
	return nil
}

// exercise makes each change, each one tried so a failure shows them
// all, and is whether the watcher saw each as it should.
func (w *watch) exercise(by string, nested bool) bool {
	ok := true
	w.do(by, "echo after >> '"+w.dir+"/watched.txt'")
	ok = w.saw("(MODIFY|CLOSE_WRITE|CREATE) watched.txt") && ok
	w.do(by, "echo after >> '"+w.dir+"/"+w.sub+"/nested.txt'")
	if nested {
		ok = w.saw("(MODIFY|CLOSE_WRITE|CREATE) "+w.sub+"/nested.txt") && ok
	}
	w.do(by, "echo made > '"+w.dir+"/made.txt'")
	ok = w.saw("(CREATE|MOVED_TO) made.txt") && ok
	w.do(by, "rm '"+w.dir+"/made.txt'")
	ok = w.saw("(DELETE|MOVED_FROM) made.txt") && ok
	// Not nested: the subfolder's change is never seen, even after the rest.
	if !nested && slices.ContainsFunc(w.events(), func(l string) bool { return strings.Contains(l, " "+w.sub+"/") }) {
		w.missed = append(w.missed, "the subfolder's change was seen")
		ok = false
	}
	return ok
}

// do runs script on this machine (by "host") or in the VM.
func (w *watch) do(by, script string) {
	if by == "host" {
		_, _ = w.s.Run(Cmd{Args: []string{"bash", "-c", script}})
		return
	}
	_, _ = w.s.InVM(w.vm, script)
}

// saw waits up to 5 s for an event matching re.
func (w *watch) saw(re string) bool {
	m := regexp.MustCompile("^" + re + "$")
	end := time.Now().Add(5 * time.Second)
	for !time.Now().After(end) {
		if slices.ContainsFunc(w.events(), m.MatchString) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	w.missed = append(w.missed, "nothing for "+re+" within 5 s")
	return false
}

// events is what the watcher printed and wrote so far, each line once.
func (w *watch) events() []string {
	var out []string
	text := readText(filepath.Join(w.wd, "events"))
	if w.proc != nil {
		text += "\n" + w.proc.Out()
	}
	for l := range strings.SplitSeq(text, "\n") {
		if l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// WatchMisses checks that the watcher sees none of this machine's
// changes (Lima's mounts pass on no file events: polling).
func WatchMisses(s *S, vm, dir string) error {
	ev, _ := WatchSees(s, vm, dir, "host", true)
	if !slices.Contains(ev, "ready") {
		return errors.New("the watcher never got ready")
	}
	for _, l := range ev {
		if l != "ready" {
			return errors.New("the watcher saw:\n" + strings.Join(ev, "\n"))
		}
	}
	return nil
}
