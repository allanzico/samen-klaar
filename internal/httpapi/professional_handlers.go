package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type ProfessionalDirectory interface {
	matching.Directory
	ListProfessionalMatches(context.Context, string, workflow.Location, string) ([]workflow.Match, error)
	CatalogOffer(context.Context, string) (workflow.CatalogOffer, error)
	AvailableProfessionalSlots(context.Context, string, string) ([]workflow.Appointment, error)
}

type ProfessionalService interface {
	ProfessionalProfile(context.Context, string) (workflow.ProfessionalProfile, bool, error)
	SaveProfessionalProfile(context.Context, string, workflow.ProfessionalProfile) (workflow.ProfessionalProfile, error)
	AddProfessionalAvailability(context.Context, string, workflow.Appointment) (string, error)
	ListProfessionalAvailability(context.Context, string) ([]workflow.AvailabilitySlot, error)
}

type OperatorService interface {
	ListProfessionalApplications(context.Context) ([]workflow.ProfessionalApplication, error)
	ListWorkExceptions(context.Context) ([]workflow.OperatorCase, error)
	NotificationDeliveryHealth(context.Context) (workflow.NotificationDeliveryHealth, error)
	ReviewProfessionalApplication(context.Context, string, string, string, workflow.ProfessionalReviewChecks) error
	ResolveWorkCase(context.Context, string, string, string, string, string) error
	BeginWorkRefund(context.Context, string, string, string) (workflow.State, error)
	CompleteWorkRefund(context.Context, string, string, string) error
}

