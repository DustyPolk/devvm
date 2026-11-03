package paths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
)

// Dirs holds the folders devvm keeps its state in. Every file devvm writes
// outside a VM is under one of them, never in a project folder.
type Dirs struct {
	// Home is the user's home folder: $HOME, which must be absolute. It's
	// the one place devvm reads HOME; everything else takes it from here.
	Home string
	// Data is devvm's data folder: $XDG_DATA_HOME/devvm, by default
	// ~/.local/share/devvm.
	Data string
	// Config is devvm's config folder: $XDG_CONFIG_HOME/devvm, by default
	// ~/.config/devvm.
	Config string
	// Lima is devvm's own Lima home: $DEVVM_LIMA_HOME, else Data/lima. It
	// never follows LIMA_HOME, so Lima settings meant for other VMs don't
	// reach devvm's.
	Lima string
}

// FromEnv finds devvm's folders from the environment.
func FromEnv() (Dirs, error) {
	return fromEnv(os.Getenv)
}

func fromEnv(getenv func(string) string) (Dirs, error) {
	home := getenv("HOME")
	if !filepath.IsAbs(home) {
		return Dirs{}, msg.HomeNotAbsolute()
	}
	d := Dirs{
		Home:   filepath.Clean(home),
		Data:   filepath.Join(xdg(getenv, "XDG_DATA_HOME", home, ".local", "share"), "devvm"),
		Config: filepath.Join(xdg(getenv, "XDG_CONFIG_HOME", home, ".config"), "devvm"),
	}
	d.Lima = filepath.Join(d.Data, "lima")
	if l := getenv("DEVVM_LIMA_HOME"); l != "" {
		abs, err := filepath.Abs(l)
		if err != nil {
			return Dirs{}, err
		}
		d.Lima = abs
	}
	return d, nil
}

// xdg reads an XDG base directory variable. The spec says to ignore a
// relative path, so that falls back to the default too.
func xdg(getenv func(string) string, name, home string, def ...string) string {
	if v := getenv(name); filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	return filepath.Join(append([]string{home}, def...)...)
}

// VMs is the folder of per-VM state files.
func (d Dirs) VMs() string { return filepath.Join(d.Data, "vms") }

// VMState is one VM's state file.
func (d Dirs) VMState(vm string) string { return filepath.Join(d.VMs(), vm) }

// Images is the folder of images' records, one folder per image key.
func (d Dirs) Images() string { return filepath.Join(d.Data, "images") }

// Image is the folder of the image with this key.
func (d Dirs) Image(key string) string { return filepath.Join(d.Images(), key) }

// ImageRecord is the record of the image with this key.
func (d Dirs) ImageRecord(key string) string { return filepath.Join(d.Image(key), "record.json") }

// ImageLocks holds the lock of each image being built, one file per key.
func (d Dirs) ImageLocks() string { return filepath.Join(d.Images(), "locks") }

// Rebuilds holds the rebuild counters, which go into image keys. They
// can't be in an image's folder: a counter is an input to the key that
// names it.
func (d Dirs) Rebuilds() string { return filepath.Join(d.Images(), "rebuilds.json") }

// RebuildsLock is held by the devvm changing a rebuild counter, which
// reads Rebuilds and writes it whole.
func (d Dirs) RebuildsLock() string { return filepath.Join(d.ImageLocks(), "rebuilds") }

// Cache holds what devvm makes again whenever it's needed, which may be
// deleted at any time: devvm's own mise, the shared Nix cache and the
// digests --from bases were last looked up at.
func (d Dirs) Cache() string { return filepath.Join(d.Data, "cache") }

// FromDigests caches the digest of each image a base was built from, per
// reference.
func (d Dirs) FromDigests() string { return filepath.Join(d.Cache(), "from-digests.json") }

// Run holds files for running VMs: sandbox profiles, proxy pids, the QEMU
// wrapper.
func (d Dirs) Run() string { return filepath.Join(d.Data, "run") }

// Logs holds each VM's network policy decisions and proxy log.
func (d Dirs) Logs() string { return filepath.Join(d.Data, "logs") }

// RunLogs holds the logs of devvm's runs: builds and the commands they ran.
func (d Dirs) RunLogs() string { return filepath.Join(d.Logs(), "runs") }

// CA holds the certificate authority VMs trust for the proxy.
func (d Dirs) CA() string { return filepath.Join(d.Data, "ca") }

// Tools is the shared tools folder, mounted read-only into VMs.
func (d Dirs) Tools() string { return filepath.Join(d.Data, "tools") }

// ToolLock is the lock file the shared tools are resolved into.
func (d Dirs) ToolLock() string { return filepath.Join(d.Data, "mise.lock") }

