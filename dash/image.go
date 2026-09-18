package dash

import (
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	// The decoders a plugin is likely to hand over, registered so image.Decode
	// can sniff the format from the file itself rather than trusting a suffix.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// maxImagePixels bounds what a document may make the dashboard decode. A plugin
// that points at a 20000x20000 PNG is a mistake or a weapon, and either way the
// panel should say so rather than allocate it.
const maxImagePixels = 4096 * 4096

// imageEntry is one decoded picture, keyed by the file it came from and the state
// it was in when it was read.
type imageEntry struct {
	modTime time.Time
	size    int64
	image   image.Image
	err     error
}

// images is the decode cache, one per process: the dashboard holds one document
// per panel, so a cache per panel would only decode the same file once per panel.
var images = struct {
	mu      sync.Mutex
	entries map[string]imageEntry
}{entries: map[string]imageEntry{}}

// loadImage reads the picture a document row points at. The path must be
// absolute: a document is data, and data is not resolved against the working
// directory of whatever printed it. The stat is taken every call, so a plugin
// that writes the file a moment after printing its document is picked up on the
// next frame instead of being cached as missing forever.
func loadImage(path string) (image.Image, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("image row: %q is not an absolute path", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	images.mu.Lock()
	entry, seen := images.entries[path]
	images.mu.Unlock()
	if seen && entry.modTime.Equal(info.ModTime()) && entry.size == info.Size() {
		return entry.image, entry.err
	}

	img, err := decodeImage(path)
	images.mu.Lock()
	images.entries[path] = imageEntry{modTime: info.ModTime(), size: info.Size(), image: img, err: err}
	images.mu.Unlock()
	return img, err
}

// decodeImage sniffs the format, refuses an absurd size before allocating it, and
// hands back whatever the file actually is.
func decodeImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, err
	}
	if config.Width*config.Height > maxImagePixels {
		return nil, fmt.Errorf("image row: %s is %dx%d", path, config.Width, config.Height)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(file)
	return img, err
}
