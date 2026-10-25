package vm

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
)

// Size is a VM's size as devvm stores it: CPUs a whole number, memory and
// disk in GiB (decimals allowed). "" is unset.
type Size struct {
	CPUs, Memory, Disk string
}

var (
	wholeNumber = regexp.MustCompile(`^[1-9][0-9]*$`)
	gibNumber   = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
)

// ParseCPUs checks a CPU count, as given to --cpus.
func ParseCPUs(v string) (string, error) {
	if !wholeNumber.MatchString(v) {
		return "", msg.CpusNotNumber(strconv.Quote(v))
	}
	return v, nil
}

// ParseGiB checks a size in GiB, as given to flag (--memory, --disk): N,
// NG or NGiB, decimals allowed. It returns N.
func ParseGiB(flag, v string) (string, error) {
	n := strings.TrimSuffix(v, "iB")
	n = strings.TrimSuffix(strings.TrimSuffix(n, "G"), "g")
	if !gibNumber.MatchString(n) || !strings.ContainsAny(n, "123456789") {
		return "", msg.SizeNotGib(flag, strconv.Quote(v))
	}
	return n, nil
}

// Parse checks every size set, as flags or a size file give them.
func (s Size) Parse() (Size, error) {
	var err error
	if s.CPUs != "" {
		if s.CPUs, err = ParseCPUs(s.CPUs); err != nil {
			return Size{}, err
		}
	}
	if s.Memory != "" {
		if s.Memory, err = ParseGiB("--memory", s.Memory); err != nil {
			return Size{}, err
		}
	}
	if s.Disk != "" {
		if s.Disk, err = ParseGiB("--disk", s.Disk); err != nil {
			return Size{}, err
		}
	}
	return s, nil
}

// Or returns s with what it leaves unset taken from def.
func (s Size) Or(def Size) Size {
	if s.CPUs == "" {
		s.CPUs = def.CPUs
	}
	if s.Memory == "" {
		s.Memory = def.Memory
	}
	if s.Disk == "" {
		s.Disk = def.Disk
	}
	return s
}

// Host is what a host can give a VM.
type Host struct {
	CPUs      int
	MemoryGiB int // whole GiB, rounded down
}

// ThisHost returns this machine's CPUs and memory.
func ThisHost() (Host, error) {
	mem, err := platform.Memory()
	if err != nil {
		return Host{}, err
	}
	return Host{CPUs: runtime.NumCPU(), MemoryGiB: int(mem >> 30)}, nil //nolint:gosec // GiB fit in an int
}

// Check refuses a parsed size the host can't give, or a disk smaller than
// minDisk GiB (the image's, which can't shrink; 0 for no limit). vm names
// the VM for the disk's refusal.
func (s Size) Check(h Host, minDisk float64, vm string) error {
	b := s.Backend()
	if b.CPUs > h.CPUs {
		return msg.TooManyCpus(strconv.Itoa(h.CPUs), s.CPUs)
	}
	if b.MemoryGiB > float64(h.MemoryGiB) {
		return msg.TooMuchMemory(strconv.Itoa(h.MemoryGiB), s.Memory)
	}
	if b.DiskGiB > 0 && b.DiskGiB < minDisk {
		return msg.DiskShrink(strconv.FormatFloat(minDisk, 'g', -1, 64), vm, s.Disk)
	}
	return nil
}

// Backend returns a parsed size as the backend has it.
func (s Size) Backend() backend.Size {
	var b backend.Size
	b.CPUs, _ = strconv.Atoi(s.CPUs)
	b.MemoryGiB, _ = strconv.ParseFloat(s.Memory, 64)
	b.DiskGiB, _ = strconv.ParseFloat(s.Disk, 64)
	return b
}

// pendingSize returns what of the size st asks for the VM, which has has,
// lacks: zero where it matches. Disks only grow, so a smaller one asked
// for is had.
func pendingSize(st *state.VM, has backend.Size) backend.Size {
	want := Size{CPUs: st.Get(KeyCPUs), Memory: st.Get(KeyMemory), Disk: st.Get(KeyDisk)}.Backend()
	var p backend.Size
	if want.CPUs != 0 && want.CPUs != has.CPUs {
		p.CPUs = want.CPUs
	}
	if want.MemoryGiB != 0 && want.MemoryGiB != has.MemoryGiB {
		p.MemoryGiB = want.MemoryGiB
	}
	if want.DiskGiB > has.DiskGiB {
		p.DiskGiB = want.DiskGiB
	}
	return p
}