func (s *Server) professionalProfileHandler(w http.ResponseWriter, r *http.Request) {
	if s.professionals == nil {
		http.Error(w, "professional accounts are not configured", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	if r.Method == http.MethodGet {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		profile, found, err := s.professionals.ProfessionalProfile(ctx, principal.User.ID)
		if err != nil {
			http.Error(w, "professional profile could not be loaded", http.StatusServiceUnavailable)
			return
		}
		if !found {
			writeJSON(w, map[string]any{"profile": nil})
			return
		}
		writeJSON(w, map[string]any{"profile": profile})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input workflow.ProfessionalProfile
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid professional profile", http.StatusBadRequest)
		return
	}
	if input.BaseLocation != nil {
		if input.BaseLocation.Selection != "" {
			location, err := s.selectedLocation(principal.User.ID, input.BaseLocation)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			input.BaseLocation = &location
		} else {
			previous, found, err := s.professionals.ProfessionalProfile(r.Context(), principal.User.ID)
			if err != nil || !found || previous.BaseLocation == nil || previous.BaseLocation.ID != input.BaseLocation.ID {
				http.Error(w, "Select your base location from the suggestions.", http.StatusBadRequest)
				return
			}
			input.BaseLocation = previous.BaseLocation
		}
		input.Latitude, input.Longitude = input.BaseLocation.Latitude, input.BaseLocation.Longitude
		input.ServiceArea = input.BaseLocation.Label
	} else if s.locations != nil {
		previous, found, err := s.professionals.ProfessionalProfile(r.Context(), principal.User.ID)
		if err != nil || !found || previous.BaseLocation != nil {
			http.Error(w, "Select your base location and travel radius.", http.StatusBadRequest)
			return
		}
	}
	if err := validateProfessionalProfile(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	input.AccountID = principal.User.ID
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	profile, err := s.professionals.SaveProfessionalProfile(ctx, principal.User.ID, input)
	if err != nil {
		http.Error(w, "professional profile could not be saved", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"profile": profile})
}

func validateProfessionalProfile(profile *workflow.ProfessionalProfile) error {
	profile.DisplayName = strings.TrimSpace(profile.DisplayName)
	profile.Category = strings.TrimSpace(profile.Category)
	profile.ServiceName = strings.TrimSpace(profile.ServiceName)
	profile.ServiceDescription = strings.TrimSpace(profile.ServiceDescription)
	profile.ServiceArea = strings.TrimSpace(profile.ServiceArea)
	profile.Currency = strings.TrimSpace(profile.Currency)
	profile.MaterialsPolicy = strings.TrimSpace(profile.MaterialsPolicy)
	if len(profile.DisplayName) < 2 || len(profile.DisplayName) > 80 {
		return errors.New("display name must be 2 to 80 characters")
	}
	if profile.Category != "Plumbing" && profile.Category != "Carpentry" && profile.Category != "Electrical assessment" {
		return errors.New("choose a supported service category")
	}
	if len(profile.ServiceName) < 3 || len(profile.ServiceName) > 100 {
		return errors.New("service name must be 3 to 100 characters")
	}
	if len(profile.ServiceDescription) > 600 {
		return errors.New("service description must be 600 characters or fewer")
	}
	if len(profile.ServiceArea) < 2 || len(profile.ServiceArea) > 120 {
		return errors.New("list at least one Amsterdam neighbourhood or postcode area, up to 120 characters")
	}
	if profile.BaseLocation == nil {
		for _, area := range strings.FieldsFunc(profile.ServiceArea, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '|' }) {
			if !workflow.IsAmsterdamServiceArea(area) {
				return errors.New("service areas must be in Amsterdam or use an Amsterdam 10xx postcode area")
			}
		}
	} else if !workflow.ValidGeocodedLocation(*profile.BaseLocation) || profile.BaseLocation.Kind != "address" && profile.BaseLocation.Kind != "postcode" || profile.TravelRadiusKM < 1 || profile.TravelRadiusKM > 100 {
		return errors.New("choose a base address or postcode and a travel radius from 1 to 100 km")
	}
	if len(profile.Currency) != 3 || profile.Currency != strings.ToUpper(profile.Currency) {
		return errors.New("currency must be an uppercase three-letter code")
	}
	for _, char := range profile.Currency {
		if char < 'A' || char > 'Z' {
			return errors.New("currency must be an uppercase three-letter code")
		}
	}
	if profile.PriceMinor <= 0 || profile.PriceMinor > 100_000_000 || profile.CalloutMinor < 0 || profile.CalloutMinor > 10_000_000 {
		return errors.New("service and call-out prices are outside the supported range")
	}
	if profile.DurationMinutes < 15 || profile.DurationMinutes > 360 {
		return errors.New("duration must be between 15 and 360 minutes")
	}
	if len(profile.MaterialsPolicy) < 3 || len(profile.MaterialsPolicy) > 300 {
		return errors.New("materials policy must be 3 to 300 characters")
	}
	if len(profile.Exclusions) > 10 {
		return errors.New("add no more than 10 scope exclusions")
	}
	for i := range profile.Exclusions {
		profile.Exclusions[i] = strings.TrimSpace(profile.Exclusions[i])
		if len(profile.Exclusions[i]) == 0 || len(profile.Exclusions[i]) > 120 {
			return errors.New("each exclusion must be 1 to 120 characters")
		}
	}
	if math.IsNaN(profile.Latitude) || math.IsInf(profile.Latitude, 0) || math.IsNaN(profile.Longitude) || math.IsInf(profile.Longitude, 0) || math.Abs(profile.Latitude) > 90 || math.Abs(profile.Longitude) > 180 {
		return errors.New("professional location coordinates are invalid")
	}
	if (profile.Latitude == 0) != (profile.Longitude == 0) {
		return errors.New("set both location coordinates or leave both blank")
	}
	return nil
}

func (s *Server) professionalAvailabilityHandler(w http.ResponseWriter, r *http.Request) {
	if s.professionals == nil {
		http.Error(w, "professional availability is not configured", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	if r.Method == http.MethodGet {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if r.URL.Query().Has("from") || r.URL.Query().Has("days") {
			service, ok := s.scheduleService(w)
			if !ok {
				return
			}
			days := 7
			if value := r.URL.Query().Get("days"); value != "" {
				var err error
				days, err = strconv.Atoi(value)
				if err != nil {
					http.Error(w, "Choose from 1 to 90 days of availability.", http.StatusBadRequest)
					return
				}
			}
			window, err := service.ProfessionalAvailabilityWindow(ctx, principal.User.ID, r.URL.Query().Get("from"), days)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, window)
			return
		}
		slots, err := s.professionals.ListProfessionalAvailability(ctx, principal.User.ID)
		if err != nil {
			http.Error(w, "availability could not be loaded", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]any{"slots": slots})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		StartUTC string `json:"startUtc"`
		EndUTC   string `json:"endUtc"`
		Timezone string `json:"timezone"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "invalid availability slot", http.StatusBadRequest)
		return
	}
	start, startErr := time.Parse(time.RFC3339, input.StartUTC)
	end, endErr := time.Parse(time.RFC3339, input.EndUTC)
	zone, zoneErr := time.LoadLocation(input.Timezone)
	if startErr != nil || endErr != nil || zoneErr != nil || end.Sub(start) < 15*time.Minute || end.Sub(start) > 6*time.Hour || start.Before(time.Now().Add(15*time.Minute)) || start.After(time.Now().AddDate(1, 0, 0)) {
		http.Error(w, "choose a future time window of 15 minutes to 6 hours within the next year, with a valid timezone", http.StatusBadRequest)
		return
	}
	slot := workflow.Appointment{StartUTC: start.UTC().Format(time.RFC3339), EndUTC: end.UTC().Format(time.RFC3339), Timezone: zone.String()}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	profile, found, err := s.professionals.ProfessionalProfile(ctx, principal.User.ID)
	if err != nil || !found {
		http.Error(w, "load your service profile before adding availability", http.StatusConflict)
		return
	}
	if end.Sub(start) < time.Duration(profile.DurationMinutes)*time.Minute {
		http.Error(w, "allow the full published service duration", http.StatusBadRequest)
		return
	}
	id, err := s.professionals.AddProfessionalAvailability(ctx, principal.User.ID, slot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]string{"id": id, "status": "available"})
}

func (s *Server) operatorQueueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.operators == nil {
		http.Error(w, "operator queue is not configured", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	applications, err := s.operators.ListProfessionalApplications(ctx)
	if err != nil {
		http.Error(w, "operator queue could not be loaded", http.StatusServiceUnavailable)
		return
	}
	cases, err := s.operators.ListWorkExceptions(ctx)
	if err != nil {
		http.Error(w, "operator queue could not be loaded", http.StatusServiceUnavailable)
		return
	}
	notificationDelivery, err := s.operators.NotificationDeliveryHealth(ctx)
	if err != nil {
		http.Error(w, "operator queue could not be loaded", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"professionalApplications": applications, "cases": cases, "notificationDelivery": notificationDelivery})
}

func (s *Server) professionalReviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.operators == nil {
		http.Error(w, "operator queue is not configured", http.StatusServiceUnavailable)
		return
	}
	var input struct {
		Decision string                            `json:"decision"`
		Checks   workflow.ProfessionalReviewChecks `json:"checks"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		http.Error(w, "invalid review action", http.StatusBadRequest)
		return
	}
	if input.Decision != "save" && input.Decision != "approve" && input.Decision != "reject" {
		http.Error(w, "decision must be save, approve, or reject", http.StatusBadRequest)
		return
	}
	if err := input.Checks.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.Decision == "approve" && !input.Checks.ReadyToApprove() {
		http.Error(w, "complete all required professional checks before approval", http.StatusConflict)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.operators.ReviewProfessionalApplication(ctx, principal.User.ID, r.PathValue("id"), input.Decision, input.Checks); err != nil {
		http.Error(w, "professional application could not be reviewed", http.StatusConflict)
		return
	}
	status := map[string]string{"save": "pending", "approve": "active", "reject": "rejected"}[input.Decision]
	writeJSON(w, map[string]string{"status": status})
}

func (s *Server) resolveWorkCaseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.operators == nil {
		http.Error(w, "operator queue is not configured", http.StatusServiceUnavailable)
		return
	}
	var input struct {
		Kind      string `json:"kind"`
		Decision  string `json:"decision"`
		Rationale string `json:"rationale"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid operator case resolution", http.StatusBadRequest)
		return
	}
	if input.Kind != "late-cancellation" && input.Kind != "late-reschedule" && input.Kind != "withdrawal" && input.Kind != "job-issue" {
		http.Error(w, "unsupported operator case type", http.StatusBadRequest)
		return
	}
	if input.Decision == "" || len(strings.TrimSpace(input.Rationale)) < 10 || len(input.Rationale) > 1000 {
		http.Error(w, "decision and a 10 to 1000 character rationale are required", http.StatusBadRequest)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if input.Kind == "job-issue" && input.Decision == "refund" {
		if s.refunds == nil {
			http.Error(w, "refund processing is not configured", http.StatusServiceUnavailable)
			return
		}
		state, err := s.operators.BeginWorkRefund(ctx, principal.User.ID, r.PathValue("workspaceId"), input.Rationale)
		if err != nil {
			http.Error(w, "paid issue is not ready for refund", http.StatusConflict)
			return
		}
		refundID, err := s.refunds.Refund(ctx, state)
		if err != nil {
			http.Error(w, "PayPal refund is uncertain; retry this operator action after checking PayPal status", http.StatusBadGateway)
			return
		}
		if err := s.operators.CompleteWorkRefund(ctx, principal.User.ID, r.PathValue("workspaceId"), refundID); err != nil {
			http.Error(w, "refund completed at PayPal; retry this action to finish recording the case", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]string{"status": "refunded"})
		return
	}
	if err := s.operators.ResolveWorkCase(ctx, principal.User.ID, r.PathValue("workspaceId"), input.Kind, input.Decision, input.Rationale); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]string{"status": "resolved"})
}
