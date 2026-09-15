package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Place is a geocoding result.
type Place struct {
	Name      string
	Latitude  float64
	Longitude float64
	Country   string
	Admin     string
}

// Label returns a human-readable "City, Country" label.
func (p Place) Label() string {
	switch {
	case p.Name != "" && p.Country != "":
		return p.Name + ", " + p.Country
	case p.Name != "":
		return p.Name
	default:
		return fmt.Sprintf("%.3f, %.3f", p.Latitude, p.Longitude)
	}
}

// Base URLs are variables so tests can point lookups at a local server.
var (
	geocodeBase = "https://geocoding-api.open-meteo.com/v1/search"
	zipBase     = "https://api.zippopotam.us"
)

// Geocode turns a city name, "City, Country", or US ZIP code into coordinates.
// It uses Open-Meteo's keyless geocoding API first and falls back to
// Zippopotam for numeric postal codes.
func Geocode(ctx context.Context, query string) (Place, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Place{}, errors.New("provider: enter a city or postal code")
	}
	place, err := openMeteoGeocode(ctx, query)
	if err == nil {
		return place, nil
	}
	if isPostalCode(query) {
		if zip, zipErr := zipGeocode(ctx, query); zipErr == nil {
			return zip, nil
		}
	}
	return Place{}, err
}

type openMeteoGeoResponse struct {
	Results []struct {
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Country   string  `json:"country"`
		Admin1    string  `json:"admin1"`
	} `json:"results"`
}

func openMeteoGeocode(ctx context.Context, query string) (Place, error) {
	params := url.Values{}
	params.Set("name", query)
	params.Set("count", "1")
	params.Set("language", "en")
	params.Set("format", "json")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, geocodeBase+"?"+params.Encode(), nil)
	if err != nil {
		return Place{}, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return Place{}, err
	}
	defer response.Body.Close()
	var payload openMeteoGeoResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Place{}, err
	}
	if len(payload.Results) == 0 {
		return Place{}, fmt.Errorf("provider: no match for %q", query)
	}
	result := payload.Results[0]
	return Place{
		Name:      result.Name,
		Latitude:  result.Latitude,
		Longitude: result.Longitude,
		Country:   result.Country,
		Admin:     result.Admin1,
	}, nil
}

type zipResponse struct {
	Places []struct {
		Name      string `json:"place name"`
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
		State     string `json:"state"`
	} `json:"places"`
}

// zipGeocode looks up a US ZIP code through Zippopotam. Non-numeric postal
// codes are handled by the main geocoder instead.
func zipGeocode(ctx context.Context, code string) (Place, error) {
	if !isPostalCode(code) || len(code) != 5 {
		return Place{}, fmt.Errorf("provider: %q is not a US ZIP code", code)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, zipBase+"/us/"+url.PathEscape(code), nil)
	if err != nil {
		return Place{}, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return Place{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Place{}, fmt.Errorf("provider: no match for ZIP %q", code)
	}
	var payload zipResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Place{}, err
	}
	if len(payload.Places) == 0 {
		return Place{}, fmt.Errorf("provider: no match for ZIP %q", code)
	}
	place := payload.Places[0]
	latitude, err1 := strconv.ParseFloat(place.Latitude, 64)
	longitude, err2 := strconv.ParseFloat(place.Longitude, 64)
	if err1 != nil || err2 != nil {
		return Place{}, fmt.Errorf("provider: bad coordinates for ZIP %q", code)
	}
	return Place{Name: place.Name, Latitude: latitude, Longitude: longitude, Admin: place.State}, nil
}

func isPostalCode(query string) bool {
	if len(query) < 4 || len(query) > 10 {
		return false
	}
	for _, r := range query {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
