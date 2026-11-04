//go:build !unix

package platform

// CloseInheritedOnExec does nothing here: no file is inherited past the
// standard three.
func CloseInheritedOnExec() {}
