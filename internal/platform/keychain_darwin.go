package platform

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// HasKeychain reports whether secrets live in the system's keychain here
// (macOS's), rather than in files.
const HasKeychain = true

// KeychainGet returns the generic password for service and account from
// the user's keychains, through security(1): ErrNoSecret when there's none.
// The value is read from security's output, never logged.
func KeychainGet(ctx context.Context, service, account string) (string, error) {
	out, err := exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-a", account, "-w").Output() //nolint:gosec // devvm's own arguments
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() == 44 {
			return "", ErrNoSecret // errSecItemNotFound
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// KeychainSet saves value as the generic password for service and account
// in the user's default keychain, replacing one there, labeled label. It
// goes through security -i on stdin, so the value is in no process's
// arguments; none of service, account, label and value may hold a quote,
// a backslash or a line break, which would end its command line. Its
// output is discarded.
func KeychainSet(ctx context.Context, service, account, label, value string) error {
	line, err := keychainLine(service, account, label, value)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "security", "-i")
	cmd.Stdin = strings.NewReader(line)
	return cmd.Run()
}

// keychainLineMax bounds security -i's command line, which it reads in
// lines of 4096 bytes: a longer one would be cut, the item saved with what
// fits (in place of the one there, -U), and the rest run as commands.
const keychainLineMax = 4000

// keychainLine is KeychainSet's command line for security -i, refused
// before anything runs when a part holds a quote, a backslash or a line
// break, which would end it, or when it's past keychainLineMax.
func keychainLine(service, account, label, value string) (string, error) {
	for _, s := range []string{service, account, label, value} {
		if strings.ContainsAny(s, "\"\\\r\n") {
			return "", errors.New("a quote, backslash or line break in a Keychain item")
		}
	}
	line := "add-generic-password -U -s \"" + service + "\" -a \"" + account + "\" -l \"" + label + "\" -w \"" + value + "\"\n"
	if len(line) > keychainLineMax {
		return "", errors.New("a Keychain item too long for security -i's line")
	}
	return line, nil
}

// KeychainDelete deletes the generic password for service and account:
// ErrNoSecret when there's none.
func KeychainDelete(ctx context.Context, service, account string) error {
	err := exec.CommandContext(ctx, "security", "delete-generic-password", "-s", service, "-a", account).Run() //nolint:gosec // devvm's own arguments
	if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() == 44 {
		return ErrNoSecret
	}
	return err
}

// ExcludeFromBackup keeps dir out of Time Machine's backups, unless it is
// already (tmutil). It's checked every time: a folder made before, or
// restored from a backup, gets it too.
func ExcludeFromBackup(ctx context.Context, dir string) error {
	out, err := exec.CommandContext(ctx, "tmutil", "isexcluded", dir).Output() //nolint:gosec // devvm's own folder
	if err == nil && strings.Contains(string(out), "[Excluded]") {
		return nil
	}
	return exec.CommandContext(ctx, "tmutil", "addexclusion", dir).Run() //nolint:gosec // devvm's own folder
}
