package build

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/settings"
)

// Features returns the base image features there are: devvm's own
// (features/<name>.sh in Assets), sorted, then the user's
// (paths.Dirs.Features), sorted. shadowed holds the user's scripts named
// like one of devvm's, which are ignored.
func (b *Env) Features() (features, shadowed []string) {
	own := scripts(b.Assets, "features")
	features = slices.Clone(own)
	for _, n := range scripts(os.DirFS(b.Dirs.Features()), ".") {
		if slices.Contains(own, n) {
			shadowed = append(shadowed, filepath.Join(b.Dirs.Features(), n+".sh"))
		} else {
			features = append(features, n)
		}
	}
	return features, shadowed
}

// scripts returns the names of the regular <name>.sh files in dir of fsys
// that a feature can have, sorted. No folder: none.
func scripts(fsys fs.FS, dir string) []string {
	var names []string
	entries, _ := fs.ReadDir(fsys, dir)
	for _, e := range entries {
		n, ok := strings.CutSuffix(e.Name(), ".sh")
		if ok && image.FeatureName(n) && regular(fsys, path.Join(dir, e.Name())) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}

// regular reports whether name in fsys is a regular file, links followed.
func regular(fsys fs.FS, name string) bool {
	fi, err := fs.Stat(fsys, name)
	return err == nil && fi.Mode().IsRegular()
}

// FeatureScript is a feature's script: devvm's own when it has one by
// that name, else the user's. No script is an error matching
// fs.ErrNotExist.
func (b *Env) FeatureScript(name string) (backend.Script, error) {
	s := backend.Script{Name: name + ".sh"}
	own, user := ownFeature(name), os.DirFS(b.Dirs.Features())
	var err error
	switch {
	case regular(b.Assets, own):
		s.Data, err = fs.ReadFile(b.Assets, own)
	case regular(user, s.Name):
		s.Data, err = fs.ReadFile(user, s.Name)
	default:
		err = fs.ErrNotExist
	}
	return s, err
}

// ownFeature is the file of devvm's own feature by that name, in its
// files.
func ownFeature(name string) string { return path.Join("features", name+".sh") }

// featureStep is a feature's step in base's recipe: its script
// (FeatureScript), devvm's own with the prelude before it.
func (b *Env) featureStep(name string) (image.Step, error) {
	s, err := b.FeatureScript(name)
	if err == nil && regular(b.Assets, ownFeature(name)) {
		s.Data, err = image.OwnScript(b.Assets, ownFeature(name))
	}
	return image.Step{Name: s.Name, Data: s.Data}, err
}

// chosenFeatures returns the features chosen for base, as config.toml or
// settings.json chooses them, or until they're chosen, every feature.
func (b *Env) chosenFeatures() ([]string, error) {
	def, err := settings.Read(b.Dirs)
	if err != nil || def.Features.Chosen {
		return def.Features.Names, err
	}
	all, _ := b.Features()
	return all, nil
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
	names := b.features()
	features := make([]Feature, len(names))
	for i, n := range names {
		s, _ := b.FeatureScript(n)
		features[i] = Feature{Name: n, Desc: description(s.Data), On: slices.Contains(before, n)}
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
	names := b.features()
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

// features is Features, warning about the user's features it ignores.
func (b Choice) features() []string {
	names, shadowed := b.Features()
	for _, file := range shadowed {
		b.Warn(msg.FeatureShadowed(strings.TrimSuffix(filepath.Base(file), ".sh"), file))
	}
	return names
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
