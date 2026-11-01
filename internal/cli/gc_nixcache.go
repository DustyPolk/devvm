package cli

import (
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// nixCachePlan is what devvm gc deletes of the shared Nix cache: its least
// recently used files while it's over its cap, or with --nix-cache all of
// them.
type nixCachePlan struct {
	dir   string
	cap   int64 // nix-cache.size, or its default
	limit int64 // what it keeps: its cap, or 0 for all of it
	files int
	size  int64
}

// nixCacheCap is the shared Nix cache's cap as the retention settings
// give it: its default, with a warning, when they can't be read.
func nixCacheCap(u *ui.UI, d paths.Dirs) int64 {
	r, err := state.ReadRetention(d)
	if err != nil {
		u.Warn(msg.RetentionUnreadable().Because(err))
	}
	return r.NixCacheCap()
}

// planNixCache reads what the shared Nix cache holds; all is --nix-cache.
func planNixCache(u *ui.UI, d paths.Dirs, all bool) (nixCachePlan, error) {
	p := nixCachePlan{dir: d.NixCache(), cap: nixCacheCap(u, d)}
	p.limit = p.cap
	if all {
		p.limit = 0
	}
	var err error
	p.files, p.size, err = proxy.NixCacheUsage(p.dir)
	return p, err
}

// any reports whether gc deletes anything of the cache.
func (p nixCachePlan) any() bool { return p.files > 0 && (p.limit == 0 || p.size > p.limit) }

func (p nixCachePlan) show(u *ui.UI) {
	switch {
	case !p.any():
	case p.limit == 0:
		u.Say(msg.GCNixCache(p.files, humanSize(p.size)))
	default:
		u.Say(msg.GCNixCacheOver(humanSize(p.cap), humanSize(p.size)))
	}
}

// run deletes it, under the lock the VMs' proxies take to fill it.
func (p nixCachePlan) run(u *ui.UI) error {
	if !p.any() {
		return nil
	}
	n, freed, err := proxy.TrimNixCache(p.dir, p.limit)
	if err != nil {
		return err
	}
	if n > 0 {
		u.Info(msg.DeletedNixCache(n, humanSize(freed)))
	}
	return nil
}
