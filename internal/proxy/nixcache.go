package proxy

// The shared Nix cache (-nix-cache DIR): the proxy answers http://devvm.proxy/nix/ itself, as a
// binary cache that only the host fills, from cache.nixos.org alone. A VM can only make the host
// fetch: it asks for nix-cache-info, a narinfo or a NAR, by an exact pattern of paths (nixPath), and
// nothing it sends is stored or names a file beyond that pattern. Before keeping a file the host
// checks it: a narinfo's signature against the pinned cache.nixos.org-1 key (parseNarinfo), and a
// NAR's SHA-256 and size against the FileHash and FileSize of a checked narinfo that names it. Each
// VM's own Nix checks the signature again, and the NarHash, which the host can't (xz and zstd aren't
// in the standard library). Files are written whole and renamed in, the user's alone; the cache
// stays under its cap (-nix-cache-max), the least recently used files deleted first, under a lock
// every proxy and devvm gc share. Anything that fails answers 404, so the VM's Nix goes to
// cache.nixos.org itself, through the proxy as any other request.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// NixCacheDefault is the shared Nix cache's cap when no setting gives one: 10 GiB.
const NixCacheDefault int64 = 10 << 30

const (
	nixHost    = "cache.nixos.org"
	nixKeyName = "cache.nixos.org-1"
	// cache.nixos.org-1's Ed25519 public key, as Nix's own default trusted-public-keys has it.
	nixKey = "6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY="
	// nix32Chars is Nix's base-32 alphabet: digits and letters without e, o, u and t.
	nix32Chars = "0123456789abcdfghijklmnpqrsvwxyz"
	// nixInfoMax is as much of a narinfo as is read.
	nixInfoMax = 1 << 20
	// nixCacheInfo is the cache's own nix-cache-info: priority 30 comes before cache.nixos.org's 40.
	nixCacheInfo = "StoreDir: /nix/store\nWantMassQuery: 1\nPriority: 30\n"
	// nixTmpAge is how old a file being written is when a trim takes it for a dead proxy's.
	nixTmpAge = time.Hour
)

var (
	nixCacheDir       = ""              // -nix-cache: the cache's folder; "": no cache
	nixCacheMax       = NixCacheDefault // -nix-cache-max: its cap, in bytes
	nixBase           = "https://" + nixHost
	nixDial           = func(ctx context.Context) (net.Conn, error) { return dialOut(ctx, nixHost, 443) }
	nixFloor          = upstreamAllowed // tests: no DNS
	nixClient         = newNixClient()
	nixMu             sync.Mutex
	nixKnown          = map[string]narMeta{} // NAR path (nar/…) → what a checked narinfo says of it
	nixKnownMax       = 1 << 16
	nixFetching       = map[string]chan struct{}{} // NARs this proxy is fetching, by path
	nixSlots          = make(chan struct{}, nixFetchMax)
	errNixUnsupported = errors.New("not a narinfo devvm reads")
)

// nixFetchMax is how many NARs one proxy fetches at once.
const nixFetchMax = 4

// narMeta is what a checked narinfo says of its NAR's file.
type narMeta struct {
	fileHash string // sha256:<nix32>
	fileSize int64
}

// newNixClient makes the client the cache fetches with: to cache.nixos.org only, through the floor
// or the company proxy (dialOut), with the system's roots and the user's CAs, never following a
// redirect.
func newNixClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:     func(ctx context.Context, _, _ string) (net.Conn, error) { return nixDial(ctx) },
			TLSClientConfig: &tls.Config{RootCAs: upstreamCAs, ServerName: nixHost}, ForceAttemptHTTP2: true,
			IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// --- what a VM may ask for ---

type nixKind int

const (
	nixNone nixKind = iota
	nixInfo
	nixNarinfo
	nixNar
)

func isNix32(s string) bool {
	for i := range len(s) {
		if !strings.ContainsRune(nix32Chars, rune(s[i])) {
			return false
		}
	}
	return s != ""
}

