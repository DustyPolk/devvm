package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/docker"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/ui"
)

// newDockerConnectCmd is devvm _docker-connect: ssh's ProxyCommand for a
// Docker VM, which joins its standard input and output to an sshd of its
// own in the VM's container, docker exec running in devvm's place
// (docker.Connect). Hidden: it's not for users.
func newDockerConnectCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:                docker.ConnectCmd + " -docker PATH -data DIR [-env KEY=VALUE...] VM",
		Short:              "Connect ssh to a Docker VM's sshd",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := docker.Connect(cmd.Context(), args, platform.Exec); err != nil {
				if m, ok := errors.AsType[msg.M](err); ok {
					u.Die(m)
				}
				u.Die(msg.DockerConnectFailed(err.Error(), "").Because(err))
			}
			return nil
		},
	}
}

// newDockerPortsCmd is devvm _docker-ports: the port forwarder of a Docker
// VM that forwards ports (docker.ForwardMain).
func newDockerPortsCmd(u *ui.UI) *cobra.Command {
	return newForwarderCmd(u, "_docker-ports", "Forward a Docker VM's ports", func(cmd *cobra.Command) forwarderMain {
		return func(args []string, script []byte, reserved func(string) (map[int]bool, error)) error {
			return docker.ForwardMain(cmd.Context(), args, script, reserved, u.Stderr())
		}
	})
}
