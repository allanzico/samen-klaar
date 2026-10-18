package httpapi

import (
	"context"
	"encoding/json"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"io"
	"net/http"
	"time"
)

type caseDetailStore interface {
	OperatorCaseDetail(context.Context, string, string, string) (workflow.OperatorCaseDetail, error)
	UpdateOperatorCase(context.Context, string, string, string, string, string, string, string) error
	ParticipantCaseNotes(context.Context, string, string, string) ([]workflow.CaseNote, error)
}
type evidencePolicyStore interface {
	EvidencePolicy(context.Context, string) (workflow.ServiceEvidencePolicy, error)
	ProposeEvidencePolicy(context.Context, string, []workflow.EvidenceRequirement) (workflow.ServiceEvidencePolicy, error)
	ReviewEvidencePolicy(context.Context, string, string, bool) (workflow.ServiceEvidencePolicy, error)
	ListEvidencePolicies(context.Context) ([]workflow.ServiceEvidencePolicy, error)
}

func (s *Server) operatorCaseDetailHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := s.store.(caseDetailStore)
	if !ok {
		http.Error(w, "operator case records are unavailable", 503)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	op := principalForRequest(r).User.ID
	workspace := r.PathValue("workspaceId")
	kind := r.URL.Query().Get("kind")
	if r.Method == http.MethodGet {
		detail, err := store.OperatorCaseDetail(ctx, op, workspace, kind)
		if err != nil {
			http.Error(w, err.Error(), 403)
			return
		}
		writeJSON(w, detail)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var input struct{ Action, Recipient, Note, WorkItemID string }
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "invalid case action", 400)
		return
	}
	if err := store.UpdateOperatorCase(ctx, op, workspace, kind, input.Action, input.Recipient, input.Note, input.WorkItemID); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	detail, err := store.OperatorCaseDetail(ctx, op, workspace, kind)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, detail)
}
func (s *Server) operatorEvidenceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	store, ok := s.store.(caseDetailStore)
	if !ok || s.r2 == nil {
		http.Error(w, "case evidence unavailable", 503)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	detail, err := store.OperatorCaseDetail(ctx, principalForRequest(r).User.ID, r.PathValue("workspaceId"), r.URL.Query().Get("kind"))
	if err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	asset := r.PathValue("id")
	found := false
	for _, e := range detail.State.Job.Evidence {
		if e.AssetID == asset {
			found = true
		}
	}
	if !found {
		http.Error(w, "case evidence not found", 404)
		return
	}
	body, ct, err := s.r2.Get(ctx, detail.State.Job.ID, asset)
	if err != nil {
		http.Error(w, "case evidence unavailable", 502)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.Copy(w, body)
}
func (s *Server) participantSupportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	store, ok := s.store.(caseDetailStore)
	if !ok {
		http.Error(w, "support records unavailable", 503)
		return
	}
	p := principalForRequest(r)
	notes, err := store.ParticipantCaseNotes(r.Context(), p.User.ID, string(p.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, "request support records unavailable", 403)
		return
	}
	writeJSON(w, map[string]any{"notes": notes})
}
func (s *Server) professionalEvidencePolicyHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := s.store.(evidencePolicyStore)
	if !ok || s.professionals == nil {
		http.Error(w, "service evidence policies unavailable", 503)
		return
	}
	p := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if r.Method == http.MethodGet {
		profile, found, err := s.professionals.ProfessionalProfile(ctx, p.User.ID)
		if err != nil || !found {
			http.Error(w, "professional profile unavailable", 409)
			return
		}
		policy, err := store.EvidencePolicy(ctx, profile.ID)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		writeJSON(w, policy)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var input struct {
		Requirements []workflow.EvidenceRequirement `json:"requirements"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "invalid evidence requirements", 400)
		return
	}
	policy, err := store.ProposeEvidencePolicy(ctx, p.User.ID, input.Requirements)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, policy)
}
func (s *Server) operatorEvidencePolicyHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := s.store.(evidencePolicyStore)
	if !ok {
		http.Error(w, "evidence policies unavailable", 503)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if r.Method == http.MethodGet {
		policies, err := store.ListEvidencePolicies(ctx)
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		writeJSON(w, map[string]any{"policies": policies})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var input struct {
		ProfileID string `json:"profileId"`
		Approve   bool   `json:"approve"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "invalid policy review", 400)
		return
	}
	policy, err := store.ReviewEvidencePolicy(ctx, principalForRequest(r).User.ID, input.ProfileID, input.Approve)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, policy)
}
