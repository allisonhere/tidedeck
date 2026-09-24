// Command tideplug checks a TideDeck plugin before it is installed: that its
// manifest is valid, that its program runs and prints a document the panel can
// draw, and that the rows, programs and settings the manifest promises are
// there. It runs the plugin exactly as the dashboard would, with every setting
// at its default.
//
//	go run ./cmd/tideplug validate path/to/plugin [more plugins...]
//
// It exits non-zero when any plugin has a failure; warnings alone do not.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/allisonhere/tideui/dash"
)

const usage = `usage: tideplug validate PLUGIN-DIR [PLUGIN-DIR...]

Checks each plugin: the manifest, its entry point and open/edit programs,
a real run of the panel with default settings, and the document it prints.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "validate" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	status := 0
	for i, dir := range args[1:] {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintln(stdout, dir)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		findings := dash.CheckPlugin(ctx, dir)
		cancel()
		for _, f := range findings {
			mark := "  ✓ "
			switch f.Level {
			case dash.Warn:
				mark = "  ! "
			case dash.Fail:
				mark = "  ✗ "
			}
			fmt.Fprintln(stdout, mark+f.Text)
		}
		if dash.Failed(findings) {
			status = 1
		}
	}
	return status
}
