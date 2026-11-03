// Package build builds images through a backend and keeps their records:
// base, and layers on it (devvm's built-in ones and the user's). An image is built once per key and
// reused while its key matches; rebuilding one bumps its rebuild counter,
// which changes its key.
//
// Env is what images are built with, and what the groups share: keys,
// layers, the features there are, base's C library. Builds builds and
// rebuilds images; Images lists and deletes them, with their records;
// Choice chooses base's features.
package build

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/trust"
)

// Env is what images on one backend are built with, shared by Builds,
// Images and Choice; its methods are what they share: image keys and
// chains, the user's layers, the features there are and those chosen,
// base's C library and deleting an image.
type Env struct {
	Dirs    paths.Dirs
	Assets  fs.FS // devvm's own files (assets.FS): base.yaml, layers/, features/, oci/
	Backend backend.Backend
	Review  backend.Review // what Backend's features are checked against
	Tools   *tools.Env     // the shared tools image names hold
	// From is the container image base starts from (--from); "" for the
	// backend's own Ubuntu. Its images are apart from other bases'.
	From string
	// Version is devvm's version, which each image's record keeps.
	Version string
	// Lookup looks up From's digest when building; nil for the internet's
	// (from.NewLookup).
	Lookup *from.Lookup
	// What it prints and asks, its caller's: Confirm is [Y/n]; Pick asks
	// which features base gets, each as chosen so far; Terminal reports
	// whether it can ask; LogNote says where the run log is, once a run.
	Info, Warn, Say func(msg.M)
	Confirm         func(msg.M) bool
	Pick            func([]Feature) []Feature
	Terminal        func() bool
	LogNote         func()
	// Force is a rebuild's --force: it replaces the images built on the
	// one it rebuilds without asking, which it needs without a terminal.
	Force bool
	// FreeSpace returns the free space on the volume holding a path
	// (platform.Host's), checked before each build; nil checks none.
	FreeSpace func(path string) (uint64, error)
	// WaitNote is how long a build waits for another devvm building the
	// same image before it says so; 0 for buildWaitNote.
	WaitNote time.Duration

	looked bool // From's digest was looked up in this run
}

// Builds builds images, and builds them again.
type Builds struct{ *Env }

// Images is the backend's images and their records: listing and deleting
// them.
type Images struct{ *Env }

// Choice is base's features: asking which it gets, or setting them.
type Choice struct{ *Env }

// Inputs returns what the backend's image keys hash, read from disk:
// From's digest as last recorded.
func (b *Env) Inputs() (image.Inputs, error) {
	digest, err := from.Recorded(b.Dirs, b.From)
	if err != nil {
		return image.Inputs{}, err
	}
	features, err := b.chosenFeatures()
	if err != nil {
		return image.Inputs{}, err
	}
	var own map[string][]byte
	if kf, ok := b.Backend.(backend.KeyFiles); ok {
		own = kf.KeyFiles()
	}
	rebuilds, err := state.ReadRebuilds(b.Dirs)
	if err != nil {
		return image.Inputs{}, err
	}
	return image.Inputs{
		Backend:      b.Backend.Name(),
		BackendFiles: own,
		Assets:       b.Assets,
		Layers:       os.DirFS(b.Dirs.Layers()),
		Features:     os.DirFS(b.Dirs.Features()),
		BaseFeatures: features,
		Rebuilds:     rebuilds,
		From:         b.From,
		FromDigest:   digest,
	}, nil
}

// Key returns an image's key as it is now.
func (b *Env) Key(name string) (string, error) {
	in, err := b.Inputs()
	if err != nil {
		return "", err
	}
	return in.Key(name)
}

// Chain returns an image's chain as it is now.
func (b *Env) Chain(name string) (image.Chain, error) {
	in, err := b.Inputs()
	if err != nil {
		return nil, err
	}
	return in.Chain(name)
}

// lookUp looks up From's digest, once a run, before any key that builds
// is worked out: every key holds base's.
func (b Builds) lookUp(ctx context.Context) error {
	if b.From == "" || b.looked {
		return nil
	}
	l := b.Lookup
	if l == nil {
		l = from.NewLookup(b.Dirs, b.Warn)
	}
	if err := l.Refresh(ctx, b.From); err != nil {
		return err
	}
	b.looked = true
	return nil
}

