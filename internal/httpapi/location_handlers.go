package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type LocationSearch interface {
	Search(context.Context, string, string, bool) ([]workflow.Location, error)
}

type locationSelection struct {
	AccountID string            `json:"accountId"`
	Expires   int64             `json:"expires"`
	Location  workflow.Location `json:"location"`
}

func (s *Server) locationSearchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.locations == nil {
		http.Error(w, "Location search is unavailable. Please try again later.", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	if !s.limiter.allow("locations:"+principal.User.ID, 2, 12, time.Now()) {
		http.Error(w, "Please wait a moment before searching again.", http.StatusTooManyRequests)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 || len(query) > 256 || len(strings.Fields(query)) > 20 || strings.Contains(query, ";") {
		http.Error(w, "Enter an address, postcode or city in 2 to 256 characters.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()
	locations, err := s.locations.Search(ctx, query, r.URL.Query().Get("language"), true)
	if err != nil {
		http.Error(w, "Location search is unavailable. Please try again.", http.StatusBadGateway)
		return
	}
	result := []workflow.Location{}
	for _, location := range locations {
		if !workflow.ValidGeocodedLocation(location) {
			continue
		}
		if r.URL.Query().Get("purpose") == "base" && location.Kind != "address" && location.Kind != "postcode" {
			continue
		}
		if r.URL.Query().Get("purpose") == "address" && location.Kind != "address" {
			continue
		}
		location.Selection = s.signLocation(principal.User.ID, location)
		result = append(result, location)
	}
	writeJSON(w, map[string]any{"locations": result, "attribution": "© Mapbox"})
}

// A short-lived account-bound signature prevents clients from inventing coordinates,
// radii or a country by editing the geocoder's result. No provider token reaches the browser.
func (s *Server) signLocation(accountID string, l workflow.Location) string {
	l.Selection = ""
	data, _ := json.Marshal(locationSelection{accountID, time.Now().Add(time.Hour).Unix(), l})
	mac := hmac.New(sha256.New, s.locationKey)
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) selectedLocation(accountID string, input *workflow.Location) (workflow.Location, error) {
	invalid := errors.New("Select a location from the suggestions again.")
	if input == nil || len(input.Selection) > 8192 {
		return workflow.Location{}, invalid
	}
	parts := strings.Split(input.Selection, ".")
	if len(parts) != 2 {
		return workflow.Location{}, invalid
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return workflow.Location{}, invalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return workflow.Location{}, invalid
	}
	mac := hmac.New(sha256.New, s.locationKey)
	mac.Write(data)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return workflow.Location{}, invalid
	}
	var selected locationSelection
	if json.Unmarshal(data, &selected) != nil || selected.AccountID != accountID || selected.Expires < time.Now().Unix() || !workflow.ValidGeocodedLocation(selected.Location) {
		return workflow.Location{}, invalid
	}
	return selected.Location, nil
}

func newLocationKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("cannot initialize location selections")
	}
	return key
}

func (s *Server) resolveVisitAddress(ctx context.Context, accountID, value string, input *workflow.Location) (workflow.Location, error) {
	if input != nil && input.Selection != "" {
		l, err := s.selectedLocation(accountID, input)
		if err != nil {
			return workflow.Location{}, err
		}
		if l.Kind != "address" || l.Address == "" {
			return workflow.Location{}, errors.New("Choose a full street address for the visit.")
		}
		return l, nil
	}
	if s.locations == nil {
		return workflow.Location{}, errors.New("Location search is unavailable. Please try again later.")
	}
	results, err := s.locations.Search(ctx, value, "en", false)
	if err != nil {
		return workflow.Location{}, errors.New("The visit address could not be checked. Please try again.")
	}
	addresses := []workflow.Location{}
	for _, l := range results {
		if workflow.ValidGeocodedLocation(l) && l.Kind == "address" && l.Address != "" {
			addresses = append(addresses, l)
		}
	}
	if len(addresses) != 1 {
		return workflow.Location{}, errors.New("Choose a full street address from the location suggestions.")
	}
	return addresses[0], nil
}
