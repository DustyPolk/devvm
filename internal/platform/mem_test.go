//go:build darwin || linux

package platform

import "testing"

func TestMemory(t *testing.T) {
	m, err := Memory()
	if err != nil || m < 1<<28 {
		t.Errorf("Memory() = %d, %v", m, err)
	}
}
