package provider

import (
	"net/url"
	"strings"
)

// NewsSource is a well-known feed offered as a starting point, so a fresh
// install shows headlines before anyone has pasted a URL. Every entry is a
// plain RSS or Atom endpoint that Feed already understands; nothing here is
// privileged, and a hand-typed URL is treated exactly the same way.
type NewsSource struct {
	Name    string
	URL     string
	Topic   string // "world" or "tech", for grouping in a picker
	Default bool   // ticked on a fresh install
}

// newsSources is checked by hand: feed URLs rot (Reuters and AP have both
// dropped their public feeds), so an entry is only listed once it has been
// fetched and parsed. Order is stable and is the order a picker shows.
//
// The list is a convenience, not a compatibility promise — entries may be
// added or retired as feeds move. A URL passed straight to Feed always works
// whether or not it appears here.
var newsSources = []NewsSource{
	{Name: "BBC World", URL: "https://feeds.bbci.co.uk/news/world/rss.xml", Topic: "world", Default: true},
	{Name: "NPR News", URL: "https://feeds.npr.org/1001/rss.xml", Topic: "world"},
	{Name: "Guardian World", URL: "https://www.theguardian.com/world/rss", Topic: "world"},
	{Name: "Al Jazeera", URL: "https://www.aljazeera.com/xml/rss/all.xml", Topic: "world"},
	{Name: "NYT Home", URL: "https://rss.nytimes.com/services/xml/rss/nyt/HomePage.xml", Topic: "world"},
	{Name: "NASA", URL: "https://www.nasa.gov/feed/", Topic: "world"},
	{Name: "Hacker News", URL: "https://news.ycombinator.com/rss", Topic: "tech", Default: true},
	{Name: "Ars Technica", URL: "https://feeds.arstechnica.com/arstechnica/index", Topic: "tech", Default: true},
	{Name: "The Verge", URL: "https://www.theverge.com/rss/index.xml", Topic: "tech"},
	{Name: "Lobsters", URL: "https://lobste.rs/rss", Topic: "tech"},
	{Name: "Slashdot", URL: "https://rss.slashdot.org/Slashdot/slashdotMain", Topic: "tech"},
	{Name: "LWN", URL: "https://lwn.net/headlines/newrss", Topic: "tech"},
	{Name: "Phoronix", URL: "https://www.phoronix.com/rss.php", Topic: "tech"},
}

// NewsSources returns the curated catalogue in display order.
func NewsSources() []NewsSource {
	return append([]NewsSource(nil), newsSources...)
}

// NewsSourceByURL finds a catalogue entry by its URL. Comparison goes through
// CanonicalFeedURL, so a hand-typed URL still matches its catalogue row.
func NewsSourceByURL(rawURL string) (NewsSource, bool) {
	want := CanonicalFeedURL(rawURL)
	if want == "" {
		return NewsSource{}, false
	}
	for _, source := range newsSources {
		if CanonicalFeedURL(source.URL) == want {
			return source, true
		}
	}
	return NewsSource{}, false
}

// DefaultNewsSources returns the sources a fresh install starts with: enough
// for the panel to have something in it, few enough to stay quiet.
func DefaultNewsSources() []NewsSource {
	var out []NewsSource
	for _, source := range newsSources {
		if source.Default {
			out = append(out, source)
		}
	}
	return out
}

// CanonicalFeedURL normalizes a feed URL for comparison. Scheme and host are
// lowercased and http and https are treated alike, but the path is left as
// typed because feed paths are often case-sensitive.
func CanonicalFeedURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return strings.TrimSuffix(strings.ToLower(raw), "/")
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
	path := strings.TrimSuffix(parsed.Path, "/")
	canonical := host + path
	if parsed.RawQuery != "" {
		canonical += "?" + parsed.RawQuery
	}
	return canonical
}
