package build

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/state"
)

// Listing: by name, unfinished builds last; base on Ubuntu with its
// release date and digest, which the images on it show too.
func TestListing(t *testing.T) {
	b, f := testEnv(t)
	ctx := t.Context()
	f.Built.Origin = "https://cloud-images.ubuntu.com/releases/resolute/release-20261001/u.img sha256:0123456789abcdef"
	if err := (Builds{b}).Build(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	foo, err := b.Key("foo")
	if err != nil {
		t.Fatal(err)
	}
	f.ImageKeys = append(f.ImageKeys, "000000000000") // no record: unfinished
	f.ImageSizes = map[string]int64{foo: 1 << 30}
	write(t, filepath.Join(b.Dirs.Layers(), "foo.sh"), "#!/bin/bash\necho edited\n")
	ls, err := (Images{b}).Listing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ name, state, origin, id string }
	var got []row
	for _, l := range ls {
		got = append(got, row{l.Name, l.State, l.Origin, l.DigestID})
		if l.Sized != (l.Key == foo || l.Name == "base" || l.Name == "") || l.Key == foo && l.Size != 1<<30 {
			t.Errorf("%s: size %d, sized %v", l.Key, l.Size, l.Sized)
		}
	}
	want := []row{
		{"base", StateCurrent, "ubuntu 26.04 (20261001)", "0123456789ab"},
		{"foo", StateStale, "ubuntu 26.04 (20261001)", "0123456789ab"},
		{"", StateUnfinished, "", ""},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Listing:\n%v\nwant\n%v", got, want)
	}
}

// A --from image lists its image and digest.
func TestListingFrom(t *testing.T) {
	b, f := testEnv(t)
	f.ImageKeys = []string{"0123456789ab"}
	rec := state.Image{Name: "base", Chain: "base=0123456789ab", From: "node:24", Digest: "node@sha256:fedcba9876543210", Libc: "gnu"}
	if err := state.WriteImage(b.Dirs, "0123456789ab", rec); err != nil {
		t.Fatal(err)
	}
	ls, err := (Images{b}).Listing(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 1 || ls[0].Origin != "node:24" || ls[0].DigestID != "fedcba987654" || ls[0].State != StateStale {
		t.Errorf("Listing: %+v", ls)
	}
}

// Delete: by name or by key, after every one is found; never shared tools
// or what isn't built.
func TestDelete(t *testing.T) {
	b, f := testEnv(t)
	ctx := t.Context()
	if err := (Builds{b}).Build(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	base, err := b.Key("base")
	if err != nil {
		t.Fatal(err)
	}
	foo, err := b.Key("foo")
	if err != nil {
		t.Fatal(err)
	}
	f.ImageKeys = append(f.ImageKeys, "000000000000")
	for _, refs := range [][]string{{"node"}, {"foo+codex"}, {"bar"}, {"nope"}, {"fedcba987654"}, {"000000000000", "bar"}} {
		if err := (Images{b}).Delete(ctx, refs); err == nil {
			t.Errorf("Delete %q: no error", refs)
		}
	}
	if msg.IDOf((Images{b}).Delete(ctx, []string{"node"})) != "tool_not_layer" || msg.IDOf((Images{b}).Delete(ctx, []string{"bar"})) != "no_image_named" ||
		msg.IDOf((Images{b}).Delete(ctx, []string{"nope"})) != "unknown_layer" {
		t.Error("Delete: not the messages expected")
	}
	if len(f.ImageKeys) != 3 {
		t.Fatalf("a failed Delete deleted: %v left", f.ImageKeys)
	}
	if err := (Images{b}).Delete(ctx, []string{"000000000000", "base+foo", foo}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.ImageKeys, []string{base}) {
		t.Errorf("left %v, want base's %s", f.ImageKeys, base)
	}
	if _, err := state.ReadImage(b.Dirs, foo); err == nil {
		t.Error("foo's record stays")
	}
	for _, s := range []string{"build.sh", "bootify.sh", "kernel.sh"} {
		write(t, filepath.Join(assetsDir(b), "oci", s), "#!/bin/bash\n")
	}
	b.From = "node:24"
	if err := (Images{b}).Delete(ctx, []string{"base"}); msg.IDOf(err) != "no_image_named_from" {
		t.Errorf("another base's: %v, want no_image_named_from", err)
	}
}
