package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/allisonhere/tideui"
	"github.com/allisonhere/tideui/dash"
)

func fixture() Snapshot {
	return Snapshot{
		SchemaVersion: 1,
		Site:          Site{ID: 1, Name: "Allie", Domain: "alliehere.com"},
		Preset:        "7d",
		RangeLabel:    "Last 7 days",
		Live:          3,
		Summary:       Counts{Visitors: 412, PageViews: 980, Sessions: 510},
		Previous:      Counts{Visitors: 380, PageViews: 900, Sessions: 470},
		TopPages: []TopRow{
			{Label: "/", Visitors: 120, PageViews: 300},
			{Label: "/blog/privacy", Visitors: 60, PageViews: 90},
			{Label: "/pricing", Visitors: 30, PageViews: 44},
		},
		TopSources: []TopRow{{Label: "Google Search", Visitors: 150, PageViews: 200}},
		Trend: []TrendPoint{
			{Label: "Sep 14", Visitors: 40},
			{Label: "Sep 15", Visitors: 80},
			{Label: "Sep 16", Visitors: 20},
		},
	}
}

// findRow returns the first row with a label, so a test can assert what a row
// says without depending on its position.
func findRow(t *testing.T, doc dash.Doc, label string) dash.Row {
	t.Helper()
	for _, row := range doc.Rows {
		if row.Label == label {
			return row
		}
	}
	t.Fatalf("no row labelled %q in %#v", label, doc.Rows)
	return dash.Row{}
}

// The panel's job: who is on now, and how the period went.
func TestRenderDrawsThePulse(t *testing.T) {
	doc := Render(fixture(), nil, "https://stats.example/index.php?site=1&preset=7d", nil)

	if doc.Badge == nil || !strings.Contains(doc.Badge.Text, "3") {
		t.Fatalf("badge = %#v, want the live count", doc.Badge)
	}
	live := findRow(t, doc, "On site")
	if live.Type != "metric" || live.Value != "3" {
		t.Fatalf("live row = %#v, want a metric of 3", live)
	}
	visitors := findRow(t, doc, "Visitors")
	if !strings.Contains(visitors.Value, "412") || !strings.Contains(visitors.Value, "▲") {
		t.Fatalf("visitors row = %#v, want the total and a rise", visitors)
	}
	views := findRow(t, doc, "Views")
	if !strings.Contains(views.Value, "980") {
		t.Fatalf("views row = %#v, want the total", views)
	}
}

// A fall in traffic is shown as one, in a warning tone.
func TestRenderShowsAFall(t *testing.T) {
	snapshot := fixture()
	snapshot.Summary.Visitors = 300
	doc := Render(snapshot, nil, "", nil)
	row := findRow(t, doc, "Visitors")
	if !strings.Contains(row.Value, "▼") || row.Tone != "warning" {
		t.Fatalf("a fall drew %#v", row)
	}
}

// The spark is 0..1 samples, downsampled when the period is long.
func TestRenderNormalisesTheSpark(t *testing.T) {
	doc := Render(fixture(), nil, "", nil)
	spark := findRow(t, doc, "Trend")
	if spark.Type != "spark" {
		t.Fatalf("trend row = %#v, want a spark", spark)
	}
	if len(spark.History) != 3 {
		t.Fatalf("history has %d samples, want 3", len(spark.History))
	}
	if spark.History[1] != 1.0 {
		t.Fatalf("the peak sample is %v, want 1", spark.History[1])
	}
	for _, sample := range spark.History {
		if sample < 0 || sample > 1 {
			t.Fatalf("sample %v is outside 0..1", sample)
		}
	}

	snapshot := fixture()
	snapshot.Trend = make([]TrendPoint, 30)
	for i := range snapshot.Trend {
		snapshot.Trend[i].Visitors = i
	}
	long := Render(snapshot, nil, "", nil)
	if got := len(findRow(t, long, "Trend").History); got != 24 {
		t.Fatalf("a 30-bucket trend gave %d samples, want 24", got)
	}
}

// One bucket is a point, not a line, so no spark is drawn for it.
func TestRenderSkipsASparkWithoutAShape(t *testing.T) {
	snapshot := fixture()
	snapshot.Trend = []TrendPoint{{Label: "Today", Visitors: 5}}
	doc := Render(snapshot, nil, "", nil)
	for _, row := range doc.Rows {
		if row.Label == "Trend" {
			t.Fatal("a single-bucket trend drew a spark")
		}
	}
}

