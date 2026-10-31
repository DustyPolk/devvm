package desktop

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

// TestE2EDesktop checks, on a real VM's desktop as devvm desktop serves
// it (DEVVM_E2E_DESKTOP, 127.0.0.1:PORT, with DEVVM_E2E_PASSWORD), that a
// viewer with a wrong password is refused, and one with the password gets
// the VM's desktop: its ServerInit and a screen update. tests/e2e.sh runs
// it; otherwise it's skipped.
func TestE2EDesktop(t *testing.T) {
	addr, password := os.Getenv("DEVVM_E2E_DESKTOP"), os.Getenv("DEVVM_E2E_PASSWORD")
	if addr == "" {
		t.Skip("tests/e2e.sh runs it, on a VM of its own")
	}
	if _, res, _ := viewer(t, addr, "RFB 003.008\n", wrong(password)); res == resultOK {
		t.Fatal("a wrong password got in")
	}
	c, res, reason := viewer(t, addr, "RFB 003.008\n", password)
	if res != resultOK {
		t.Fatalf("the password was refused: %q", reason)
	}
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := c.Write([]byte{1}); err != nil { // ClientInit, shared
		t.Fatal(err)
	}
	w, h := serverInit(t, c)
	if w == 0 || h == 0 {
		t.Fatalf("a %dx%d desktop", w, h)
	}
	// DEVVM_E2E_CLIPBOARD: this clipboard sent, then a key event (the
	// control), for the VM's server to show what reached it.
	if clip := os.Getenv("DEVVM_E2E_CLIPBOARD"); clip != "" {
		m := binary.BigEndian.AppendUint32([]byte{msgClientCutText, 0, 0, 0}, uint32(len(clip))) //nolint:gosec // a short test value
		m = append(m, clip...)
		m = append(m, msgKeyEvent, 1, 0, 0, 0, 0, 0, 'k')
		if _, err := c.Write(m); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		t.Log("a clipboard and a key event sent")
		return
	}
	req := []byte{msgUpdateRequest, 0, 0, 0, 0, 0}
	req = binary.BigEndian.AppendUint16(req, w)
	req = binary.BigEndian.AppendUint16(req, h)
	if _, err := c.Write(req); err != nil {
		t.Fatal(err)
	}
	var typ [1]byte
	if _, err := io.ReadFull(c, typ[:]); err != nil || typ[0] != 0 {
		t.Fatalf("no screen update: type %d, %v", typ[0], err)
	}
	t.Logf("a %dx%d desktop, its first update read", w, h)
}

// wrong is another password than p, of the same length.
func wrong(p string) string {
	if p == "aaaaaaaa" {
		return "bbbbbbbb"
	}
	return "aaaaaaaa"
}

// serverInit reads the VM's ServerInit and returns its size.
func serverInit(t *testing.T, c net.Conn) (uint16, uint16) {
	t.Helper()
	h := make([]byte, 24) // size, pixel format, name length
	if _, err := io.ReadFull(c, h); err != nil {
		t.Fatal(err)
	}
	name := make([]byte, binary.BigEndian.Uint32(h[20:]))
	if _, err := io.ReadFull(c, name); err != nil {
		t.Fatal(err)
	}
	t.Logf("the desktop's name: %q", name)
	return binary.BigEndian.Uint16(h), binary.BigEndian.Uint16(h[2:])
}
