package provider

import (
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