// Layers returns base, then the built-in layers and the user's, sorted:
// the scripts in the layers folder with names a layer can have (not a
// shared tool's or a built-in layer's).
func (b *Env) Layers() []string {
	names := slices.Concat([]string{image.Base}, image.Builtins)
	entries, _ := os.ReadDir(b.Dirs.Layers()) // no folder: no layers
	for _, e := range entries {
		n, ok := strings.CutSuffix(e.Name(), ".sh")
		if ok && image.LayerName(n) && !image.IsBuiltin(n) && b.isLayer(n) {
			names = append(names, n)
		}
	}
	slices.Sort(names[1:])
	return names
}

// LayerData is a layer's script: devvm's own for a built-in layer, else
// the user's.
func (b *Env) LayerData(layer string) ([]byte, error) {
	if image.IsBuiltin(layer) {
		return fs.ReadFile(b.Assets, "layers/"+layer+".sh")
	}
	return os.ReadFile(b.userLayer(layer))
}

func (b *Env) userLayer(layer string) string {
	return filepath.Join(b.Dirs.Layers(), layer+".sh")
}

func (b *Env) isLayer(layer string) bool {
	if image.IsBuiltin(layer) {
		return true
	}
	fi, err := os.Stat(b.userLayer(layer))
	return err == nil && fi.Mode().IsRegular()
}

// CheckLayers refuses an image name with a part that isn't base, a
// built-in layer or one of the user's layers: a shared tool's name says
// shared tools go in a VM, not an image. Names become file names: nothing
// else gets past it.
func (b *Env) CheckLayers(name string) error {
	for p := range strings.SplitSeq(name, "+") {
		switch {
		case image.IsTool(p):
			return msg.ToolNotLayer(p)
		case p != image.Base && (!image.LayerName(p) || !b.isLayer(p)):
			return msg.UnknownLayer(p, strings.Join(b.Layers(), " "))
		}
	}
	return nil
}

// BaseLibc is the C library of base as it is now: its record's, gnu when
// it isn't built. The shared tools are built for it.
func (b *Env) BaseLibc() (string, error) {
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

// ToolTarget is what the shared tools of a new VM on this backend are
// built for: base's C library as it is now (BaseLibc), and the backend's
// format.
func (b *Env) ToolTarget() (tools.Target, error) {
	libc, err := b.BaseLibc()
	if err != nil {
		return tools.Target{}, err
	}
	return tools.Target{Libc: libc, Format: b.Backend.ToolFormat()}, nil
}

// Build builds an image, and the images under it, unless they're built.
func (b Builds) Build(ctx context.Context, name string) error {
	name = image.Normalize(name)
	if err := b.CheckLayers(name); err != nil {
		return err
	}
	// Every key holds base's, so its features are chosen first.
	if err := Choice(b).chooseFeatures(); err != nil {
		return err
	}
	return b.build(ctx, name)
}

func (b Builds) build(ctx context.Context, name string) error {
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
	// One devvm at a time builds it: another waits, then finds it built,
	// or left over from a build that didn't finish.
	unlock, err := b.lockBuild(ctx, name, key)
	if err != nil {
		return err
	}
	defer unlock()
	// What building it needs, only when it's built: an image the backend
	// has, with its record, needs none of it.
	done, leftover, err := b.built(ctx, key)
	if err != nil || done {
		return err
	}
	if err := b.Review.Require(b.Backend, name, spec.Features()...); err != nil {
		return err
	}
	if leftover {
		b.Info(msg.ImageLeftover(name, key))
		if err := b.deleteLocked(ctx, key, false); err != nil {
			return err
		}
	}
	libc := ""
	data, err := fs.ReadFile(b.Assets, "layers/base.sh")
	if err != nil {
		return err
	}
	script := backend.Script{Name: "base.sh", Data: data}
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
		data, err := b.LayerData(layer)
		if err != nil {
			return err
		}
		script = backend.Script{Name: layer + ".sh", Data: data}
	}
	spec.Scripts = []backend.Script{script}
	if name == image.Base {
		// Then its features, in the order chosen.
		features, err := b.featureScripts()
		if err != nil {
			return err
		}
		spec.Scripts = append(spec.Scripts, features...)
	}
	// The user's CAs, trusted while it builds: the image keeps none of
	// them, so its key holds none.
	cas, err := trust.Read(b.Dirs.TrustedCAs())
	if err != nil {
		return err
	}
	if spec.Trust, err = cas.Build(b.Assets); err != nil {
		return err
	}
	for i, s := range spec.Scripts {
		spec.Scripts[i] = cas.Script(s)
	}
	// Before anything of this image is made; its parent is built.
	need := func(f backend.Floors) uint64 { return f.Image }
	if name == image.Base {
		need = func(f backend.Floors) uint64 { return f.Base }
	}
	if err := (backend.Space{Free: b.FreeSpace}).Check(b.Backend, name, need); err != nil {
		return err
	}

	switch {
	case b.From != "":
		b.Info(msg.BuildingImageFrom(b.From, name, key))
	case name == image.Base:
		b.Info(msg.BuildingImage(name, key))
		b.Say(msg.BaseBuildTime())
	default:
		b.Info(msg.BuildingImage(name, key))
	}
	b.LogNote()
	out, err := b.Backend.BuildImage(ctx, spec)
	if err != nil {
		return err
	}
	// The record is written last: an image without one didn't finish.
	// Every image on a --from base records it and its digest.
	rec := state.Image{Name: name, Chain: chain.String(), From: b.From, Digest: digest, Libc: libc, Built: time.Now().UTC().Truncate(time.Second), MadeBy: b.Version}
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
	b.sayFitted(out.Fitted)
	b.Info(msg.ImageReady(name))
	return nil
}