// SizeText returns a size as devvm size shows it, without what's unset:
// cpus=4 memory=8GiB disk=100GiB.
func SizeText(s backend.Size) string {
	var f []string
	if s.CPUs > 0 {
		f = append(f, KeyCPUs+"="+strconv.Itoa(s.CPUs))
	}
	if s.MemoryGiB > 0 {
		f = append(f, KeyMemory+"="+strconv.FormatFloat(s.MemoryGiB, 'g', -1, 64)+"GiB")
	}
	if s.DiskGiB > 0 {
		f = append(f, KeyDisk+"="+strconv.FormatFloat(s.DiskGiB, 'g', -1, 64)+"GiB")
	}
	return strings.Join(f, " ")
}

// fields returns a size's keys and values, as the size file has them.
func (s Size) fields() [][2]string {
	return [][2]string{{KeyCPUs, s.CPUs}, {KeyMemory, s.Memory}, {KeyDisk, s.Disk}}
}

// defaultSize reads the size new VMs get, checked: the file may have
// been edited by hand.
func (e *Env) defaultSize() (Size, error) {
	def, err := state.ReadSize(e.Dirs)
	if err != nil {
		return Size{}, err
	}
	return Size{CPUs: def.CPUs, Memory: def.Memory, Disk: def.Disk}.Parse()
}

// ShowSize writes to w the size of the folder's VM (cpus=4 memory=8GiB
// disk=100GiB), saying what changes at its next start; with global, the
// size new VMs get, a key=value line each, empty for the image's.
func (e *Env) ShowSize(ctx context.Context, w io.Writer, global bool) error {
	if global {
		def, err := e.defaultSize()
		if err != nil {
			return err
		}
		var b strings.Builder
		for _, kv := range def.fields() {
			b.WriteString(kv[0] + "=" + kv[1] + "\n")
		}
		_, err = io.WriteString(w, b.String())
		return err
	}
	t, err := e.folderVM(ctx, msg.NoVMSize)
	if err != nil {
		return err
	}
	in, err := t.b.Inspect(ctx, t.f.Name)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, SizeText(in.Size)+"\n"); err != nil {
		return err
	}
	if p := pendingSize(t.st, in.Size); p != (backend.Size{}) {
		e.Note(msg.SizePending(SizeText(p), t.f.Name))
	}
	return nil
}

// SetSize changes the folder's VM's size to what s sets: at once when it's
// stopped, from its next start when it runs. A disk only grows. With
// global, it changes the size new VMs get instead.
func (e *Env) SetSize(ctx context.Context, s Size, global bool) error {
	s, err := s.Parse()
	if err != nil {
		return err
	}
	host, err := e.Host()
	if err != nil {
		return err
	}
	if global {
		old, err := e.defaultSize()
		if err != nil {
			return err
		}
		if err := s.Check(host, 0, ""); err != nil {
			return err
		}
		def := s.Or(old)
		if err := state.WriteSize(e.Dirs, state.Size{CPUs: def.CPUs, Memory: def.Memory, Disk: def.Disk}); err != nil {
			return err
		}
		e.Info(msg.SizeDefaults(SizeText(def.Backend())))
		return nil
	}
	t, err := e.folderVM(ctx, msg.NoVMSize)
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	if err := e.Registry.Require(b, vm, s.Backend().Features()...); err != nil {
		return err
	}
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	if err := s.Check(host, in.Size.DiskGiB, vm); err != nil {
		return err
	}
	for _, kv := range s.fields() {
		if kv[1] == "" {
			continue
		}
		if err := st.Set(kv[0], kv[1]); err != nil {
			return err
		}
	}
	if err := e.applySize(ctx, b, st, in); err != nil {
		return err
	}
	// Otherwise applySize said it waits for the next start.
	if in, err = b.Inspect(ctx, vm); err != nil {
		return err
	}
	if pendingSize(st, in.Size) == (backend.Size{}) {
		e.Info(msg.VMSize(SizeText(in.Size), vm))
	}
	return nil
}

// BaseDisk returns the disk size in GiB that base.yaml in root gives base,
// and so every image and new VM (disks can't shrink); 0 when it sets none.
func BaseDisk(root string) float64 {
	f, err := os.Open(filepath.Join(root, "base.yaml")) //nolint:gosec // devvm's own file
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if v, ok := strings.CutPrefix(s.Text(), "disk:"); ok {
			g, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(v), "GiB"), 64)
			return g
		}
	}
	return 0
}
