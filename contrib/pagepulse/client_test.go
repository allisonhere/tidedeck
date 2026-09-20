package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A summary arrives whole: the panel draws every field, so the client is checked
// against the exact shape api.php returns rather than a convenient subset.
func TestClientParsesASummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("resource"); got != "summary" {
			t.Errorf("resource = %q, want summary", got)
		}
		if got := r.URL.Query().Get("site"); got != "alliehere.com" {
			t.Errorf("site = %q, want the setting", got)
		}
		if got := r.URL.Query().Get("preset"); got != "7d" {
			t.Errorf("preset = %q, want the setting", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want the bearer key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"schemaVersion": 1,
			"site": {"id": 1, "name": "Allie", "domain": "alliehere.com", "timezone": "Australia/Sydney"},
			"preset": "7d", "rangeLabel": "Last 7 days", "live": 3,
			"summary": {"visitors": 412, "pageViews": 980, "sessions": 510},
			"previous": {"visitors": 380, "pageViews": 900, "sessions": 470},
			"topPages": [
				{"label": "/", "visitors": 120, "pageViews": 300},
				{"label": "/blog", "visitors": 60, "pageViews": 90}
			],
			"topSources": [{"label": "Google Search", "visitors": 150, "pageViews": 200}],
			"trend": [{"label": "Sep 14", "visitors": 40, "pageViews": 90, "sessions": 50}]
		}`))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, Key: "secret", HTTP: server.Client()}
	snapshot, err := client.Summary(context.Background(), "alliehere.com", "7d")
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if snapshot.Live != 3 {
		t.Fatalf("live = %d, want 3", snapshot.Live)
	}
	if snapshot.Summary.Visitors != 412 || snapshot.Previous.PageViews != 900 {
		t.Fatalf("summary/previous did not arrive whole: %+v", snapshot)
	}
	if len(snapshot.TopPages) != 2 || snapshot.TopPages[0].Label != "/" {
		t.Fatalf("top pages did not arrive in order: %+v", snapshot.TopPages)
	}
	if len(snapshot.Trend) != 1 || snapshot.Trend[0].Visitors != 40 {
		t.Fatalf("trend did not arrive: %+v", snapshot.Trend)
	}
}

// Each refusal says what to fix. A bare "401" would leave the reader guessing
// between a key that is wrong and one that was never switched on.
func TestClientReportsItsStatus(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"wrong key", http.StatusUnauthorized, `{"error":"unauthorized"}`, "API key"},
		{"api off", http.StatusServiceUnavailable, `{"error":"api disabled"}`, "switched off"},
		{"unknown site", http.StatusNotFound, `{"error":"unknown site"}`, "no such site"},
		{"no api", http.StatusNotFound, "<html>Not Found</html>", "deployed"},
		{"server error", http.StatusInternalServerError, `{"error":"boom"}`, "500"},
		{"not json", http.StatusOK, "<html>nope</html>", "not a document"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer server.Close()

			client := Client{BaseURL: server.URL, Key: "k", HTTP: server.Client()}
			_, err := client.Summary(context.Background(), "1", "7d")
			if err == nil {
				t.Fatalf("a %d was accepted", c.status)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error %q does not name %q", err, c.wantErr)
			}
		})
	}
}

// A host that is not there is an error, not an empty snapshot: a panel that
// treated it as data would draw a confident zero.
func TestClientPassesThroughANetworkError(t *testing.T) {
	client := Client{BaseURL: "http://127.0.0.1:1", HTTP: &http.Client{}}
	if _, err := client.Summary(context.Background(), "1", "7d"); err == nil {
		t.Fatal("an unreachable host was accepted")
	}
}

// Without a URL there is nothing to ask, and the message says which setting is
// missing rather than building a request for "/api.php".
func TestClientNeedsAURL(t *testing.T) {
	client := Client{}
	if _, err := client.Summary(context.Background(), "1", "7d"); err == nil ||
		!strings.Contains(err.Error(), "URL") {
		t.Fatalf("a blank URL gave %v", err)
	}
}

// The site list is what turns the site setting into a list.
func TestClientListsSites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("resource"); got != "sites" {
			t.Errorf("resource = %q, want sites", got)
		}
		_, _ = w.Write([]byte(`{"schemaVersion":1,"sites":[
			{"id":1,"name":"Allie","domain":"alliehere.com","timezone":"UTC"},
			{"id":2,"name":"Blog","domain":"blog.example.com","timezone":"UTC"}
		]}`))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTP: server.Client()}
	sites, err := client.Sites(context.Background())
	if err != nil {
		t.Fatalf("Sites: %v", err)
	}
	if len(sites) != 2 || sites[1].Name != "Blog" {
		t.Fatalf("sites = %+v", sites)
	}
}
