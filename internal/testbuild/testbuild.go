// Package testbuild says whether this devvm is a test build: one the tests
// make (tests/e2e, the CLI tests) with
//
//	-ldflags "-X github.com/w3cj/devvm/internal/testbuild.testBuild=yes"
//
// Only a test build reads the stand-ins tests put in its environment
// (internal/cli's Overrides). Releases never set it, so they read none.
package testbuild

// testBuild is "yes" only in a test build, set with -ldflags -X.
var testBuild = ""

// On reports whether this is a test build.
func On() bool { return testBuild == "yes" }
