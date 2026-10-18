package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

const maxRequestPhoto = 8 << 20

// requestPhotoNamespace keeps pre-booking photos apart from job evidence in private storage.
func requestPhotoNamespace(workspaceID string) string { return "request-" + workspaceID }

// requestPhotoUploadHandler lets the owning customer attach a picture of the problem. The image is
// stored privately and recorded on the request; it is never sent to an AI provider.
func (s *Server) requestPhotoUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.r2 == nil {
		http.Error(w, "private photo storage is not enabled", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestPhoto+1<<20)
	if err := r.ParseMultipartForm(maxRequestPhoto); err != nil {
		http.Error(w, "upload must be a photo no larger than 8 MB", http.StatusRequestEntityTooLarge)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "select a photo", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxRequestPhoto+1))
	if err != nil || len(content) == 0 || len(content) > maxRequestPhoto {
		http.Error(w, "upload must be a non-empty photo no larger than 8 MB", http.StatusRequestEntityTooLarge)
		return
	}
	contentType := http.DetectContentType(content)
	if len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP" {
		contentType = "image/webp"
	}
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		http.Error(w, "supported photo formats are JPEG, PNG and WebP", http.StatusUnsupportedMediaType)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	workspaceID := r.URL.Query().Get("workspaceId")
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), workspaceID)
	if err != nil {
		http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
		return
	}
	if len(state.RequestPhotos) >= workflow.MaxRequestPhotos {
		http.Error(w, "a request can have at most six photos", http.StatusConflict)
		return
	}
	assetBytes := make([]byte, 18)
	if _, err := rand.Read(assetBytes); err != nil {
		http.Error(w, "could not allocate a photo id", http.StatusInternalServerError)
		return
	}
	assetID := base64.RawURLEncoding.EncodeToString(assetBytes)
	namespace := requestPhotoNamespace(state.WorkspaceID)
	if err := s.r2.Put(ctx, namespace, assetID, contentType, content); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	input := Action{Action: "record-request-photo", Actor: string(principal.ActiveRole), PhotoAddedAt: time.Now().UTC().Format(time.RFC3339),
		Evidence: Evidence{AssetID: assetID, ContentType: contentType, Label: "Photo of the problem", Kind: "photo"}}
	state, err = s.applyAction(ctx, principal.User.ID, string(principal.ActiveRole), input, workspaceID)
	if err != nil {
		s.r2.Delete(ctx, namespace, assetID)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, state)
}

// requestPhotoGetHandler serves a request photo to the owning customer and, once a professional is
// assigned, to that professional only. The asset must be recorded on the workspace being read.
func (s *Server) requestPhotoGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.r2 == nil {
		http.Error(w, "private photo storage is not enabled", http.StatusServiceUnavailable)
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
	found := false
	for _, photo := range state.RequestPhotos {
		found = found || photo.AssetID == assetID
	}
	if !found || (string(principal.ActiveRole) == "professional" && state.Job.ID == "") {
		http.Error(w, "photo not found for this request", http.StatusNotFound)
		return
	}
	body, contentType, err := s.r2.Get(ctx, requestPhotoNamespace(state.WorkspaceID), assetID)
	if err != nil {
		http.Error(w, "photo unavailable", http.StatusBadGateway)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "inline; filename=photo")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, body)
}
