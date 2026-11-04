// Package devcontainer reads a project's devcontainer.json as input for a
// new VM: Find finds it in the project folder, Parse reads it (JSON with
// comments and trailing commas) into a Plan, what devvm does with it, and
// refuses what devvm won't do. The file is the project's, so the VM's: a
// plan only shapes what runs inside the VM, and nothing in it ever runs on
// the host, reaches its paths or environment, or adds mounts.
package devcontainer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/jsonc"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/shell"
)

// Where a project's devcontainer.json is looked for, relative to its
// folder, in this order.
const (
	File     = ".devcontainer/devcontainer.json"
	RootFile = ".devcontainer.json"
)

// maxSize is the most of a devcontainer.json devvm reads.
const maxSize = 1 << 20

// Its commands' properties, in the order they run.
const (
	OnCreate      = "onCreateCommand"
	UpdateContent = "updateContentCommand"
	PostCreate    = "postCreateCommand"
	PostStart     = "postStartCommand"
)

// The features devvm has layers for: devcontainers' own, by ID.
const (
	featurePrefix  = "ghcr.io/devcontainers/features/"
	dockerInDocker = "docker-in-docker"
	dockerOutside  = "docker-outside-of-docker"
)

// ErrTooLarge is a devcontainer.json larger than devvm reads.
var ErrTooLarge = errors.New("larger than 1 MiB")

// ErrNotFile is a devcontainer.json that isn't a regular file.
var ErrNotFile = errors.New("not a regular file")

// Find returns the project's devcontainer.json in fsys, its folder: its
// path in it and its contents; "" when there's none. A link is followed
// only as far as fsys lets it (os.Root's stays inside the folder), and
// only a regular file is read.
func Find(fsys fs.FS) (string, []byte, error) {
	for _, name := range []string{File, RootFile} {
		fi, err := fs.Stat(fsys, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return name, nil, err
		}
		if !fi.Mode().IsRegular() {
			return name, nil, ErrNotFile
		}
		data, err := read(fsys, name)
		return name, data, err
	}
	return "", nil, nil
}

// read reads a regular file of at most maxSize.
func read(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if fi, err := f.Stat(); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, ErrNotFile
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, ErrTooLarge
	}
	return data, nil
}

