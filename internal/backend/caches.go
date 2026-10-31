package backend

import "context"

// CacheEntry is one entry of a backend's own cache that devvm's builds
// put there: what devvm gc --caches frees.
type CacheEntry struct {
	What string // what it is, as gc lists it
	Path string // the file or folder, "" for one in the backend's engine
	Name string // its name in the engine (a Docker volume's)
	Size int64
}

// CacheCleaner is a backend whose own caches devvm's builds fill, and
// which can tell devvm's share of them apart: entries only devvm's
// builds make, that no image or VM of devvm's needs. Never the user's
// own: a backend that can't tell an entry apart leaves it.
type CacheCleaner interface {
	// DevvmCache lists devvm's share of the backend's caches. It changes
	// nothing.
	DevvmCache(ctx context.Context) ([]CacheEntry, error)
	// DeleteCache deletes entries DevvmCache listed, each checked again
	// first: one that isn't devvm's any more stays.
	DeleteCache(ctx context.Context, entries []CacheEntry) error
}
