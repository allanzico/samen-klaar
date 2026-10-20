package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type customerProfileStore interface {
	CustomerProfile(context.Context, string) (workflow.CustomerProfile, error)
	SaveCustomerProfile(context.Context, string, workflow.CustomerProfile) (workflow.CustomerProfile, error)
}

var customerPhone = regexp.MustCompile(`^[+0-9 ()-]{6,30}$`)

func (s *Server) customerProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, ok := s.store.(customerProfileStore)
	if !ok {
		http.Error(w, "Customer profiles are unavailable.", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	id := principalForRequest(r).User.ID
	profile, err := store.CustomerProfile(ctx, id)
	if err != nil {
		http.Error(w, "Could not load your profile.", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodPut {
		var input struct {
			DisplayName string             `json:"displayName"`
			Phone       string             `json:"phone"`
			Location    *workflow.Location `json:"location"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			http.Error(w, "Check your profile details.", http.StatusBadRequest)
			return
		}
		input.DisplayName = strings.TrimSpace(input.DisplayName)
		input.Phone = strings.TrimSpace(input.Phone)
		if len(input.DisplayName) < 2 || len(input.DisplayName) > 80 || input.Phone != "" && !customerPhone.MatchString(input.Phone) {
			http.Error(w, "Enter a name of 2–80 characters and a valid phone number.", http.StatusBadRequest)
			return
		}
		if input.Location != nil {
			if input.Location.Selection != "" {
				canonical, err := s.selectedLocation(id, input.Location)
				if err != nil {
					http.Error(w, "Choose your location from the suggestions.", http.StatusBadRequest)
					return
				}
				input.Location = &canonical
			} else if profile.Location != nil && profile.Location.ID == input.Location.ID {
				input.Location = profile.Location
			} else {
				http.Error(w, "Choose your location from the suggestions.", http.StatusBadRequest)
				return
			}
		}
		profile, err = store.SaveCustomerProfile(ctx, id, workflow.CustomerProfile{DisplayName: input.DisplayName, Phone: input.Phone, Location: input.Location})
		if err != nil {
			http.Error(w, "Could not save your profile.", http.StatusServiceUnavailable)
			return
		}
	}
	writeJSON(w, profile)
}
