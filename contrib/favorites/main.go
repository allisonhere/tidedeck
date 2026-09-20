// Command favorites is a panel that previews a list of links and the form that
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
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/tideui"
)

// usage is what the program says when it does not understand its arguments.
const usage = `favorites - a list of links, with a form to keep it

  favorites render        print the panel document
  favorites open <link>   open that link in the browser
  favorites open add      add a new favorite
  favorites edit <link>   edit the favorite with that link
  favorites edit add      add a new favorite
  favorites path          print the file the list lives in
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
			fmt.Fprintf(stderr, "favorites: %v\n", marshalErr)
			return 1
		}
		fmt.Fprintln(stdout, string(document))
		return 0

	case "open":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "favorites: open needs a link, or 'add' for a new favorite")
			return 2
		}
		if err := openRow(store, args[1]); err != nil {
			fmt.Fprintf(stderr, "favorites: %v\n", err)
			return 1
		}
		return 0

	case "edit":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "favorites: edit needs a link, or 'add' for a new favorite")
			return 2
		}
		if err := edit(store, args[1]); err != nil {
			fmt.Fprintf(stderr, "favorites: %v\n", err)
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
		fmt.Fprintf(stderr, "favorites: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

// openRow is what a row's enter key does. The program that owns the list decides,
// which is why the manifest declares one command for every row: a link goes to the
// browser, and the add row opens a blank form.
func openRow(store Store, id string) error {
	if id == addRowID {
		return runForm(store, addRowID)
	}
	if !openable(id) {
		return fmt.Errorf("%s is not a link a browser should open", id)
	}
	if err := runOpener(id); err != nil {
		return fmt.Errorf("opening %s: %w", id, err)
	}
	return nil
}

// The two things a row's keys do, held as variables so a test can watch them
// without opening a browser and without a terminal.
var (
	// runOpener hands a link to the desktop's opener. argv and never a shell: the
	// link comes out of a file the reader owns, so it is passed as one argument
	// rather than as part of a command line.
	runOpener = func(link string) error {
		return exec.Command("xdg-open", link).Run()
	}
	// runForm opens the form, which is what the add row and the edit key do.
	runForm = edit
)

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
