package image

// Recipe is what an image is made of, built once (internal/build): its
// key hashes it (Input), and its backend runs it as it is
// (backend.ImageSpec).
type Recipe struct {
	// Steps run in order in the image's VM: base.sh and then base's
	// features, or a layer's script.
	Steps []Step
}

// Step is one step of a recipe: a script fed on its standard input to
// bash with Args, as the VM's user, who has sudo, or as root.
type Step struct {
	Name string // for status lines and errors: base.sh
	Data []byte
	Args []string
	Root bool
}