// nixPath is what p, a path under /nix/, asks for: nix-cache-info, <32 nix32>.narinfo, or
// nar/<52 nix32>.nar with no suffix or .xz, .zst or .bz2. Anything else is nixNone.
func nixPath(p string) nixKind {
	if p == "nix-cache-info" {
		return nixInfo
	}
	if h, ok := strings.CutSuffix(p, ".narinfo"); ok && len(h) == 32 && isNix32(h) {
		return nixNarinfo
	}
	rest, ok := strings.CutPrefix(p, "nar/")
	if !ok || len(rest) < 52 || !isNix32(rest[:52]) {
		return nixNone
	}
	switch rest[52:] {
	case ".nar", ".nar.xz", ".nar.zst", ".nar.bz2":
		return nixNar
	}
	return nixNone
}

// nix32 is h in Nix's base 32, as narinfos write hashes.
func nix32(h []byte) string {
	n := (len(h)*8-1)/5 + 1
	out := make([]byte, 0, n)
	for i := n - 1; i >= 0; i-- {
		b := i * 5
		j, k := b/8, b%8
		c := h[j] >> k
		if j+1 < len(h) {
			c |= h[j+1] << (8 - k)
		}
		out = append(out, nix32Chars[c&0x1f])
	}
	return string(out)
}

// --- narinfos ---

// narinfo is what the cache reads of one.
type narinfo struct {
	storePath, url, fileHash, narHash string
	fileSize, narSize                 int64
	refs, sigs                        []string
}

// validName: a store path's name, as Nix takes one.
func validName(s string) bool {
	if s == "" || len(s) > 211 || s[0] == '.' {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune("+-._?=", c) {
			return false
		}
	}
	return true
}

// validBase: a store path's base name, <32 nix32>-<name>.
func validBase(s string) bool {
	return len(s) > 33 && isNix32(s[:32]) && s[32] == '-' && validName(s[33:])
}

func sha256Hash(s string) bool {
	h, ok := strings.CutPrefix(s, "sha256:")
	return ok && len(h) == 52 && isNix32(h)
}

// readNarinfo reads a narinfo's fields, each once but Sig.
func readNarinfo(data []byte) (narinfo, error) {
	var ni narinfo
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if k == "Sig" {
			ni.sigs = append(ni.sigs, v)
			continue
		}
		fields := map[string]*string{"StorePath": &ni.storePath, "URL": &ni.url, "FileHash": &ni.fileHash, "NarHash": &ni.narHash}
		var size *int64
		switch k {
		case "FileSize":
			size = &ni.fileSize
		case "NarSize":
			size = &ni.narSize
		case "References":
			ni.refs = strings.Fields(v)
		default:
			if fields[k] == nil {
				continue // Deriver, Compression, CA and the rest: not checked here
			}
		}
		if seen[k] {
			return ni, fmt.Errorf("%s twice", k)
		}
		seen[k] = true
		if size != nil {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 1 || n > 1<<40 {
				return ni, fmt.Errorf("%s isn't a size", k)
			}
			*size = n
		} else if f := fields[k]; f != nil {
			*f = v
		}
	}
	return ni, nil
}

