package lima

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// sandboxExec is macOS's sandbox-exec, by its full path: never one found
// on PATH.
const sandboxExec = "/usr/bin/sandbox-exec"

// profile is what a VM's Seatbelt profile allows its Lima hostagent, the
// process that runs the VM and its user-mode network.
type profile struct {
	// Internet: everything but the host's loopback (image builds).
	// Otherwise nothing but what's below.
	Internet bool
	// LimaDir is the VM's folder in Lima's home, whose unix sockets
	// (ssh's control socket, VZ's network socket) Lima uses.
	LimaDir string
	// AgentSocket is the ssh-agent Lima's ssh logs in with and forwards;
	// "" for none.
	AgentSocket string
	SSHPort     int // the VM's ssh forward on host loopback
	ProxyPort   int // devvm's proxy on host loopback; 0 for none
}

// The profile allows everything else, so the hostagent runs as usual. No
// DNS: names resolve in the proxy.
var profileTmpl = template.Must(template.New("sb").Funcs(template.FuncMap{"sb": sbPath}).Parse(`(version 1)
(allow default)
{{- if .Internet}}
(deny network-outbound (remote ip "localhost:*"))
{{- else}}
(deny network-outbound)
(allow network-outbound (remote unix-socket (subpath {{sb .LimaDir}})))
{{- with .AgentSocket}}
(allow network-outbound (remote unix-socket (path-literal {{sb .}})))
{{- end}}
(deny mach-lookup (global-name "com.apple.dnssd.service"))
{{- end}}
(allow network-outbound (remote ip "localhost:{{.SSHPort}}"))
{{- with .ProxyPort}}
(allow network-outbound (remote ip "localhost:{{.}}"))
{{- end}}
`))

func (p profile) render() string {
	var b bytes.Buffer
	if err := profileTmpl.Execute(&b, p); err != nil {
		panic(err) // the template and its data are devvm's own
	}
	return b.String()
}

// sbPath is a path as Seatbelt matches it, with its folder's links
// resolved (Seatbelt sees real paths), as a profile string.
func sbPath(p string) string {
	dir := filepath.Dir(p)
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return sbString(filepath.Join(dir, filepath.Base(p)))
}

// sbString quotes s as a Seatbelt string: backslashes and double quotes
// escaped, so s can't end the string.
func sbString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// writeProfile writes a VM's profile into the run folder, owner-only.
func writeProfile(path string, p profile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFile(path, []byte(p.render()), 0o600)
}

// writeFile writes a file whole, through a temporary file renamed over it.
func writeFile(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil { //nolint:gosec // devvm's own folders

		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
