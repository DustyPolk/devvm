//go:build !unix

package platform

// ShareFileLimit does nothing here: there's no limit to share.
func ShareFileLimit() error { return nil }
