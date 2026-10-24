// Package build builds images through a backend and keeps their records:
// base, and the user's layers on it. An image is built once per key and
// reused while its key matches; rebuilding one bumps its rebuild counter,
// which changes its key.
package build

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// Builder builds images on one backend.
type Builder struct {
	Dirs    paths.Dirs
	Root    string // devvm's own files: base.yaml, layers/ (paths.Root)
	Backend backend.Backend
}

// Inputs returns what the backend's image keys hash, read from disk.
func (b *Builder) Inputs() image.Inputs {
	return image.Inputs{
		Backend:  b.Backend.Name(),
		Root:     os.DirFS(b.Root),
		Layers:   os.DirFS(b.Dirs.Layers()),
		Rebuilds: os.DirFS(b.Dirs.Rebuilds()),
	}
}

// Normalize returns an image's canonical name: its layers bottom-up,
// without base, which every image is built on; base alone when there are
// none.
func Normalize(name string) string {
	var parts []string
	for p := range strings.SplitSeq(name, "+") {
		if p != image.Base {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return image.Base
	}
	return strings.Join(parts, "+")
}

// Layers returns base and the user's layers, sorted: the scripts in the
// layers folder with names a layer can have.
func (b *Builder) Layers() []string {
	names := []string{image.Base}
	entries, _ := os.ReadDir(b.Dirs.Layers()) // no folder: no layers
	for _, e := range entries {
		n, ok := strings.CutSuffix(e.Name(), ".sh")
		if ok && image.LayerName(n) && b.isLayer(n) {
			names = append(names, n)
		}
	}
	slices.Sort(names[1:])
	return names
}

func (b *Builder) layerScript(layer string) string {
	return filepath.Join(b.Dirs.Layers(), layer+".sh")
}

func (b *Builder) isLayer(layer string) bool {
	fi, err := os.Stat(b.layerScript(layer))
	return err == nil && fi.Mode().IsRegular()
}

// CheckLayers refuses an image name with a layer that isn't base or one
// of the user's. Names become file names: nothing else gets past it.
func (b *Builder) CheckLayers(name string) error {
	for p := range strings.SplitSeq(name, "+") {
		if p != image.Base && (!image.LayerName(p) || !b.isLayer(p)) {
			return msg.UnknownLayer(p, strings.Join(b.Layers(), " "))
		}
	}
	return nil
}

// Build builds an image, and the images under it, unless they're built.
func (b *Builder) Build(ctx context.Context, name string) error {
	name = Normalize(name)
	if err := b.CheckLayers(name); err != nil {
		return err
	}
	return b.build(ctx, name)
}

func (b *Builder) build(ctx context.Context, name string) error {
	chain, err := b.Inputs().Chain(name)
	if err != nil {
		return err
	}
	key := chain.Key()
	spec := backend.ImageSpec{Image: name, Key: key, Net: backend.NetInternet}
	if err := backend.Require(b.Backend, name, spec.Features()...); err != nil {
		return err
	}
	if done, err := b.built(ctx, name, key); err != nil || done {
		return err
	}
	libc := ""
	script := backend.Script{Name: "base.sh", Path: filepath.Join(b.Root, "layers", "base.sh")}
	if parent := image.Parent(name); parent != "" {
		if err := b.build(ctx, parent); err != nil {
			return err
		}
		spec.Parent = chain[len(chain)-2].Key
		rec, err := state.ReadImage(b.Dirs, spec.Parent)
		if err != nil {
			return err
		}
		libc = rec.Libc
		layer := name[strings.LastIndex(name, "+")+1:]
		script = backend.Script{Name: layer + ".sh", Path: b.layerScript(layer)}
	}
	spec.Scripts = []backend.Script{script}

	ui.Info(msg.BuildingImage(name, key))
	if name == image.Base {
		ui.Say(msg.BaseBuildTime())
	}
	ui.LogNote()
	out, err := b.Backend.BuildImage(ctx, spec)
	if err != nil {
		return err
	}
	// The record is written last: an image without one didn't finish.
	rec := state.Image{Name: name, Chain: chain.String(), Libc: libc}
	if name == image.Base {
		rec.Digest, rec.Libc = out.Origin, out.Libc
	}
	if rec.Libc == "" {
		rec.Libc = "gnu"
	}
	if err := state.WriteImage(b.Dirs, key, rec); err != nil {
		return err
	}
	ui.Info(msg.ImageReady(name))
	return nil
}

// built reports whether the image with key is built: the backend has it,
// and its record. One without a record is left over from a build that
// didn't finish, and is deleted.
func (b *Builder) built(ctx context.Context, name, key string) (bool, error) {
	keys, err := b.Backend.Images(ctx)
	if err != nil || !slices.Contains(keys, key) {
		return false, err
	}
	_, err = state.ReadImage(b.Dirs, key)
	if err == nil {
		return true, nil
	}
	if _, corrupt := errors.AsType[*state.CorruptError](err); !corrupt && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	ui.Info(msg.ImageLeftover(name, key))
	return false, b.deleteImage(ctx, key)
}

func (b *Builder) deleteImage(ctx context.Context, key string) error {
	if err := b.Backend.DeleteImage(ctx, key); err != nil {
		return err
	}
	return state.RemoveImage(b.Dirs, key)
}

// Image is a built image as List finds it.
type Image struct {
	Key   string
	Name  string // "" for one without a record: a build that didn't finish
	From  string
	Chain image.Chain
	// Current is whether its key is still what its inputs hash to. One
	// that isn't is stale: nothing new is built on it.
	Current bool
}

// List returns the backend's images, with their records.
func (b *Builder) List(ctx context.Context) ([]Image, error) {
	keys, err := b.Backend.Images(ctx)
	if err != nil {
		return nil, err
	}
	in := b.Inputs()
	var out []Image
	for _, key := range keys {
		rec, err := state.ReadImage(b.Dirs, key)
		if _, corrupt := errors.AsType[*state.CorruptError](err); corrupt || errors.Is(err, fs.ErrNotExist) {
			out = append(out, Image{Key: key})
			continue
		} else if err != nil {
			return nil, err
		}
		img := Image{Key: key, Name: rec.Name, From: rec.From, Chain: image.ParseChain(rec.Chain)}
		in.From, in.FromDigest = rec.From, ""
		if rec.From != "" {
			if in.FromDigest, err = state.FromDigest(b.Dirs, rec.From); err != nil {
				return nil, err
			}
		}
		if k, err := in.Key(rec.Name); err == nil && k == key {
			img.Current = true
		}
		out = append(out, img)
	}
	return out, nil
}

// Rebuild builds an image again, from its parent as it is: its rebuild
// counter changes, so it gets a new key. Then it deletes the stale images
// (the old one and those built on it among them) that no VM uses. When
// images are built on it, it asks first in a terminal.
func (b *Builder) Rebuild(ctx context.Context, name string) error {
	name = Normalize(name)
	if err := b.CheckLayers(name); err != nil {
		return err
	}
	old, err := b.Inputs().Key(name)
	if err != nil {
		return err
	}
	imgs, err := b.List(ctx)
	if err != nil {
		return err
	}
	var above []string
	for _, img := range imgs {
		if img.Current && img.Name != name && slices.Contains(img.Chain, image.Link{Name: name, Key: old}) {
			above = append(above, img.Name)
		}
	}
	if len(above) > 0 {
		ui.Say(msg.RebuildReplaces(strings.Join(above, " "), name))
		if ui.StdinIsTerminal() && !ui.Confirm(msg.Continue()) {
			ui.Info(msg.NothingChanged())
			return nil
		}
	}
	if err := state.BumpRebuild(b.Dirs, name, ""); err != nil {
		return err
	}
	if err := b.build(ctx, name); err != nil {
		return err
	}
	if imgs, err = b.List(ctx); err != nil {
		return err
	}
	for _, img := range imgs {
		if img.Current {
			continue
		}
		if err := b.deleteImage(ctx, img.Key); err != nil {
			// One a VM uses stays, saying so.
			if m, ok := errors.AsType[msg.M](err); ok {
				ui.Warn(m)
				continue
			}
			return err
		}
		if img.Name == "" {
			ui.Info(msg.DeletedUnfinishedBuild(img.Key))
		} else {
			ui.Info(msg.DeletedOldImage(img.Name, img.Key))
		}
	}
	return nil
}

// WarnIfOutdated warns when an image a VM was made from has changed since:
// chain is the VM's, as its state records it. It never changes the VM.
func (b *Builder) WarnIfOutdated(chain string) error {
	changed, err := b.Inputs().Outdated(image.ParseChain(chain))
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		ui.Warn(msg.VMOutdated(strings.Join(changed, ", ")))
	}
	return nil
}
