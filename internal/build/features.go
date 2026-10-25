package build

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// Features returns the base image features there are: devvm's own
// (features/<name>.sh), sorted, then the user's (paths.Dirs.Features),
// sorted. shadowed holds the user's scripts named like one of devvm's,
// which are ignored.
func (b *Builder) Features() (features, shadowed []string) {
	own := scripts(filepath.Join(b.Root, "features"))
	features = slices.Clone(own)
	for _, n := range scripts(b.Dirs.Features()) {
		if slices.Contains(own, n) {
			shadowed = append(shadowed, filepath.Join(b.Dirs.Features(), n+".sh"))
		} else {
			features = append(features, n)
		}
	}
	return features, shadowed
}

// scripts returns the names of the regular <name>.sh files in dir that a
// feature can have, sorted. No folder: none.
func scripts(dir string) []string {
	var names []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		n, ok := strings.CutSuffix(e.Name(), ".sh")
		if ok && image.FeatureName(n) && regular(filepath.Join(dir, e.Name())) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}

func regular(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// FeatureScript is a feature's script: devvm's own when it has one by
// that name, else the user's.
func (b *Builder) FeatureScript(name string) string {
	if own := filepath.Join(b.Root, "features", name+".sh"); regular(own) {
		return own
	}
	return filepath.Join(b.Dirs.Features(), name+".sh")
}

// chosenFeatures returns the features chosen for base: the base-features
// file's, or until there is one, every feature.
func (b *Builder) chosenFeatures() ([]string, error) {
	chosen, err := state.ReadBaseFeatures(b.Dirs)
	if errors.Is(err, fs.ErrNotExist) {
		all, _ := b.Features()
		return all, nil
	}
	return chosen, err
}

// chooseFeatures asks which features base gets, the first time anything
// is built (PickFeatures).
func (b *Builder) chooseFeatures() error {
	if _, err := os.Stat(b.Dirs.BaseFeatures()); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	_, err := b.PickFeatures()
	return err
}

// PickFeatures asks which features base gets (Pick), starting from those
// chosen (every one until they're chosen). It saves and prints the choice,
// and reports whether it changed base's features.
func (b *Builder) PickFeatures() (changed bool, err error) {
	before, err := b.chosenFeatures()
	if err != nil {
		return false, err
	}
	names := b.features()
	features := make([]Feature, len(names))
	for i, n := range names {
		features[i] = Feature{Name: n, Desc: description(b.FeatureScript(n)), On: slices.Contains(before, n)}
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
func (b *Builder) SetFeatures(want []string) (changed bool, err error) {
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
func (b *Builder) features() []string {
	names, shadowed := b.Features()
	for _, file := range shadowed {
		b.Warn(msg.FeatureShadowed(strings.TrimSuffix(filepath.Base(file), ".sh"), file))
	}
	return names
}

// saveFeatures saves and prints the chosen features, reporting whether
// they differ from those before.
func (b *Builder) saveFeatures(before, chosen []string) (bool, error) {
	if err := state.WriteBaseFeatures(b.Dirs, chosen); err != nil {
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
func description(path string) string {
	f, err := os.Open(path) //nolint:gosec // devvm's or the user's feature
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
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

// featureScripts returns the scripts of base's chosen features, in order,
// skipping any without one.
func (b *Builder) featureScripts() ([]string, error) {
	chosen, err := b.chosenFeatures()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, f := range chosen {
		p := b.FeatureScript(f)
		if !regular(p) {
			b.Warn(msg.UnknownFeature(f))
			continue
		}
		paths = append(paths, p)
	}
	return paths, nil
}