// sayFitted says what base got when the backend fitted it to the host
// (backend.Built's Fitted); nothing when it didn't.
func (b Builds) sayFitted(f backend.Size) {
	if f != (backend.Size{}) {
		b.Info(msg.BaseFitted(strconv.Itoa(f.CPUs), strconv.FormatFloat(f.MemoryGiB, 'g', -1, 64)))
	}
}

// buildWaitNote is how long a build waits for another devvm building the
// same image before it says so, when Env's WaitNote is 0.
const buildWaitNote = 3 * time.Second

// lockBuild holds the lock of the image with key, named name, while it's
// built, waiting for another devvm that holds it and saying so. The
// system lets go of it when its devvm ends, however it ends.
func (b Builds) lockBuild(ctx context.Context, name, key string) (unlock func(), err error) {
	path, err := state.ImageLock(b.Dirs, key)
	if err != nil {
		return nil, err
	}
	after := cmp.Or(b.WaitNote, buildWaitNote)
	return platform.LockFileWaiting(ctx, path, after, func() { b.Info(msg.ImageBuildWaiting(name)) })
}

// bumpRebuild changes the rebuild counter of the image named name on this
// backend and base, under the counters' lock: two devvms rebuilding at
// once each keep their change.
func (b Builds) bumpRebuild(ctx context.Context, name string) error {
	if err := os.MkdirAll(b.Dirs.ImageLocks(), 0o700); err != nil {
		return err
	}
	unlock, err := platform.LockFile(ctx, b.Dirs.RebuildsLock())
	if err != nil {
		return err
	}
	defer unlock()
	return state.BumpRebuild(b.Dirs, b.Backend.Name(), b.From, name)
}

// lockFree holds the lock of the image with key, for deleting it, unless
// another devvm is building it (image_building).
func (b *Env) lockFree(key string) (unlock func(), err error) {
	path, err := state.ImageLock(b.Dirs, key)
	if err != nil {
		return nil, err
	}
	unlock, ok, err := platform.TryLockFile(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, msg.ImageBuilding(key)
	}
	return unlock, nil
}

// building reports whether another devvm is building the image with key.
func (b *Env) building(key string) (bool, error) {
	path, err := state.ImageLock(b.Dirs, key)
	if err != nil {
		return false, err
	}
	return platform.LockHeld(path)
}

