package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleOpenMeteo = `{
  "current": {"time":"2026-09-14T15:00","temperature_2m":72.4,"apparent_temperature":70.2,"weather_code":2,"wind_speed_10m":9.1},
  "hourly": {
    "time":["2026-09-14T15:00","2026-09-14T16:00","2026-09-14T17:00","2026-09-14T18:00"],
    "temperature_2m":[72.4,71.0,69.5,66.2],
    "weather_code":[2,2,3,61],
    "precipitation_probability":[10,15,20,25]
  },
  "daily": {
    "time":["2026-09-14","2026-09-15"],
    "weather_code":[2,61],
    "temperature_2m_max":[76.0,71.0],
    "temperature_2m_min":[61.0,58.0],
    "precipitation_probability_max":[12,80]
  }
}`

func TestWeatherProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleOpenMeteo))
	}))
	defer server.Close()

	previous := openMeteoBase
	openMeteoBase = server.URL
	t.Cleanup(func() { openMeteoBase = previous })

	fetch := Weather(WeatherOptions{Fahrenheit: true, WindMPH: true, Location: "Testville", Days: 2})
	data, err := fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if data.Temperature != 72 || data.Unit != "F" || data.Condition != "Partly Cloudy" {
		t.Fatalf("current = %+v", data)
	}
	if data.High != 76 || data.Low != 61 {
		t.Fatalf("high/low = %d/%d", data.High, data.Low)
	}
	if data.WindSpeed != 9 || data.WindUnit != "mph" {
		t.Fatalf("wind = %d %s", data.WindSpeed, data.WindUnit)
	}
	if !data.HasFeelsLike || data.FeelsLike != 70 {
		t.Fatalf("feels-like = %d (has=%v), want 70", data.FeelsLike, data.HasFeelsLike)
	}
	if len(data.Hourly) == 0 {
		t.Fatal("no hourly forecast")
	}
	if data.Hourly[0].Condition == "" {
		t.Fatalf("hourly point has no condition: %+v", data.Hourly[0])
	}
	if len(data.Daily) != 2 || data.Daily[0].Label != "Today" {
		t.Fatalf("daily = %+v", data.Daily)
	}
}

func TestConditionForCode(t *testing.T) {
	cases := map[int]string{
		0: "Clear", 2: "Partly Cloudy", 3: "Overcast", 45: "Fog",
		61: "Rain", 71: "Snow", 80: "Showers", 95: "Thunderstorm",
	}
	for code, want := range cases {
		if got := conditionForCode(code); got != want {
			t.Fatalf("conditionForCode(%d) = %q, want %q", code, got, want)
		}
	}
}
