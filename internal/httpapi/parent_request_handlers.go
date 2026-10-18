package httpapi

import (
	"context"
	"encoding/json"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"net/http"
	"time"
)

type parentRequestStore interface {
	ParentRequest(context.Context, string, string) (workflow.ParentRequest, error)
	AddRequestJobs(context.Context, string, string, []workflow.JobPlan, string) (workflow.ParentRequest, error)
}

func (s *Server) parentRequestHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := s.store.(parentRequestStore)
	if !ok {
		http.Error(w, "request planning unavailable", 503)
		return
	}
	p := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	workspace := r.URL.Query().Get("workspaceId")
	if r.Method == http.MethodGet {
		parent, err := store.ParentRequest(ctx, p.User.ID, workspace)
		if err != nil {
			http.Error(w, err.Error(), 403)
			return
		}
		writeJSON(w, parent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var input struct {
		Jobs []workflow.JobPlan `json:"jobs"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "invalid job plans", 400)
		return
	}
	parent, err := store.AddRequestJobs(ctx, p.User.ID, workspace, input.Jobs, p.User.Locale)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, parent)
}
