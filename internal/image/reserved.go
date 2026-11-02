package image

import "slices"

// Reserved are the words devvm keeps for its commands: every command's
// and subcommand's name, the hidden ones too, and the words kept for
// commands to come. A layer can't take one, so devvm WORD never means both
// a command and an image. A test checks that every command's name is
// here.
var Reserved = []string{
	// devvm's commands.
	"adopt", "build", "code", "completion", "desktop", "doctor", "gc", "git-protect", "group", "help", "history",
	"host", "images", "layer", "limactl", "link", "ls", "network", "ports", "rebuild", "restart", "retention",
	"rm", "secret", "setup", "size", "space", "ssh-agent", "stop", "tools", "version",
	// The hidden ones.
	"_docker-connect", "_docker-ports", "_lima-ports", "_links", "_msg", "_proxy", "_qemu", "_refresh-tools",
	"_smolvm-ports",
	// Their commands.
	"add", "allow", "deny", "export", "get", "join", "leave", "log", "new", "publish", "set", "unpublish", "unuse",
	"update", "use",
}

// IsReserved reports whether a word is one devvm keeps for its commands.
func IsReserved(name string) bool { return slices.Contains(Reserved, name) }