// Sum is a devcontainer.json's SHA-256, in hex: what a VM's state records
// of the file it was made from.
func Sum(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

// Command is one command a devcontainer.json runs in the VM.
type Command struct {
	// Name is what the file calls it: its property, with a named
	// command's name after it (postCreateCommand server).
	Name string `json:"name"`
	// Argv is what runs: a string's through sh -c, an array as it is.
	Argv []string `json:"argv"`
}

// Var is an environment variable a devcontainer.json sets: its name, and
// its value as a POSIX shell word, which another of the VM's variables
// may be part of (${containerEnv:PATH} is "${PATH}").
type Var struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Plan is what devvm does with a devcontainer.json for a new VM.
type Plan struct {
	File   string    // its path in the project folder
	Sum    string    // its SHA-256 (Sum)
	Image  string    // image: what the VM's base starts from (--from); "" for devvm's own
	Layers []string  // what its features map to: devvm's layers and toolchains
	Ports  string    // forwardPorts on localhost, as a ports list (3000,5173); "" for none
	Env    []Var     // containerEnv and remoteEnv, remoteEnv's winning, by name
	Create []Command // onCreateCommand, updateContentCommand, postCreateCommand: once, in order
	Start  []Command // postStartCommand: at each start
	// Notes are warnings about what's ignored, one line each.
	Notes []msg.M
}

// parser is one file's reading: its path and the project folder.
type parser struct {
	file, workspace string
	plan            Plan
}

// Parse reads a devcontainer.json, file being its path in the project
// folder, which is at workspace in the VM as on the host. It refuses (a
// msg.M) invalid JSON, values of the wrong kind, a container it would
// build, features devvm has no layer for, and variables devvm doesn't fill
// in: ${localEnv:…} (${env:…}) above all, which would put this machine's
// environment in the VM.
func Parse(file string, data []byte, workspace string) (Plan, error) {
	p := &parser{file: file, workspace: workspace, plan: Plan{File: file, Sum: Sum(data)}}
	top, err := p.decode(data)
	if err != nil {
		return Plan{}, err
	}
	keys := make([]string, 0, len(top))
	for k := range top {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var ignored, unknown []string
	for _, k := range keys {
		switch kind(k) {
		case keyBuild:
			return Plan{}, msg.DevcontainerBuild(file, k)
		case keyIgnored:
			ignored = append(ignored, k)
		case keyUnknown:
			unknown = append(unknown, k)
		case keyInitialize:
			p.note(msg.DevcontainerInitialize(file))
		}
	}
	if err := p.properties(top); err != nil {
		return Plan{}, err
	}
	if len(ignored) > 0 {
		p.note(msg.DevcontainerIgnored(file, strings.Join(ignored, " ")))
	}
	if len(unknown) > 0 {
		p.note(msg.DevcontainerUnknown(file, strings.Join(unknown, " ")))
	}
	return p.plan, nil
}

// properties maps the properties devvm uses.
func (p *parser) properties(top map[string]json.RawMessage) error {
	if raw, ok := top["image"]; ok {
		var s string
		if err := p.value("image", raw, &s); err != nil {
			return err
		}
		img, err := p.subst("image", s, modeLiteral)
		if err != nil {
			return err
		}
		p.plan.Image = img
	}
	if raw, ok := top["features"]; ok {
		if err := p.features(raw); err != nil {
			return err
		}
	}
	if raw, ok := top["forwardPorts"]; ok {
		if err := p.ports(raw); err != nil {
			return err
		}
	}
	if err := p.env(top); err != nil {
		return err
	}
	for _, k := range []string{OnCreate, UpdateContent, PostCreate, PostStart} {
		raw, ok := top[k]
		if !ok {
			continue
		}
		cmds, err := p.commands(k, raw)
		if err != nil {
			return err
		}
		if k == PostStart {
			p.plan.Start = cmds
		} else {
			p.plan.Create = append(p.plan.Create, cmds...)
		}
	}
	return nil
}

func (p *parser) note(m msg.M) { p.plan.Notes = append(p.plan.Notes, m) }

// decode reads the file's top-level object, comments and trailing commas
// left out.
func (p *parser) decode(data []byte) (map[string]json.RawMessage, error) {
	clean := jsonc.Strip(data)
	var top map[string]json.RawMessage
	err := json.Unmarshal(clean, &top)
	if syn, ok := errors.AsType[*json.SyntaxError](err); ok {
		line, col := position(data, syn.Offset)
		return nil, msg.DevcontainerInvalid(strconv.Itoa(col), p.file, strconv.Itoa(line)).Because(err)
	}
	if err != nil || top == nil {
		// Not an object: where its value starts.
		line, col := position(data, int64(len(clean)-len(bytes.TrimLeft(clean, " \t\r\n"))+1))
		return nil, msg.DevcontainerInvalid(strconv.Itoa(col), p.file, strconv.Itoa(line))
	}
	return top, nil
}

// position is the line and column, from 1, of the byte before offset.
func position(data []byte, offset int64) (line, col int) {
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	if offset > 0 {
		offset--
	}
	before := data[:offset]
	line = bytes.Count(before, []byte("\n")) + 1
	col = len(before) - bytes.LastIndexByte(before, '\n')
	return line, col
}

// value decodes a property's value into v, refusing one of another kind.
func (p *parser) value(key string, raw json.RawMessage, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return msg.DevcontainerBadValue(p.file, key).Because(err)
	}
	return nil
}

// features maps features to devvm's layers: devcontainers' own
// docker-in-docker to the docker layer, and those named as one of devvm's
// toolchains (image.Toolchains) to it. Their options are ignored; their
// scripts are never fetched or run.
func (p *parser) features(raw json.RawMessage) error {
	var feats map[string]json.RawMessage
	if err := p.value("features", raw, &feats); err != nil {
		return err
	}
	ids := make([]string, 0, len(feats))
	for id := range feats {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var withOptions []string
	for _, id := range ids {
		layer, err := p.feature(id)
		if err != nil {
			return err
		}
		if !slices.Contains(p.plan.Layers, layer) {
			p.plan.Layers = append(p.plan.Layers, layer)
		}
		if hasOptions(feats[id]) {
			withOptions = append(withOptions, id)
		}
	}
	if len(withOptions) > 0 {
		p.note(msg.DevcontainerFeatureOptions(strings.Join(withOptions, " "), p.file))
	}
	return nil
}

// feature is the layer a feature's ID maps to.
func (p *parser) feature(id string) (string, error) {
	name, ok := strings.CutPrefix(featureName(id), featurePrefix)
	switch {
	case !ok:
	case name == dockerInDocker:
		return image.DockerLayer, nil
	case name == dockerOutside:
		return "", msg.DevcontainerDockerOutside(id, p.file)
	case slices.Contains(image.Toolchains, name):
		return name, nil
	}
	return "", msg.DevcontainerFeature(id, p.file)
}

// featureName is a feature's ID without its version: its tag or digest.
func featureName(id string) string {
	id = strings.ToLower(id)
	if i := strings.Index(id, "@"); i >= 0 {
		id = id[:i]
	}
	if i := strings.LastIndex(id, ":"); i > strings.LastIndex(id, "/") {
		id = id[:i]
	}
	return id
}

// hasOptions reports whether a feature's value sets options: an object
// with any, or a version string.
func hasOptions(raw json.RawMessage) bool {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s != ""
	}
	var o map[string]json.RawMessage
	if json.Unmarshal(raw, &o) == nil {
		return len(o) > 0
	}
	return !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// ports maps forwardPorts on this machine's localhost (3000,
// "localhost:3000", "127.0.0.1:3000") to a ports list; other hosts' are
// ignored, said.
func (p *parser) ports(raw json.RawMessage) error {
	var entries []json.RawMessage
	if err := p.value("forwardPorts", raw, &entries); err != nil {
		return err
	}
	var list, ignored []string
	for _, e := range entries {
		port, ok, err := p.port(e)
		if err != nil {
			return err
		}
		if !ok {
			ignored = append(ignored, string(bytes.TrimSpace(e)))
			continue
		}
		if s := strconv.Itoa(port); !slices.Contains(list, s) {
			list = append(list, s)
		}
	}
	p.plan.Ports = strings.Join(list, ",")
	if len(ignored) > 0 {
		p.note(msg.DevcontainerPortsIgnored(p.file, strings.Join(ignored, " ")))
	}
	return nil
}

// port reads a forwardPorts entry: its port, false for one devvm doesn't
// forward.
func (p *parser) port(raw json.RawMessage) (int, bool, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		v, err := strconv.Atoi(n.String())
		return v, err == nil && v >= 1 && v <= 65535, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, false, msg.DevcontainerBadValue(p.file, "forwardPorts").Because(err)
	}
	s, err := p.subst("forwardPorts", s, modeLiteral)
	if err != nil {
		return 0, false, err
	}
	host, port, ok := strings.Cut(s, ":")
	if !ok || host != "localhost" && host != "127.0.0.1" {
		return 0, false, nil
	}
	v, err := strconv.Atoi(port)
	return v, err == nil && v >= 1 && v <= 65535 && strconv.Itoa(v) == port, nil
}

// env maps containerEnv and remoteEnv to the VM's environment, remoteEnv's
// winning. A remoteEnv value of null sets nothing.
func (p *parser) env(top map[string]json.RawMessage) error {
	vars := map[string]string{}
	for _, k := range []string{"containerEnv", "remoteEnv"} {
		raw, ok := top[k]
		if !ok {
			continue
		}
		var m map[string]*string
		if err := p.value(k, raw, &m); err != nil {
			return err
		}
		for name, v := range m {
			if !shell.EnvName(name) {
				return msg.DevcontainerEnvName(p.file, name)
			}
			if v == nil {
				if k == "containerEnv" {
					return msg.DevcontainerBadValue(p.file, k)
				}
				continue
			}
			word, err := p.subst(k, *v, modeEnv)
			if err != nil {
				return err
			}
			vars[name] = word
		}
	}
	names := make([]string, 0, len(vars))
	for n := range vars {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		p.plan.Env = append(p.plan.Env, Var{Name: n, Value: vars[n]})
	}
	return nil
}

// commands reads a command property: a string runs through sh -c, an
// array as it is, an object's commands each, sorted by name.
func (p *parser) commands(key string, raw json.RawMessage) ([]Command, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil && obj != nil {
		names := make([]string, 0, len(obj))
		for n := range obj {
			names = append(names, n)
		}
		slices.Sort(names)
		var cmds []Command
		for _, n := range names {
			argv, err := p.command(key, obj[n])
			if err != nil {
				return nil, err
			}
			if len(argv) > 0 {
				cmds = append(cmds, Command{Name: key + " " + n, Argv: argv})
			}
		}
		return cmds, nil
	}
	argv, err := p.command(key, raw)
	if err != nil || len(argv) == 0 {
		return nil, err
	}
	return []Command{{Name: key, Argv: argv}}, nil
}

// command reads one command: a string or an array of strings; nil for an
// empty one.
func (p *parser) command(key string, raw json.RawMessage) ([]string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		s, err := p.subst(key, s, modeShell)
		if err != nil || strings.TrimSpace(s) == "" {
			return nil, err
		}
		return []string{"sh", "-c", s}, nil
	}
	var argv []string
	if err := json.Unmarshal(raw, &argv); err != nil {
		return nil, msg.DevcontainerBadValue(p.file, key).Because(err)
	}
	for i, a := range argv {
		v, err := p.subst(key, a, modeLiteral)
		if err != nil {
			return nil, err
		}
		argv[i] = v
	}
	if len(argv) == 0 || argv[0] == "" {
		return nil, nil
	}
	return argv, nil
}
