package paths

import "path/filepath"

// VM is the folder of one VM's files (Dirs.VM), and each file in it:
//
//	state     its state
//	policy    its own network policy rules
//	backend/  its backend's files: what it set (record.json), the ssh
//	          key and host key it logs in with, its port forwarder's
//	run/      what its runs leave: pids, its proxy's files, its sandbox
//	          profile, its secrets' bindings, its links' list
//	logs/     its proxy's decisions and output, its link session's
//
// None of them is a socket, so none has the socket limit (SocketMax):
// the sockets VMs need are in the run folder, by hash.
type VM struct{ Dir string }

// State is the VM's state file.
func (v VM) State() string { return filepath.Join(v.Dir, "state") }

// Policy holds the VM's own network policy rules.
func (v VM) Policy() string { return filepath.Join(v.Dir, "policy") }

// Backend is the folder of the VM's backend's files.
func (v VM) Backend() string { return filepath.Join(v.Dir, "backend") }

// Run is the folder of what the VM's runs leave: pids, its proxy's files,
// its sandbox profile, its secrets' bindings and its links' list.
func (v VM) Run() string { return filepath.Join(v.Dir, "run") }

// Logs is the folder of the VM's logs.
func (v VM) Logs() string { return filepath.Join(v.Dir, "logs") }

// Proxy is one of the files of the VM's network policy proxy in Run,
// named by ext: .pid, .status, .auth, .decider, .cas, .upstream, and
// what it started with (.policy, .bin, .ca).
func (v VM) Proxy(ext string) string { return filepath.Join(v.Run(), "proxy"+ext) }

// BuildProxy is the folder of a build VM's proxy, behind a company proxy:
// its files and logs, kept when the build fails.
func (v VM) BuildProxy() string { return filepath.Join(v.Run(), "build-proxy") }

// Profile is the sandbox profile Lima's hostagent runs under on macOS.
func (v VM) Profile() string { return filepath.Join(v.Run(), "sandbox.sb") }

// Started records the hostagent the VM was started on, and how it was
// confined.
func (v VM) Started() string { return filepath.Join(v.Run(), "started") }

// Secrets is the bindings of the VM's secrets that its proxy reads.
func (v VM) Secrets() string { return filepath.Join(v.Run(), "secrets.json") }

// SecretsPolicy is the network policy rules its secrets' hosts add.
func (v VM) SecretsPolicy() string { return filepath.Join(v.Run(), "secrets.policy") }

// LinksPID is the pid file of the VM's link session.
func (v VM) LinksPID() string { return filepath.Join(v.Run(), "links.pid") }

// LinkList is the list of the VM's links that its link session writes
// and its proxy reads.
func (v VM) LinkList() string { return filepath.Join(v.Run(), "links.json") }

// EventLog is the VM's proxy's decisions (JSON lines).
func (v VM) EventLog() string { return filepath.Join(v.Logs(), "network.jsonl") }

// ProxyLog is the VM's proxy's own output.
func (v VM) ProxyLog() string { return filepath.Join(v.Logs(), "proxy.log") }

// LinksLog is the VM's link session's own output.
func (v VM) LinksLog() string { return filepath.Join(v.Logs(), "links.log") }

// Record is what the VM's backend set on it, which the backend itself
// doesn't show (smolvm's).
func (v VM) Record() string { return filepath.Join(v.Backend(), "record.json") }

// Key is the ssh key devvm logs in to the VM with, on a backend that
// gives it one.
func (v VM) Key() string { return filepath.Join(v.Backend(), "key") }

// KnownHosts is the VM's sshd's host key, as its backend last read it.
func (v VM) KnownHosts() string { return filepath.Join(v.Backend(), "known_hosts") }

// ProxyURL holds the VM's proxy's address, password and all, for a
// backend that passes it to the VM's commands by file (smolvm's).
func (v VM) ProxyURL() string { return filepath.Join(v.Backend(), "proxy") }

// Ports records which of the VM's ports its backend forwards, on a
// backend whose own config doesn't (Lima's).
func (v VM) Ports() string { return filepath.Join(v.Backend(), "ports") }

// Forwarder is one of the files of the VM's port forwarder, named by ext:
// .pid, .status, .log, .lock, and .known_hosts, the host key it pins.
func (v VM) Forwarder(ext string) string { return filepath.Join(v.Backend(), "ports"+ext) }
