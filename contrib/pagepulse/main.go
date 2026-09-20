// Command pagepulse is a tidedeck panel that previews a PagePulse analytics
// install: who is on the site now, the busy page, and where the traffic came
// from. The dashboard runs it two ways: "render" prints a panel document, and
// "open" hands a row's URL to the desktop's browser.
//
// It asks the same JSON endpoint the PagePulse dashboard reads, so the pane and
// the site cannot disagree about the numbers.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/allisonhere/tideui/dash"
)

const usage = `pagepulse - a PagePulse analytics panel

  pagepulse render        print the panel document
  pagepulse sites         print the sites the account has
  pagepulse open <url>    open a http or https URL in the browser
`

// analytics is the two calls the panel makes, so a test can stand in for the
// network without a server.
type analytics interface {
	Summary(ctx context.Context, site, preset string) (Snapshot, error)
	Sites(ctx context.Context) ([]Site, error)
}

// Settings is what the dashboard passes in as TIDEDECK_PLUGIN_* variables.
type Settings struct {
	URL    string
	Key    string
	Site   string
	Preset string
}

func main() {
	client := Client{
		BaseURL: os.Getenv("TIDEDECK_PLUGIN_URL"),
		Key:     os.Getenv("TIDEDECK_PLUGIN_KEY"),
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, client))
}

// run is the program without os.Exit in it, so every verb can be tested with a
// fake client and buffers instead of a network and a terminal.
func run(args []string, stdout, stderr io.Writer, api analytics) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "render":
		return renderPanel(stdout, stderr, settings(), api)
	case "sites":
		return listSites(stdout, stderr, settings(), api)
	case "open":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "pagepulse: open needs a URL")
			return 2
		}
		if err := openURL(args[1]); err != nil {
			fmt.Fprintf(stderr, "pagepulse: %v\n", err)
			return 1
		}
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "pagepulse: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

// settings reads the plugin's settings from the environment, the way every
// plugin is configured. The preset has a default because a blank one is not a
// period PagePulse knows.
func settings() Settings {
	cfg := Settings{
		URL:    os.Getenv("TIDEDECK_PLUGIN_URL"),
		Key:    os.Getenv("TIDEDECK_PLUGIN_KEY"),
		Site:   os.Getenv("TIDEDECK_PLUGIN_SITE"),
		Preset: os.Getenv("TIDEDECK_PLUGIN_PRESET"),
	}
	if strings.TrimSpace(cfg.Preset) == "" {
		cfg.Preset = "7d"
	}
	return cfg
}

// renderPanel asks for a snapshot and prints the document. A missing setting or
// a failed call is drawn rather than returned: the pane has to explain itself,
// and a non-zero exit would leave the last good content up instead.
func renderPanel(stdout, stderr io.Writer, cfg Settings, api analytics) int {
	if err := checkSettings(cfg); err != nil {
		return writeDocument(stdout, stderr, Render(Snapshot{}, nil, dashboardURL(cfg), err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// The site list is best-effort: it only fills the settings screen's list,
	// and losing it must not cost the panel its numbers.
	sites, _ := api.Sites(ctx)
	snapshot, err := api.Summary(ctx, cfg.Site, cfg.Preset)
	return writeDocument(stdout, stderr, Render(snapshot, sites, dashboardURL(cfg), err))
}

// listSites is a debugging convenience: it prints what the site setting's list
// is built from, which is the first question when the wrong site is showing.
func listSites(stdout, stderr io.Writer, cfg Settings, api analytics) int {
	if err := checkSettings(cfg); err != nil {
		fmt.Fprintf(stderr, "pagepulse: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sites, err := api.Sites(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "pagepulse: %v\n", err)
		return 1
	}
	for _, site := range sites {
		fmt.Fprintf(stdout, "%d\t%s\t%s\n", site.ID, site.Name, site.Domain)
	}
	return 0
}

// writeDocument prints a document as one line of JSON, which is what the
// dashboard parses.
func writeDocument(stdout, stderr io.Writer, doc dash.Doc) int {
	encoded, err := json.Marshal(doc)
	if err != nil {
		fmt.Fprintf(stderr, "pagepulse: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}

// checkSettings names the one setting that is missing, because that is the only
// thing the reader can act on.
func checkSettings(cfg Settings) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("no PagePulse URL is set")
	}
	if strings.TrimSpace(cfg.Key) == "" {
		return fmt.Errorf("no PagePulse API key is set")
	}
	return nil
}

// dashboardURL builds the PagePulse page the panel's footer opens, carrying the
// site and period so it lands on what the pane was showing.
func dashboardURL(cfg Settings) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if base == "" {
		return ""
	}
	query := url.Values{}
	if site := strings.TrimSpace(cfg.Site); site != "" {
		query.Set("site", site)
	}
	if preset := strings.TrimSpace(cfg.Preset); preset != "" {
		query.Set("preset", preset)
	}
	if len(query) == 0 {
		return base + "/index.php"
	}
	return base + "/index.php?" + query.Encode()
}

// openURL hands a page or the dashboard to the desktop's opener. Only the web
// schemes are opened, and as a single argv argument rather than through a shell,
// because the URL comes from a document a plugin printed.
func openURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%s is not a URL a browser should open", raw)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return fmt.Errorf("%s is not a URL a browser should open", raw)
		}
	default:
		return fmt.Errorf("only http and https links are opened, not %q", parsed.Scheme)
	}
	return runOpener(raw)
}

// runOpener is a variable so a test can watch where a link goes without opening
// a browser.
var runOpener = func(link string) error {
	return exec.Command("xdg-open", link).Run()
}
