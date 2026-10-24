// Package platform holds the code that differs between host systems,
// behind build tags: *_unix.go for macOS and Linux, *_other.go for the
// rest, which get an error until they're supported.
package platform
