package settings

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devvmhq/devvm/internal/state"
)

// The retention settings' keys, as devvm retention takes them and under
// [retention] in config.toml. Every setting is off unless one sets it.
const (
	RetainRunLogs      = "logs.runs"        // run logs kept, the newest; 20 when off
	RetainHistoryAge   = "history.age"      // exports of agents' history older than this are deleted
	RetainHistoryCount = "history.count"    // exports of agents' history kept, the newest
	RetainToolsAge     = "tools.unused-age" // current tool images no VM has for this long are deleted
	RetainOrphansAge   = "orphans.age"      // what devvm keeps of VMs gone from their backend, after this long
	RetainNixCacheSize = "nix-cache.size"   // the shared Nix cache's cap; NixCacheDefault when off
)

// NixCacheDefault is the shared Nix cache's cap when nix-cache.size is
// off: 10 GiB.
const NixCacheDefault int64 = 10 << 30

// RetentionKeys are the retention settings' keys, in the order devvm
// space shows them.
func RetentionKeys() []string {
	return []string{RetainRunLogs, RetainHistoryAge, RetainHistoryCount, RetainToolsAge, RetainOrphansAge, RetainNixCacheSize}
}

// Retention is the retention settings in effect: 0 is off.
type Retention struct {
	RunLogs, ExportsCount            int
	ExportsAge, ToolsAge, OrphansAge time.Duration
	NixCacheSize                     int64
	// Raw is each setting that's on as written, by key; Sources is where
	// each came from, FromBuiltin for one that's off.
	Raw     map[string]string
	Sources map[string]Source
}

// NixCacheCap is the shared Nix cache's cap: nix-cache.size, or
// NixCacheDefault when it's off.
func (r Retention) NixCacheCap() int64 {
	if r.NixCacheSize > 0 {
		return r.NixCacheSize
	}
	return NixCacheDefault
}

// Off reports whether no setting is on.
func (r Retention) Off() bool { return len(r.Raw) == 0 }

// Source is where a retention setting came from: FromBuiltin for one
// that's off.
func (r Retention) Source(key string) Source {
	if s, ok := r.Sources[key]; ok {
		return s
	}
	return FromBuiltin
}

// set sets one setting from its value as written; "" leaves it off.
func (r *Retention) set(key, val string) error {
	if !slices.Contains(RetentionKeys(), key) {
		return fmt.Errorf("unknown key %q", key)
	}
	if val == "" {
		return nil
	}
	var err error
	switch key {
	case RetainRunLogs:
		r.RunLogs, err = count(val)
	case RetainHistoryCount:
		r.ExportsCount, err = count(val)
	case RetainHistoryAge:
		r.ExportsAge, err = state.ParseDuration(val)
	case RetainToolsAge:
		r.ToolsAge, err = state.ParseDuration(val)
	case RetainOrphansAge:
		r.OrphansAge, err = state.ParseDuration(val)
	case RetainNixCacheSize:
		r.NixCacheSize, err = state.ParseSize(val)
	}
	if err != nil {
		return fmt.Errorf("%s=%s: %w", key, val, err)
	}
	if r.Raw == nil {
		r.Raw = map[string]string{}
	}
	r.Raw[key] = val
	return nil
}

// checkRetention refuses a value key can't have.
func checkRetention(key, val string) error { return new(Retention).set(key, val) }

// errCount is a count that isn't a whole number from 1.
var errCount = errors.New("a count is a whole number from 1 to 10000")

func count(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 10000 || strconv.Itoa(n) != s {
		return 0, errCount
	}
	return n, nil
}

// RetainOff is the value that turns a retention setting off.
const RetainOff = "off"

// ParseRetention reads retention settings as devvm retention takes them,
// without reading or writing anything: each KEY=VALUE has a known key and
// a value that key takes, or off. It returns them by key, "" for off.
func ParseRetention(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range pairs {
		key, val, ok := strings.Cut(p, "=")
		switch {
		case !ok:
			return nil, fmt.Errorf("%q: not key=value", p)
		case !slices.Contains(RetentionKeys(), key):
			return nil, fmt.Errorf("unknown key %q", key)
		case val == "":
			return nil, fmt.Errorf("%s= has no value, and %s=%s turns it off", key, key, RetainOff)
		case val == RetainOff:
			out[key] = ""
			continue
		}
		if err := checkRetention(key, val); err != nil {
			return nil, err
		}
		out[key] = val
	}
	return out, nil
}
