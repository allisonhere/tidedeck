package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeAPI stands in for PagePulse so the verbs can be driven without a server.
type fakeAPI struct {
	snapshot Snapshot
	err      error
	sites    []Site
	sitesErr error
}

func (f fakeAPI) Summary(context.Context, string, string) (Snapshot, error) {
	return f.snapshot, f.err
}

func (f fakeAPI) Sites(context.Context) ([]Site, error) {
	return f.sites, f.sitesErr
}

// setSettings points the plugin at a PagePulse and a key, the way the dashboard
// does with environment variables.
func setSettings(t *testing.T) {
	t.Helper()
	t.Setenv("TIDEDECK_PLUGIN_URL", "https://stats.example")
	t.Setenv("TIDEDECK_PLUGIN_KEY", "secret")
	t.Setenv("TIDEDECK_PLUGIN_SITE", "Allie")
	t.Setenv("TIDEDECK_PLUGIN_PRESET", "7d")
}

// What render prints is what the dashboard parses, so its shape is asserted:
// one line, one JSON document, the schema version the dashboard reads.
func TestRunRenderPrintsADocument(t *testing.T) {
	setSettings(t)
	api := fakeAPI{snapshot: fixture(), sites: []Site{{ID: 1, Name: "Allie"}}}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"render"}, &stdout, &stderr, api); code != 0 {
		t.Fatalf("render exited %d: %s", code, stderr.String())
	}
	if lines := strings.Count(strings.TrimSpace(stdout.String()), "\n"); lines != 0 {
		t.Fatalf("render printed %d extra lines: %q", lines, stdout.String())
	}
	var doc struct {
		SchemaVersion int `json:"schemaVersion"`
		Rows          []struct {
			Label string `json:"label"`
			ID    string `json:"id"`
		} `json:"rows"`
		Options map[string][]string `json:"options"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("render printed something that is not a document: %v", err)
	}
	if doc.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", doc.SchemaVersion)
	}
	if len(doc.Rows) == 0 {
		t.Fatal("render drew no rows")
	}
	if len(doc.Options["site"]) == 0 {
		t.Fatal("render offered no sites for the settings screen")
	}
}

// A missing setting is a document that says so, with a zero exit: a non-zero
// exit would keep the last good content up and hide the message.
func TestRunRenderExplainsAMissingSetting(t *testing.T) {
	t.Setenv("TIDEDECK_PLUGIN_URL", "")
	t.Setenv("TIDEDECK_PLUGIN_KEY", "secret")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"render"}, &stdout, &stderr, fakeAPI{}); code != 0 {
		t.Fatalf("render exited %d, want a document that explains it", code)
	}
	var doc struct {
		Rows []struct {
			Value string `json:"value"`
			Tone  string `json:"tone"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("the explanation is not a document: %v", err)
	}
	if len(doc.Rows) == 0 || doc.Rows[0].Tone != "warning" {
		t.Fatalf("a missing URL did not explain itself: %q", stdout.String())
	}
	if !strings.Contains(doc.Rows[0].Value, "URL") {
		t.Fatalf("the explanation = %q, want it to name the setting", doc.Rows[0].Value)
	}
}

// A missing key is its own message, because it is fixed in a different place.
func TestRunRenderExplainsAMissingKey(t *testing.T) {
	t.Setenv("TIDEDECK_PLUGIN_URL", "https://stats.example")
	t.Setenv("TIDEDECK_PLUGIN_KEY", "")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"render"}, &stdout, &stderr, fakeAPI{}); code != 0 {
		t.Fatalf("render exited %d", code)
	}
	if !strings.Contains(stdout.String(), "API key") {
		t.Fatalf("render did not name the key: %q", stdout.String())
	}
}

// A failed call is drawn, with the error's own words.
func TestRunRenderDrawsAFailedCall(t *testing.T) {
	setSettings(t)
	api := fakeAPI{err: errTest}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"render"}, &stdout, &stderr, api); code != 0 {
		t.Fatalf("render exited %d on a failed call", code)
	}
	if !strings.Contains(stdout.String(), "unreachable") {
		t.Fatalf("the failure did not reach the document: %q", stdout.String())
	}
}

var errTest = testError("PagePulse unreachable")

type testError string

func (e testError) Error() string { return string(e) }

// The sites verb prints what the settings list is built from.
func TestRunSitesPrintsTheList(t *testing.T) {
	setSettings(t)
	api := fakeAPI{sites: []Site{{ID: 1, Name: "Allie", Domain: "alliehere.com"}}}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"sites"}, &stdout, &stderr, api); code != 0 {
		t.Fatalf("sites exited %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "alliehere.com") {
		t.Fatalf("sites printed %q", stdout.String())
	}
}

// A row's enter key hands a web link to the desktop's opener, as one argument.
func TestRunOpenHandsALinkToTheOpener(t *testing.T) {
	var opened []string
	defer swapOpener(func(link string) error {
		opened = append(opened, link)
		return nil
	})()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"open", "https://alliehere.com/"}, &stdout, &stderr, nil); code != 0 {
		t.Fatalf("open exited %d: %s", code, stderr.String())
	}
	if len(opened) != 1 || opened[0] != "https://alliehere.com/" {
		t.Fatalf("the opener was given %v, want the link alone", opened)
	}
}

// A scheme no browser should be handed never reaches the opener, and the
// refusal names it.
func TestRunOpenRefusesNonWebSchemes(t *testing.T) {
	for _, link := range []string{"javascript:alert(1)", "file:///etc/passwd"} {
		t.Run(link, func(t *testing.T) {
			opened := false
			defer swapOpener(func(string) error {
				opened = true
				return nil
			})()

			var stdout, stderr bytes.Buffer
			if code := run([]string{"open", link}, &stdout, &stderr, nil); code != 1 {
				t.Fatalf("opening %s exited %d, want 1", link, code)
			}
			if opened {
				t.Fatalf("%s reached the opener", link)
			}
			scheme := strings.SplitN(link, ":", 2)[0]
			if !strings.Contains(stderr.String(), scheme) {
				t.Fatalf("the refusal does not name %s: %q", scheme, stderr.String())
			}
		})
	}
}

// Misuse is a usage message and a non-zero exit.
func TestRunRefusesWhatItCannotDo(t *testing.T) {
	for _, args := range [][]string{nil, {"open"}, {"frobnicate"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr, nil); code == 0 {
			t.Fatalf("%v exited 0", args)
		}
		if !strings.Contains(stderr.String(), "pagepulse") {
			t.Fatalf("%v said nothing useful: %q", args, stderr.String())
		}
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr, nil); code != 0 {
		t.Fatalf("help exited %d", code)
	}
	if !strings.Contains(stdout.String(), "pagepulse render") {
		t.Fatalf("help does not list the verbs: %q", stdout.String())
	}
}

// swapOpener lets a test watch where a link goes without opening a browser.
func swapOpener(replacement func(string) error) func() {
	previous := runOpener
	runOpener = replacement
	return func() { runOpener = previous }
}
