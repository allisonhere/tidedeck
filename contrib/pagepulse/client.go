package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Site is one PagePulse site, as api.php's sites resource reports it.
type Site struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Timezone string `json:"timezone"`
}

// Counts is a period's totals: the same three numbers the dashboard shows.
type Counts struct {
	Visitors  int `json:"visitors"`
	PageViews int `json:"pageViews"`
	Sessions  int `json:"sessions"`
}

// TopRow is one line of a top-pages or top-sources list.
type TopRow struct {
	Label     string `json:"label"`
	Visitors  int    `json:"visitors"`
	PageViews int    `json:"pageViews"`
}

// TrendPoint is one bucket of the traffic chart.
type TrendPoint struct {
	Label     string `json:"label"`
	Visitors  int    `json:"visitors"`
	PageViews int    `json:"pageViews"`
	Sessions  int    `json:"sessions"`
}

// Snapshot is what api.php's summary resource returns.
type Snapshot struct {
	SchemaVersion int          `json:"schemaVersion"`
	GeneratedAt   string       `json:"generatedAt"`
	Site          Site         `json:"site"`
	Preset        string       `json:"preset"`
	RangeLabel    string       `json:"rangeLabel"`
	Live          int          `json:"live"`
	Summary       Counts       `json:"summary"`
	Previous      Counts       `json:"previous"`
	TopPages      []TopRow     `json:"topPages"`
	TopSources    []TopRow     `json:"topSources"`
	Trend         []TrendPoint `json:"trend"`
}

// sitesEnvelope is the sites resource's outer shape.
type sitesEnvelope struct {
	SchemaVersion int    `json:"schemaVersion"`
	Sites         []Site `json:"sites"`
}

// Client talks to one PagePulse install.
type Client struct {
	BaseURL string
	Key     string
	HTTP    *http.Client
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 12 * time.Second}
}

// Summary asks for one site's snapshot over a preset period.
func (c Client) Summary(ctx context.Context, site, preset string) (Snapshot, error) {
	params := url.Values{}
	params.Set("resource", "summary")
	if site != "" {
		params.Set("site", site)
	}
	if preset != "" {
		params.Set("preset", preset)
	}
	var snapshot Snapshot
	_, err := c.get(ctx, params, &snapshot)
	return snapshot, err
}

// Sites asks which sites the account has, so the settings screen can offer them
// as a list rather than as a box to guess in.
func (c Client) Sites(ctx context.Context) ([]Site, error) {
	params := url.Values{}
	params.Set("resource", "sites")
	var envelope sitesEnvelope
	_, err := c.get(ctx, params, &envelope)
	return envelope.Sites, err
}

// get performs one request and decodes it. The status is returned alongside the
// error so a caller can tell a refusal from a document that would not parse.
func (c Client) get(ctx context.Context, params url.Values, out any) (int, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return 0, fmt.Errorf("no PagePulse URL is set")
	}
	endpoint, err := url.Parse(base + "/api.php")
	if err != nil {
		return 0, fmt.Errorf("the PagePulse URL is not a URL: %w", err)
	}
	endpoint.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, err
	}
	if c.Key != "" {
		// The header first: a key in the query would land in the host's log.
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, fmt.Errorf("could not reach PagePulse: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("could not read PagePulse's answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, apiError(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return resp.StatusCode, fmt.Errorf("PagePulse answered with something that is not a document: %w", err)
	}
	return resp.StatusCode, nil
}

// apiError turns a non-200 into something a reader can act on: the two key
// states are named rather than left as a bare status, because "not switched on"
// and "wrong key" are fixed in different places.
func apiError(status int, body []byte) error {
	var envelope struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	detail := strings.TrimSpace(envelope.Error)

	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("PagePulse refused the API key")
	case http.StatusServiceUnavailable:
		return fmt.Errorf("PagePulse's read API is switched off (generate a key in its Settings)")
	case http.StatusNotFound:
		// The API's own 404 carries a JSON error and means the site is not
		// there. A bare 404 is the host's page answering instead, which means
		// api.php was never deployed - a different thing to fix.
		if detail != "" {
			return fmt.Errorf("PagePulse has no such site")
		}
		return fmt.Errorf("PagePulse answered 404: api.php is not deployed")
	}
	if detail == "" {
		detail = http.StatusText(status)
	}
	return fmt.Errorf("PagePulse answered %d: %s", status, detail)
}
