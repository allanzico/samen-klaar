package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type AccountLocaleStore interface {
	AccountLocale(context.Context, string) (string, error)
	SetAccountLocale(context.Context, string, string) error
}

func (s *Server) accountLocaleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, ok := s.store.(AccountLocaleStore)
	if !ok {
		http.Error(w, "account preferences are not configured", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	accountID := principalForRequest(r).User.ID
	if r.Method == http.MethodPost {
		var input struct {
			Locale string `json:"locale"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.Locale != "en" && input.Locale != "nl" {
			http.Error(w, "choose Dutch or English", http.StatusBadRequest)
			return
		}
		if err := store.SetAccountLocale(ctx, accountID, input.Locale); err != nil {
			http.Error(w, "account preference could not be saved", http.StatusServiceUnavailable)
			return
		}
	}
	locale, err := store.AccountLocale(ctx, accountID)
	if err != nil {
		http.Error(w, "account preference could not be loaded", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]string{"locale": locale})
}
