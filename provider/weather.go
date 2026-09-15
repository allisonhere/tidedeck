package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/allisonhere/tideui"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// openMeteoBase is a variable so tests can point Weather at a local server.
var openMeteoBase = "https://api.open-meteo.com/v1/forecast"

// WeatherOptions configures the Open-Meteo weather source.
type WeatherOptions struct {
	Latitude   float64
	Longitude  float64
	Location   string
	Fahrenheit bool // false uses Celsius
	WindMPH    bool // false uses km/h
	Days       int  // forecast days, 1-16; zero uses 5
}

// Weather builds a weather source using the Open-Meteo API, which needs no key.
func Weather(opts WeatherOptions) func(context.Context) (tideui.WeatherData, error) {
	days := opts.Days
	if days <= 0 {
		days = 5
	}
	unit := "celsius"
	unitLabel := "C"
	if opts.Fahrenheit {
		unit = "fahrenheit"
		unitLabel = "F"
	}
	windUnit := "kmh"
	windLabel := "km/h"
	if opts.WindMPH {
		windUnit = "mph"
		windLabel = "mph"
	}
	return func(ctx context.Context) (tideui.WeatherData, error) {
		query := url.Values{}
		query.Set("latitude", fmt.Sprintf("%.4f", opts.Latitude))
		query.Set("longitude", fmt.Sprintf("%.4f", opts.Longitude))
		query.Set("current", "temperature_2m,apparent_temperature,weather_code,wind_speed_10m")
		query.Set("hourly", "temperature_2m,weather_code,precipitation_probability")
		query.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max")
		query.Set("temperature_unit", unit)
		query.Set("wind_speed_unit", windUnit)
		query.Set("timezone", "auto")
		query.Set("forecast_days", fmt.Sprintf("%d", days))

		endpoint := openMeteoBase + "?" + query.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return tideui.WeatherData{}, err
		}
		response, err := httpClient.Do(request)
		if err != nil {
			return tideui.WeatherData{}, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return tideui.WeatherData{}, fmt.Errorf("provider: open-meteo status %d", response.StatusCode)
		}
		var payload openMeteo
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			return tideui.WeatherData{}, err
		}
		return payload.weather(opts.Location, unitLabel, windLabel), nil
	}
}

type openMeteo struct {
	Current struct {
		Time                string  `json:"time"`
		Temperature         float64 `json:"temperature_2m"`
		ApparentTemperature float64 `json:"apparent_temperature"`
		WeatherCode         int     `json:"weather_code"`
		WindSpeed           float64 `json:"wind_speed_10m"`
	} `json:"current"`
	Hourly struct {
		Time          []string  `json:"time"`
		Temperature   []float64 `json:"temperature_2m"`
		WeatherCode   []int     `json:"weather_code"`
		Precipitation []int     `json:"precipitation_probability"`
	} `json:"hourly"`
	Daily struct {
		Time          []string  `json:"time"`
		WeatherCode   []int     `json:"weather_code"`
		High          []float64 `json:"temperature_2m_max"`
		Low           []float64 `json:"temperature_2m_min"`
		Precipitation []int     `json:"precipitation_probability_max"`
	} `json:"daily"`
}

func (p openMeteo) weather(location, unitLabel, windLabel string) tideui.WeatherData {
	data := tideui.WeatherData{
		Location:     location,
		Temperature:  int(round(p.Current.Temperature)),
		Unit:         unitLabel,
		Condition:    conditionForCode(p.Current.WeatherCode),
		FeelsLike:    int(round(p.Current.ApparentTemperature)),
		HasFeelsLike: true,
		WindSpeed:    int(round(p.Current.WindSpeed)),
		WindUnit:     windLabel,
		Updated:      time.Now(),
	}
	if len(p.Daily.High) > 0 {
		data.High = int(round(p.Daily.High[0]))
		data.Low = int(round(p.Daily.Low[0]))
	}
	if len(p.Daily.Precipitation) > 0 {
		data.RainChance = p.Daily.Precipitation[0]
	}
	if len(p.Hourly.Time) > 0 {
		data.Hourly = p.hourly(nowHourIndex(p.Hourly.Time))
	}
	for i, label := range p.Daily.Time {
		if i >= len(p.Daily.High) || i >= len(p.Daily.Low) {
			break
		}
		condition := ""
		if i < len(p.Daily.WeatherCode) {
			condition = conditionForCode(p.Daily.WeatherCode[i])
		}
		data.Daily = append(data.Daily, tideui.ForecastPoint{
			Label:       dayLabelForDaily(label, i),
			Temperature: int(round(p.Daily.High[i])),
			Condition:   condition,
		})
	}
	return data
}

func (p openMeteo) hourly(start int) []tideui.ForecastPoint {
	var points []tideui.ForecastPoint
	for i := start; i < len(p.Hourly.Time) && len(points) < 4; i++ {
		point := tideui.ForecastPoint{Label: hourLabel(p.Hourly.Time[i])}
		if i < len(p.Hourly.Temperature) {
			point.Temperature = int(round(p.Hourly.Temperature[i]))
		}
		if i < len(p.Hourly.WeatherCode) {
			point.Condition = conditionForCode(p.Hourly.WeatherCode[i])
		}
		if i < len(p.Hourly.Precipitation) {
			point.RainChance = p.Hourly.Precipitation[i]
		}
		points = append(points, point)
	}
	return points
}

func nowHourIndex(times []string) int {
	now := time.Now()
	for i, value := range times {
		parsed, err := time.Parse("2006-01-02T15:04", value)
		if err != nil {
			continue
		}
		if !parsed.Before(now) {
			return i
		}
	}
	return max(0, len(times)-1)
}

func hourLabel(value string) string {
	parsed, err := time.Parse("2006-01-02T15:04", value)
	if err != nil {
		return value
	}
	return parsed.Format("3PM")
}

func dayLabelForDaily(value string, index int) string {
	if index == 0 {
		return "Today"
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return parsed.Format("Mon")
}

// conditionForCode maps WMO weather codes to short descriptions.
func conditionForCode(code int) string {
	switch {
	case code == 0:
		return "Clear"
	case code <= 2:
		return "Partly Cloudy"
	case code == 3:
		return "Overcast"
	case code <= 48:
		return "Fog"
	case code <= 55:
		return "Drizzle"
	case code <= 67:
		return "Rain"
	case code <= 77:
		return "Snow"
	case code <= 82:
		return "Showers"
	case code <= 86:
		return "Snow Showers"
	default:
		return "Thunderstorm"
	}
}

func round(value float64) float64 {
	if value < 0 {
		return float64(int(value - 0.5))
	}
	return float64(int(value + 0.5))
}
