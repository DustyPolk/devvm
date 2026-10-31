package backend

// gib is a GiB in bytes.
const gib = 1 << 30

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
// doesn't start.
func FitMemory(wantGiB float64, hostBytes uint64) float64 {
	h := float64(hostBytes / gib)
	if wantGiB+2 <= h {
		return wantGiB
	}
	return h / 2
}