// built reports whether the image with key is built: the backend has it,
// and its record. One without a record (leftover) is left over from a
// build that didn't finish, for the caller to delete; the caller holds
// its lock, so no other devvm is building it.
func (b Builds) built(ctx context.Context, key string) (done, leftover bool, err error) {
	keys, err := b.Backend.Images(ctx)
	if err != nil || !slices.Contains(keys, key) {
		return false, false, err
	}
	_, err = state.ReadImage(b.Dirs, key)
	if err == nil {
		return true, false, nil
	}
	if _, corrupt := errors.AsType[*state.CorruptError](err); !corrupt && !errors.Is(err, fs.ErrNotExist) {
		return false, false, err
	}
	return false, true, nil
}

// deleteImage deletes the image with key and its record, unless another
// devvm is building it (image_building), then its lock's file, which a
// build of it makes again.
func (b *Env) deleteImage(ctx context.Context, key string) error {
	unlock, err := b.lockFree(key)
	if err != nil {
		return err
	}
	defer unlock()
	return b.deleteLocked(ctx, key, true)
}

// deleteLocked deletes the image with key and its record, its lock held,
// and the lock's file when unlocking follows (a build that deletes a
// leftover keeps it).
func (b *Env) deleteLocked(ctx context.Context, key string, lockFile bool) error {
	if err := b.Backend.DeleteImage(ctx, key); err != nil {
		return err
	}
	if err := state.RemoveImage(b.Dirs, key); err != nil {
		return err
	}
	if !lockFile {
		return nil
	}
	path, err := state.ImageLock(b.Dirs, key)
	if err != nil {
		return err
	}
	// Safe while held: a devvm that then gets the lock on the removed file
	// tries again (platform.LockFile).
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
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
	// Building is whether another devvm is building it: it has no record
	// and its lock is held. One without a record that isn't is unfinished.
	Building bool
}

// List returns the backend's images, with their records.
func (b Images) List(ctx context.Context) ([]Image, error) {
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
			building, err := b.building(key)
			if err != nil {
				return nil, err
			}
			out = append(out, Image{Key: key, Building: building})
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
// images are built on it, it asks first in a terminal, and refuses
// without one (force_needed), unless Force.
func (b Builds) Rebuild(ctx context.Context, name string) error {
	name = image.Normalize(name)
	if err := b.CheckLayers(name); err != nil {
		return err
	}
	if err := Choice(b).chooseFeatures(); err != nil {
		return err
	}
	old, err := b.Key(name)
	if err != nil {
		return err
	}
	imgs, err := Images(b).List(ctx)
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
		b.Say(msg.RebuildReplaces(strings.Join(above, " "), name))
		switch {
		case b.Force:
		case !b.Terminal():
			return msg.ForceNeeded("devvm rebuild")
		case !b.Confirm(msg.Continue()):
			b.Info(msg.NothingChanged())
			return nil
		}
	}
	if err := b.bumpRebuild(ctx, name); err != nil {
		return err
	}
	if err := b.build(ctx, name); err != nil {
		return err
	}
	if imgs, err = Images(b).List(ctx); err != nil {
		return err
	}
	for _, img := range imgs {
		// One another devvm is building isn't left over.
		if img.Current || img.Building {
			continue
		}
		if err := b.deleteImage(ctx, img.Key); err != nil {
			// One a VM uses, or one another devvm has started building
			// since, stays, saying so.
			if m, ok := errors.AsType[msg.M](err); ok {
				b.Warn(m)
				continue
			}
			return err
		}
		if img.Name == "" {
			b.Info(msg.DeletedUnfinishedBuild(img.Key))
		} else {
			b.Info(msg.DeletedOldImage(img.Name, img.Key))
		}
	}
	return nil
}

// WarnIfOutdated warns when an image a VM was made from has changed since:
// chain is the VM's, as its state records it. It never changes the VM.
func (b Builds) WarnIfOutdated(chain string) error {
	in, err := b.Inputs()
	if err != nil {
		return err
	}
	changed, err := in.Outdated(image.ParseChain(chain))
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		b.Warn(msg.VMOutdated(strings.Join(changed, ", ")))
	}
	return nil
}
