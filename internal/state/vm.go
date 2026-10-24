package state

import (
	"errors"
	"io/fs"
	"os"

	"github.com/w3cj/devvm/internal/paths"
)

// VMNames returns the names of the VMs that have state, sorted.
func VMNames(d paths.Dirs) ([]string, error) {
	es, err := os.ReadDir(d.VMs())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range es {
		if e.Type().IsRegular() && checkName("VM name", e.Name()) == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Keys of a VM's state. Other lowercase keys (a-z, 0-9, _) may be set too.
const (
	KeyImage   = "image"   // the image the VM was made from, plus layers added in place
	KeyDir     = "dir"     // the project folder
	KeyFolder  = "folder"  // the project folder's identity, to notice it was replaced
	KeyChain   = "chain"   // its image's build chain
	KeyFrom    = "from"    // the image its base started from, if not the default
	KeyBackend = "backend" // the backend running it
)

// VM is one VM's state, a key=value file. It lives as long as the VM:
// creating a VM replaces it, and removing the VM removes it.
type VM struct {
	name   string
	path   string
	kv     kv
	exists bool
}

// NewVM is what a VM's state starts with.
type NewVM struct {
	Image, Dir, Folder, Chain, From, Backend string
}

// ReadVM reads a VM's state. A VM with no state file has empty state:
// every key reads as "", and Exists reports false.
func ReadVM(d paths.Dirs, name string) (*VM, error) {
	if err := checkName("VM name", name); err != nil {
		return nil, err
	}
	v := &VM{name: name, path: d.VMState(name)}
	return v, v.load()
}

func (v *VM) load() error {
	k, err := readKV(v.path, isWord)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		v.kv, v.exists = newKV(), false
		return nil
	case err != nil:
		return err
	}
	v.kv, v.exists = k, true
	return nil
}

// CreateVM writes a new VM's state, replacing any it had.
func CreateVM(d paths.Dirs, name string, n NewVM) (*VM, error) {
	if err := checkName("VM name", name); err != nil {
		return nil, err
	}
	v := &VM{name: name, path: d.VMState(name), kv: newKV()}
	for _, p := range [][2]string{
		{KeyImage, n.Image},
		{KeyDir, n.Dir},
		{KeyFolder, n.Folder},
		{KeyChain, n.Chain},
		{KeyFrom, n.From},
		{KeyBackend, n.Backend},
	} {
		if err := checkPair(p[0], p[1], isWord); err != nil {
			return nil, err
		}
		v.kv.set(p[0], p[1])
	}
	if err := v.write(); err != nil {
		return nil, err
	}
	return v, nil
}

// Name is the VM's name.
func (v *VM) Name() string { return v.name }

// Exists reports whether the VM has a state file.
func (v *VM) Exists() bool { return v.exists }

// Get returns a key's value, or "" when it isn't set.
func (v *VM) Get(key string) string { return v.kv.get(key) }

// Set sets a key and writes the state file. It reads the file again first,
// so keys another devvm set since are kept.
func (v *VM) Set(key, val string) error {
	if err := checkPair(key, val, isWord); err != nil {
		return err
	}
	if err := v.load(); err != nil {
		return err
	}
	v.kv.set(key, val)
	return v.write()
}

func (v *VM) write() error {
	if err := writeFile(v.path, v.kv.bytes()); err != nil {
		return err
	}
	v.exists = true
	return nil
}

// Remove deletes the VM's state file, if it has one.
func (v *VM) Remove() error {
	if err := removeFile(v.path); err != nil {
		return err
	}
	v.kv, v.exists = newKV(), false
	return nil
}
