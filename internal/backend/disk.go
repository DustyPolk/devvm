package backend

import (
	"errors"
	"math"
	"strconv"

	"github.com/w3cj/devvm/internal/msg"
)

// The free space devvm wants on the host before each step that writes
// disks, with a margin over what the step was measured to take. Measured
// read-only on macOS (Lima vz, APFS) in the e2e sandbox's Lima home, as
// allocated size (du -k, as platform.DiskUsage counts: blocks, not a
// sparse disk's length), and on a Linux host (QEMU, qcow2) as the docs'
// design notes record it. Each wants checking on a fresh e2e run.
const (
	// FreeBase is what building base wants. Base's disk took 2.6-2.9 GiB
	// without features and 5.1 GiB with every feature (the e2e --from
	// tier), 2.0 GB as qcow2 on QEMU. Lima also keeps Ubuntu's cloud image
	// in its cache (not measured), and a --from base first converts its
	// image in a throwaway VM of its own (another base-sized disk). About
	// twice the largest.
	FreeBase uint64 = 12 << 30
	// FreeImage is what building any other image wants. On APFS a layer
	// is a clone of its parent and takes only what its script writes (an
	// image built two minutes after base came to 3.2 GiB against base's
	// 2.9 GiB, twice in the sandbox), but on QEMU a clone copies the
	// parent's whole disk (up to base's 5.1 GiB with every feature), then
	// the layer's installs.
	FreeImage uint64 = 8 << 30
	// FreeTools is what building shared tool images wants. Ten tools took
	// 0.85 GB as images and 1.5 GB installed in the build VM, which is a
	// clone of base: free on APFS, a full copy of base's disk on QEMU (up
	// to 5.1 GiB).
	FreeTools uint64 = 8 << 30
	// FreeVM is what making a VM wants. A new VM takes almost nothing (an
	// APFS clone of its image, a 12 MB qcow2 overlay on QEMU after its first
	// boot), then grows with its setup and the project's own installs, which
	// a few GiB covers.
	FreeVM uint64 = 4 << 30
)

// Storage is a backend whose images and VMs are files on the host's own
// disks. DiskDirs returns a folder on each volume it writes them to,
// made or not. A backend whose disks are elsewhere (Docker's, in its
// engine's storage) doesn't implement it, and its free space isn't
// checked.
type Storage interface {
	DiskDirs() []string
}

// Space checks the host's free space before a step that writes disks.
// Free returns the free space on the volume holding a path
// (platform.FreeSpace); with Free nil nothing is checked.
type Space struct {
	Free func(path string) (uint64, error)
}

// Check refuses when a volume b writes to, or one of extra (folders the
// step writes to itself), has less than need bytes free (disk_low, for
// what: the image or VM the step makes). A system devvm can't read free
// space on isn't checked.
func (s Space) Check(b Backend, what string, need uint64, extra ...string) error {
	dir, free, err := s.lowest(b, extra)
	if err != nil || dir == "" || free >= need {
		return err
	}
	return msg.DiskLow(dir, GiB(free), what, strconv.FormatUint(need>>30, 10))
}

// Warning is doctor's warning when a volume b writes to has less free
// than building base wants; false when it has enough, or can't be read.
func (s Space) Warning(b Backend) (msg.M, bool) {
	dir, free, err := s.lowest(b, nil)
	if err != nil || dir == "" || free >= FreeBase {
		return msg.M{}, false
	}
	return msg.DiskLowWarning(dir, GiB(free), strconv.FormatUint(FreeBase>>30, 10)), true
}

// lowest returns the folder, of b's and extra, on the volume with the
// least free space, and that space; "" when none is checked.
func (s Space) lowest(b Backend, extra []string) (string, uint64, error) {
	if s.Free == nil {
		return "", 0, nil
	}
	var dirs []string
	if st, ok := b.(Storage); ok {
		dirs = st.DiskDirs()
	}
	dirs = append(dirs, extra...)
	low, lowFree := "", uint64(math.MaxUint64)
	for _, d := range dirs {
		free, err := s.Free(d)
		if errors.Is(err, errors.ErrUnsupported) {
			return "", 0, nil
		}
		if err != nil {
			return "", 0, msg.DiskFreeUnknown(d).Because(err)
		}
		if free < lowFree {
			low, lowFree = d, free
		}
	}
	return low, lowFree, nil
}

// GiB shows a size in bytes as GiB, to one decimal, rounded down: free
// space just short of a floor never shows as the floor.
func GiB(n uint64) string {
	return strconv.FormatFloat(math.Floor(float64(n)/(1<<30)*10)/10, 'f', 1, 64)
}
