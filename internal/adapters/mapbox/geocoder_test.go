package mapbox

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGeocodingUsesPermanentDutchResultsAndKeepsAddressesPrivate(t *testing.T) {
	g := New("private-test-token")
	g.client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if q.Get("permanent") != "true" || q.Get("country") != "nl" || q.Get("language") != "nl" || q.Get("autocomplete") != "true" || q.Get("access_token") != "private-test-token" {
			t.Fatalf("incorrect geocoding parameters (token omitted): country=%s permanent=%s language=%s", q.Get("country"), q.Get("permanent"), q.Get("language"))
		}
		body := `{"features":[{"properties":{"mapbox_id":"address-1","feature_type":"address","name":"Damrak 1","full_address":"Damrak 1, 1012 LG Amsterdam, Netherlands","coordinates":{"latitude":52.37,"longitude":4.9},"context":{"country":{"country_code":"NL"},"place":{"name":"Amsterdam"},"postcode":{"name":"1012 LG"}}}},{"bbox":[4.7,52.2,5.1,52.5],"properties":{"mapbox_id":"city-1","feature_type":"place","name":"Amsterdam","coordinates":{"latitude":52.37,"longitude":4.9},"context":{"country":{"country_code":"NL"}}}},{"properties":{"mapbox_id":"outside","feature_type":"place","name":"London","coordinates":{"latitude":51.5,"longitude":-0.1},"context":{"country":{"country_code":"GB"}}}}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	results, err := g.Search(context.Background(), "Damrak", "nl", true)
	if err != nil || len(results) != 2 {
		t.Fatalf("results=%+v error=%v", results, err)
	}
	if strings.Contains(results[0].Label, "Damrak") || results[0].Address == "" || results[0].Postcode != "1012 LG" {
		t.Fatal("exact address leaked into public label or address was lost")
	}
	if results[1].City != "Amsterdam" || len(results[1].Bounds) != 4 {
		t.Fatal("city search lost bounds")
	}
}

func TestGeocodingErrorsNeverExposeTokenOrQuery(t *testing.T) {
	g := New("private-test-token")
	g.client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) { return nil, errors.New(r.URL.String()) })}
	_, err := g.Search(context.Background(), "Private street 12", "en", false)
	if err == nil || strings.Contains(err.Error(), "private-test-token") || strings.Contains(err.Error(), "Private") {
		t.Fatalf("unsafe error: %v", err)
	}
}
