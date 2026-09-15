package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleYahoo = `{"chart":{"result":[{"meta":{
  "symbol":"AMD","currency":"USD","regularMarketPrice":162.40,"chartPreviousClose":159.52
}}]}}`

func TestFetchQuote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleYahoo))
	}))
	defer server.Close()

	previous := yahooChartBase
	yahooChartBase = server.URL + "/"
	t.Cleanup(func() { yahooChartBase = previous })

	quote, err := fetchQuote(context.Background(), "AMD")
	if err != nil {
		t.Fatal(err)
	}
	if quote.Symbol != "AMD" || quote.Currency != "USD" {
		t.Fatalf("quote = %+v", quote)
	}
	if quote.Price != 162.40 {
		t.Fatalf("price = %v", quote.Price)
	}
	if quote.ChangePct < 1.7 || quote.ChangePct > 1.9 {
		t.Fatalf("change = %v, want ~1.8", quote.ChangePct)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[float64]string{
		512:                       "512 B",
		1536:                      "1.5 KB",
		640 * 1024 * 1024 * 1024:  "640 GB",
		13.1 * 1024 * 1024 * 1024: "13.1 GB",
	}
	for input, want := range cases {
		if got := humanBytes(input); got != want {
			t.Fatalf("humanBytes(%v) = %q, want %q", input, got, want)
		}
	}
}
