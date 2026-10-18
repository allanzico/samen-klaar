package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func (s *Server) evidenceUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.r2 == nil {
		http.Error(w, "private evidence storage is not enabled", http.StatusServiceUnavailable)
		return
	}
	const maxEvidence = 10 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxEvidence+1<<20)
	if err := r.ParseMultipartForm(maxEvidence); err != nil {
		http.Error(w, "upload must be a file no larger than 10 MB", http.StatusRequestEntityTooLarge)
		return
	}
	principal := principalForRequest(r)
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "select an evidence file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxEvidence+1))
	if err != nil || len(content) == 0 || len(content) > maxEvidence {
		http.Error(w, "upload must be a non-empty file no larger than 10 MB", http.StatusRequestEntityTooLarge)
		return
	}
	contentType := http.DetectContentType(content)
	if len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP" {
		contentType = "image/webp"
	}
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" && contentType != "application/pdf" {
		http.Error(w, "supported evidence formats are JPEG, PNG, WebP, and PDF", http.StatusUnsupportedMediaType)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
		return
	}
	if !state.Job.WorkStarted || state.Job.Paid {
		http.Error(w, "evidence can be submitted after work starts and before payment", http.StatusConflict)
		return
	}
	assetBytes := make([]byte, 18)
	if _, err := rand.Read(assetBytes); err != nil {
		http.Error(w, "could not allocate evidence id", http.StatusInternalServerError)
		return
	}
	assetID := base64.RawURLEncoding.EncodeToString(assetBytes)
	if err := s.r2.Put(ctx, state.Job.ID, assetID, contentType, content); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	kind, label := "document", "Evidence document"
	if strings.HasPrefix(contentType, "image/") {
		kind, label = "photo", "Evidence photo"
	}
	input := Action{Action: "record-evidence", Actor: string(principal.ActiveRole), Evidence: Evidence{
		AssetID: assetID, Label: label, Description: "Uploaded to private Cloudflare R2 storage for this job.",
		Kind: kind, ContentType: contentType, ScopeVersion: state.Job.ScopeVersion, CompletionRound: workflow.EvidenceRound(state.Job), WorkItemIDs: r.MultipartForm.Value["workItemId"],
	}}
	jobID := state.Job.ID
	state, err = s.applyAction(ctx, principal.User.ID, string(principal.ActiveRole), input, r.URL.Query().Get("workspaceId"))
	if err != nil {
		s.r2.Delete(ctx, jobID, assetID)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, state)
}

func (s *Server) evidenceGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.r2 == nil {
		http.Error(w, "private evidence storage is not enabled", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
		return
	}
	assetID := r.PathValue("id")
	owned := false
	for _, evidence := range state.Job.Evidence {
		if evidence.AssetID != "" && evidence.AssetID == assetID {
			owned = true
			break
		}
	}
	if !owned {
		http.Error(w, "evidence file not found for this job", http.StatusNotFound)
		return
	}
	body, contentType, err := s.r2.Get(ctx, state.Job.ID, assetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "inline; filename=evidence")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, body)
}

func (s *Server) evidenceReviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ai == nil || s.r2 == nil {
		http.Error(w, "File checks are temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
		return
	}
	if !state.Job.EvidenceSubmitted || state.Job.Paid {
		http.Error(w, "evidence review is available after completion is submitted and before payment", http.StatusConflict)
		return
	}
	var selection struct {
		AssetIDs []string `json:"assetIds"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&selection); err != nil || len(selection.AssetIDs) > 5 {
		http.Error(w, "select up to five uploaded evidence files", http.StatusBadRequest)
		return
	}
	selected := make(map[string]bool, len(selection.AssetIDs))
	for _, id := range selection.AssetIDs {
		if id == "" || selected[id] {
			http.Error(w, "select distinct uploaded evidence files", http.StatusBadRequest)
			return
		}
		selected[id] = true
	}
	for id := range selected {
		found := false
		for _, evidence := range state.Job.Evidence {
			if evidence.AssetID == id {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "selected evidence does not belong to this request", http.StatusBadRequest)
			return
		}
	}
	files, loadErr := loadEvidenceReviewFiles(ctx, s.r2, state.Job, selected)
	if loadErr != nil {
		http.Error(w, loadErr.message, loadErr.status)
		return
	}
	jobForReview := state.Job
	jobForReview.ServiceAddress = ""
	review, err := s.ai.ReviewEvidence(ctx, jobForReview, files)
	if err != nil {
		http.Error(w, "SamenKlaar could not check these files right now. Please try again.", http.StatusBadGateway)
		return
	}
	state, err = s.applyAction(ctx, principal.User.ID, string(principal.ActiveRole), Action{Action: "record-evidence-review", Actor: string(principal.ActiveRole), EvidenceReview: review, ExpectedScope: state.Job.ScopeVersion, ExpectedEvidenceRound: workflow.EvidenceRound(state.Job)}, r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, state)
}

type reviewFileError struct {
	status  int
	message string
}

func loadEvidenceReviewFiles(ctx context.Context, storage EvidenceStorage, job Job, selected map[string]bool) ([]EvidenceBlob, *reviewFileError) {
	const maxTotal = 12 << 20
	var total int
	files := make([]EvidenceBlob, 0, 5)
	for i := len(job.Evidence) - 1; i >= 0; i-- {
		evidence := job.Evidence[i]
		if !workflow.CurrentEvidence(job, evidence) || evidence.AssetID == "" || len(selected) > 0 && !selected[evidence.AssetID] {
			continue
		}
		if len(files) == 5 {
			break
		}
		body, contentType, err := storage.Get(ctx, job.ID, evidence.AssetID)
		if err != nil {
			return nil, &reviewFileError{http.StatusBadGateway, "an evidence file could not be loaded for review"}
		}
		content, readErr := io.ReadAll(io.LimitReader(body, (10<<20)+1))
		_ = body.Close()
		if readErr != nil || len(content) == 0 || len(content) > 10<<20 {
			return nil, &reviewFileError{http.StatusRequestEntityTooLarge, "each review file must be no larger than 10 MB"}
		}
		if total+len(content) > maxTotal {
			if len(selected) > 0 {
				return nil, &reviewFileError{http.StatusRequestEntityTooLarge, "selected files exceed 12 MB together; choose a smaller batch"}
			}
			continue
		}
		if contentType != evidence.ContentType {
			return nil, &reviewFileError{http.StatusUnsupportedMediaType, "an evidence file's stored type did not match its job record"}
		}
		files = append(files, EvidenceBlob{ID: evidence.AssetID, ContentType: contentType, Label: evidence.Label, Data: content})
		total += len(content)
	}
	if len(files) == 0 {
		return nil, &reviewFileError{http.StatusConflict, "upload at least one file before checking the completion record"}
	}
	return files, nil
}
