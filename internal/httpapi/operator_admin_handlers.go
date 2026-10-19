package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type operatorAdminStore interface {
	OperatorUsers(context.Context, string, workflow.DirectoryFilter) (workflow.AdminUserPage, error)
	OperatorUser(context.Context, string, string) (workflow.AdminUserDetail, error)
	AdministerUser(context.Context, string, string, workflow.AdminUserAction) error
	OperatorLocations(context.Context, string, string, string, int) (workflow.AdminLocationPage, error)
}

func operatorDirectoryFilter(r *http.Request) (workflow.DirectoryFilter, error) {
	q := r.URL.Query()
	filter := workflow.DirectoryFilter{Query: q.Get("q"), Role: q.Get("role"), Status: q.Get("status"), Location: q.Get("location")}
	if q.Get("offset") != "" {
		var err error
		filter.Offset, err = strconv.Atoi(q.Get("offset"))
		if err != nil {
			return filter, errors.New("invalid directory page")
		}
	}
	return filter, filter.Validate()
}

func adminResponseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workflow.ErrAdminForbidden):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, workflow.ErrAdminNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, workflow.ErrAdminConflict), errors.Is(err, workflow.ErrAdminProtected), errors.Is(err, workflow.ErrAdminUnrestorable):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, "Administration is temporarily unavailable. Please try again.", http.StatusServiceUnavailable)
	}
}

func (s *Server) operatorUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filter, err := operatorDirectoryFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	store, ok := s.store.(operatorAdminStore)
	if !ok {
		adminResponseError(w, errors.New("unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := store.OperatorUsers(ctx, principalForRequest(r).User.ID, filter)
	if err != nil {
		adminResponseError(w, err)
		return
	}
	writeJSON(w, page)
}

func (s *Server) operatorUserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, ok := s.store.(operatorAdminStore)
	if !ok {
		adminResponseError(w, errors.New("unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	operatorID, accountID := principalForRequest(r).User.ID, r.PathValue("id")
	if r.Method == http.MethodPost {
		var input workflow.AdminUserAction
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid account action", http.StatusBadRequest)
			return
		}
		if err := input.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.AdministerUser(ctx, operatorID, accountID, input); err != nil {
			adminResponseError(w, err)
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
		return
	}
	detail, err := store.OperatorUser(ctx, operatorID, accountID)
	if err != nil {
		adminResponseError(w, err)
		return
	}
	writeJSON(w, detail)
}

func (s *Server) operatorLocationsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Location status has its own vocabulary; account role/status cannot alter this query.
	query := r.URL.Query()
	status := query.Get("status")
	if status != "" && status != "available" && status != "uncovered" && status != "waitlisted" {
		http.Error(w, "invalid coverage status", http.StatusBadRequest)
		return
	}
	copyRequest := r.Clone(r.Context())
	copyURL := *r.URL
	query.Del("status")
	query.Del("role")
	copyURL.RawQuery = query.Encode()
	copyRequest.URL = &copyURL
	filter, err := operatorDirectoryFilter(copyRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	store, ok := s.store.(operatorAdminStore)
	if !ok {
		adminResponseError(w, errors.New("unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := store.OperatorLocations(ctx, principalForRequest(r).User.ID, filter.Query, status, filter.Offset)
	if err != nil {
		adminResponseError(w, err)
		return
	}
	writeJSON(w, page)
}
