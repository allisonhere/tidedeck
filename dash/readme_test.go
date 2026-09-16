package dash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/x/ansi"
)

// The README's plugin examples are the format's documentation, so they are
// held to the format: the manifest it shows must validate, and the document it
// shows must render every row it claims. A documented example that no longer
// works is worse than no example.
func TestReadmeExamples(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```json\n(.*?)```").FindAllStringSubmatch(string(data), -1)
	var manifests, docs int
	for _, block := range blocks {
		body := block[1]
		switch {
		case strings.Contains(body, `"entryPoints"`):
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(dir, "render.sh")
			if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{}'\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			manifest, err := LoadManifest(dir)
			if err != nil {
				t.Fatalf("README manifest does not validate: %v", err)
			}
			if manifest.Interval() != 5*time.Minute {
				t.Fatalf("interval = %v", manifest.Interval())
			}
			if got := manifest.Fields(); len(got) != 1 || got[0].Key != "plugins.drbayless.ai-usage.binary" {
				t.Fatalf("fields = %#v", got)
			}
			manifests++
		case strings.Contains(body, `"rows"`):
			var doc Doc
			if err := json.Unmarshal([]byte(body), &doc); err != nil {
				t.Fatalf("README document does not parse: %v", err)
			}
			renderer := tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})
			out := ansi.Strip(RenderDoc(renderer, doc, 40, false))
			for _, want := range []string{"Session", "MEM", "CPU", "Balance", "Credits", "DETAIL"} {
				if !strings.Contains(out, want) {
					t.Fatalf("rendered document is missing %q:\n%s", want, out)
				}
			}
			detail := ansi.Strip(RenderDoc(renderer, doc, 40, true))
			if !strings.Contains(detail, "Claude Pro") {
				t.Fatalf("detail did not render:\n%s", detail)
			}
			docs++
		}
	}
	if manifests != 1 || docs != 1 {
		t.Fatalf("found %d manifests and %d documents in the README", manifests, docs)
	}
}
