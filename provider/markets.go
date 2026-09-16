package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/allisonhere/tideui"
)

// yahooChartBase is a variable so tests can point Markets at a local server.
var yahooChartBase = "https://query1.finance.yahoo.com/v8/finance/chart/"

// Markets builds a watchlist source using the public Yahoo Finance chart
// endpoint, which needs no key. It is best-effort: symbols that fail are
// skipped, and an error is returned only when nothing succeeds.
func Markets(symbols ...string) func(context.Context) ([]tideui.MarketQuote, error) {
	cloned := append([]string(nil), symbols...)
	return func(ctx context.Context) ([]tideui.MarketQuote, error) {
		var quotes []tideui.MarketQuote
		var firstErr error
		for _, symbol := range cloned {
			quote, err := fetchQuote(ctx, symbol)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			quotes = append(quotes, quote)
		}
		if len(quotes) == 0 && firstErr != nil {
			return nil, firstErr
		}
		return quotes, nil
	}
}

type yahooChart struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol             string  `json:"symbol"`
				Currency           string  `json:"currency"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				PreviousClose      float64 `json:"previousClose"`
			} `json:"meta"`
			Indicators struct {
				Quote []struct {
					High []float64 `json:"high"`
					Low  []float64 `json:"low"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
	} `json:"chart"`
}

func fetchQuote(ctx context.Context, symbol string) (tideui.MarketQuote, error) {
	endpoint := fmt.Sprintf("%s%s?range=1d&interval=1d", yahooChartBase, url.PathEscape(symbol))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tideui.MarketQuote{}, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; tidedeck/1.0)")
	response, err := httpClient.Do(request)
	if err != nil {
		return tideui.MarketQuote{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return tideui.MarketQuote{}, fmt.Errorf("provider: yahoo %s status %d", symbol, response.StatusCode)
	}
	var payload yahooChart
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return tideui.MarketQuote{}, err
	}
	if len(payload.Chart.Result) == 0 {
		return tideui.MarketQuote{}, fmt.Errorf("provider: no quote for %s", symbol)
	}
	meta := payload.Chart.Result[0].Meta
	previous := meta.ChartPreviousClose
	if previous == 0 {
		previous = meta.PreviousClose
	}
	change := 0.0
	if previous > 0 {
		change = (meta.RegularMarketPrice - previous) / previous * 100
	}
	quoteSymbol := meta.Symbol
	if quoteSymbol == "" {
		quoteSymbol = symbol
	}
	var high, low float64
	if len(payload.Chart.Result[0].Indicators.Quote) > 0 {
		quote := payload.Chart.Result[0].Indicators.Quote[0]
		if len(quote.High) > 0 {
			high = quote.High[len(quote.High)-1]
		}
		if len(quote.Low) > 0 {
			low = quote.Low[len(quote.Low)-1]
		}
	}
	return tideui.MarketQuote{
		Symbol:    quoteSymbol,
		Price:     meta.RegularMarketPrice,
		High:      high,
		Low:       low,
		ChangePct: change,
		Currency:  meta.Currency,
	}, nil
}
