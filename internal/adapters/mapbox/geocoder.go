// Package mapbox resolves saved marketplace locations using permanent Geocoding v6 results.
package mapbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type Geocoder struct {
	token    string
	client   *http.Client
	endpoint string
}

func New(token string) *Geocoder {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return &Geocoder{token: strings.TrimSpace(token), client: &http.Client{Timeout: 6 * time.Second}, endpoint: "https://api.mapbox.com/search/geocode/v6/forward"}
}

func (g *Geocoder) Search(ctx context.Context, query, language string, autocomplete bool) ([]workflow.Location, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 || len(query) > 256 || len(strings.Fields(query)) > 20 || strings.Contains(query, ";") {
		return nil, errors.New("enter an address, postcode or city in 2 to 256 characters")
	}
	if language != "nl" {
		language = "en"
	}
	values := url.Values{"q": {query}, "access_token": {g.token}, "country": {"nl"}, "language": {language}, "types": {"address,postcode,place,locality,neighborhood"}, "limit": {"5"}, "permanent": {"true"}, "autocomplete": {"false"}, "proximity": {"4.9041,52.3676"}}
	if autocomplete {
		values.Set("autocomplete", "true")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.endpoint+"?"+values.Encode(), nil)
	if err != nil {
		return nil, errors.New("location search is unavailable")
	}
	res, err := g.client.Do(req)
	// Do not return the request URL: it contains the token and the person's query.
	if err != nil {
		return nil, errors.New("location search is unavailable; try again")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("location search is unavailable; try again")
	}
	var body struct {
		Features []struct {
			BBox       []float64 `json:"bbox"`
			Properties struct {
				ID             string `json:"mapbox_id"`
				Kind           string `json:"feature_type"`
				Name           string `json:"name"`
				FullAddress    string `json:"full_address"`
				PlaceFormatted string `json:"place_formatted"`
				Coordinates    struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				} `json:"coordinates"`
				MatchCode struct {
					Confidence string `json:"confidence"`
				} `json:"match_code"`
				Context struct {
					Country struct {
						Code string `json:"country_code"`
					} `json:"country"`
					Place struct {
						Name string `json:"name"`
					} `json:"place"`
					Postcode struct {
						Name string `json:"name"`
					} `json:"postcode"`
					Neighborhood struct {
						Name string `json:"name"`
					} `json:"neighborhood"`
				} `json:"context"`
			} `json:"properties"`
		} `json:"features"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body) != nil {
		return nil, errors.New("location search returned an unreadable response")
	}
	result := []workflow.Location{}
	for _, feature := range body.Features {
		p := feature.Properties
		l := workflow.Location{ID: p.ID, Source: "mapbox", Kind: p.Kind, Label: p.Name, City: p.Context.Place.Name, Postcode: p.Context.Postcode.Name, Country: strings.ToLower(p.Context.Country.Code), Neighborhood: p.Context.Neighborhood.Name, Latitude: p.Coordinates.Latitude, Longitude: p.Coordinates.Longitude, Bounds: feature.BBox}
		if l.Kind == "place" {
			l.City = p.Name
		}
		if l.Kind == "postcode" {
			l.Postcode = p.Name
		}
		if l.Kind == "address" {
			if !autocomplete && (p.MatchCode.Confidence == "low" || p.MatchCode.Confidence == "medium") {
				continue
			}
			l.Address = p.FullAddress
			if l.Address == "" {
				l.Address = p.Name + ", " + p.PlaceFormatted
			}
			l.Label = strings.TrimSpace(l.Postcode + " " + l.City)
			if l.Label == "" {
				continue
			}
		} else if l.City != "" && l.City != l.Label {
			l.Label += ", " + l.City
		}
		if workflow.ValidGeocodedLocation(l) {
			result = append(result, l)
		}
	}
	return result, nil
}
