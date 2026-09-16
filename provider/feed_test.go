package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const sampleRSS = `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <title>Example News</title>
  <item><title>Linux 6.12 released &amp; ready</title><pubDate>Mon, 14 Sep 2026 09:00:00 +0000</pubDate></item>
  <item><title>Second story</title><pubDate>Mon, 14 Sep 2026 08:00:00 +0000</pubDate></item>
</channel></rss>`

const sampleAtom = `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <entry><title>Atom story</title><updated>2026-09-14T09:30:00Z</updated></entry>
</feed>`

func TestParseFeedRSSAndAtom(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	rss, err := parseFeed(strings.NewReader(sampleRSS), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rss) != 2 {
		t.Fatalf("rss items = %d, want 2", len(rss))
	}
	if rss[0].headline.Title != "Linux 6.12 released & ready" {
		t.Fatalf("title = %q", rss[0].headline.Title)
	}
	if rss[0].headline.Source != "Example News" {
		t.Fatalf("source = %q", rss[0].headline.Source)
	}
	if rss[0].headline.Age != "1h" {
		t.Fatalf("age = %q, want 1h", rss[0].headline.Age)
	}
	if !rss[0].published.After(rss[1].published) {
		t.Fatal("rss items not in newest-first order")
	}

	atom, err := parseFeed(strings.NewReader(sampleAtom), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(atom) != 1 || atom[0].headline.Title != "Atom story" {
		t.Fatalf("atom parse = %+v", atom)
	}
	if atom[0].headline.Source != "Atom Feed" {
		t.Fatalf("atom source = %q", atom[0].headline.Source)
	}
}

func TestStripHTML(t *testing.T) {
	got := stripHTML("<b>Hello</b> &amp; welcome&nbsp;back")
	if got != "Hello & welcome back" {
		t.Fatalf("stripHTML = %q", got)
	}
}

func TestHumanAge(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second: "0m",
		18 * time.Minute: "18m",
		3 * time.Hour:    "3h",
		50 * time.Hour:   "2d",
		-1 * time.Hour:   "0m",
	}
	for duration, want := range cases {
		if got := humanAge(duration); got != want {
			t.Fatalf("humanAge(%v) = %q, want %q", duration, got, want)
		}
	}
}

// sampleRDF is RSS 1.0: <item> elements are siblings of <channel> under
// <rdf:RDF>, not children of it, and dates come from <dc:date>.
const sampleRDF = `<?xml version="1.0" encoding="ISO-8859-1"?>
<rdf:RDF xmlns="http://purl.org/rss/1.0/" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel rdf:about="https://example.org/"><title>RDF News</title></channel>
<item rdf:about="https://example.org/a"><title>RDF story</title><link>https://example.org/a</link><dc:date>2026-09-14T09:00:00+00:00</dc:date></item>
<item rdf:about="https://example.org/b"><title>Older RDF story</title><link>https://example.org/b</link><dc:date>2026-09-14T08:00:00+00:00</dc:date></item>
</rdf:RDF>`

func TestParseFeedRDF(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	items, err := parseFeed(strings.NewReader(sampleRDF), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("rdf items = %d, want 2", len(items))
	}
	if items[0].headline.Title != "RDF story" {
		t.Fatalf("title = %q", items[0].headline.Title)
	}
	if items[0].headline.Source != "RDF News" {
		t.Fatalf("source = %q", items[0].headline.Source)
	}
	if items[0].headline.Age != "1h" {
		t.Fatalf("age = %q, want 1h (dc:date not parsed?)", items[0].headline.Age)
	}
	if items[0].link != "https://example.org/a" {
		t.Fatalf("link = %q", items[0].link)
	}
}

// Declared ISO-8859-1. encoding/xml refuses this outright without a
// CharsetReader, whatever the bytes actually are.
const sampleLatin1 = "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>" +
	"<rss version=\"2.0\"><channel><title>Caf\xe9 Press</title>" +
	"<item><title>Caf\xe9 ferm\xe9</title><pubDate>Mon, 14 Sep 2026 09:00:00 +0000</pubDate></item>" +
	"</channel></rss>"

// Windows-1252 differs from latin-1 only in 0x80-0x9F; 0x92 is a right quote.
const sample1252 = "<?xml version=\"1.0\" encoding=\"windows-1252\"?>" +
	"<rss version=\"2.0\"><channel><title>Quotes</title>" +
	"<item><title>It\x92s fine</title><pubDate>Mon, 14 Sep 2026 09:00:00 +0000</pubDate></item>" +
	"</channel></rss>"

func TestParseFeedLegacyCharsets(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	latin, err := parseFeed(strings.NewReader(sampleLatin1), now)
	if err != nil {
		t.Fatalf("latin-1 feed: %v", err)
	}
	if len(latin) != 1 || latin[0].headline.Title != "Café fermé" {
		t.Fatalf("latin-1 title = %#v", latin)
	}
	if latin[0].headline.Source != "Café Press" {
		t.Fatalf("latin-1 source = %q", latin[0].headline.Source)
	}
	win, err := parseFeed(strings.NewReader(sample1252), now)
	if err != nil {
		t.Fatalf("windows-1252 feed: %v", err)
	}
	if len(win) != 1 || win[0].headline.Title != "It’s fine" {
		t.Fatalf("windows-1252 title = %#v", win)
	}
}

