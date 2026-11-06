package build

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/from"
	"github.com/devvmhq/devvm/internal/image"
	"github.com/devvmhq/devvm/internal/state"
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
func (b *Env) plan(in inputs, name image.Name) ([]planned, error) {
	if err := name.Check(); err != nil {
		return nil, err
	}
	layers := name.Layers()
	if b.Backend.Name() == "" {
		return nil, errors.New("image keys need a backend")
	}
	base, err := b.basePlan(in)
	if err != nil {
		return nil, err
	}
	plans := []planned{base}
	apt, err := image.OwnScript(b.Assets, "layers/apt.sh")
	if err != nil && len(layers) > 0 {
		return nil, err
	}
	// Each layer's script runs with package lists, which base keeps none
	// of (packages.go).
	lists := image.Step{Name: "apt.sh", Data: apt, Args: []string{"lists"}, Root: true}
	for i, img := range name.Under() {
		layer := layers[i]
		p := planned{Input: image.Input{Image: img, Parent: plans[i].Key()}}
		data, err := b.LayerData(layer)
		switch {
		case errors.Is(err, fs.ErrNotExist) && !image.IsBuiltin(layer):
			p.Missing = layer + ".sh"
		case err != nil:
			return nil, fmt.Errorf("reading the %s layer's script: %w", layer, err)
		default:
			p.Recipe.Steps = []image.Step{lists, {Name: layer + ".sh", Data: data}}
		}
		plans = append(plans, b.keyed(in, p))
	}
	return plans, nil
}

// basePlan is base's: the backend's own steps (Origin), then apt.sh begin
// as root, base.sh and its chosen features' scripts, in order, and apt.sh
// end as root (base's packages, packages.go).
func (b *Env) basePlan(in inputs) (planned, error) {
	p := planned{Input: image.Input{Image: image.Base}, from: in.from}
	if in.from != nil {
		p.From, p.FromDigest = in.from.Ref, in.from.Digest
	}
	var err error
	if p.Recipe.Origin, err = b.Backend.Origin(in.from); err != nil {
		return planned{}, err
	}
	data, err := image.OwnScript(b.Assets, "layers/base.sh")
	if err != nil {
		return planned{}, err
	}
	apt, err := image.OwnScript(b.Assets, "layers/apt.sh")
	if err != nil {
		return planned{}, err
	}
	p.Recipe.Steps = []image.Step{{Name: "apt.sh", Data: apt, Args: []string{"begin"}, Root: true}, {Name: "base.sh", Data: data}}
	for _, f := range in.features {
		if !image.FeatureName(f) {
			return planned{}, fmt.Errorf("invalid feature name %q", f)
		}
		s, err := b.featureStep(f)
		if errors.Is(err, fs.ErrNotExist) {
			p.unknown = append(p.unknown, f)
			continue
		}
		if err != nil {
			return planned{}, err
		}
		p.Recipe.Steps = append(p.Recipe.Steps, s)
	}
	p.Recipe.Steps = append(p.Recipe.Steps, image.Step{Name: "apt.sh", Data: apt, Args: []string{"end"}, Root: true})
	return b.keyed(in, p), nil
}

// keyed is p on this backend, with its rebuild counter.
func (b *Env) keyed(in inputs, p planned) planned {
	p.Backend = b.Backend.Name()
	ref := ""
	if in.from != nil {
		ref = in.from.Ref
	}
	p.Rebuild = in.rebuilds.Counter(p.Backend, ref, p.Image.String())
	return p
}

// chainOf is plans' chain: each image's name and key.
func chainOf(plans []planned) image.Chain {
	c := make(image.Chain, len(plans))
	for i, p := range plans {
		c[i] = image.Link{Name: p.Image.String(), Key: p.Key()}
	}
	return c
}

// chain is name's chain as in has it.
func (b *Env) chain(in inputs, name image.Name) (image.Chain, error) {
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
		now, err := b.chain(in, image.Name(l.Name))
		if err != nil {
			return nil, err
		}
		if now.Key() != l.Key {
			changed = append(changed, l.Name)
		}
	}
	return changed, nil
}
