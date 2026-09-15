package provider

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/allisonhere/tideui"
)

// Feed builds a news source from one or more RSS or Atom URLs.
func Feed(urls ...string) func(context.Context) ([]tideui.Headline, error) {
	cloned := append([]string(nil), urls...)
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
			response, err := httpClient.Do(request)
			if err != nil {
				failures = append(failures, err.Error())
				continue
			}
			parsed, err := parseFeed(response.Body, time.Now())
			response.Body.Close()
			if err != nil {
				failures = append(failures, err.Error())
				continue
			}
			items = append(items, parsed...)
		}
		if len(items) == 0 && len(failures) > 0 {
			return nil, fmt.Errorf("provider: feeds: %s", strings.Join(failures, "; "))
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].published.After(items[j].published) })
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
}

type rssDocument struct {
	Channel struct {
		Title string `xml:"title"`
		Items []struct {
			Title   string `xml:"title"`
			PubDate string `xml:"pubDate"`
			Date    string `xml:"date"`
			Updated string `xml:"updated"`
		} `xml:"item"`
	} `xml:"channel"`
	Title   string `xml:"title"`
	Entries []struct {
		Title     string `xml:"title"`
		Updated   string `xml:"updated"`
		Published string `xml:"published"`
	} `xml:"entry"`
}

// parseFeed reads either an RSS 2.0 or an Atom document.
func parseFeed(reader io.Reader, now time.Time) ([]feedItem, error) {
	body, err := io.ReadAll(io.LimitReader(reader, 4<<20))
	if err != nil {
		return nil, err
	}
	var document rssDocument
	if err := xml.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	source := firstNonEmpty(document.Channel.Title, document.Title)

	var items []feedItem
	for _, entry := range document.Channel.Items {
		published := firstNonEmpty(entry.PubDate, entry.Updated, entry.Date)
		items = append(items, newFeedItem(entry.Title, source, published, now))
	}
	for _, entry := range document.Entries {
		published := firstNonEmpty(entry.Published, entry.Updated)
		items = append(items, newFeedItem(entry.Title, source, published, now))
	}
	return items, nil
}

func newFeedItem(title, source, published string, now time.Time) feedItem {
	clean := stripHTML(title)
	if clean == "" {
		clean = "(untitled)"
	}
	item := feedItem{headline: tideui.Headline{Title: clean, Source: source, Unread: true, Tone: tideui.ToneAccent}}
	if parsed, ok := parseFeedTime(published); ok {
		item.published = parsed
		item.headline.Age = humanAge(now.Sub(parsed))
	}
	return item
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
