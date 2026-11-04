package devcontainer

import (
	"path"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/shell"
)

// What a property is to devvm.
const (
	keyUsed       = iota // mapped (Plan)
	keySilent            // ignored without a word: names and editors' settings
	keyIgnored           // ignored, named
	keyUnknown           // not devcontainer.json's, named
	keyBuild             // builds a container: refused
	keyInitialize        // runs on the host: never runs, said
)

// kind is what a top-level property is to devvm.
func kind(key string) int {
	switch key {
	case "image", "features", "forwardPorts", "containerEnv", "remoteEnv",
		OnCreate, UpdateContent, PostCreate, PostStart:
		return keyUsed
	case "name", "customizations", "$schema":
		return keySilent
	case "build", "dockerFile", "dockerfile", "dockerComposeFile":
		return keyBuild
	case "initializeCommand":
		return keyInitialize
	case "postAttachCommand", "privileged", "capAdd", "securityOpt", "mounts", "runArgs",
		"workspaceMount", "workspaceFolder", "remoteUser", "containerUser", "updateRemoteUserUID",
		"init", "shutdownAction", "overrideCommand", "hostRequirements", "appPort",
		"portsAttributes", "otherPortsAttributes", "userEnvProbe", "waitFor",
		"overrideFeatureInstallOrder", "context", "service", "runServices":
		return keyIgnored
	}
	return keyUnknown
}

// How a string's variables are filled in.
const (
	modeLiteral = iota // the string as it is: an image, a port, an argument
	modeShell          // a command sh -c runs: ${containerEnv:X} is the VM's ${X}
	modeEnv            // a variable's value, as a shell word: ${containerEnv:X} is the VM's "${X}"
)

// subst fills in a string's devcontainer.json variables, key being its
// property: ${containerWorkspaceFolder} (and its Basename) is the project
// folder, at the same path in the VM; ${containerEnv:X}, in a command or
// a variable, the VM's own X. Any other of devcontainer.json's variables
// is refused, ${localEnv:X} (${env:X}) above all: devvm never puts this
// machine's environment in a VM. ${…} that isn't one of them is left as it
// is, for the VM's shell. A NUL is refused: no command or variable can
// hold one.
func (p *parser) subst(key, s string, mode int) (string, error) {
	if strings.IndexByte(s, 0) >= 0 {
		return "", msg.DevcontainerBadValue(p.file, key)
	}
	// out is what's done; text, the literal text after it, which a shell
	// word quotes as one.
	var out, text strings.Builder
	lit := func(t string) { text.WriteString(t) }
	flush := func() {
		switch {
		case text.Len() == 0:
		case mode == modeEnv:
			out.WriteString(shell.Quote(text.String()))
		default:
			out.WriteString(text.String())
		}
		text.Reset()
	}
	rest := s
	for {
		i := strings.Index(rest, "${")
		if i < 0 {
			break
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			break
		}
		whole := rest[i : i+j+1]
		lit(rest[:i])
		rest = rest[i+j+1:]
		name := whole[2 : len(whole)-1]
		switch v, isEnv := strings.CutPrefix(name, "containerEnv:"); {
		case strings.HasPrefix(name, "localEnv:") || strings.HasPrefix(name, "env:"):
			return "", msg.DevcontainerLocalEnv(p.file, whole)
		case name == "containerWorkspaceFolder":
			lit(p.workspace)
		case name == "containerWorkspaceFolderBasename":
			lit(path.Base(p.workspace))
		case isEnv && mode != modeLiteral && shell.EnvName(v):
			flush()
			if mode == modeEnv {
				out.WriteString(`"${` + v + `}"`)
			} else {
				out.WriteString("${" + v + "}")
			}
		case isEnv, name == "localWorkspaceFolder", name == "localWorkspaceFolderBasename", name == "devcontainerId":
			return "", msg.DevcontainerVariable(p.file, whole)
		default:
			lit(whole)
		}
	}
	lit(rest)
	flush()
	if mode == modeEnv && out.Len() == 0 {
		return "''", nil
	}
	return out.String(), nil
}
