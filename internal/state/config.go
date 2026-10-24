package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/w3cj/devvm/internal/paths"
)

// Size is the default size of new VMs, as written in the size file
// (cpus=, memory=, disk=). "" is unset. The values are kept as written:
// callers check them, since the file may be edited by hand.
type Size struct {
	CPUs, Memory, Disk string
}

var sizeKeys = oneOf("cpus", "memory", "disk")

// ReadSize reads the default size of new VMs. A missing file sets nothing.
func ReadSize(d paths.Dirs) (Size, error) {
	k, err := readKV(d.Size(), sizeKeys)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Size{}, nil
	case err != nil:
		return Size{}, err
	}
	return Size{CPUs: k.get("cpus"), Memory: k.get("memory"), Disk: k.get("disk")}, nil
}

// WriteSize writes the default size of new VMs, leaving out what's unset.
func WriteSize(d paths.Dirs, s Size) error {
	k := newKV()
	for _, p := range [][2]string{{"cpus", s.CPUs}, {"memory", s.Memory}, {"disk", s.Disk}} {
		if p[1] == "" {
			continue
		}
		if err := checkPair(p[0], p[1], sizeKeys); err != nil {
			return err
		}
		k.set(p[0], p[1])
	}
	return writeFile(d.Size(), k.bytes())
}

// readValue reads a file holding one value on one line. A missing or empty
// file is "".
func readValue(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // paths come from paths.Dirs
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", nil
	case err != nil:
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if strings.ContainsAny(v, "\n\r") {
		return "", &CorruptError{Path: path, Line: 2, Reason: "more than one line"}
	}
	return v, nil
}

func writeValue(path, v string) error {
	if v == "" || strings.ContainsAny(v, "\n\r") {
		return fmt.Errorf("invalid value %q for %s", v, filepath.Base(path))
	}
	return writeFile(path, []byte(v+"\n"))
}

// ReadPorts reads which ports new VMs forward by default (all, none or a
// list), as written: callers check it. A missing file is "".
func ReadPorts(d paths.Dirs) (string, error) { return readValue(d.Ports()) }

// WritePorts writes which ports new VMs forward by default.
func WritePorts(d paths.Dirs, ports string) error { return writeValue(d.Ports(), ports) }

var featureName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ReadBaseFeatures reads the features chosen for the base image, one per
// line, in order. Until they're chosen there's no file: that's an error
// that matches fs.ErrNotExist. An empty file chose none.
func ReadBaseFeatures(d paths.Dirs) ([]string, error) {
	path := d.BaseFeatures()
	data, err := os.ReadFile(path) //nolint:gosec // paths come from paths.Dirs
	if err != nil {
		return nil, err
	}
	features := []string{}
	seen := map[string]bool{}
	for i, line := range lines(data) {
		f := strings.TrimSpace(line)
		switch {
		case f == "":
			continue
		case !featureName.MatchString(f):
			return nil, &CorruptError{path, i + 1, fmt.Sprintf("invalid feature name %q", f)}
		case seen[f]:
			return nil, &CorruptError{path, i + 1, fmt.Sprintf("%s is listed twice", f)}
		}
		seen[f] = true
		features = append(features, f)
	}
	return features, nil
}

// WriteBaseFeatures writes the features chosen for the base image.
func WriteBaseFeatures(d paths.Dirs, features []string) error {
	var b strings.Builder
	seen := map[string]bool{}
	for _, f := range features {
		if !featureName.MatchString(f) || seen[f] {
			return fmt.Errorf("invalid or repeated feature %q", f)
		}
		seen[f] = true
		b.WriteString(f + "\n")
	}
	return writeFile(d.BaseFeatures(), []byte(b.String()))
}

// AgentKey is a key added to devvm's SSH agent.
type AgentKey struct {
	File        string // the key file: an absolute path
	Fingerprint string // its fingerprint when added (SHA256:…), if known
	Options     string // how it was added (confirm, lifetime=…), as given
}

// ReadAgentKeys reads the keys added to devvm's SSH agent: one per line,
// with its fields separated by tabs. A missing file has none.
func ReadAgentKeys(d paths.Dirs) ([]AgentKey, error) {
	path := d.AgentKeys()
	data, err := os.ReadFile(path) //nolint:gosec // paths come from paths.Dirs
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, err
	}
	var keys []AgentKey
	for i, line := range lines(data) {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) > 3 {
			return nil, &CorruptError{path, i + 1, "more than 3 fields"}
		}
		f = append(f, "", "")
		k := AgentKey{File: f[0], Fingerprint: f[1], Options: f[2]}
		if !filepath.IsAbs(k.File) {
			return nil, &CorruptError{path, i + 1, fmt.Sprintf("key file %q is not an absolute path", k.File)}
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// WriteAgentKeys writes the keys added to devvm's SSH agent.
func WriteAgentKeys(d paths.Dirs, keys []AgentKey) error {
	var b strings.Builder
	for _, k := range keys {
		if !filepath.IsAbs(k.File) {
			return fmt.Errorf("key file %q is not an absolute path", k.File)
		}
		for _, v := range []string{k.File, k.Fingerprint, k.Options} {
			if strings.ContainsAny(v, "\t\n\r") {
				return fmt.Errorf("agent key field %q has a tab or line break", v)
			}
		}
		b.WriteString(k.File + "\t" + k.Fingerprint + "\t" + k.Options + "\n")
	}
	return writeFile(d.AgentKeys(), []byte(b.String()))
}
