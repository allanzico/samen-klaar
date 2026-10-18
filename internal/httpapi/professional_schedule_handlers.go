package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type ProfessionalScheduleService interface {
	ProfessionalSchedule(context.Context, string) (workflow.ProfessionalScheduleView, error)
	SaveProfessionalSchedule(context.Context, string, workflow.ProfessionalSchedule) (workflow.ProfessionalScheduleView, error)
	PreviewProfessionalSchedule(context.Context, string, workflow.ProfessionalSchedule) (workflow.SchedulePreview, error)
	ProfessionalAvailabilityWindow(context.Context, string, string, int) (workflow.ProfessionalAvailabilityWindow, error)
	PreviewProfessionalDaysOff(context.Context, string, workflow.ProfessionalDayOff) (workflow.DaysOffPreview, error)
	AddProfessionalDaysOff(context.Context, string, workflow.ProfessionalDayOff) (workflow.ProfessionalDayOff, int, error)
	RemoveProfessionalDaysOff(context.Context, string, string) (int, error)
	RemoveProfessionalAvailability(context.Context, string, string) error
	RestoreProfessionalAvailability(context.Context, string, string) error
}

func (s *Server) scheduleService(w http.ResponseWriter) (ProfessionalScheduleService, bool) {
	service, ok := s.professionals.(ProfessionalScheduleService)
	if !ok {
		http.Error(w, "Working hours are not available in this environment.", http.StatusServiceUnavailable)
	}
	return service, ok
}

func decodeScheduleInput(w http.ResponseWriter, r *http.Request, input any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		http.Error(w, "Check the working hours or days-off details.", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) professionalScheduleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service, ok := s.scheduleService(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	accountID := principalForRequest(r).User.ID
	var view workflow.ProfessionalScheduleView
	var err error
	if r.Method == http.MethodGet {
		view, err = service.ProfessionalSchedule(ctx, accountID)
	} else {
		var input workflow.ProfessionalSchedule
		if !decodeScheduleInput(w, r, &input) {
			return
		}
		view, err = service.SaveProfessionalSchedule(ctx, accountID, input)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, view)
}

func (s *Server) professionalSchedulePreviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service, ok := s.scheduleService(w)
	if !ok {
		return
	}
	var input workflow.ProfessionalSchedule
	if !decodeScheduleInput(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	preview, err := service.PreviewProfessionalSchedule(ctx, principalForRequest(r).User.ID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, preview)
}

func (s *Server) professionalDaysOffPreviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service, ok := s.scheduleService(w)
	if !ok {
		return
	}
	var input workflow.ProfessionalDayOff
	if !decodeScheduleInput(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	preview, err := service.PreviewProfessionalDaysOff(ctx, principalForRequest(r).User.ID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, preview)
}

func (s *Server) professionalDaysOffHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service, ok := s.scheduleService(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	accountID := principalForRequest(r).User.ID
	if r.Method == http.MethodPost {
		if r.URL.Path != "/api/professional/days-off" {
			http.NotFound(w, r)
			return
		}
		var input workflow.ProfessionalDayOff
		if !decodeScheduleInput(w, r, &input) {
			return
		}
		day, removed, err := service.AddProfessionalDaysOff(ctx, accountID, input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"dayOff": day, "removed": removed})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/professional/days-off/")
	if id == r.URL.Path || id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	added, err := service.RemoveProfessionalDaysOff(ctx, accountID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]int{"added": added})
}

func (s *Server) professionalAvailabilityMaintenanceHandler(w http.ResponseWriter, r *http.Request) {
	service, ok := s.scheduleService(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	accountID := principalForRequest(r).User.ID
	if r.URL.Path == "/api/professional/availability/restore" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			StartUTC string `json:"startUtc"`
		}
		if !decodeScheduleInput(w, r, &input) {
			return
		}
		if err := service.RestoreProfessionalAvailability(ctx, accountID, input.StartUTC); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "restored"})
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/professional/availability/")
	if id == "" || id == r.URL.Path || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if err := service.RemoveProfessionalAvailability(ctx, accountID, id); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
