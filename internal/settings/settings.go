// Package settings reads the defaults new VMs and devvm's own upkeep get,
// from two files: config.toml in devvm's config folder, which you edit and
// devvm never writes, and settings.json in its data folder, which devvm's
// commands write (devvm setup, devvm size -g, devvm ports -g, devvm
// network -g, devvm retention). A flag wins over config.toml, config.toml
// over settings.json, and settings.json over devvm's built-in default; a
// command refuses to save a key config.toml sets. Both files are checked
// whole each time they're read, so a typo fails the command that reads
// it, naming the file, and config.toml's line.
package settings

import (
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

// Source is where a default in effect came from.
type Source string

// The sources, in the order the first that sets a default wins.
const (
	FromConfig   Source = "config.toml"
	FromSettings Source = "settings.json"
	FromBuiltin  Source = "built-in"
)

// Label is s as devvm's text output shows it after a value, following a
// #: config.toml's path, which you edit, or devvm's own, as devvm network
// labels its built-in policies.
func (s Source) Label(d paths.Dirs) string {
	switch s {
	case FromConfig:
		return d.ConfigFile()
	case FromSettings:
		return "devvm: settings.json"
	case FromBuiltin:
	}
	return "devvm: default"
}

// Value is a default in effect and where it came from.
type Value struct {
	Value  string
	Source Source
}

// List is a default list in effect: base's features, or new VMs' agents
// or layers. Chosen is false while nothing sets it (FromBuiltin), and then
// Names is nil; once chosen, Names may be empty.
type List struct {
	Names  []string
	Chosen bool
	Source Source
}

// Defaults are the defaults in effect, each with where it came from.
type Defaults struct {
	// Network is new VMs' network policy: NetworkAsk unless set.
	Network Value
	// Ports is what new VMs forward (ParsePorts): PortsAll unless set.
	Ports Value
	// CPUs, Memory and Disk are new VMs' size (ParseCPUs, ParseGiB, in
	// GiB): "" for the image's.
	CPUs, Memory, Disk Value
	// Features are base's features; Agents new VMs' coding agents; Layers
	// the layers new VMs' images get, bottom first, in the order an image's
	// name joins them.
	Features, Agents, Layers List
	Retention                Retention
}

// Values are what config.toml or settings.json sets, as settings.json
// holds them: nil, or a key left out, for what it leaves unset. A Values
// given to Save is a change: what it sets is saved, a retention key set
// to "" turned off.
type Values struct {
	Network   *string           `json:"network,omitempty"`
	Ports     *string           `json:"ports,omitempty"`
	Size      SizeValues        `json:"size,omitzero"`
	Features  *[]string         `json:"features,omitempty"`
	Agents    *[]string         `json:"agents,omitempty"`
	Layers    *[]string         `json:"layers,omitempty"`
	Retention map[string]string `json:"retention,omitempty"`
}

// SizeValues are new VMs' CPUs, memory and disk in Values.
type SizeValues struct {
	CPUs   *string `json:"cpus,omitempty"`
	Memory *string `json:"memory,omitempty"`
	Disk   *string `json:"disk,omitempty"`
}

// Of returns a pointer to v, for Values.
func Of[T any](v T) *T { return &v }

// The keys of Values, as config.toml names them.
const (
	KeyNetwork  = "network"
	KeyPorts    = "ports"
	KeyCPUs     = "size.cpus"
	KeyMemory   = "size.memory"
	KeyDisk     = "size.disk"
	KeyFeatures = "features"
	KeyAgents   = "agents"
	KeyLayers   = "layers"
	// KeyRetention is the table the retention settings' keys are in:
	// retention.history.age.
	KeyRetention = "retention"
)

// scalars returns the keys whose values are strings, as config.toml names
// them, with the field of v each is.
func (v *Values) scalars() []struct {
	key string
	p   **string
} {
	return []struct {
		key string
		p   **string
	}{
		{KeyNetwork, &v.Network},
		{KeyPorts, &v.Ports},
		{KeyCPUs, &v.Size.CPUs},
		{KeyMemory, &v.Size.Memory},
		{KeyDisk, &v.Size.Disk},
	}
}

// lists returns the keys whose values are lists, with the field of v each
// is.
func (v *Values) lists() []struct {
	key string
	p   **[]string
} {
	return []struct {
		key string
		p   **[]string
	}{{KeyFeatures, &v.Features}, {KeyAgents, &v.Agents}, {KeyLayers, &v.Layers}}
}

// Set returns the keys v sets, as config.toml names them: retention's
// under KeyRetention.
func (v Values) Set() []string {
	var keys []string
	for _, s := range v.scalars() {
		if *s.p != nil {
			keys = append(keys, s.key)
		}
	}
	for _, l := range v.lists() {
		if *l.p != nil {
			keys = append(keys, l.key)
		}
	}
	for _, k := range RetentionKeys() {
		if _, ok := v.Retention[k]; ok {
			keys = append(keys, KeyRetention+"."+k)
		}
	}
	return keys
}

// check refuses a value devvm can't use, naming its key. A retention key
// set to "" passes only with off.
func (v Values) check(off bool) (key string, err error) {
	checks := map[string]func(string) error{
		KeyNetwork: checkNetwork, KeyPorts: func(s string) error { _, err := ParsePorts(s); return err },
		KeyCPUs:   func(s string) error { _, err := ParseCPUs("cpus", s); return err },
		KeyMemory: func(s string) error { _, err := ParseGiB("memory", s); return err },
		KeyDisk:   func(s string) error { _, err := ParseGiB("disk", s); return err },
	}
	for _, s := range v.scalars() {
		if *s.p != nil {
			if err := checks[s.key](**s.p); err != nil {
				return s.key, err
			}
		}
	}
	lists := map[string]func([]string) error{KeyFeatures: checkFeatures, KeyAgents: checkAgents, KeyLayers: checkLayers}
	for _, l := range v.lists() {
		if *l.p != nil {
			if err := lists[l.key](**l.p); err != nil {
				return l.key, err
			}
		}
	}
	for k, val := range v.Retention {
		if val == "" && off && slices.Contains(RetentionKeys(), k) {
			continue
		}
		if err := checkRetention(k, val); err != nil {
			return KeyRetention + "." + k, err
		}
	}
	return "", nil
}

// Read reads the defaults in effect from config.toml and settings.json,
// each checked whole: a file devvm can't read, an unknown key in
// config.toml or a value devvm can't use is an error naming the file
// (state.CorruptError).
func Read(d paths.Dirs) (Defaults, error) {
	config, err := readConfig(d.ConfigFile())
	if err != nil {
		return Defaults{}, err
	}
	saved, err := readSaved(d.Settings())
	if err != nil {
		return Defaults{}, err
	}
	return resolve(config, saved), nil
}

// resolve is the defaults config and saved give, config's winning.
func resolve(config, saved Values) Defaults {
	pick := func(c, s *string, def string) Value {
		switch {
		case c != nil:
			return Value{*c, FromConfig}
		case s != nil:
			return Value{*s, FromSettings}
		}
		return Value{def, FromBuiltin}
	}
	list := func(c, s *[]string) List {
		switch {
		case c != nil:
			return List{slices.Clone(*c), true, FromConfig}
		case s != nil:
			return List{slices.Clone(*s), true, FromSettings}
		}
		return List{Source: FromBuiltin}
	}
	d := Defaults{
		Network:  pick(config.Network, saved.Network, NetworkAsk),
		Ports:    pick(config.Ports, saved.Ports, PortsAll),
		CPUs:     pick(config.Size.CPUs, saved.Size.CPUs, ""),
		Memory:   pick(config.Size.Memory, saved.Size.Memory, ""),
		Disk:     pick(config.Size.Disk, saved.Size.Disk, ""),
		Features: list(config.Features, saved.Features), Agents: list(config.Agents, saved.Agents),
		Layers:    list(config.Layers, saved.Layers),
		Retention: Retention{Raw: map[string]string{}, Sources: map[string]Source{}},
	}
	// Each was checked as it was read.
	for _, k := range RetentionKeys() {
		if v, ok := config.Retention[k]; ok {
			_ = d.Retention.set(k, v)
			d.Retention.Sources[k] = FromConfig
		} else if v, ok := saved.Retention[k]; ok {
			_ = d.Retention.set(k, v)
			d.Retention.Sources[k] = FromSettings
		}
	}
	// What's parsed is kept in its parsed form.
	if p, err := ParsePorts(d.Ports.Value); err == nil {
		d.Ports.Value = p
	}
	if m, err := ParseGiB("memory", d.Memory.Value); err == nil {
		d.Memory.Value = m
	}
	if g, err := ParseGiB("disk", d.Disk.Value); err == nil {
		d.Disk.Value = g
	}
	return d
}

// Save saves change's values in settings.json, keeping what it leaves
// unset, and returns the defaults in effect then. A key config.toml sets
// is refused (set_in_config) before anything is written, as is a value
// devvm can't use; a retention key set to "" is turned off.
func Save(d paths.Dirs, change Values) (Defaults, error) {
	config, err := readConfig(d.ConfigFile())
	if err != nil {
		return Defaults{}, err
	}
	for _, k := range change.Set() {
		if slices.Contains(config.Set(), k) {
			return Defaults{}, msg.SetInConfig(d.ConfigFile(), k)
		}
	}
	if _, err := change.check(true); err != nil {
		return Defaults{}, err
	}
	saved, err := readSaved(d.Settings())
	if err != nil {
		return Defaults{}, err
	}
	saved.merge(change)
	if err := writeSaved(d.Settings(), saved); err != nil {
		return Defaults{}, err
	}
	return resolve(config, saved), nil
}

// merge sets what change sets on v; a retention key set to "" is taken
// out.
func (v *Values) merge(change Values) {
	for i, s := range change.scalars() {
		if *s.p != nil {
			*v.scalars()[i].p = *s.p
		}
	}
	for i, l := range change.lists() {
		if *l.p != nil {
			*v.lists()[i].p = Of(append([]string{}, (**l.p)...)) // never null: chosen, maybe none
		}
	}
	for k, val := range change.Retention {
		if val == "" {
			delete(v.Retention, k)
			continue
		}
		if v.Retention == nil {
			v.Retention = map[string]string{}
		}
		v.Retention[k] = val
	}
}

// Source is where the default key names (KeyNetwork…, or a retention
// setting's under KeyRetention) came from.
func (d Defaults) Source(key string) Source {
	switch key {
	case KeyNetwork:
		return d.Network.Source
	case KeyPorts:
		return d.Ports.Source
	case KeyCPUs:
		return d.CPUs.Source
	case KeyMemory:
		return d.Memory.Source
	case KeyDisk:
		return d.Disk.Source
	case KeyFeatures:
		return d.Features.Source
	case KeyAgents:
		return d.Agents.Source
	case KeyLayers:
		return d.Layers.Source
	}
	if k, ok := cutRetention(key); ok {
		return d.Retention.Source(k)
	}
	return FromBuiltin
}

// cutRetention returns a retention setting's key from its config.toml key.
func cutRetention(key string) (string, bool) {
	k, ok := strings.CutPrefix(key, KeyRetention+".")
	return k, ok && slices.Contains(RetentionKeys(), k)
}
