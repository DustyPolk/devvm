package desktop

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// The viewer's messages (RFC 6143 7.5, and the extensions TigerVNC's
// server offers).
const (
	msgSetPixelFormat    = 0
	msgSetEncodings      = 2
	msgUpdateRequest     = 3
	msgKeyEvent          = 4
	msgPointerEvent      = 5
	msgClientCutText     = 6
	msgContinuousUpdates = 150
	msgFence             = 248
	msgXvp               = 250
	msgSetDesktopSize    = 251
	msgGII               = 253
	msgQEMU              = 255
	qemuExtendedKeyEvent = 0
	qemuAudio            = 1
	qemuAudioSetFormat   = 2
	giiBigEndian         = 0x80
	fenceMaxPayload      = 64
)

// filter copies the viewer's messages (after its security result: its
// ClientInit, then messages) from viewer to vm, leaving out its clipboard
// (ClientCutText, extended clipboard ones included). A message it can't
// read the length of ends the connection: what it doesn't know never
// reaches the VM.
func filter(vm io.Writer, viewer io.Reader, logf func(string, ...any)) error {
	in := bufio.NewReader(viewer)
	// ClientInit: the shared flag.
	if _, err := io.CopyN(vm, in, 1); err != nil {
		return err
	}
	logged := false
	for {
		t, err := in.ReadByte()
		if err != nil {
			return err
		}
		if t == msgClientCutText {
			if err := skipCutText(in); err != nil {
				return err
			}
			if !logged {
				logf("the viewer's clipboard kept from the VM")
				logged = true
			}
			continue
		}
		msg, err := message(in, t)
		if err != nil {
			return err
		}
		if _, err := vm.Write(msg); err != nil {
			return err
		}
	}
}

// skipCutText reads past a ClientCutText message after its type: 3
// padding bytes, a signed length (negative for the extended clipboard's),
// then that many bytes.
func skipCutText(in *bufio.Reader) error {
	h, err := readN(in, 7)
	if err != nil {
		return err
	}
	n := int64(int32(binary.BigEndian.Uint32(h[3:]))) //nolint:gosec // the length is signed, as the protocol has it
	if n < 0 {
		n = -n
	}
	_, err = io.CopyN(io.Discard, in, n)
	return err
}

// message reads the rest of a message of type t and returns it whole, its
// type first.
func message(in *bufio.Reader, t byte) ([]byte, error) {
	switch t {
	case msgSetPixelFormat:
		return fixed(in, t, 19)
	case msgUpdateRequest, msgContinuousUpdates:
		return fixed(in, t, 9)
	case msgKeyEvent:
		return fixed(in, t, 7)
	case msgPointerEvent:
		return fixed(in, t, 5)
	case msgXvp:
		return fixed(in, t, 3)
	case msgSetEncodings:
		// A padding byte, then a count of 4-byte encodings.
		return counted(in, t, 3, func(h []byte) int { return 4 * int(binary.BigEndian.Uint16(h[1:])) })
	case msgFence:
		// 3 padding bytes, 4 of flags, then a length of at most 64.
		m, err := counted(in, t, 8, func(h []byte) int { return int(h[7]) })
		if err == nil && int(m[8]) > fenceMaxPayload {
			return nil, fmt.Errorf("a fence of %d bytes", m[8])
		}
		return m, err
	case msgSetDesktopSize:
		// A padding byte, width and height, a count of 16-byte screens and
		// a padding byte.
		return counted(in, t, 7, func(h []byte) int { return 16 * int(h[5]) })
	case msgGII:
		// Its byte order and subtype, then a length in that order.
		return counted(in, t, 3, func(h []byte) int {
			if h[0]&giiBigEndian != 0 {
				return int(binary.BigEndian.Uint16(h[1:]))
			}
			return int(binary.LittleEndian.Uint16(h[1:]))
		})
	case msgQEMU:
		return qemu(in, t)
	}
	return nil, fmt.Errorf("a message of type %d, which the relay doesn't know", t)
}

// qemu reads the rest of a QEMU message: an extended key event, or an
// audio one.
func qemu(in *bufio.Reader, t byte) ([]byte, error) {
	sub, err := in.ReadByte()
	if err != nil {
		return nil, err
	}
	switch sub {
	case qemuExtendedKeyEvent:
		rest, err := readN(in, 10)
		return append([]byte{t, sub}, rest...), err
	case qemuAudio:
		op, err := readN(in, 2)
		if err != nil {
			return nil, err
		}
		m := append([]byte{t, sub}, op...)
		if binary.BigEndian.Uint16(op) != qemuAudioSetFormat {
			return m, nil
		}
		rest, err := readN(in, 6)
		return append(m, rest...), err
	}
	return nil, fmt.Errorf("a QEMU message of subtype %d, which the relay doesn't know", sub)
}

// fixed reads a message of n bytes after its type.
func fixed(in *bufio.Reader, t byte, n int) ([]byte, error) {
	rest, err := readN(in, n)
	return append([]byte{t}, rest...), err
}

// counted reads a message whose header (n bytes after its type) says how
// many bytes follow (more).
func counted(in *bufio.Reader, t byte, n int, more func(h []byte) int) ([]byte, error) {
	h, err := readN(in, n)
	if err != nil {
		return nil, err
	}
	body, err := readN(in, more(h))
	return append(append([]byte{t}, h...), body...), err
}

func readN(in io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(in, b)
	return b, err
}
