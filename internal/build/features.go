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
	"github.com/w3cj/devvm/internal/ui"
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
// is built: every feature is chosen to start with, and without a
// terminal that's the choice. It's saved in the base-features file.
func (b *Builder) chooseFeatures() error {
	if _, err := os.Stat(b.Dirs.BaseFeatures()); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	names, shadowed := b.Features()
	for _, file := range shadowed {
		ui.Warn(msg.FeatureShadowed(strings.TrimSuffix(filepath.Base(file), ".sh"), file))
	}
	choices := make([]ui.Choice, len(names))
	for i, n := range names {
		choices[i] = ui.Choice{Name: n, Desc: description(b.FeatureScript(n)), On: true}
	}
	var chosen []string
	for _, c := range ui.Pick(msg.FeaturesHelp(), choices) {
		if c.On {
			chosen = append(chosen, c.Name)
		}
	}
	if err := state.WriteBaseFeatures(b.Dirs, chosen); err != nil {
		return err
	}
	if len(chosen) == 0 {
		ui.Info(msg.BaseFeaturesNone())
	} else {
		ui.Info(msg.BaseFeatures(strings.Join(chosen, " ")))
	}
	return nil
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
			ui.Warn(msg.UnknownFeature(f))
			continue
		}
		paths = append(paths, p)
	}
	return paths, nil
}
