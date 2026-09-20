// Command plugincatalog writes the machine-readable catalogue of the plugins
// bundled with the app, so a site can discover and describe them without
// re-reading every manifest itself.
//
// Run it from the repository root. The test in the dash package fails whenever
// the catalogue and the manifests drift, so this is the only thing that writes
// the file:
//
//	go run ./cmd/plugincatalog
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/allisonhere/tideui/dash"
)

func main() {
	contrib := flag.String("contrib", "contrib", "directory holding one plugin per subdirectory")
	repository := flag.String("repository", dash.CatalogRepository, "repository URL the install sources are built from")
	out := flag.String("out", filepath.Join("plugins", "index.json"), "catalogue to write")
	flag.Parse()

	catalog, err := dash.BuildCatalog(*repository, *contrib)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plugincatalog: %v\n", err)
		os.Exit(1)
	}
	data, err := catalog.Encode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "plugincatalog: %v\n", err)
		os.Exit(1)
	}
	if dir := filepath.Dir(*out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "plugincatalog: %v\n", err)
			os.Exit(1)
		}
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "plugincatalog: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d plugins)\n", *out, len(catalog.Plugins))
}
