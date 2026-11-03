package state

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/vmname"
)

// VMNames returns the names of the VMs that have state, sorted: the name
// each VM's folder (paths.Dirs.VMDir) records. Anything else in the VMs'
// folder, and a folder without state, isn't a VM.
func VMNames(d paths.Dirs) ([]string, error) {
	es, err := os.ReadDir(d.VMs())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range es {
		if !e.IsDir() || !vmname.IsHash(e.Name()) {
			continue
		}
		v, err := ReadHash(d, e.Name())
		if err != nil {
			return nil, err
		}
		if v.Exists() && v.Name() != "" {
			names = append(names, v.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

// Backends returns the backends the VMs' states name: the only ones that
// hold devvm's VMs.
func Backends(d paths.Dirs) (map[string]bool, error) {
	vms, err := VMNames(d)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, vm := range vms {
		st, err := ReadVM(d, vm)
		if err != nil {
			return nil, err
		}
		if name := st.Get(KeyBackend); name != "" {
			used[name] = true
		}
	}
	return used, nil
}

// Keys of a VM's state. Other lowercase keys (a-z, 0-9, _) may be set too.
const (
	// KeyName is the VM's name, chosen when it's made for the backend it's
	// made on (project.New), and never again: a folder's VM is found by
	// its hash (ReadHash), whatever name that backend gave it.
	KeyName = "name"
	// KeyAlias is the name ssh and editors reach it by besides its own,
	// chosen when it's made (sshconfig.ChooseAlias), and never again.
	KeyAlias = "alias"
	KeyImage = "image" // the image the VM was made from: base, or its layers joined with +
	KeyDir   = "dir"   // the project folder
	// KeyToolNames is the VM's shared tools by name, space-separated, in
	// their order (image.SortTools): node claude. Its tool images are
	// KeyTools in internal/vm.
	KeyToolNames = "tool_names"
	KeyFolder    = "folder"  // the project folder's identity, to notice it was replaced
	KeyChain     = "chain"   // its image's build chain
	KeyFrom      = "from"    // the image its base started from, if not the default
	KeyBackend   = "backend" // the backend running it
	// KeyID is a random ID CreateVM gives every VM, so a link can tell a
	// VM made again in the same folder from the one it linked.
	KeyID = "id"
)

// newID is a VM's KeyID: 128 random bits, in hex.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// VM is one VM's state, a key=value file in its folder. It lives as long
// as the VM: creating a VM replaces it, and removing the VM removes it.
type VM struct {
	name string
	// path is its state file; "" for a name whose folder is another VM's
	// (ReadVM), which is never written.
	path   string
	kv     kv
	exists bool
}

// NewVM is what a VM's state starts with: Tools are its shared tools'
// names, Alias the name ssh reaches it by (KeyAlias).
type NewVM struct {
	Image, Dir, Folder, Chain, From, Backend, Alias string
	Tools                                           []string
}

// ReadHash reads the state in the VM folder named by hash (a project
// folder's, project.Hash). A folder with no state file has empty state:
// every key reads as "", Name too, and Exists reports false.
func ReadHash(d paths.Dirs, hash string) (*VM, error) {
	if !vmname.IsHash(hash) {
		return nil, fmt.Errorf("invalid VM hash %q", hash)
	}
	v := &VM{path: paths.VM{Dir: d.VMDir(hash)}.State()}
	if err := v.load(); err != nil {
		return nil, err
	}
	v.name = v.kv.get(KeyName)
	return v, nil
}

// ReadVM reads the state of the project VM named name, in the folder its
// name's hash names. A VM with no state file has empty state: every key
// reads as "", and Exists reports false; so does one whose folder records
// another name, whose state is never written through this one.
func ReadVM(d paths.Dirs, name string) (*VM, error) {
	h, ok := vmname.HashOf(name)
	if !ok {
		return nil, fmt.Errorf("invalid VM name %q", name)
	}
	v, err := ReadHash(d, h)
	if err != nil {
		return nil, err
	}
	if v.exists && v.name != "" && v.name != name {
		return &VM{name: name, kv: newKV()}, nil
	}
	v.name = name
	return v, nil
}

func (v *VM) load() error {
	if v.path == "" {
		v.kv, v.exists = newKV(), false
		return nil
	}
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

// CreateVM writes a new project VM's state, replacing any its folder had,
// with its name, its alias and a new KeyID.
func CreateVM(d paths.Dirs, name string, n NewVM) (*VM, error) {
	if !vmname.IsVM(name) {
		return nil, fmt.Errorf("invalid VM name %q", name)
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	v := &VM{name: name, path: d.VM(name).State(), kv: newKV()}
	for _, p := range [][2]string{
		{KeyName, name},
		{KeyAlias, n.Alias},
		{KeyID, id},
		{KeyImage, n.Image},
		{KeyToolNames, strings.Join(n.Tools, " ")},
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

// ToolNames returns the VM's shared tools by name (KeyToolNames): node
// claude.
func (v *VM) ToolNames() []string {
	if v.Get(KeyToolNames) == "" {
		return nil
	}
	return strings.Fields(v.Get(KeyToolNames))
}

// Set sets a key and writes the state file. It reads the file again first,
// so keys another devvm set since are kept.
func (v *VM) Set(key, val string) error {
	if err := checkPair(key, val, isWord); err != nil {
		return err
	}
	if v.path == "" {
		return fmt.Errorf("%s: its folder is another VM's", v.name)
	}
	if err := v.load(); err != nil {
		return err
	}
	if v.kv.get(KeyName) == "" && v.name != "" {
		v.kv.set(KeyName, v.name)
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

// Remove deletes the VM's state file, if it has one: the rest of its
// folder is its teardown's.
func (v *VM) Remove() error {
	if v.path == "" {
		return nil
	}
	if err := removeFile(v.path); err != nil {
		return err
	}
	v.kv, v.exists = newKV(), false
	return nil
}
