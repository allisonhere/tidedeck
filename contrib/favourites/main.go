// Command favourites is a panel that previews a list of links and the form that
// keeps it. The dashboard runs it two ways: "render" prints a panel document,
// and "edit" opens the form with the terminal handed over, which is what the
// pane's enter key does for a row.
//
// The list is a plain JSON file the reader can also edit by hand, so the form is
// a convenience rather than the only way in.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
)

// usage is what the program says when it does not understand its arguments.
const usage = `favourites - a list of links, with a form to keep it

  favourites render        print the panel document
  favourites edit <link>   edit the favourite with that link
  favourites edit add      add a new favourite
  favourites path          print the file the list lives in
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the program without os.Exit in it, so every path of it can be tested
// without a terminal.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	// The path setting arrives the way every other plugin setting does, as an
	// environment variable named after the setting. Blank means the default
	// location, which is what most people will use.
	store := Store{Path: ResolvePath(os.Getenv("TIDEDECK_PLUGIN_PATH"))}

	switch args[0] {
	case "render":
		list, err := store.Load()
		// A list that cannot be read is still drawn: the document explains it.
		// Failing here would leave the panel showing its last good content
		// forever, which looks like a panel that is merely quiet.
		document, marshalErr := json.Marshal(Render(list, err))
		if marshalErr != nil {
			fmt.Fprintf(stderr, "favourites: %v\n", marshalErr)
			return 1
		}
		fmt.Fprintln(stdout, string(document))
		return 0

	case "edit":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "favourites: edit needs a link, or 'add' for a new favourite")
			return 2
		}
		if err := edit(store, args[1]); err != nil {
			fmt.Fprintf(stderr, "favourites: %v\n", err)
			return 1
		}
		return 0

	case "path":
		fmt.Fprintln(stdout, store.Path)
		return 0

	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0

	default:
		fmt.Fprintf(stderr, "favourites: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

// edit runs the form with the terminal in hand. The dashboard hands it over for
// exactly this: the program that owns the data is the program that changes it.
func edit(store Store, id string) error {
	form, err := newEditor(store, id)
	if err != nil {
		return err
	}
	// The alternate screen, because a form is a place the reader goes and comes
	// back from: the dashboard they left is still there underneath it.
	_, err = tea.NewProgram(&screen{editor: form, renderer: themeRenderer()}, tea.WithAltScreen()).Run()
	return err
}

// themeRenderer is the theme the form draws in.
//
// A plugin program cannot ask the dashboard what theme it resolved - the setting
// it reads would have to be duplicated here, and a plugin that guessed wrong
// would draw a form in colours the rest of the screen is not using. So it draws
// in the default theme and its README says so; a mismatch is cosmetic and not
// worth each plugin reimplementing a theme resolver for.
func themeRenderer() tideui.Renderer {
	return tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})
}
