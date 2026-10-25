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
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
)

// Builder builds images on one backend.
type Builder struct {
	Dirs    paths.Dirs
	Root    string // devvm's own files: base.yaml, layers/, features/, oci/ (paths.Root)
	Backend backend.Backend
	Tools   *tools.Tools // the shared tools image names hold
	// From is the container image base starts from (--from); "" for the
	// backend's own Ubuntu. Its images are apart from other bases'.
	From string
	// Lookup looks up From's digest when building; nil for the internet's
	// (from.NewLookup).
	Lookup *from.Lookup

	looked bool // From's digest was looked up in this run
}

// Inputs returns what the backend's image keys hash, read from disk:
// From's digest as last recorded.
func (b *Builder) Inputs() (image.Inputs, error) {
	digest, err := from.Recorded(b.Dirs, b.From)
	if err != nil {
		return image.Inputs{}, err
	}
	features, err := b.chosenFeatures()
	if err != nil {
		return image.Inputs{}, err
	}
	return image.Inputs{
		Backend:      b.Backend.Name(),
		Root:         os.DirFS(b.Root),
		Layers:       os.DirFS(b.Dirs.Layers()),
		Features:     os.DirFS(b.Dirs.Features()),
		BaseFeatures: features,
		Rebuilds:     os.DirFS(b.Dirs.Rebuilds()),
		From:         b.From,
		FromDigest:   digest,
	}, nil
}

// Key returns an image's key as it is now.
func (b *Builder) Key(name string) (string, error) {
	in, err := b.Inputs()
	if err != nil {
		return "", err
	}
	return in.Key(name)
}

// Chain returns an image's chain as it is now.
func (b *Builder) Chain(name string) (image.Chain, error) {
	in, err := b.Inputs()
	if err != nil {
		return nil, err
	}
	return in.Chain(name)
}

// lookUp looks up From's digest, once a run, before any key that builds
// is worked out: every key holds base's.
func (b *Builder) lookUp(ctx context.Context) error {
	if b.From == "" || b.looked {
		return nil
	}
	l := b.Lookup
	if l == nil {
		l = from.NewLookup(b.Dirs)
	}
	if err := l.Refresh(ctx, b.From); err != nil {
		return err
	}
	b.looked = true
	return nil
}

// Layers returns base and the user's layers, sorted: the scripts in the
// layers folder with names a layer can have (not a shared tool's).
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

// LayerScript is the script of one of the user's layers.
func (b *Builder) LayerScript(layer string) string {
	return filepath.Join(b.Dirs.Layers(), layer+".sh")
}

func (b *Builder) isLayer(layer string) bool {
	fi, err := os.Stat(b.LayerScript(layer))
	return err == nil && fi.Mode().IsRegular()
}

// Environments returns what an image can be made of besides the agents:
// base, the toolchains and the user's layers.
func (b *Builder) Environments() []string {
	layers := b.Layers()
	return slices.Concat(layers[:1], image.Toolchains, layers[1:])
}

// CheckLayers refuses an image name with a part that isn't base, a shared
// tool or one of the user's layers. Names become file names: nothing else
// gets past it.
func (b *Builder) CheckLayers(name string) error {
	for p := range strings.SplitSeq(name, "+") {
		if p != image.Base && !image.IsTool(p) && (!image.LayerName(p) || !b.isLayer(p)) {
			return msg.UnknownLayer(strings.Join(image.Agents, " "), strings.Join(b.Environments(), " "), p)
		}
	}
	return nil
}

// BaseLibc is the C library of base as it is now: its record's, gnu when
// it isn't built. The shared tools are built for it.
func (b *Builder) BaseLibc() (string, error) {
	key, err := b.Key(image.Base)
	if err != nil {
		return "", err
	}
	rec, err := state.ReadImage(b.Dirs, key)
	if _, corrupt := errors.AsType[*state.CorruptError](err); corrupt || errors.Is(err, fs.ErrNotExist) {
		return "gnu", nil
	} else if err != nil {
		return "", err
	}
	if rec.Libc == "" {
		return "gnu", nil
	}
	return rec.Libc, nil
}

