package cli

import (
	"io/fs"
	"os"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/testbuild"
	"github.com/w3cj/devvm/internal/tools"
)

// Overrides are the stand-ins the tests put in a test build's
// environment, in place of what devvm would find on this machine:
// the backends' (registry.Overrides) and the host's mise. readOverrides is
// the only place devvm reads them, and only a test build
// (testbuild.On) does: a release has none, whatever its environment says.
type Overrides struct {
	registry.Overrides
	// HostMise (DEVVM_HOST_MISE) stands in for devvm's pinned mise.
	HostMise string
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
			Unreviewed: getenv("DEVVM_UNREVIEWED"),
			KVM:        getenv("DEVVM_KVM"),
			UbuntuURL:  getenv("DEVVM_UBUNTU_URL"),
		},
		HostMise: getenv("DEVVM_HOST_MISE"),
	}
}

// newRegistry returns the registry of devvm's backends, with this
// build's overrides.
func newRegistry(d paths.Dirs, assets fs.FS, u registry.UI) (*backend.Registry, error) {
	return registry.New(d, assets, u, readOverrides(os.Getenv).Overrides)
}

// thisHost is the host devvm runs on, as this build reads it: a test
// build's with its overrides.
func thisHost() platform.Host { return platform.NewHost(readOverrides(os.Getenv).Host) }

// newTools returns the shared tools in devvm's folders, on u, checking
// host's free space before a build, with this build's mise.
func newTools(d paths.Dirs, assets fs.FS, u tools.UI, host platform.Host) *tools.Env {
	t := tools.New(d, assets, u, host)
	t.Mise = readOverrides(os.Getenv).HostMise
	return t
}
