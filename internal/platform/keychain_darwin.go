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
