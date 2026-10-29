package devcontainer

import (
	"path"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
)

// strip returns JSON with comments (devcontainer.json's JSONC): its
// comments, trailing commas and byte order mark blanked with spaces, so
// what's left is JSON at the same offsets, newlines kept. Anything inside
// a string stays as it is.
func strip(data []byte) []byte {
	out := []byte(string(data))
	if len(out) >= 3 && out[0] == 0xef && out[1] == 0xbb && out[2] == 0xbf {
		out[0], out[1], out[2] = ' ', ' ', ' '
	}
	blank := func(from, to int) {
		for i := from; i < to && i < len(out); i++ {
			if out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
			}
		}
	}
	// Comments first: a comma before one is still trailing.
	for i := 0; i < len(out); i++ {
		switch {
		case out[i] == '"':
			i = stringEnd(out, i)
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '/':
			end := i
			for end < len(out) && out[end] != '\n' {
				end++
			}
			blank(i, end)
			i = end
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '*':
			end := strings.Index(string(out[i+2:]), "*/")
			if end < 0 {
				// Unterminated: the JSON then ends early, and says so.
				blank(i, len(out))
				return out
			}
			blank(i, i+2+end+2)
			i += 2 + end + 1
		}
	}
	for i := 0; i < len(out); i++ {
		switch out[i] {
		case '"':
			i = stringEnd(out, i)
		case ',':
			j := i + 1
			for j < len(out) && strings.IndexByte(" \t\r\n", out[j]) >= 0 {
				j++
			}
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				out[i] = ' '
			}
		}
	}
	return out
}

// stringEnd returns the index of the quote that ends the string starting
// at start, or the last index when it doesn't end.
func stringEnd(b []byte, start int) int {
	for i := start + 1; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return len(b) - 1
}

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
			out.WriteString(shellQuote(text.String()))
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
			return "", msg.DevcontainerLocalEnv(p.file, msg.Shown(whole))
		case name == "containerWorkspaceFolder":
			lit(p.workspace)
		case name == "containerWorkspaceFolderBasename":
			lit(path.Base(p.workspace))
		case isEnv && mode != modeLiteral && envName.MatchString(v):
			flush()
			if mode == modeEnv {
				out.WriteString(`"${` + v + `}"`)
			} else {
				out.WriteString("${" + v + "}")
			}
		case isEnv, name == "localWorkspaceFolder", name == "localWorkspaceFolderBasename", name == "devcontainerId":
			return "", msg.DevcontainerVariable(p.file, msg.Shown(whole))
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

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
