package provider

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
)

// Feed builds a news source from one or more RSS or Atom URLs. Feeds that
// fail are skipped rather than failing the whole set, so one dead URL does
// not empty the panel.
func Feed(urls ...string) func(context.Context) ([]tideui.Headline, error) {
	cloned := dedupeStrings(urls)
	return func(ctx context.Context) ([]tideui.Headline, error) {
		var items []feedItem
		var failures []string
		for _, feedURL := range cloned {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
			if err != nil {
				failures = append(failures, err.Error())
				continue
			}
			request.Header.Set("User-Agent", "tidedeck/1.0 (+https://github.com/allisonhere/tidedeck)")
			request.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, */*;q=0.8")
			response, err := httpClient.Do(request)
			if err != nil {
				failures = append(failures, err.Error())
				continue
			}
			// Check the status before parsing: an error page is usually HTML,
			// and feeding that to the XML decoder reports a parse failure
			// rather than the 404 or 429 that actually happened.
			if response.StatusCode != http.StatusOK {
				response.Body.Close()
				failures = append(failures, fmt.Sprintf("%s: status %d", feedURL, response.StatusCode))
				continue
			}
			parsed, err := parseFeed(response.Body, time.Now())
			response.Body.Close()
			if err != nil {
				// Name the feed: this error is the only diagnostic the app
				// gets, and "XML syntax error" across a dozen sources says
				// nothing about which one broke.
				failures = append(failures, fmt.Sprintf("%s: %v", feedURL, err))
				continue
			}
			// A channel title is often a sentence ("World news | The Guardian"),
			// which crowds out the headline in a narrow panel. A catalogue
			// entry knows the short name to show instead.
			if source, ok := NewsSourceByURL(feedURL); ok {
				for i := range parsed {
					parsed[i].headline.Source = source.Name
				}
			}
			for i := range parsed {
				if parsed[i].headline.Source == "" {
					parsed[i].headline.Source = feedHost(feedURL)
				}
			}
			items = append(items, parsed...)
		}
		if len(items) == 0 && len(failures) > 0 {
			return nil, fmt.Errorf("provider: feeds: %s", strings.Join(failures, "; "))
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].published.After(items[j].published) })
		items = dedupeItems(items)
		if len(items) > 40 {
			items = items[:40]
		}
		headlines := make([]tideui.Headline, 0, len(items))
		for _, item := range items {
			headlines = append(headlines, item.headline)
		}
		return headlines, nil
	}
}

type feedItem struct {
	headline  tideui.Headline
	published time.Time
	// link identifies the story across feeds. It is not shown anywhere, so it
	// stays off the public Headline model until something renders it.
	link string
}

// feedLink covers both spellings of a link: RSS puts the URL in the element
// text, Atom puts it in an href attribute and may list several.
type feedLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Text string `xml:",chardata"`
}

// url returns the link's address whichever way it was written.
func (l feedLink) url() string { return firstNonEmpty(l.Text, l.Href) }

// feedEntry covers an item or entry from any of the three dialects. Go matches
// XML by local name, so "date" also catches <dc:date>.
type feedEntry struct {
	Title     string     `xml:"title"`
	Links     []feedLink `xml:"link"`
	GUID      string     `xml:"guid"`
	About     string     `xml:"about,attr"` // rdf:about, RSS 1.0
	ID        string     `xml:"id"`
	PubDate   string     `xml:"pubDate"`
	Date      string     `xml:"date"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
}

// published picks the first date the entry offers, in the order the dialects
// prefer them.
func (e feedEntry) date() string {
	return firstNonEmpty(e.Published, e.PubDate, e.Updated, e.Date)
}

// identity is what two copies of the same story share. Atom entries can carry
// several links, so the one that points at the story itself wins over an
// "edit" or "replies" relation.
func (e feedEntry) identity() string {
	for _, link := range e.Links {
		if link.Rel == "" || link.Rel == "alternate" {
			if url := link.url(); strings.TrimSpace(url) != "" {
				return url
			}
		}
	}
	for _, link := range e.Links {
		if url := link.url(); strings.TrimSpace(url) != "" {
			return url
		}
	}
	return firstNonEmpty(e.GUID, e.About, e.ID)
}

type rssDocument struct {
	Channel struct {
		Title string      `xml:"title"`
		Items []feedEntry `xml:"item"`
	} `xml:"channel"`
	Title string `xml:"title"`
	// Items at the document root are RSS 1.0 (RDF), where <item> elements are
	// siblings of <channel> rather than children of it. Without this they
	// parse to nothing at all, with no error to explain why.
	RDFItems []feedEntry `xml:"item"`
	Entries  []feedEntry `xml:"entry"`
}

// parseFeed reads an RSS 2.0, RSS 1.0 (RDF) or Atom document.
func parseFeed(reader io.Reader, now time.Time) ([]feedItem, error) {
	body, err := io.ReadAll(io.LimitReader(reader, 4<<20))
	if err != nil {
		return nil, err
	}
	// A byte-order mark or stray whitespace ahead of the declaration makes
	// encoding/xml reject an otherwise valid feed.
	body = bytes.TrimLeft(bytes.TrimPrefix(body, []byte("\xef\xbb\xbf")), " \t\r\n")

	var document rssDocument
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.CharsetReader = charsetReader
	// Feeds use HTML entities that XML does not define; without this a single
	// &nbsp; in one title fails the whole document.
	decoder.Entity = xml.HTMLEntity
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	source := firstNonEmpty(document.Channel.Title, document.Title)

	var items []feedItem
	for _, group := range [][]feedEntry{document.Channel.Items, document.RDFItems, document.Entries} {
		for _, entry := range group {
			items = append(items, newFeedItem(entry, source, now))
		}
	}
	return items, nil
}

// dedupeItems drops repeats of a story that several feeds carry, keeping the
// first (and so, after sorting, the newest) copy.
func dedupeItems(items []feedItem) []feedItem {
	seen := make(map[string]bool, len(items))
	out := items[:0]
	for _, item := range items {
		key := item.link
		if key == "" {
			key = strings.ToLower(item.headline.Title) + "\x00" + item.headline.Source
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func newFeedItem(entry feedEntry, source string, now time.Time) feedItem {
	clean := stripHTML(entry.Title)
	if clean == "" {
		clean = "(untitled)"
	}
	item := feedItem{
		headline: tideui.Headline{Title: clean, Source: source, Unread: true, Tone: tideui.ToneAccent},
		link:     strings.TrimSpace(entry.identity()),
	}
	if parsed, ok := parseFeedTime(entry.date()); ok {
		item.published = parsed
		item.headline.Age = humanAge(now.Sub(parsed))
	}
	return item
}

// dedupeStrings drops repeats while keeping order, so the same feed listed
// twice is fetched once.
func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

// feedHost names a feed by its host when the document carries no title, so a
// headline is still attributable to something.
func feedHost(feedURL string) string {
	parsed, err := url.Parse(feedURL)
	if err != nil || parsed.Host == "" {
		return "feed"
	}
	return strings.TrimPrefix(parsed.Host, "www.")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseFeedTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		time.RFC1123Z, time.RFC1123, time.RFC3339, time.RFC822Z, time.RFC822,
		"Mon, 2 Jan 2006 15:04:05 -0700", "2006-01-02T15:04:05Z", "2006-01-02",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func humanAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// stripHTML removes tags and decodes a few common entities from feed titles.
func stripHTML(value string) string {
	var builder strings.Builder
	inTag := false
	for _, r := range value {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			builder.WriteRune(r)
		}
	}
	text := builder.String()
	replacer := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`,
		"&#39;", "'", "&apos;", "'", "&nbsp;", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(text)), " ")
}