func TestParseFeedTolerance(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	// A byte-order mark before the declaration.
	withBOM, err := parseFeed(strings.NewReader("\xef\xbb\xbf"+sampleRSS), now)
	if err != nil {
		t.Fatalf("BOM feed: %v", err)
	}
	if len(withBOM) != 2 {
		t.Fatalf("BOM feed items = %d, want 2", len(withBOM))
	}
	// HTML entities that XML does not define must not fail the document.
	entities := `<?xml version="1.0"?><rss version="2.0"><channel><title>E</title>
	<item><title>Caf&eacute;&nbsp;stop</title><pubDate>Mon, 14 Sep 2026 09:00:00 +0000</pubDate></item>
	</channel></rss>`
	parsed, err := parseFeed(strings.NewReader(entities), now)
	if err != nil {
		t.Fatalf("entity feed: %v", err)
	}
	if len(parsed) != 1 || parsed[0].headline.Title != "Café stop" {
		t.Fatalf("entity title = %#v", parsed)
	}
}

func TestFeedReportsStatusAndKeepsGoodSources(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("<html><body>slow down</body></html>"))
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleRSS))
	}))
	defer good.Close()

	// A bad source alongside a good one must not empty the panel.
	items, err := Feed(bad.URL, good.URL)(context.Background())
	if err != nil {
		t.Fatalf("mixed sources: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("mixed sources = %d items, want 2", len(items))
	}
	// Alone, it must say what actually happened.
	_, err = Feed(bad.URL)(context.Background())
	if err == nil {
		t.Fatal("expected an error from a 429 source")
	}
	if !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), bad.URL) {
		t.Fatalf("error should name the status and the feed: %v", err)
	}
}

func TestFeedDedupesAcrossSources(t *testing.T) {
	shared := `<?xml version="1.0"?><rss version="2.0"><channel><title>%s</title>
	<item><title>Same wire story</title><link>https://example.org/wire/1</link><pubDate>Mon, 14 Sep 2026 09:00:00 +0000</pubDate></item>
	</channel></rss>`
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, shared, "Paper One")
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, shared, "Paper Two")
	}))
	defer second.Close()

	items, err := Feed(first.URL, second.URL)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("same link from two feeds = %d headlines, want 1", len(items))
	}
	// The same URL listed twice is fetched once.
	items, err = Feed(first.URL, first.URL)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("duplicate URL = %d headlines, want 1", len(items))
	}
}

func TestNewsSourceCatalogue(t *testing.T) {
	sources := NewsSources()
	if len(sources) == 0 {
		t.Fatal("catalogue is empty")
	}
	names, urls := map[string]bool{}, map[string]bool{}
	defaults := 0
	for _, source := range sources {
		if source.Name == "" || source.URL == "" {
			t.Fatalf("incomplete entry: %#v", source)
		}
		if !strings.HasPrefix(source.URL, "https://") {
			t.Fatalf("%s is not https: %s", source.Name, source.URL)
		}
		if names[source.Name] {
			t.Fatalf("duplicate name %q", source.Name)
		}
		if key := CanonicalFeedURL(source.URL); urls[key] {
			t.Fatalf("duplicate URL %q", source.URL)
		} else {
			urls[key] = true
		}
		names[source.Name] = true
		if source.Default {
			defaults++
		}
		if found, ok := NewsSourceByURL(source.URL); !ok || found.Name != source.Name {
			t.Fatalf("%s does not resolve by its own URL", source.Name)
		}
	}
	if defaults == 0 {
		t.Fatal("no default sources: a fresh install would show nothing")
	}
	if len(DefaultNewsSources()) != defaults {
		t.Fatalf("DefaultNewsSources = %d, want %d", len(DefaultNewsSources()), defaults)
	}
	// Mutating the returned slice must not affect the catalogue.
	sources[0].Name = "mutated"
	if NewsSources()[0].Name == "mutated" {
		t.Fatal("NewsSources returns the live catalogue")
	}
}

func TestCanonicalFeedURL(t *testing.T) {
	same := []string{
		"https://example.org/feed",
		"https://example.org/feed/",
		"http://example.org/feed",
		"https://www.example.org/feed",
		"  https://EXAMPLE.org/feed  ",
	}
	want := CanonicalFeedURL(same[0])
	for _, raw := range same[1:] {
		if got := CanonicalFeedURL(raw); got != want {
			t.Fatalf("CanonicalFeedURL(%q) = %q, want %q", raw, got, want)
		}
	}
	// Paths are case-sensitive and must not be folded together.
	if CanonicalFeedURL("https://example.org/Feed") == want {
		t.Fatal("path case should be preserved")
	}
	if CanonicalFeedURL("   ") != "" {
		t.Fatal("blank URL should canonicalize to empty")
	}
}
