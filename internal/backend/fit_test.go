package backend

import "testing"

func TestFitCPUs(t *testing.T) {
	for _, c := range []struct{ want, host, fitted int }{
		{4, 16, 4},
		{4, 4, 4},
		{4, 2, 2},
		{4, 1, 1},
		{4, 0, 4}, // unknown
		{4, -1, 4},
	} {
		if got := FitCPUs(c.want, c.host); got != c.fitted {
			t.Errorf("FitCPUs(%d, %d) = %d, want %d", c.want, c.host, got, c.fitted)
		}
	}
}

// base's memory: base.yaml's, or half the host's whole GiB when that
// leaves it less than 2 GiB.
func TestFitMemory(t *testing.T) {
	for _, c := range []struct {
		want      float64
		hostBytes uint64
		fitted    float64
	}{
		{8, 16*gib + gib/2, 8},
		{8, 10*gib + gib/2, 8},
		{8, 10 * gib, 8},
		{8, 9*gib + gib/2, 4.5},
		{8, 8*gib + gib/2, 4},
		{8, 7*gib + gib/2, 3.5},
		{2, 4 * gib, 2},
		{8, 0, 0},
	} {
		if got := FitMemory(c.want, c.hostBytes); got != c.fitted {
			t.Errorf("FitMemory(%v, %d) = %v, want %v", c.want, c.hostBytes, got, c.fitted)
		}
	}
}

// The most a VM may ask for: the host's whole GiB less 2, or half of them
// on a host of 4 GiB or less. Whatever FitMemory gives is allowed.
func TestMaxMemory(t *testing.T) {
	for _, c := range []struct {
		hostBytes uint64
		max       float64
	}{
		{32 * gib, 30},
		{8*gib + gib/2, 6},
		{8 * gib, 6},
		{5 * gib, 3},
		{4 * gib, 2},
		{3 * gib, 1.5},
		{1 * gib, 0.5},
		{0, 0},
	} {
		if got := MaxMemory(c.hostBytes); got != c.max {
			t.Errorf("MaxMemory(%d) = %v, want %v", c.hostBytes, got, c.max)
		}
		for _, want := range []float64{0.5, 1, 2, 3.5, 6, 8, 64} {
			if fit := FitMemory(want, c.hostBytes); fit > MaxMemory(c.hostBytes) {
				t.Errorf("FitMemory(%v, %d) = %v, more than MaxMemory's %v", want, c.hostBytes, fit, MaxMemory(c.hostBytes))
			}
		}
	}
}