// parseNarinfo reads a narinfo for the store path whose hash part is hash, and checks it: its
// fields' forms, and a signature by cache.nixos.org-1 over its fingerprint
// (1;StorePath;NarHash;NarSize;References, each reference a full store path, joined by commas).
func parseNarinfo(data []byte, hash string) (narinfo, error) {
	ni, err := readNarinfo(data)
	if err != nil {
		return ni, err
	}
	base, ok := strings.CutPrefix(ni.storePath, "/nix/store/")
	switch {
	case !ok || !validBase(base) || base[:32] != hash:
		return ni, errors.New("its StorePath isn't the path asked for")
	case nixPath(ni.url) != nixNar:
		return ni, fmt.Errorf("%w: its URL isn't nar/<hash>.nar", errNixUnsupported)
	case !sha256Hash(ni.fileHash) || !sha256Hash(ni.narHash):
		return ni, fmt.Errorf("%w: its hashes aren't sha256 in base 32", errNixUnsupported)
	case ni.url[4:56] != ni.narHash[7:] && ni.url[4:56] != ni.fileHash[7:]:
		// The URL isn't signed: a NAR is kept only under a name of its own narinfo's hashes, so
		// no narinfo can put a file in another NAR's place.
		return ni, fmt.Errorf("%w: its URL isn't named by its NarHash or FileHash", errNixUnsupported)
	case ni.fileSize == 0 || ni.narSize == 0:
		return ni, errors.New("a size is missing")
	}
	refs := make([]string, 0, len(ni.refs))
	for _, r := range ni.refs {
		if !validBase(r) {
			return ni, errors.New("a reference isn't a store path")
		}
		refs = append(refs, "/nix/store/"+r)
	}
	slices.Sort(refs)
	fp := "1;" + ni.storePath + ";" + ni.narHash + ";" + strconv.FormatInt(ni.narSize, 10) + ";" + strings.Join(refs, ",")
	key, _ := base64.StdEncoding.DecodeString(nixKey)
	for _, s := range ni.sigs {
		name, sig, _ := strings.Cut(s, ":")
		raw, err := base64.StdEncoding.DecodeString(sig)
		if name == nixKeyName && err == nil && ed25519.Verify(key, []byte(fp), raw) {
			return ni, nil
		}
	}
	return ni, errors.New("no signature by " + nixKeyName + " matches it")
}

// remember what a checked narinfo says of its NAR, for the VM's request for it next.
func remember(ni narinfo) {
	nixMu.Lock()
	defer nixMu.Unlock()
	if len(nixKnown) >= nixKnownMax {
		clear(nixKnown)
	}
	nixKnown[ni.url] = narMeta{ni.fileHash, ni.fileSize}
}

func known(p string) (narMeta, bool) {
	nixMu.Lock()
	defer nixMu.Unlock()
	m, ok := nixKnown[p]
	return m, ok
}

// --- serving ---

// serveNix answers a request under http://devvm.proxy/nix/. Its event says what came of it: a hit,
// fetched, or not found and why (logged, never kept).
func serveNix(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/nix/")
	kind := nixPath(p)
	e := Event{Event: "nix-cache", Host: nixHost, Port: 443, Method: r.Method, Path: r.URL.Path, Agent: r.UserAgent()}
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || r.URL.RawQuery != "" || r.URL.ForceQuery ||
		r.URL.EscapedPath() != r.URL.Path || kind == nixNone {
		e.Reason = "refused: not a request the Nix cache takes"
		record(e)
		http.Error(w, "devvm proxy: not a request the Nix cache takes", http.StatusBadRequest)
		return
	}
	if nixCacheDir == "" {
		http.Error(w, "devvm proxy: no Nix cache", http.StatusNotFound)
		return
	}
	if ok, why := pol.decide(nixHost, 443); !ok {
		e.Reason = "not found: " + nixHost + " isn't allowed (" + why + ")"
		record(e)
		http.Error(w, "devvm proxy: "+nixHost+" isn't allowed", http.StatusNotFound)
		return
	}
	var err error
	switch kind {
	case nixInfo:
		e.Reason = "served"
		serveBytes(w, r, []byte(nixCacheInfo), "text/x-nix-cache-info", time.Time{})
	case nixNarinfo:
		err = serveNarinfo(w, r, p, &e)
	case nixNar:
		err = serveNar(w, r, p, &e)
	case nixNone: // refused above
	}
	if err != nil {
		e.Reason = "not found: " + err.Error()
		http.Error(w, "devvm proxy: not in the Nix cache", http.StatusNotFound)
	}
	record(e)
}

func serveBytes(w http.ResponseWriter, r *http.Request, data []byte, ctype string, mod time.Time) {
	w.Header().Set("Content-Type", ctype)
	http.ServeContent(w, r, "", mod, strings.NewReader(string(data)))
}

