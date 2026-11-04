package build

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/state"
)

// inputs is what images' recipes and keys are made of, read once: what
// base starts from, its features as chosen and the rebuild counters.
type inputs struct {
	from     *backend.From // nil for the backend's own Ubuntu
	features []string      // base's, in the order chosen
	rebuilds state.Rebuilds
}

// inputs reads them as they are now: From with its digest as last
// recorded.
func (b *Env) inputs() (inputs, error) {
	var in inputs
	var err error
	if in.from, err = b.fromAsRecorded(b.From); err != nil {
		return inputs{}, err
	}
	if in.features, err = b.chosenFeatures(); err != nil {
		return inputs{}, err
	}
	if in.rebuilds, err = state.ReadRebuilds(b.Dirs); err != nil {
		return inputs{}, err
	}
	return in, nil
}

// fromAsRecorded is ref (--from) with its digest as last recorded; nil for
// "", the backend's own Ubuntu.
func (b *Env) fromAsRecorded(ref string) (*backend.From, error) {
	if ref == "" {
		return nil, nil
	}
	digest, err := from.Recorded(b.Dirs, ref)
	if err != nil {
		return nil, err
	}
	return &backend.From{Ref: ref, Digest: digest}, nil
}

// planned is an image as it's built: what its key hashes, its recipe
// among it, which its build is given as it is.
type planned struct {
	image.Input
	from *backend.From // base only: what it starts from; nil for Ubuntu
	// unknown is base's chosen features without a script: its recipe
	// skips them.
	unknown []string
}

// plan returns name's plan and that of every image it's built on, base
// first, each keyed on its parent's key: a changed key anywhere in it
// changes every key above.
func (b *Env) plan(in inputs, name string) ([]planned, error) {
	layers, err := image.Layers(name)
	if err != nil {
		return nil, err
	}
	if b.Backend.Name() == "" {
		return nil, errors.New("image keys need a backend")
	}
	base, err := b.basePlan(in)
	if err != nil {
		return nil, err
	}
	plans := []planned{base}
	for i, layer := range layers {
		img := strings.Join(layers[:i+1], "+")
		p := planned{Input: image.Input{Image: img, Parent: plans[i].Key()}}
		data, err := b.LayerData(layer)
		switch {
		case errors.Is(err, fs.ErrNotExist) && !image.IsBuiltin(layer):
			p.Missing = layer + ".sh"
		case err != nil:
			return nil, fmt.Errorf("reading the %s layer's script: %w", layer, err)
		default:
			p.Recipe.Steps = []image.Step{{Name: layer + ".sh", Data: data}}
		}
		plans = append(plans, b.keyed(in, p))
	}
	return plans, nil
}

// basePlan is base's: the files the backend reads itself (its own,
// base.yaml and with From the conversion's scripts), then base.sh and its
// chosen features' scripts, in order.
func (b *Env) basePlan(in inputs) (planned, error) {
	p := planned{Input: image.Input{Image: image.Base, Own: map[string][]byte{}}, from: in.from}
	if in.from != nil {
		p.From, p.FromDigest = in.from.Ref, in.from.Digest
	}
	for name, data := range b.Backend.KeyFiles() {
		p.Own["backend "+name] = data
	}
	own := []string{"base.yaml"}
	if in.from != nil {
		own = append(own, image.ConversionScripts...)
	}
	for _, name := range own {
		data, err := fs.ReadFile(b.Assets, name)
		if err != nil {
			return planned{}, fmt.Errorf("reading devvm's %s: %w", name, err)
		}
		p.Own[name] = data
	}
	data, err := fs.ReadFile(b.Assets, "layers/base.sh")
	if err != nil {
		return planned{}, fmt.Errorf("reading devvm's layers/base.sh: %w", err)
	}
	p.Recipe.Steps = []image.Step{{Name: "base.sh", Data: data}}
	for _, f := range in.features {
		if !image.FeatureName(f) {
			return planned{}, fmt.Errorf("invalid feature name %q", f)
		}
		s, err := b.FeatureScript(f)
		if errors.Is(err, fs.ErrNotExist) {
			p.unknown = append(p.unknown, f)
			continue
		}
		if err != nil {
			return planned{}, err
		}
		p.Recipe.Steps = append(p.Recipe.Steps, image.Step{Name: s.Name, Data: s.Data})
	}
	return b.keyed(in, p), nil
}

// keyed is p on this backend, with its rebuild counter.
func (b *Env) keyed(in inputs, p planned) planned {
	p.Backend = b.Backend.Name()
	ref := ""
	if in.from != nil {
		ref = in.from.Ref
	}
	p.Rebuild = in.rebuilds.Counter(p.Backend, ref, p.Image)
	return p
}

// chainOf is plans' chain: each image's name and key.
func chainOf(plans []planned) image.Chain {
	c := make(image.Chain, len(plans))
	for i, p := range plans {
		c[i] = image.Link{Name: p.Image, Key: p.Key()}
	}
	return c
}

// chain is name's chain as in has it.
func (b *Env) chain(in inputs, name string) (image.Chain, error) {
	plans, err := b.plan(in, name)
	if err != nil {
		return nil, err
	}
	return chainOf(plans), nil
}

// outdated returns the images in chain, base first, whose key in has
// changed since it was made: an edited script, other features, a rebuild.
func (b *Env) outdated(in inputs, chain image.Chain) ([]string, error) {
	var changed []string
	for _, l := range chain {
		now, err := b.chain(in, l.Name)
		if err != nil {
			return nil, err
		}
		if now.Key() != l.Key {
			changed = append(changed, l.Name)
		}
	}
	return changed, nil
}
