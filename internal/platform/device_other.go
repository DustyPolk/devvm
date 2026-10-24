//go:build !unix

package platform

import "io/fs"

// SameDevice reports whether two files are on the same file system: on
// this system, always yes.
func SameDevice(fs.FileInfo, fs.FileInfo) bool { return true }

// DevIno returns a file's device and inode: none on this system.
func DevIno(fs.FileInfo) string { return "" }
