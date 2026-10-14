package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type NotificationStore interface {
	ListNotifications(context.Context, string, string) ([]workflow.Notification, error)
	MarkNotificationRead(context.Context, string, string, int64) (bool, error)
}

func (s *Server) notificationListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, ok := s.store.(NotificationStore)
	if !ok {
		http.Error(w, "notifications are not configured", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	items, err := store.ListNotifications(ctx, principal.User.ID, string(principal.ActiveRole))
	if err != nil {
		http.Error(w, "notifications could not be loaded", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"notifications": items})
}

func (s *Server) notificationReadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, ok := s.store.(NotificationStore)
	if !ok {
		http.Error(w, "notifications are not configured", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	var input struct {
		ID int64 `json:"id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.ID <= 0 {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	updated, err := store.MarkNotificationRead(ctx, principal.User.ID, string(principal.ActiveRole), input.ID)
	if err != nil {
		http.Error(w, "notification could not be updated", http.StatusServiceUnavailable)
		return
	}
	if !updated {
		http.Error(w, "notification not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{"status": "read"})
}
