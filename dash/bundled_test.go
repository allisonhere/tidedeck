package dash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The plugins that ship with the app go through the same loader a user-installed
// one does, so they are checked the same way - and here, because a manifest that
// stops validating is a pane that silently disappears for everyone who updates.
func TestBundledPluginsValidateAndWearAColourGlyph(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "contrib"))
	if err != nil {
		t.Skipf("no bundled plugins to check: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("contrib holds no plugins at all")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			manifest, err := LoadManifest(filepath.Join("..", "contrib", entry.Name()))
			if err != nil {
				t.Fatalf("the dashboard cannot load this plugin: %v", err)
			}
			if problems := manifest.Validate(); len(problems) > 0 {
				t.Fatalf("the dashboard would reject this plugin: %s", strings.Join(problems, "; "))
			}
			// A pane's glyph is a colour emoji, two cells wide. One cell is a
			// monochrome symbol from a text font, which reads as a mark beside
			// every other pane rather than as this pane\'s icon.
			if width := ansi.StringWidth(manifest.Panel.Glyph); width != 2 {
				t.Fatalf("glyph %q is %d cells wide, want 2: a colour emoji", manifest.Panel.Glyph, width)
			}
		})
	}
}