// serveNarinfo answers a narinfo: the cache's copy when it still checks, else cache.nixos.org's,
// checked, then kept.
func serveNarinfo(w http.ResponseWriter, r *http.Request, p string, e *Event) error {
	hash := p[:32]
	file := filepath.Join(nixCacheDir, p)
	if data, err := readUpTo(file, nixInfoMax); err == nil {
		if ni, err := parseNarinfo(data, hash); err == nil {
			touch(file)
			remember(ni)
			e.Reason = "hit"
			serveBytes(w, r, data, "text/x-nix-narinfo", time.Time{})
			return nil
		}
		_ = os.Remove(file) //nolint:gosec // a name nixPath checked, in the cache's folder
	}
	if err := nixFloor(r.Context()); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	resp, err := nixGet(ctx, p)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, nixInfoMax+1))
	if err != nil {
		return err
	}
	if len(data) > nixInfoMax {
		return errors.New("its narinfo is too large")
	}
	ni, err := parseNarinfo(data, hash)
	if err != nil {
		return fmt.Errorf("its narinfo is refused: %w", err)
	}
	if err := keepBytes(file, data); err != nil {
		logf("nix cache: %v", err)
	}
	remember(ni)
	e.Reason = "fetched"
	serveBytes(w, r, data, "text/x-nix-narinfo", time.Time{})
	return nil
}

// serveNar answers a NAR: the cache's copy, else cache.nixos.org's when a checked narinfo names it,
// its SHA-256 and size checked against that narinfo's before it's kept and served.
func serveNar(w http.ResponseWriter, r *http.Request, p string, e *Event) error {
	file := filepath.Join(nixCacheDir, filepath.FromSlash(p))
	m, ok := known(p)
	if serveCachedNar(w, r, file, m, ok) {
		e.Reason = "hit"
		return nil
	}
	switch {
	case !ok:
		return errors.New("no checked narinfo names it")
	case m.fileSize > nixCacheMax:
		return errors.New("it's larger than the cache's cap")
	case r.Method == http.MethodHead:
		return errors.New("a HEAD fetches nothing")
	}
	if err := nixFloor(r.Context()); err != nil {
		return err
	}
	if err := fetchOnce(r.Context(), p, file, m); err != nil {
		return err
	}
	if !serveCachedNar(w, r, file, m, ok) {
		return errors.New("it went before it was served")
	}
	e.Reason = "fetched"
	return nil
}

// serveCachedNar serves the cache's copy of a NAR, if it has one. When a narinfo this proxy checked
// names it, a copy of another size isn't that NAR, so it goes.
func serveCachedNar(w http.ResponseWriter, r *http.Request, file string, m narMeta, ok bool) bool {
	f, err := os.Open(file) //nolint:gosec // a name nixPath checked, in the cache's folder
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() || (ok && fi.Size() != m.fileSize) {
		_ = os.Remove(file) //nolint:gosec // a name nixPath checked, in the cache's folder
		return false
	}
	touch(file)
	w.Header().Set("Content-Type", "application/x-nix-nar")
	http.ServeContent(w, r, "", time.Time{}, f)
	return true
}

// fetchOnce fetches a NAR into the cache: one fetch of each at a time in this proxy, which other
// requests for it wait for, and at most nixFetchMax at once.
func fetchOnce(ctx context.Context, p, file string, m narMeta) error {
	nixMu.Lock()
	if ch, busy := nixFetching[p]; busy {
		nixMu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil // whether it's there, the caller sees
	}
	ch := make(chan struct{})
	nixFetching[p] = ch
	nixMu.Unlock()
	defer func() {
		nixMu.Lock()
		delete(nixFetching, p)
		nixMu.Unlock()
		close(ch)
	}()
	select {
	case nixSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-nixSlots }()
	return fetchNar(ctx, p, file, m)
}

// upstreamAllowed: the floor (or the company proxy's) for cache.nixos.org, as for any request.
func upstreamAllowed(ctx context.Context) error {
	if _, why := check(ctx, nixHost, 443, false); why != "" {
		return errors.New(nixHost + " is blocked: " + why)
	}
	return nil
}

// nixGet fetches p from cache.nixos.org: its answer when that's 200. Nothing of the VM's request
// goes with it.
func nixGet(ctx context.Context, p string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nixBase+"/"+p, nil) //nolint:gosec // cache.nixos.org alone, a path nixPath checked
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "devvm-nix-cache")
	resp, err := nixClient.Do(req) //nolint:gosec // as above; its dialer reaches cache.nixos.org alone
	if err != nil {
		logf("nix cache: %s: %v", p, err) // can name addresses the VM shouldn't learn
		return nil, errors.New(nixHost + " couldn't be reached")
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s answered %d", nixHost, resp.StatusCode)
	}
	return resp, nil
}

