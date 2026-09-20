package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/allisonhere/tideui/dash"
)

// Render turns one PagePulse snapshot into the document the panel draws.
//
// problem is what went wrong asking, if anything, and comes first: a panel that
// cannot reach PagePulse explains that rather than drawing the zeroes an empty
// snapshot would carry, which read as a real, quiet site.
func Render(snapshot Snapshot, sites []Site, dashboard string, problem error) dash.Doc {
	doc := dash.Doc{SchemaVersion: dash.DocSchemaVersion, Options: siteOptions(sites)}

	if problem != nil {
		doc.Badge = &dash.DocBadge{Text: "offline", Tone: "warning"}
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "text", Label: "PagePulse", Value: problem.Error(), Tone: "warning",
		})
		doc.Rows = appendDashboard(doc.Rows, dashboard)
		return doc
	}

	name := snapshot.Site.Name
	if strings.TrimSpace(name) == "" {
		name = snapshot.Site.Domain
	}

	doc.Badge = &dash.DocBadge{Text: badgeText(snapshot.Live), Tone: badgeTone(snapshot.Live)}
	doc.Rows = append(doc.Rows, dash.Row{
		Type: "metric", Label: "On site", Value: strconv.Itoa(snapshot.Live), Tone: "accent",
	})
	doc.Rows = append(doc.Rows,
		deltaRow("Visitors", snapshot.Summary.Visitors, snapshot.Previous.Visitors),
		deltaRow("Views", snapshot.Summary.PageViews, snapshot.Previous.PageViews),
		deltaRow("Sessions", snapshot.Summary.Sessions, snapshot.Previous.Sessions),
	)

	// The pulse itself. A single bucket is a flat line, which says less than the
	// number beside it already did, so the row only appears when there is a
	// shape to see.
	if history := sparkHistory(snapshot.Trend); len(history) > 1 {
		doc.Rows = append(doc.Rows, dash.Row{
			Type: "spark", Label: "Trend", Value: thousands(snapshot.Summary.Visitors), History: history,
		})
	}

	if len(snapshot.TopPages) > 0 {
		hot := snapshot.TopPages[0]
		doc.Rows = append(doc.Rows,
			dash.Row{Type: "divider", Label: "HOT PAGE"},
			dash.Row{
				Type: "text", Label: hot.Label, Value: counts(hot), Tone: "accent",
				ID: pageURL(snapshot.Site.Domain, hot.Label),
			})
		if rest := snapshot.TopPages[1:]; len(rest) > 0 {
			doc.Rows = append(doc.Rows, dash.Row{Type: "divider", Label: "TOP PAGES"})
			for _, page := range rest {
				doc.Rows = append(doc.Rows, dash.Row{
					Type: "text", Label: page.Label, Value: counts(page),
					ID: pageURL(snapshot.Site.Domain, page.Label),
				})
			}
		}
	}

	if len(snapshot.TopSources) > 0 {
		doc.Rows = append(doc.Rows, dash.Row{Type: "divider", Label: "SOURCES"})
		for _, source := range snapshot.TopSources {
			doc.Rows = append(doc.Rows, dash.Row{
				Type: "text", Label: source.Label, Value: counts(source), Tone: "muted",
			})
		}
	}

	doc.Rows = appendDashboard(doc.Rows, dashboard)
	return doc
}

// deltaRow is a total and how it moved against the period before. A previous
// period with no traffic has no percentage to show, so it shows none rather
// than an infinity dressed up as growth.
func deltaRow(label string, current, previous int) dash.Row {
	value := thousands(current)
	if previous <= 0 {
		return dash.Row{Type: "text", Label: label, Value: value}
	}
	percent := (float64(current) - float64(previous)) / float64(previous) * 100
	arrow, tone := "▲", "good"
	if percent < 0 {
		arrow, tone, percent = "▼", "warning", -percent
	}
	if percent < 1 {
		tone = "muted"
	}
	return dash.Row{
		Type: "text", Label: label,
		Value: fmt.Sprintf("%s  %s%.0f%%", value, arrow, percent), Tone: tone,
	}
}

// counts is how a page or source line reads: what it got and how many looked.
func counts(row TopRow) string {
	return fmt.Sprintf("%s visitors · %s views", thousands(row.Visitors), thousands(row.PageViews))
}

// pageURL composes the tracked page's absolute URL from the site domain and the
// path PagePulse reports, which is what the row's enter key opens.
func pageURL(domain, path string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return ""
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "https://" + domain + path
}

// sparkHistory normalises the trend to the 0..1 samples a spark draws, summing
// adjacent buckets when the period has more of them than a line can show.
func sparkHistory(trend []TrendPoint) []float64 {
	const want = 24
	values := make([]float64, 0, len(trend))
	for _, point := range trend {
		values = append(values, float64(point.Visitors))
	}
	if len(values) == 0 {
		return nil
	}
	if len(values) > want {
		bucketed := make([]float64, want)
		for i, value := range values {
			bucketed[i*want/len(values)] += value
		}
		values = bucketed
	}
	peak := 0.0
	for _, value := range values {
		if value > peak {
			peak = value
		}
	}
	if peak == 0 {
		return nil
	}
	for i := range values {
		values[i] /= peak
	}
	return values
}

// siteOptions offers the site setting every site the account has, so a manifest
// written before the account existed does not leave an empty box to guess in.
func siteOptions(sites []Site) map[string][]string {
	if len(sites) == 0 {
		return nil
	}
	names := make([]string, 0, len(sites)+1)
	names = append(names, "")
	for _, site := range sites {
		if name := strings.TrimSpace(site.Name); name != "" {
			names = append(names, name)
		} else {
			names = append(names, site.Domain)
		}
	}
	return map[string][]string{"site": names}
}

// appendDashboard ends every document with the way into PagePulse itself, where
// the rest of the detail lives. It is the one row that is not a tracked page.
func appendDashboard(rows []dash.Row, dashboard string) []dash.Row {
	if strings.TrimSpace(dashboard) == "" {
		return rows
	}
	return append(rows,
		dash.Row{Type: "spacer"},
		dash.Row{Type: "text", Label: "PagePulse", Value: "open dashboard", ID: dashboard, Tone: "accent"})
}

func badgeText(live int) string {
	if live == 0 {
		return "quiet"
	}
	return strconv.Itoa(live) + " live"
}

func badgeTone(live int) string {
	if live == 0 {
		return "muted"
	}
	return "accent"
}

// thousands groups a count so a five- or six-figure number stays readable.
func thousands(n int) string {
	digits := strconv.Itoa(n)
	negative := strings.HasPrefix(digits, "-")
	if negative {
		digits = digits[1:]
	}
	var out strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(digit)
	}
	if negative {
		return "-" + out.String()
	}
	return out.String()
}
