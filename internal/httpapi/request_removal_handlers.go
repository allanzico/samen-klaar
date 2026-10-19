package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

// requestRemovalStore deletes a customer's unapproved request, or takes an unapproved job off a
// professional's list. Both rules are checked again inside the store's transaction.
type requestRemovalStore interface {
	RemoveRequest(context.Context, string, string, string) (string, error)
}

// openRequestCounter reports how many of a customer's requests count towards the limit.
type openRequestCounter interface {
	OpenRequestCount(context.Context, string) (int, error)
}

// workspaceHandler serves DELETE /api/workspaces/{id}.
func (s *Server) workspaceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, ok := s.store.(requestRemovalStore)
	if !ok {
		http.Error(w, "request removal is not configured", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	role := string(principal.ActiveRole)
	workspaceID := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var photos []workflow.RequestPhoto
	if principal.ActiveRole == identity.Customer && s.r2 != nil {
		if state, err := s.currentState(ctx, principal.User.ID, role, workspaceID); err == nil {
			photos = state.RequestPhotos
		}
	}
	status, err := store.RemoveRequest(ctx, principal.User.ID, role, workspaceID)
	if err != nil {
		switch {
		case errors.Is(err, workflow.ErrRequestNotDeletable), errors.Is(err, workflow.ErrRequestNotRemovable):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, workflow.ErrRequestNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, "the request could not be removed", http.StatusServiceUnavailable)
		}
		return
	}
	// The customer's own photos of the problem go with the request. Nothing approved had any.
	for _, photo := range photos {
		s.r2.Delete(ctx, requestPhotoNamespace(workspaceID), photo.AssetID)
	}
	writeJSON(w, map[string]string{"status": status})
}

// addOpenRequests adds the customer's open-request count and the limit to a request list
// response, so the app can warn before the limit and explain the block at it.
func (s *Server) addOpenRequests(ctx context.Context, w http.ResponseWriter, principal identity.Principal, response map[string]any) bool {
	counter, ok := s.store.(openRequestCounter)
	if !ok || principal.ActiveRole != identity.Customer {
		return true
	}
	open, err := counter.OpenRequestCount(ctx, principal.User.ID)
	if err != nil {
		http.Error(w, "request list could not be loaded", http.StatusServiceUnavailable)
		return false
	}
	response["openRequests"], response["openRequestLimit"] = open, workflow.MaxOpenRequests
	return true
}
