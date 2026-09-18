package dash

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// noPlaceholdersForTests points the image transport at a terminal without the
// protocol, so an assertion about half-block cells holds wherever the suite runs:
// TERM_PROGRAM is set by the developer's own terminal and would otherwise decide
// what the document renders.
func noPlaceholdersForTests(t *testing.T) {
	t.Helper()
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("TMUX", "")
}

// writePNG writes a small solid PNG into a fresh directory and returns its path.
func writePNG(t *testing.T, w, h int, c color.RGBA) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "picture.png")
	writePNGAt(t, path, w, h, c)
	return path
}

// writePNGAt writes the same picture to a path that already exists, so a test can
// replace a file under a panel and watch it notice.
func writePNGAt(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

// A path is decoded once and kept while the file is unchanged: a panel that
// re-runs every minute should not decode the same picture every minute.
func TestLoadImageCachesUntilTheFileChanges(t *testing.T) {
	path := writePNG(t, 2, 2, color.RGBA{R: 255, A: 255})

	first, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatal("the same file was decoded twice")
	}

	// Rewriting it must be picked up: this is how a plugin's next frame reaches
	// the screen without the panel knowing it changed.
	writePNGAt(t, path, 2, 2, color.RGBA{B: 255, A: 255})
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	changed, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("a rewritten file returned the cached decode")
	}
}

// What cannot be read is reported, never guessed at.
func TestLoadImageRefusesWhatItCannotRead(t *testing.T) {
	if _, err := loadImage(filepath.Join(t.TempDir(), "absent.png")); err == nil {
		t.Fatal("a missing file loaded")
	}
	path := filepath.Join(t.TempDir(), "not-an-image.png")
	if err := os.WriteFile(path, []byte("this is not a picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadImage(path); err == nil {
		t.Fatal("a file that is not an image loaded")
	}
	// A relative path is refused: a document is data, and data is not resolved
	// against anything.
	if _, err := loadImage("picture.png"); err == nil {
		t.Fatal("a relative path loaded")
	}
}
