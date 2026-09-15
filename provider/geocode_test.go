package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeocodeCity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"name":"Berlin","latitude":52.52,"longitude":13.405,"country":"Germany","admin1":"Berlin"}]}`))
	}))
	defer server.Close()

	previous := geocodeBase
	geocodeBase = server.URL
	t.Cleanup(func() { geocodeBase = previous })

	place, err := Geocode(context.Background(), "Berlin")
	if err != nil {
		t.Fatal(err)
	}
	if place.Name != "Berlin" || place.Latitude != 52.52 || place.Longitude != 13.405 {
		t.Fatalf("place = %+v", place)
	}
	if place.Label() != "Berlin, Germany" {
		t.Fatalf("label = %q", place.Label())
	}
}

func TestGeocodeZipFallback(t *testing.T) {
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer geo.Close()
	zip := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"places":[{"place name":"Beverly Hills","latitude":"34.0901","longitude":"-118.4065","state":"California"}]}`))
	}))
	defer zip.Close()

	prevGeo, prevZip := geocodeBase, zipBase
	geocodeBase, zipBase = geo.URL, zip.URL
	t.Cleanup(func() { geocodeBase, zipBase = prevGeo, prevZip })

	place, err := Geocode(context.Background(), "90210")
	if err != nil {
		t.Fatal(err)
	}
	if place.Name != "Beverly Hills" || place.Latitude != 34.0901 || place.Longitude != -118.4065 {
		t.Fatalf("place = %+v", place)
	}
}

func TestGeocodeNoMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	previous := geocodeBase
	geocodeBase = server.URL
	t.Cleanup(func() { geocodeBase = previous })

	if _, err := Geocode(context.Background(), "Nowhereville"); err == nil {
		t.Fatal("expected an error for an unknown place")
	}
	if _, err := Geocode(context.Background(), "  "); err == nil {
		t.Fatal("expected an error for a blank query")
	}
}

func TestIsPostalCode(t *testing.T) {
	for _, code := range []string{"90210", "10115"} {
		if !isPostalCode(code) {
			t.Fatalf("%q should be a postal code", code)
		}
	}
	for _, code := range []string{"Berlin", "SW1A 1AA", "12"} {
		if isPostalCode(code) {
			t.Fatalf("%q should not be a postal code", code)
		}
	}
}
