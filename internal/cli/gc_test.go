package cli

import (
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// What gc deleted is said in the order it did: an image its backend
// refused with the backend's message, on stderr, and nothing for what it
// had none of; the package cache's sets alone still say it.
func TestShowGCDone(t *testing.T) {
	u, out, errOut := testUI()
	img := func(name, key string) vm.ListedImage {
		return vm.ListedImage{Listed: build.Listed{Image: build.Image{Key: key, Name: name}}}
	}
	showGCDone(u, vm.GCDone{
		Forgot: []string{"devvm-a-0123456789abcd"}, LinkSockets: 2,
		Images: []vm.DeletedImage{
			{ListedImage: img("base", "0123456789ab")},
			{ListedImage: img("", "ba9876543210")},
			{ListedImage: img("foo", "aaaaaaaaaaaa"), Refused: true, Why: msg.ImageInUse("aaaaaaaaaaaa", "devvm-p-0123456789abcd")},
		},
		PackagesDeleted: true, History: 1, ToolsNoVM: 3, NixFiles: 4, NixFreed: 2048,
	})
	want := "==> forgot_orphan vm=devvm-a-0123456789abcd\n" +
		"==> deleted_link_sockets count=2\n" +
		"==> deleted_image image=base key=0123456789ab\n" +
		"==> deleted_unfinished_build key=ba9876543210\n" +
		"==> deleted_packages count=0\n" +
		"==> deleted_history count=1\n" +
		"==> deleted_tools_no_vm count=3\n" +
		"==> deleted_nix_cache count=4 size=2.0K\n"
	if out.String() != want {
		t.Errorf("stdout\n%s\nwant\n%s", out, want)
	}
	if !strings.HasPrefix(errOut.String(), "devvm: image_in_use ") {
		t.Errorf("stderr %q, want image_in_use", errOut)
	}
}

// A destructive command asks before it deletes only in a terminal, never
// with --force, and without a terminal refuses unless --force.
func TestConfirmed(t *testing.T) {
	u, _, _ := testUI()
	asked := false
	ask := func() bool { asked = true; return true }
	if ok, err := confirmed(u, true, "devvm gc", ask); !ok || err != nil || asked {
		t.Errorf("--force: %v, %v, asked %v", ok, err, asked)
	}
	ok, err := confirmed(u, false, "devvm gc", ask)
	if ok || msg.NamesOf(err) != "force_needed command=devvm gc" || asked {
		t.Errorf("no terminal: %v, %v, asked %v", ok, err, asked)
	}
}