// Build builds an image name's image, and the images under it, unless
// they're built; then the shared tool images it names that the share
// lacks.
func (b *Builder) Build(ctx context.Context, name string) error {
	name = image.Normalize(name)
	if err := b.CheckLayers(name); err != nil {
		return err
	}
	// Every key holds base's, so its features are chosen first.
	if err := b.chooseFeatures(); err != nil {
		return err
	}
	if err := b.build(ctx, image.WithoutTools(name)); err != nil {
		return err
	}
	groups := image.ToolsOf(name)
	if len(groups) == 0 {
		return nil
	}
	libc, err := b.BaseLibc()
	if err != nil {
		return err
	}
	files, err := b.Tools.Files(ctx, groups, libc)
	if err != nil {
		return err
	}
	base, err := b.Key(image.Base)
	if err != nil {
		return err
	}
	return b.Tools.Build(ctx, b.Backend, base, libc, files)
}

func (b *Builder) build(ctx context.Context, name string) error {
	if err := b.lookUp(ctx); err != nil {
		return err
	}
	chain, err := b.Chain(name)
	if err != nil {
		return err
	}
	key := chain.Key()
	digest, err := from.Recorded(b.Dirs, b.From)
	if err != nil {
		return err
	}
	spec := backend.ImageSpec{Image: name, Key: key, Net: backend.NetInternet}
	if name == image.Base && b.From != "" {
		spec.From = &backend.From{Ref: b.From, Digest: digest}
	}
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
		script = backend.Script{Name: layer + ".sh", Path: b.LayerScript(layer)}
	}
	spec.Scripts = []backend.Script{script}
	if name == image.Base {
		// Then its features, in the order chosen.
		features, err := b.featureScripts()
		if err != nil {
			return err
		}
		for _, p := range features {
			spec.Scripts = append(spec.Scripts, backend.Script{Name: filepath.Base(p), Path: p})
		}
	}

	switch {
	case b.From != "":
		ui.Info(msg.BuildingImageFrom(b.From, name, key))
	case name == image.Base:
		ui.Info(msg.BuildingImage(name, key))
		ui.Say(msg.BaseBuildTime())
	default:
		ui.Info(msg.BuildingImage(name, key))
	}
	ui.LogNote()
	out, err := b.Backend.BuildImage(ctx, spec)
	if err != nil {
		return err
	}
	// The record is written last: an image without one didn't finish.
	// Every image on a --from base records it and its digest.
	rec := state.Image{Name: name, Chain: chain.String(), From: b.From, Digest: digest, Libc: libc}
	if name == image.Base {
		rec.Libc = out.Libc
		if b.From == "" {
			rec.Digest = out.Origin
		}
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
	Key    string
	Name   string // "" for one without a record: a build that didn't finish
	From   string
	Digest string // what its base started from, as its record has it
	Chain  image.Chain
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
	in, err := b.Inputs()
	if err != nil {
		return nil, err
	}
	var out []Image
	for _, key := range keys {
		rec, err := state.ReadImage(b.Dirs, key)
		if _, corrupt := errors.AsType[*state.CorruptError](err); corrupt || errors.Is(err, fs.ErrNotExist) {
			out = append(out, Image{Key: key})
			continue
		} else if err != nil {
			return nil, err
		}
		img := Image{Key: key, Name: rec.Name, From: rec.From, Digest: rec.Digest, Chain: image.ParseChain(rec.Chain)}
		// Each against its own base's inputs, as last recorded.
		in.From = rec.From
		if in.FromDigest, err = from.Recorded(b.Dirs, rec.From); err != nil {
			return nil, err
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
	name = image.Normalize(name)
	if err := b.CheckLayers(name); err != nil {
		return err
	}
	// The tools lock versions the shared tools: they aren't rebuilt.
	if t := image.ToolsOf(name); len(t) > 0 {
		return msg.ToolsNotImages(strings.Join(t, " "))
	}
	if err := b.chooseFeatures(); err != nil {
		return err
	}
	old, err := b.Key(name)
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
	if err := state.BumpRebuild(b.Dirs, name, b.From); err != nil {
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
	in, err := b.Inputs()
	if err != nil {
		return err
	}
	changed, err := in.Outdated(image.ParseChain(chain))
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		ui.Warn(msg.VMOutdated(strings.Join(changed, ", ")))
	}
	return nil
}
