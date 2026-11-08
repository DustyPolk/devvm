// Package clipboard is images on this machine's clipboard for project
// VMs: each VM's choice (off, or images), and the server that answers a
// VM's requests for the image, re-encoded so nothing but its pixels
// crosses.
package clipboard

import (
	"fmt"
	"slices"

	"github.com/devvmhq/devvm/internal/state"
)

// Port is the port of a VM's clipboard listener, on its address in the
// links' range: 1024 or above, since the VM's sshd binds a remote forward
// as the VM's user, who can't bind below it.
const Port = 7737

// Section is the name of the clipboard section in a VM's state.
const Section = "clipboard"

// InVM is a VM's clipboard section: its choice.
type InVM struct {
	Mode string `json:"mode,omitempty"` // Images or Off
}

// A VM's choices.
const (
	Off    = "off"    // nothing of the clipboard
	Images = "images" // the image on this machine's clipboard, read on request
)

// Modes are the choices, as devvm clipboard takes them.
var Modes = []string{Images, Off}

// Of returns a VM's clipboard section.
func Of(st *state.VM) (InVM, error) { return state.Section[InVM](st, Section) }

// SetMode sets a VM's choice, one of Modes, in st alone: inside st.Update.
func SetMode(st *state.VM, mode string) error {
	if !slices.Contains(Modes, mode) {
		return fmt.Errorf("unknown clipboard mode %q", mode)
	}
	return state.SetSection(st, Section, InVM{Mode: mode})
}

// Mode is a VM's choice: off unless its state says images.
func Mode(st *state.VM) string {
	c, _ := Of(st)
	if c.Mode == Images {
		return Images
	}
	return Off
}