// The hot page leads, and every page row carries the URL its enter key opens.
func TestRenderListsHotPages(t *testing.T) {
	doc := Render(fixture(), nil, "", nil)

	hot := findRow(t, doc, "/")
	if hot.ID != "https://alliehere.com/" {
		t.Fatalf("the hot page opens %q", hot.ID)
	}
	blog := findRow(t, doc, "/blog/privacy")
	if blog.ID != "https://alliehere.com/blog/privacy" {
		t.Fatalf("a page row opens %q", blog.ID)
	}
	// The hot page is drawn before the others: findRow would pass either way, so
	// the order is checked directly.
	var order []string
	for _, row := range doc.Rows {
		if row.Label == "/" || row.Label == "/blog/privacy" || row.Label == "/pricing" {
			order = append(order, row.Label)
		}
	}
	if strings.Join(order, "|") != "/|/blog/privacy|/pricing" {
		t.Fatalf("pages are ordered %v, want most-visited first", order)
	}
}

// Sources are context, not destinations, so they carry no id.
func TestRenderListsSources(t *testing.T) {
	doc := Render(fixture(), nil, "", nil)
	source := findRow(t, doc, "Google Search")
	if source.ID != "" {
		t.Fatalf("a source row is openable: %#v", source)
	}
	if source.Tone != "muted" {
		t.Fatalf("a source row is toned %q, want muted", source.Tone)
	}
}

// A failure is explained, never drawn as a quiet site.
func TestRenderOnAFailedCall(t *testing.T) {
	problem := errors.New("PagePulse refused the API key")
	doc := Render(Snapshot{}, nil, "", problem)

	if len(doc.Rows) == 0 || doc.Rows[0].Tone != "warning" {
		t.Fatalf("a failure did not explain itself: %#v", doc.Rows)
	}
	if !strings.Contains(doc.Rows[0].Value, "API key") {
		t.Fatalf("the explanation = %q", doc.Rows[0].Value)
	}
	if doc.Badge == nil || doc.Badge.Tone != "warning" {
		t.Fatalf("badge = %#v, want a warning", doc.Badge)
	}
}

// A missing setting is the same kind of explanation, so it is never a blank pane.
func TestRenderExplainsAMissingSetting(t *testing.T) {
	doc := Render(Snapshot{}, nil, "", errors.New("no PagePulse URL is set"))
	if len(doc.Rows) == 0 || !strings.Contains(doc.Rows[0].Value, "URL") {
		t.Fatalf("a missing URL drew %#v", doc.Rows)
	}
}

// Every document ends with the way into PagePulse, including a failed one.
func TestRenderAlwaysOffersTheDashboard(t *testing.T) {
	dashboard := "https://stats.example/index.php?site=1&preset=7d"
	for name, doc := range map[string]dash.Doc{
		"with data": Render(fixture(), nil, dashboard, nil),
		"failed":    Render(Snapshot{}, nil, dashboard, errors.New("boom")),
	} {
		last := doc.Rows[len(doc.Rows)-1]
		if last.ID != dashboard {
			t.Fatalf("%s: the last row opens %q, want the dashboard", name, last.ID)
		}
	}
}

// The site list becomes the setting's options, blank first so the manifest's
// default still means something.
func TestRenderOffersTheSites(t *testing.T) {
	sites := []Site{{ID: 1, Name: "Allie"}, {ID: 2, Name: "Blog"}}
	doc := Render(fixture(), sites, "", nil)
	options := doc.Options["site"]
	if len(options) != 3 || options[0] != "" || options[1] != "Allie" || options[2] != "Blog" {
		t.Fatalf("site options = %v", options)
	}
}

// Numbers past a thousand are grouped, because a dashboard is read at a glance.
func TestThousands(t *testing.T) {
	cases := map[int]string{0: "0", 42: "42", 999: "999", 1000: "1,000", 12345: "12,345", 1234567: "1,234,567"}
	for in, want := range cases {
		if got := thousands(in); got != want {
			t.Fatalf("thousands(%d) = %q, want %q", in, got, want)
		}
	}
}

// What is drawn is what the reader sees, so the document is put through the
// dashboard's own renderer at a narrow pane rather than merely parsed.
func TestRenderDrawsInAPane(t *testing.T) {
	doc := Render(fixture(), nil, "https://stats.example/index.php?site=1&preset=7d", nil)
	renderer := tideui.NewRenderer(tideui.CatppuccinMocha, tideui.StyleOptions{})
	out := ansi.Strip(dash.RenderDoc(renderer, doc, 28, false))
	for _, want := range []string{"On site", "Visitors", "HOT PAGE", "/blog/privacy"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the pane is missing %q:\n%s", want, out)
		}
	}
}
