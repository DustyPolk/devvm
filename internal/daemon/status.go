package daemon

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// StatusEnv names a background process's status file (Cmd.Status) in its
// environment: what devvm gives the processes it starts.
const StatusEnv = "DEVVM_DAEMON_STATUS"

// Ready says the process is ready, with data (a proxy's address), in the
// status file its start named, and takes that name out of its environment,
// so the programs it runs never write there. Without one it does nothing.
func Ready(data string) error {
	path := os.Getenv(StatusEnv)
	if path == "" {
		return nil
	}
	_ = os.Unsetenv(StatusEnv)
	return WriteStatus(path, os.Getpid(), data)
}

// WriteStatus writes a status file: the pid of the process that's ready,
// then its data, on one line each, in a single write.
func WriteStatus(path string, pid int, data string) error {
	return writeLine(path, strconv.Itoa(pid)+"\n"+strings.ReplaceAll(data, "\n", " "))
}

// readStatus returns what the process pid said in its status file, once
// it has said it whole: never another process's, such as an earlier
// start's.
func readStatus(path string, pid int) (string, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // devvm's own file for the process
	if err != nil {
		return "", false
	}
	first, rest, _ := strings.Cut(string(b), "\n")
	data, end := strings.CutSuffix(rest, "\n")
	return data, end && first == strconv.Itoa(pid)
}

// WaitReady waits for the process pid to say it's ready in its status file
// (Ready), looking tries times, tick apart, and returns what it said. ok
// is false once runs says it no longer runs, or when it never said so:
// one that said so and then ended isn't ready.
func WaitReady(status string, pid int, runs func() bool, tick time.Duration, tries int) (data string, ok bool) {
	for range tries {
		data, said := readStatus(status, pid)
		if !runs() {
			return "", false
		}
		if said {
			return data, true
		}
		time.Sleep(tick)
	}
	return "", false
}
