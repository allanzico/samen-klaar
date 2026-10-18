package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

// agentMandateHandler reads or replaces the mandate of the signed-in customer or professional.
// Saving is always the person's own act: an agent can never widen its own authority.
func (s *Server) agentMandateHandler(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		http.Error(w, "agents are not available", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	role := string(principal.ActiveRole)
	switch r.Method {
	case http.MethodGet:
		mandate, err := s.agents.AgentMandate(r.Context(), principal.User.ID, role)
		if err != nil {
			http.Error(w, "could not load your agent settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, mandate)
	case http.MethodPut:
		if membership, ok := principal.Membership(principal.ActiveRole); !ok || membership.Status != "active" {
			http.Error(w, "workspace access is not active", http.StatusForbidden)
			return
		}
		var mandate workflow.AgentMandate
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&mandate); err != nil {
			http.Error(w, "invalid agent settings", http.StatusBadRequest)
			return
		}
		saved, err := s.agents.SaveAgentMandate(r.Context(), principal.User.ID, role, mandate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Settle requests that were waiting on this mandate without making the person wait for it.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			s.SweepAgents(ctx)
		}()
		writeJSON(w, saved)
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// agentActivityHandler lists what the signed-in person's agent did on one request.
func (s *Server) agentActivityHandler(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		writeJSON(w, []workflow.AgentActivity{})
		return
	}
	principal := principalForRequest(r)
	workspaceID := r.URL.Query().Get("workspaceId")
	if workspaceID == "" {
		http.Error(w, "choose a request", http.StatusBadRequest)
		return
	}
	// Loading the workspace proves this person is a party to it.
	if _, err := s.currentState(r.Context(), principal.User.ID, string(principal.ActiveRole), workspaceID); err != nil {
		http.Error(w, "request was not found", http.StatusNotFound)
		return
	}
	items, err := s.agents.AgentActivity(r.Context(), workspaceID, principal.User.ID, string(principal.ActiveRole), 30)
	if err != nil {
		http.Error(w, "could not load agent activity", http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

// agentEscalationsHandler lists open questions the person's agent could not decide alone.
func (s *Server) agentEscalationsHandler(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		writeJSON(w, []workflow.AgentEscalation{})
		return
	}
	principal := principalForRequest(r)
	items, err := s.agents.AgentEscalations(r.Context(), principal.User.ID, string(principal.ActiveRole))
	if err != nil {
		http.Error(w, "could not load agent questions", http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}
