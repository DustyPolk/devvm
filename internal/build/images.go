package build

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"regexp"
	"slices"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// The states of an image, as devvm images lists them.
const (
	StateCurrent    = "current"    // its key is what its inputs hash to now
	StateStale      = "stale"      // it isn't: nothing new is built on it
	StateUnfinished = "unfinished" // it has no record: a build that didn't finish
)

// Listed is a built image as devvm images lists it.
type Listed struct {
	Image
	State string
	// Origin is what its base started from: the --from image, else
	// Ubuntu's cloud image, with its release's date when known (ubuntu
	// 26.04 (20261001)); "" for an unfinished one.
	Origin string
	// SHA256 is Origin's SHA-256 in hex, "" when unknown; DigestID, its
	// first 12 digits.
	SHA256   string
	DigestID string
	Size     int64 // the disk space it takes, when Sized
	Sized    bool
}

var sha256Hex = regexp.MustCompile(`sha256:([0-9a-f]{12,64})`)

// Listing lists the backend's images by name, those without one last,
// with their state, origin and size.
func (b Images) Listing(ctx context.Context) ([]Listed, error) {
	imgs, err := b.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Listed, 0, len(imgs))
	for _, img := range imgs {
		l := Listed{Image: img, State: StateStale}
		switch {
		case img.Name == "":
			l.State = StateUnfinished
		case img.Current:
			l.State = StateCurrent
		}
		if img.Name != "" {
			l.Origin, l.SHA256 = b.origin(img)
			l.DigestID = l.SHA256[:min(12, len(l.SHA256))]
		}
		size, err := b.Backend.ImageSize(ctx, img.Key)
		if err == nil {
			l.Size, l.Sized = size, true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		out = append(out, l)
	}
	slices.SortFunc(out, func(x, y Listed) int {
		unnamed := func(l Listed) int {
			if l.Name == "" {
				return 1
			}
			return 0
		}
		return cmp.Or(cmp.Compare(unnamed(x), unnamed(y)), cmp.Compare(x.Name, y.Name), cmp.Compare(x.Key, y.Key))
	})
	return out, nil
}

// origin is what an image's base started from, and its digest's SHA-256
// in hex. An image above base on Ubuntu's cloud image records no digest:
// its base's record has it.
func (b Images) origin(img Image) (origin, id string) {
	d := img.Digest
	if d == "" && img.From == "" && len(img.Chain) > 0 && img.Chain[0].Name == image.Base {
		if rec, err := state.ReadImage(b.Dirs, img.Chain[0].Key); err == nil && rec.From == "" {
			d = rec.Digest
		}
	}
	if m := sha256Hex.FindStringSubmatch(d); m != nil {
		id = m[1]
	}
	if img.From != "" {
		return img.From, id
	}
	origin = "ubuntu " + ubuntu.Version
	if date := ubuntu.ReleaseDate(d); date != "" {
		origin += " (" + date + ")"
	}
	return origin, id
}

// keyLike is what devvm images shows for an image's key.
var keyLike = regexp.MustCompile(`^[0-9a-f]{12}$`)

// Delete deletes images, each named by its name or by its key (an
// unfinished build has only its key), with their records. Each must be
// built: nothing is deleted until every one is found. Shared tools aren't
// images, and are refused.
func (b Images) Delete(ctx context.Context, refs []string) error {
	built, err := b.Backend.Images(ctx)
	if err != nil {
		return err
	}
	var keys, labels []string
	for _, ref := range refs {
		key, label := ref, ""
		if keyLike.MatchString(ref) && slices.Contains(built, ref) {
			label = b.recordName(ref)
		} else {
			label = image.Normalize(ref)
			if len(image.ToolsOf(label)) > 0 {
				return msg.ToolsNotImagesRm(label)
			}
			if err := b.CheckLayers(label); err != nil {
				return err
			}
			if key, err = b.Key(label); err != nil {
				return err
			}
			if !slices.Contains(built, key) {
				if b.From != "" {
					return msg.NoImageNamedFrom(b.From, label)
				}
				return msg.NoImageNamed(label)
			}
		}
		if !slices.Contains(keys, key) {
			keys, labels = append(keys, key), append(labels, label)
		}
	}
	for i, key := range keys {
		if err := b.deleteImage(ctx, key); err != nil {
			return err
		}
		if labels[i] == "" {
			b.Info(msg.DeletedUnfinishedBuild(key))
		} else {
			b.Info(msg.DeletedImage(labels[i], key))
		}
	}
	return nil
}

// DeleteKey deletes the image with this key, with its record. The backend
// may refuse it as a message: on Linux hosts, one whose disk VMs use
// (image_in_use).
func (b Images) DeleteKey(ctx context.Context, key string) error {
	return b.deleteImage(ctx, key)
}

// recordName is the name in an image's record, "" when it has none.
func (b Images) recordName(key string) string {
	rec, err := state.ReadImage(b.Dirs, key)
	if err != nil {
		return ""
	}
	return rec.Name
}
