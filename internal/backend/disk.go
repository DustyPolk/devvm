package backend

import (
	"errors"
	"math"
	"strconv"

	"github.com/devvmhq/devvm/internal/msg"
)

// Floors is the free space, in bytes, a backend wants on its disks before
// each step that writes them, with a margin over what the step was
// measured to take. Each backend's are its own, measured with statfs
// before, during and after each step (not du, which counts a clone's
// shared blocks again).
type Floors struct {
	Base  uint64 // building base
	Image uint64 // building any other image
	VM    uint64 // making a VM, with its first launch
	// Tools and PerTool are what a shared tools build wants: Tools, and
	// PerTool more for each tool it builds.
	Tools, PerTool uint64
}

// ForTools is what building n shared tool images wants.
func (f Floors) ForTools(n int) uint64 { return f.Tools + count(n)*f.PerTool }

// count is n, a number of tools, as a uint64; none when negative.
func count(n int) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}

// ToolImages is the free space building n shared tool images wants in
// devvm's data folder, where the images come out on every backend: the
// images alone, measured with statfs (the folder on its own tmpfs: 184 MB
// for claude, 194 MB for node, pnpm and bun). Ten tools came to 1.5 GiB as images on Lima, rust alone, the largest,
// to 0.6 GiB.
func ToolImages(n int) uint64 { return 512<<20 + count(n)*(256<<20) }

// Space checks the host's free space before a step that writes disks.
// Free returns the free space on the volume holding a path
// (platform.Host's); with Free nil nothing is checked.
type Space struct {
	Free func(path string) (uint64, error)
}

// Check refuses when a volume h writes to has less free than need
// returns from h's floors (disk_low, for what: the image, tools or VM the
// step makes). A backend without DiskDirs, or a system devvm can't read
// free space on, isn't checked.
func (s Space) Check(h Host, what string, need func(Floors) uint64) error {
	return s.check(h.DiskDirs(), what, need(h.Floors()))
}

// CheckDir refuses when the volume holding dir, a folder the step writes
// to itself, has less than need bytes free.
func (s Space) CheckDir(dir, what string, need uint64) error {
	return s.check([]string{dir}, what, need)
}

func (s Space) check(dirs []string, what string, need uint64) error {
	need = wholeGiB(need)
	dir, free, err := s.lowest(dirs)
	if err != nil || dir == "" || free >= need {
		return err
	}
	return msg.DiskLow(dir, GiB(free), what, strconv.FormatUint(need>>30, 10))
}

// Warning is doctor's warning when a volume h writes to has less free
// than building base wants there; false when it has enough, or can't be
// read.
func (s Space) Warning(h Host) (msg.M, bool) {
	need := wholeGiB(h.Floors().Base)
	dir, free, err := s.lowest(h.DiskDirs())
	if err != nil || dir == "" || free >= need {
		return msg.M{}, false
	}
	return msg.DiskLowWarning(dir, GiB(free), strconv.FormatUint(need>>30, 10)), true
}

// lowest returns the folder, of dirs, on the volume with the least free
// space, and that space; "" when none is checked.
func (s Space) lowest(dirs []string) (string, uint64, error) {
	if s.Free == nil {
		return "", 0, nil
	}
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

// wholeGiB rounds a size up to whole GiB, which the messages show.
func wholeGiB(n uint64) uint64 { return (n + 1<<30 - 1) >> 30 << 30 }

// GiB shows a size in bytes as GiB, to one decimal, rounded down: free
// space just short of a floor never shows as the floor.
func GiB(n uint64) string {
	return strconv.FormatFloat(math.Floor(float64(n)/(1<<30)*10)/10, 'f', 1, 64)
}
