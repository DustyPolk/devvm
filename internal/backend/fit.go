package backend

// gib is a GiB in bytes.
const gib = 1 << 30

// Base's size, which every backend gives base: its CPUs and memory, which
// FitCPUs and FitMemory fit to the host, and its disk, which every image
// and new VM gets at least, as disks can't shrink. Base's disk goes into
// its image, so each backend's Origin carries it into base's key.
const (
	BaseCPUs      = 4
	BaseMemoryGiB = 8   // GiB
	BaseDiskGiB   = 100 // GiB
)

// FitCPUs is the CPUs a VM asking for want gets on a host with hostCPUs:
// want, or the host's count when it has fewer. A hostCPUs of 0 or less
// is unknown, and gives want.
func FitCPUs(want, hostCPUs int) int {
	if hostCPUs <= 0 {
		return want
	}
	return min(want, hostCPUs)
}

// FitMemory is the memory in GiB a VM asking for wantGiB gets on a host
// with hostBytes: wantGiB, or half the host's whole GiB when wantGiB
// would leave the host less than 2 GiB, as a VM asking for all of it
// doesn't start. It never exceeds MaxMemory(hostBytes).
func FitMemory(wantGiB float64, hostBytes uint64) float64 {
	h := float64(hostBytes / gib)
	if wantGiB+2 <= h {
		return wantGiB
	}
	return h / 2
}

// Fitted is got's CPUs and memory, base's as fitted to the host, when
// either is less than want's, base's own; zero when the host gave base
// all it asks for.
func Fitted(want, got Size) Size {
	if got.CPUs < want.CPUs || got.MemoryGiB < want.MemoryGiB {
		return Size{CPUs: got.CPUs, MemoryGiB: got.MemoryGiB}
	}
	return Size{}
}

// MaxMemory is the most memory in GiB a VM may ask for on a host with
// hostBytes, by FitMemory's rule: the host's whole GiB less 2, or half of
// them on a host of 4 GiB or less, where that's more.
func MaxMemory(hostBytes uint64) float64 {
	h := float64(hostBytes / gib)
	return max(h-2, h/2)
}
