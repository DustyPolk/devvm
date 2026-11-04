package backend

// CacheEntry is one entry of a backend's own cache that devvm's builds
// put there: what devvm gc --caches frees (Housekeeping.DevvmCache).
type CacheEntry struct {
	What string // what it is, as gc lists it
	Path string // the file or folder
	Size int64
}
