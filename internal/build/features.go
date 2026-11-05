package build

import (
	"bufio"
	"bytes"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/settings"
)

// Features returns the base image features there are, sorted: devvm's
// own, features/<name>.sh in Assets. A user's extras for every VM are
// layers of theirs, chosen as setup's default layers.
func (b *Env) Features() []string {
	var names []string
	entries, _ := fs.ReadDir(b.Assets, "features")
	for _, e := range entries {
		n, ok := strings.CutSuffix(e.Name(), ".sh")
		if ok && image.FeatureName(n) && b.isFeature(n) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}

// isFeature reports whether devvm has a feature by that name: a regular
// file, links followed.
func (b *Env) isFeature(name string) bool {
	fi, err := fs.Stat(b.Assets, featureFile(name))
	return err == nil && fi.Mode().IsRegular()
}

// featureFile is the file of devvm's feature by that name, in its files.
func featureFile(name string) string { return path.Join("features", name+".sh") }

// featureStep is a feature's step in base's recipe: its script, with the
// prelude before it. No such feature is an error matching fs.ErrNotExist.
func (b *Env) featureStep(name string) (image.Step, error) {
	if !b.isFeature(name) {
		return image.Step{}, fs.ErrNotExist
	}
	data, err := image.OwnScript(b.Assets, featureFile(name))
	return image.Step{Name: name + ".sh", Data: data}, err
}

// chosenFeatures returns the features chosen for base, as config.toml or
// settings.json chooses them, or until they're chosen, every feature.
func (b *Env) chosenFeatures() ([]string, error) {
	def, err := settings.Read(b.Dirs)
	if err != nil || def.Features.Chosen {
		return def.Features.Names, err
	}
	return b.Features(), nil
}

// chooseFeatures asks which features base gets, the first time anything
// is built (PickFeatures).
func (b Choice) chooseFeatures() error {
	if def, err := settings.Read(b.Dirs); err != nil || def.Features.Chosen {
		return err
	}
	_, err := b.PickFeatures()
	return err
}

// PickFeatures asks which features base gets (Pick), starting from those
// chosen (every one until they're chosen). It saves and prints the choice,
// and reports whether it changed base's features.
func (b Choice) PickFeatures() (changed bool, err error) {
	before, err := b.chosenFeatures()
	if err != nil {
		return false, err
	}
	names := b.Features()
	features := make([]Feature, len(names))
	for i, n := range names {
		data, _ := fs.ReadFile(b.Assets, featureFile(n))
		features[i] = Feature{Name: n, Desc: description(data), On: slices.Contains(before, n)}
	}
	var chosen []string
	for _, f := range b.Pick(features) {
		if f.On {
			chosen = append(chosen, f.Name)
		}
	}
	return b.saveFeatures(before, chosen)
}

// Feature is one of base's features as Pick shows it: its name, what its
// script says it adds, and whether it's chosen.
type Feature struct {
	Name, Desc string
	On         bool
}

// SetFeatures chooses these features for base, in the order features
// run, and saves and prints the choice. It reports whether that changed
// base's features. A name that isn't a feature is no_feature.
func (b Choice) SetFeatures(want []string) (changed bool, err error) {
	before, err := b.chosenFeatures()
	if err != nil {
		return false, err
	}
	names := b.Features()
	for _, w := range want {
		if !slices.Contains(names, w) {
			return false, msg.NoFeature(w, strings.Join(names, " "))
		}
	}
	var chosen []string
	for _, n := range names {
		if slices.Contains(want, n) {
			chosen = append(chosen, n)
		}
	}
	return b.saveFeatures(before, chosen)
}

// saveFeatures saves and prints the chosen features, reporting whether
// they differ from those before.
func (b Choice) saveFeatures(before, chosen []string) (bool, error) {
	if _, err := settings.Save(b.Dirs, settings.Values{Features: settings.Of(append([]string{}, chosen...))}); err != nil {
		return false, err
	}
	if len(chosen) == 0 {
		b.Info(msg.BaseFeaturesNone())
	} else {
		b.Info(msg.BaseFeatures(strings.Join(chosen, " ")))
	}
	return !slices.Equal(before, chosen), nil
}

// description is what a feature's script says it adds: its second line,
// a comment, without the #.
func description(script []byte) string {
	s := bufio.NewScanner(bytes.NewReader(script))
	for i := 0; i < 2 && s.Scan(); i++ {
		if i == 1 {
			line, ok := strings.CutPrefix(s.Text(), "#")
			if !ok {
				return ""
			}
			return strings.TrimSpace(line)
		}
	}
	return ""
}