// fetchNar downloads p into a file of its own in the cache's folder, checks it against m, and
// renames it to file. Its room under the cap is taken first, and the file is made its full size
// at once, so what's being fetched counts against the cap as it's written.
func fetchNar(ctx context.Context, p, file string, m narMeta) error {
	var tmp *os.File
	room := onDisk(m.fileSize)
	err := reserve(room, func() error {
		var err error
		if tmp, err = os.CreateTemp(nixCacheDir, ".tmp-*"); err == nil {
			err = tmp.Truncate(m.fileSize)
		}
		return err
	})
	if tmp != nil {
		defer func() { _ = os.Remove(tmp.Name()) }()
		defer func() { _ = tmp.Close() }()
	}
	if err != nil {
		return err
	}
	kept := false
	defer func() {
		if !kept {
			release(room)
		}
	}()
	resp, err := nixGet(ctx, p)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, m.fileSize+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n != m.fileSize || "sha256:"+nix32(h.Sum(nil)) != m.fileHash {
		return errors.New("its NAR doesn't match the narinfo's FileHash and FileSize")
	}
	if err := os.Rename(tmp.Name(), file); err != nil { //nolint:gosec // a name nixPath checked, in the cache's folder
		return err
	}
	kept = true
	return nil
}

// keepBytes writes data, a narinfo, whole into the cache as file, all under the cache's lock: it's
// small, and no file of it is left for other proxies to count meanwhile.
func keepBytes(file string, data []byte) error {
	n := onDisk(int64(len(data)))
	return reserve(n, func() error {
		err := writeAtomic(file, data, 0o600)
		if err != nil {
			addSize(nixCacheDir, -n)
		}
		return err
	})
}

func readUpTo(file string, limit int64) ([]byte, error) {
	f, err := os.Open(file) //nolint:gosec // a name nixPath checked, in the cache's folder
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, limit))
}

// touch marks a file used now: the trim deletes the least recently used first.
func touch(file string) {
	now := time.Now()
	_ = os.Chtimes(file, now, now)
}

// --- the folder, its size and its cap ---

// openNixCache makes the cache's folders, the user's alone, and the client it fetches with.
func openNixCache() error {
	if nixCacheDir == "" {
		return nil
	}
	if nixCacheMax < 0 {
		return errors.New("-nix-cache-max is below 0")
	}
	for _, d := range []string{nixCacheDir, filepath.Join(nixCacheDir, "nar")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(d, 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
			return err
		}
	}
	nixClient = newNixClient()
	return nil
}

// lockNix holds the cache's lock, which every proxy and devvm gc take to change what's in it.
func lockNix(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // the cache's own lock
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}

// sizeFile records what the cache takes, kept under its lock: a trim counts it again.
func sizeFile(dir string) string { return filepath.Join(dir, ".size") }

func readSize(dir string) (int64, bool) {
	b, err := os.ReadFile(sizeFile(dir))
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	return n, err == nil && n >= 0
}

func writeSize(dir string, n int64) {
	_ = os.WriteFile(sizeFile(dir), []byte(strconv.FormatInt(n, 10)+"\n"), 0o600)
}

// errNixFull is reserve's refusal when the files being fetched alone leave no room.
var errNixFull = errors.New("the cache is full of files being fetched")

// reserve takes n bytes of room under the cache's cap, then runs then, both under the cache's
// lock. When the cache would pass its cap, it first deletes the least recently used files, down
// to nine tenths of it, so most writes count nothing. Files being fetched count but never go, so
// a fetch that would pass the cap with them is refused.
func reserve(n int64, then func() error) error {
	unlock, err := lockNix(nixCacheDir)
	if err != nil {
		return err
	}
	defer unlock()
	total, ok := readSize(nixCacheDir)
	if !ok || total+n > nixCacheMax {
		if total, _, _, err = trim(nixCacheDir, nixCacheMax-n, max(nixCacheMax/10*9-n, 0)); err != nil {
			return err
		}
		if total+n > nixCacheMax {
			writeSize(nixCacheDir, total)
			return errNixFull
		}
	}
	writeSize(nixCacheDir, total+n)
	return then()
}

