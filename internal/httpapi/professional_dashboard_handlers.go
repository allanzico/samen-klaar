package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func (s *Server) professionalDashboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, ok := s.store.(interface {
		ProfessionalDashboard(context.Context, string) (workflow.ProfessionalDashboard, error)
	})
	if !ok {
		http.Error(w, "professional overview is not configured", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	stats, err := store.ProfessionalDashboard(ctx, principalForRequest(r).User.ID)
	if err != nil {
		http.Error(w, "professional overview could not be loaded", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, stats)
}