// ToolRecord is the record beside a tool image in the share: its hash,
// what it's built for, when, and since when no VM has had it.
func (d Dirs) ToolRecord(file string) string { return filepath.Join(d.Tools(), file+".json") }

// Mise holds the mise versions devvm runs on the host.
func (d Dirs) Mise() string { return filepath.Join(d.Cache(), "mise") }

// Exports is where exported VM state goes by default.
func (d Dirs) Exports() string { return filepath.Join(d.Data, "exports") }

// ExportFolder is the file beside an export in Exports that names the
// project folder of the VM it's of: the export's path with .folder for
// .tar.gz.
func ExportFolder(export string) string { return strings.TrimSuffix(export, ".tar.gz") + ".folder" }

// NixCache is the Nix cache VMs with network policy share, which their
// proxies fill from cache.nixos.org.
func (d Dirs) NixCache() string { return filepath.Join(d.Cache(), "nix-cache") }

// AgentSock is the socket of devvm's own SSH agent.
func (d Dirs) AgentSock() string { return filepath.Join(d.Data, "agent.sock") }

// OCIBuildLock is held while a base image is built from a container image.
func (d Dirs) OCIBuildLock() string { return filepath.Join(d.Data, "oci-build.lock") }

// MakingLock is held, shared, by each devvm making a VM or giving one new
// shared tools, and tried exclusively by devvm gc --tool-images --force.
func (d Dirs) MakingLock() string { return filepath.Join(d.Data, "making.lock") }

// LimaVM is a VM's folder in devvm's Lima home.
func (d Dirs) LimaVM(vm string) string { return filepath.Join(d.Lima, vm) }

// LimaVMs holds the Lima backend's own files for each VM, outside Lima's
// home: which ports it forwards, and its port forwarder's.
func (d Dirs) LimaVMs() string { return filepath.Join(d.Data, "lima-vms") }

// Smolvm holds the smolvm backend's files: its images (packs), what it set
// on each VM, and on Linux smolvm's own state.
func (d Dirs) Smolvm() string { return filepath.Join(d.Data, "smolvm") }

// SmolvmImage is an image's folder on smolvm: its pack.
func (d Dirs) SmolvmImage(key string) string { return filepath.Join(d.Smolvm(), "images", key) }

// SmolvmVM is the record of what the smolvm backend set on a VM.
func (d Dirs) SmolvmVM(vm string) string { return filepath.Join(d.Smolvm(), "vms", vm+".json") }

// Docker holds the Docker backend's files: what it set on each VM, and
// which shared tools it unpacked into its tools volume.
func (d Dirs) Docker() string { return filepath.Join(d.Data, "docker") }

// DockerVM is the record of what the Docker backend set on a VM.
func (d Dirs) DockerVM(vm string) string { return filepath.Join(d.Docker(), "vms", vm+".json") }

// ConfigFile is config.toml, the defaults you set by hand: devvm reads it
// and never writes it.
func (d Dirs) ConfigFile() string { return filepath.Join(d.Config, "config.toml") }

// Settings is settings.json, the defaults devvm's commands set (devvm
// setup, devvm size -g, devvm retention…), below config.toml's.
func (d Dirs) Settings() string { return filepath.Join(d.Data, "settings.json") }

// AgentKeys lists the keys added to devvm's SSH agent.
func (d Dirs) AgentKeys() string { return filepath.Join(d.Data, "agent-keys.json") }

// RetentionRun is touched when retention last ran.
func (d Dirs) RetentionRun() string { return filepath.Join(d.Data, "retention.run") }

// Layers holds the user's own layers.
func (d Dirs) Layers() string { return filepath.Join(d.Config, "layers") }

// Features holds the user's own base image features.
func (d Dirs) Features() string { return filepath.Join(d.Config, "features") }

// Policy holds the user's network policy rules for every VM.
func (d Dirs) Policy() string { return filepath.Join(d.Config, "policy") }

// TrustedCAs holds the user's own CA certificates (a company's), which
// VMs and builds trust besides their systems' (internal/trust).
func (d Dirs) TrustedCAs() string { return filepath.Join(d.Config, "ca") }

// Secrets holds secrets' values on hosts without a Keychain, one file each.
func (d Dirs) Secrets() string { return filepath.Join(d.Data, "secrets") }

// SSHConfig is the SSH config devvm writes for its VMs.
func (d Dirs) SSHConfig() string { return filepath.Join(d.Data, "ssh_config") }

// KnownHosts holds the VMs' SSH host keys.
func (d Dirs) KnownHosts() string { return filepath.Join(d.Data, "known_hosts") }
