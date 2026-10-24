package vm

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
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
