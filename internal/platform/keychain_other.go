//go:build !darwin

package platform

import (
	"context"
	"errors"
)

// HasKeychain reports whether secrets live in the system's keychain here:
// not yet, so they're files.
const HasKeychain = false

// KeychainGet is unsupported here.
func KeychainGet(context.Context, string, string) (string, error) { return "", errors.ErrUnsupported }

// ExcludeFromBackup does nothing here: there's no backup devvm knows of.
func ExcludeFromBackup(context.Context, string) error { return nil }
