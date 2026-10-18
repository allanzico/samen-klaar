package httpapi

import (
	"context"
	"encoding/json"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"net/http"
	"time"
)

type coordinationStore interface {
	ListCoordinationTasks(context.Context, string, string, string) ([]workflow.CoordinationTask, error)
	ReplyCoordinationTask(context.Context, string, string, string, string) error
}

func (s *Server) coordinationHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := s.store.(coordinationStore)
	if !ok {
		http.Error(w, "coordination unavailable", 503)
		return
	}
	p := principalForRequest(r)
	workspace := r.URL.Query().Get("workspaceId")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if r.Method == http.MethodPost {
		var input struct{ TaskID, Reply string }
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid coordination response", 400)
			return
		}
		if err := store.ReplyCoordinationTask(ctx, p.User.ID, workspace, input.TaskID, input.Reply); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
	} else if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	tasks, err := store.ListCoordinationTasks(ctx, p.User.ID, string(p.ActiveRole), workspace)
	if err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	writeJSON(w, map[string]any{"tasks": tasks})
}