// release gives back room reserve took for a file that wasn't kept.
func release(n int64) {
	unlock, err := lockNix(nixCacheDir)
	if err != nil {
		return
	}
	defer unlock()
	addSize(nixCacheDir, -n)
}

// addSize changes the running total by n; the caller holds the lock.
func addSize(dir string, n int64) {
	if total, ok := readSize(dir); ok {
		writeSize(dir, max(total+n, 0))
	}
}

// nixFile is one file in the cache.
type nixFile struct {
	path    string
	size    int64 // its size or the disk it takes, whichever is more
	used    time.Time
	writing bool // a fetch's file, being written
}

// onDisk is n bytes rounded up to whole 4 KiB blocks, as a file of n bytes takes them.
func onDisk(n int64) int64 { return (n + 4095) / 4096 * 4096 }

// diskSize is a file's size or the disk its blocks take, whichever is more: a small narinfo takes
// a whole block, and a file being fetched is made its full size before it's written.
func diskSize(fi fs.FileInfo) int64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return max(fi.Size(), st.Blocks*512)
	}
	return fi.Size()
}

// cacheFiles lists the cache's narinfos, NARs and the files fetches are writing, deleting those
// that dead proxies left.
func cacheFiles(dir string) ([]nixFile, error) {
	var out []nixFile
	for _, d := range []string{dir, filepath.Join(dir, "nar")} {
		entries, err := os.ReadDir(d)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		for _, e := range entries {
			fi, err := e.Info()
			if err != nil || !fi.Mode().IsRegular() {
				continue
			}
			p, writing := filepath.Join(d, e.Name()), strings.HasPrefix(e.Name(), ".tmp-")
			if writing && time.Since(fi.ModTime()) > nixTmpAge {
				_ = os.Remove(p)
				continue
			}
			if !writing && strings.HasPrefix(e.Name(), ".") {
				continue
			}
			out = append(out, nixFile{p, diskSize(fi), fi.ModTime(), writing})
		}
	}
	return out, nil
}

// trim counts what the cache takes, the files being fetched included, and when that's over
// limit, deletes the least recently used of the rest until it takes at most target. It returns
// what the cache takes then, and how many files it deleted and what they took.
func trim(dir string, limit, target int64) (int64, int, int64, error) {
	files, err := cacheFiles(dir)
	if err != nil {
		return 0, 0, 0, err
	}
	var total, freed int64
	for _, f := range files {
		total += f.size
	}
	if total <= limit {
		return total, 0, 0, nil
	}
	slices.SortFunc(files, func(a, b nixFile) int { return a.used.Compare(b.used) })
	n := 0
	for _, f := range files {
		if total <= target {
			break
		}
		if f.writing {
			continue
		}
		if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return total, n, freed, err
		}
		total, freed = total-f.size, freed+f.size
		n++
	}
	return total, n, freed, nil
}

// NixCacheUsage is how many files the shared Nix cache in dir holds and the disk they take,
// as its cap counts it.
func NixCacheUsage(dir string) (int, int64, error) {
	files, err := cacheFiles(dir)
	n, size := 0, int64(0)
	for _, f := range files {
		if !f.writing {
			n, size = n+1, size+f.size
		}
	}
	return n, size, err
}

// TrimNixCache deletes the least recently used files of the shared Nix cache in dir until it takes
// at most limit (0: all of them, but those being fetched), under the lock the proxies take, and
// returns how many files it deleted and the disk they took.
func TrimNixCache(dir string, limit int64) (int, int64, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	unlock, err := lockNix(dir)
	if err != nil {
		return 0, 0, err
	}
	defer unlock()
	after, n, freed, err := trim(dir, limit, limit)
	writeSize(dir, after)
	return n, freed, err
}
