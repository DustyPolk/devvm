package cli

import (
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/testbuild"
)

// Overrides are the stand-ins the tests put in a test build's
// environment, in place of what devvm would find on this machine:
// the backends' (registry.Overrides) and the host's mise, and the way
// the CLI tests answer questions. readOverrides is the only place devvm
// reads them, and only a test build (testbuild.On) does: a release has
// none, whatever its environment says. A run reads them once, through
// its app.
type Overrides struct {
	registry.Overrides
	// HostMise (DEVVM_HOST_MISE) stands in for devvm's pinned mise.
	HostMise string
	// PipedAnswers (DEVVM_PIPED_ANSWERS=1) answers questions from input
	// that isn't a terminal, a line each (ui.Flags), where devvm would
	// refuse them.
	PipedAnswers bool
	// PickOnPipe (DEVVM_PICK_ON_PIPE=1) shows the picker with a terminal
	// on stdin alone (ui.Flags), as testscript's ttyin gives.
	PickOnPipe bool
}

// readOverrides reads a test build's overrides with getenv; a release's
// are empty.
func readOverrides(getenv func(string) string) Overrides {
	if !testbuild.On() {
		return Overrides{}
	}
	return Overrides{
		Overrides: registry.Overrides{
			Host: platform.Overrides{
				CPUs: getenv(platform.TestCPUs), MemoryGiB: getenv(platform.TestMemoryGiB),
				FreeGiB: getenv(platform.TestFreeGiB), ListTimeout: getenv(platform.TestListWait),
			},
			Unreviewed:    getenv("DEVVM_UNREVIEWED"),
			KVM:           getenv("DEVVM_KVM"),
			UbuntuURL:     getenv("DEVVM_UBUNTU_URL"),
			UbuntuRelease: getenv("DEVVM_UBUNTU_RELEASE"),
		},
		HostMise:     getenv("DEVVM_HOST_MISE"),
		PipedAnswers: getenv("DEVVM_PIPED_ANSWERS") == "1",
		PickOnPipe:   getenv("DEVVM_PICK_ON_PIPE") == "1",
	}
}
