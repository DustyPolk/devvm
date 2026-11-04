package paths

import "path/filepath"

// VM is the folder of one VM's files (Dirs.VM), and each file in it:
//
//	vm.json   its state: what it was made from and asked for, and what
//	          devvm made for it (state.VM)
//	lock      held by each change to vm.json
//	policy    its own network policy rules
//	run/      what its runs leave: pids, its proxy's files, its port
//	          forwarder's, its sandbox profile, its secrets' bindings,
//	          its links' list
//	logs/     its proxy's decisions and output, its link session's and
//	          its port forwarder's
//
// None of them is a socket, so none has the socket limit (SocketMax):
// the sockets VMs need are in the run folder, by hash.
type VM struct{ Dir string }

// State is the VM's state file, vm.json.
func (v VM) State() string { return filepath.Join(v.Dir, "vm.json") }

// Lock is the lock file each change to the VM's state holds (flock), so
// two devvms changing one VM at once lose neither's change.
func (v VM) Lock() string { return filepath.Join(v.Dir, "lock") }

// Policy holds the VM's own network policy rules.
func (v VM) Policy() string { return filepath.Join(v.Dir, "policy") }

// Run is the folder of what the VM's runs leave: pids, its proxy's files,
// its port forwarder's, its sandbox profile, its secrets' bindings and its
// links' list.
func (v VM) Run() string { return filepath.Join(v.Dir, "run") }

// Logs is the folder of the VM's logs.
func (v VM) Logs() string { return filepath.Join(v.Dir, "logs") }

// ProxyConfig is the config the VM's network policy proxy runs with
// (proxy.Config): everything it started with, its password among it.
func (v VM) ProxyConfig() string { return filepath.Join(v.Run(), "proxy.json") }

// ProxyPID is the VM's proxy's pid file.
func (v VM) ProxyPID() string { return filepath.Join(v.Run(), "proxy.pid") }

// ProxyStatus is the status file the VM's proxy writes once it listens.
func (v VM) ProxyStatus() string { return filepath.Join(v.Run(), "proxy.status") }

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

// Forwarder is one of the files of the VM's port forwarder in Run, named
// by ext: .pid, .status, .lock, and .known_hosts, the host key it pins.
func (v VM) Forwarder(ext string) string { return filepath.Join(v.Run(), "ports"+ext) }

// ForwarderLog is the VM's port forwarder's own output.
func (v VM) ForwarderLog() string { return filepath.Join(v.Logs(), "ports.log") }
