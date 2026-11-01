package vm

import (
	"bufio"
	"context"
	"io/fs"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
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

// memoryBytes is the host's whole GiB in bytes, 0 for 0 or less.
func (h Host) memoryBytes() uint64 {
	if h.MemoryGiB <= 0 {
		return 0
	}
	return uint64(h.MemoryGiB) << 30
}

// ThisHost returns the CPUs and memory of h, this machine as devvm reads
// it.
func ThisHost(h platform.Host) (Host, error) {
	mem, err := h.Memory()
	if err != nil {
		return Host{}, err
	}
	return Host{CPUs: h.CPUs(), MemoryGiB: int(mem >> 30)}, nil
}

// Check refuses a parsed size the host can't give: more CPUs than it has,
// or more memory than backend.MaxMemory allows, which keeps memory for
// the host as base's fitting does. It also refuses a disk smaller than
// minDisk GiB (the image's, which can't shrink; 0 for no limit). vm names
// the VM for the disk's refusal.
func (s Size) Check(h Host, minDisk float64, vm string) error {
	b := s.Backend()
	if b.CPUs > h.CPUs {
		return msg.TooManyCpus(strconv.Itoa(h.CPUs), s.CPUs)
	}
	if most := backend.MaxMemory(h.memoryBytes()); b.MemoryGiB > most {
		return msg.TooMuchMemory(strconv.Itoa(h.MemoryGiB), strconv.FormatFloat(most, 'g', -1, 64), s.Memory)
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
func (e *Env) defaultSize() (Size, error) { return readDefaultSize(e.Dirs) }

// readDefaultSize is defaultSize from d's size file.
func readDefaultSize(d paths.Dirs) (Size, error) {
	def, err := state.ReadSize(d)
	if err != nil {
		return Size{}, err
	}
	return Size{CPUs: def.CPUs, Memory: def.Memory, Disk: def.Disk}.Parse()
}

// applySize makes a stopped VM's size what its state asks for, where it
// differs (disks only grow). A running VM's waits for its next start.
func (e *Env) applySize(ctx context.Context, b backend.Backend, st *state.VM, in backend.Instance) error {
	edit := pendingSize(st, in.Size)
	if edit == (backend.Size{}) {
		return nil
	}
	if in.Status == backend.StatusRunning {
		e.Warn(msg.SizeNextStart(st.Name()))
		return nil
	}
	return b.SetSize(ctx, st.Name(), edit)
}

// SizeShown is a size as devvm size shows it: the folder's VM's, or with
// -g the one new VMs get, 0 for what's the image's.
type SizeShown struct {
	backend.Size
	// DiskUsedGiB is what the VM's disk holds, to a tenth of a GiB, on a
	// backend that sizes no disks (DiskMeasured).
	DiskUsedGiB  float64
	DiskMeasured bool
	// Pending is what changes at the VM's next start, 0 for what doesn't.
	Pending backend.Size
	Global  bool
}

// Text is the size as devvm size prints it: cpus=4 memory=8GiB
// disk=100GiB for a VM, and a key=value line each for -g, empty for the
// image's.
func (s SizeShown) Text() string {
	if s.Global {
		var b strings.Builder
		for _, kv := range [][2]string{{KeyCPUs, intText(s.CPUs)}, {KeyMemory, gibValue(s.MemoryGiB)}, {KeyDisk, gibValue(s.DiskGiB)}} {
			b.WriteString(kv[0] + "=" + kv[1] + "\n")
		}
		return b.String()
	}
	line := SizeText(s.Size)
	if s.DiskMeasured {
		line += " " + KeyDiskUsed + "=" + strconv.FormatFloat(s.DiskUsedGiB, 'f', -1, 64) + "GiB"
	}
	return line + "\n"
}

// intText and gibValue write a size's numbers as the size file has them,
// "" for 0.
func intText(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func gibValue(n float64) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// ShowSize returns the size of the folder's VM, saying what changes at
// its next start; with global, the size new VMs get.
func (e Settings) ShowSize(ctx context.Context, global bool) (SizeShown, error) {
	if global {
		def, err := e.defaultSize()
		if err != nil {
			return SizeShown{}, err
		}
		return SizeShown{Size: def.Backend(), Global: true}, nil
	}
	t, err := e.folderVM(ctx, msg.NoVMSize)
	if err != nil {
		return SizeShown{}, err
	}
	in, err := t.b.Inspect(ctx, t.f.Name)
	if err != nil {
		return SizeShown{}, err
	}
	s := SizeShown{Size: in.Size, Pending: pendingSize(t.st, in.Size)}
	used, over, err := e.diskUsed(ctx, t.b, t.f.Name)
	if err != nil {
		return SizeShown{}, err
	}
	if used >= 0 {
		s.DiskUsedGiB, s.DiskMeasured = gib(used), true
	}
	if over != nil {
		e.Warn(*over)
	}
	if s.Pending != (backend.Size{}) {
		e.Note(msg.SizePending(SizeText(s.Pending), t.f.Name))
	}
	return s, nil
}

// KeyDiskUsed is what devvm size shows of a VM whose backend sizes no
// disks (backend.DiskUser): what its disk holds, in GiB.
const KeyDiskUsed = "disk_used"

// diskUsed is the bytes the VM's disk holds on a backend that sizes no
// disks, with the warning for one past the size VMs get elsewhere; -1 on
// other backends.
func (e *Env) diskUsed(ctx context.Context, b backend.Backend, vm string) (int64, *msg.M, error) {
	du, ok := b.(backend.DiskUser)
	if !ok {
		return -1, nil, nil
	}
	used, limit, err := du.DiskUsed(ctx, vm)
	if err != nil {
		return -1, nil, err
	}
	if limit > 0 && used > limit {
		m := msg.DiskOver(b.Name(), gibText(limit), gibText(used), vm)
		return used, &m, nil
	}
	return used, nil, nil
}

// warnDisk warns when the VM's disk holds more than VMs get elsewhere, on
// a backend that sizes no disks. One it can't measure goes unsaid: the
// launch goes on.
func (e *Env) warnDisk(ctx context.Context, b backend.Backend, vm string) {
	if _, over, err := e.diskUsed(ctx, b, vm); err == nil && over != nil {
		e.Warn(*over)
	}
}

// gib is bytes in GiB, to a tenth; gibText, written out.
func gib(n int64) float64 { return math.Round(float64(n)/(1<<30)*10) / 10 }

func gibText(n int64) string { return strconv.FormatFloat(gib(n), 'f', -1, 64) }

// SetSize changes the folder's VM's size to what s sets: at once when it's
// stopped, from its next start when it runs. A disk only grows. With
// global, it changes the size new VMs get instead.
func (e Settings) SetSize(ctx context.Context, s Size, global bool) error {
	s, err := s.Parse()
	if err != nil {
		return err
	}
	host, err := e.Host()
	if err != nil {
		return err
	}
	if global {
		return SizeChoice{Dirs: e.Dirs, Assets: e.Assets, Host: host, Info: e.Info}.Set(s)
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

// BaseDisk returns the disk size in GiB that base.yaml in assets, devvm's
// own files, gives base, and so every image and new VM (disks can't
// shrink); 0 when it sets none.
func BaseDisk(assets fs.FS) float64 { return baseSetting(assets, "disk") }

// baseSetting returns the number base.yaml in assets sets for key (cpus,
// memory or disk, the last two in GiB); 0 when it sets none.
func baseSetting(assets fs.FS, key string) float64 {
	f, err := assets.Open("base.yaml")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if v, ok := strings.CutPrefix(s.Text(), key+":"); ok {
			g, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(v), "GiB"), 64)
			return g
		}
	}
	return 0
}
