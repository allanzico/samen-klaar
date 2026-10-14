package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func (s *Server) stateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	principal := principalForRequest(r)
	state, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("workspaceId"))
	if err != nil {
		http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, state)
}

func (s *Server) actionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var input Action
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid action payload", http.StatusBadRequest)
		return
	}
	principal := principalForRequest(r)
	input.Actor = string(principal.ActiveRole)
	input.CustomerAccountID = principal.User.ID
	if s.ai != nil && actionMayCallAI(input.Action) && !s.limiter.allow("ai:"+principal.User.ID, 0.2, 20, time.Now()) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "AI requests are temporarily limited; try again shortly", http.StatusTooManyRequests)
		return
	}
	if input.Action == "reset" {
		http.Error(w, "workspace resets are not supported", http.StatusGone)
		return
	}
	if input.Action == "create-request" {
		http.Error(w, "create a request through the conversation", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	if input.Action == "confirm-payment" && s.paypal != nil {
		http.Error(w, "use the separate PayPal capture action after authorization", http.StatusConflict)
		return
	}
	if (input.Action == "start-intake" || input.Action == "revise-intake") && s.ai != nil {
		if input.Actor != string(identity.Customer) || len(strings.TrimSpace(input.Value)) < 5 || len(strings.TrimSpace(input.Value)) > 500 {
			http.Error(w, "describe the job in 5 to 500 characters as the customer", http.StatusBadRequest)
			return
		}
		suggestion, err := s.ai.Clarify(ctx, "Infer from description", strings.TrimSpace(input.Value))
		if err != nil {
			http.Error(w, "request clarification is unavailable", http.StatusBadGateway)
			return
		}
		input.Suggestion = &suggestion
	}
	if input.Action == "set-location" {
		location, err := workflow.ResolveLocation(input.Location, input.Value)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		input.Location = &location
	}
	if (input.Action == "answer-intake" || input.Action == "set-location" || input.Action == "refresh-matches" || input.Action == "return-to-matches") && principal.ActiveRole == identity.Customer {
		current, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), input.WorkspaceID)
		if err != nil {
			http.Error(w, "request could not be loaded", http.StatusConflict)
			return
		}
		category := ""
		if current.Intake != nil {
			category = current.Intake.Category
		}
		if current.Intake != nil && input.Action == "answer-intake" && current.Intake.Step == "detail" {
			if s.ai != nil {
				suggestion, err := s.ai.Clarify(ctx, "Continue clarifying the trade from the original request and this answer", current.Intake.Description+"\nAdditional detail: "+input.Value)
				if err != nil {
					http.Error(w, "request clarification is unavailable", http.StatusBadGateway)
					return
				}
				input.Suggestion = &suggestion
			}
			category = workflow.ClarifiedCategory(*current.Intake, input.Value, input.Suggestion)
		}
		readyAfterDetail := current.Intake != nil && current.Intake.Step == "detail" && current.Intake.Urgency != "" && current.Intake.Budget != "" && current.Intake.Location != nil
		searchRequested := input.Action == "set-location" || input.Action == "return-to-matches" || input.Action == "refresh-matches" && current.Intake != nil && current.Intake.Location != nil
		searchAfterAnswer := input.Action == "answer-intake" && (current.Intake != nil && current.Intake.Step == "budget" && current.Intake.Location != nil || readyAfterDetail)
		if current.Intake != nil && (searchRequested || searchAfterAnswer) {
			if s.directory == nil {
				http.Error(w, "professional directory is unavailable", http.StatusServiceUnavailable)
				return
			}
			if s.ai == nil {
				http.Error(w, "service matching is unavailable", http.StatusServiceUnavailable)
				return
			}
			detail := current.Intake.Detail
			if current.Intake.Step == "detail" && input.Action == "answer-intake" {
				detail += "\n" + input.Value
			}
			location := workflow.Location{}
			if input.Action == "set-location" && input.Location != nil {
				location = *input.Location
			} else if current.Intake.Location != nil {
				location = *current.Intake.Location
			}
			input.Matches, input.SuggestedQuestion, err = matching.Find(ctx, s.directory, s.ai, category, current.Intake.Description+"\n"+detail, principal.User.ID, location)
			if err != nil {
				http.Error(w, "professional directory could not be searched", http.StatusServiceUnavailable)
				return
			}
		}
	}
	if input.Action == "select-match" && principal.ActiveRole == identity.Customer {
		current, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), input.WorkspaceID)
		if err != nil || current.Intake == nil {
			http.Error(w, "the request is no longer available", http.StatusConflict)
			return
		}
		if s.directory == nil {
			http.Error(w, "professional directory is unavailable", http.StatusServiceUnavailable)
			return
		}
		found := false
		for _, match := range current.Intake.Matches {
			if match.ID == input.Value {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "choose an active professional from these results", http.StatusConflict)
			return
		}
		offer, err := s.directory.CatalogOffer(ctx, input.Value)
		if err != nil {
			http.Error(w, "the selected professional catalog is no longer available", http.StatusConflict)
			return
		}
		if offer.AccountID == principal.User.ID {
			http.Error(w, "you cannot book your own services", http.StatusConflict)
			return
		}
		for _, match := range current.Intake.Matches {
			if match.ID == input.Value && match.ServiceName != "" && (match.ServiceName != offer.ServiceName || match.ServiceDescription != offer.ServiceDescription || match.TotalMinor != offer.PriceMinor+offer.CalloutMinor || match.Currency != offer.Currency || match.DurationMinutes != offer.DurationMinutes || match.MaterialsPolicy != offer.MaterialsPolicy || !sameStrings(match.Exclusions, offer.Exclusions)) {
				http.Error(w, "this service or price changed; refresh the recommendations before choosing", http.StatusConflict)
				return
			}
		}
		input.SelectedOffer = &offer
		input.CustomerName = principal.User.DisplayName
		input.AvailableSlots, err = s.directory.AvailableProfessionalSlots(ctx, offer.AccountID, "")
		if err != nil {
			http.Error(w, "professional availability could not be loaded", http.StatusServiceUnavailable)
			return
		}
	}
	if input.Action == "record-evidence" {
		http.Error(w, "use the private evidence upload endpoint", http.StatusBadRequest)
		return
	}
	if input.Action == "record-evidence-review" {
		http.Error(w, "use the customer evidence review endpoint", http.StatusBadRequest)
		return
	}
	if input.Action == "approve-change" && principal.ActiveRole == identity.Customer && s.paypal != nil {
		current, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), input.WorkspaceID)
		if err != nil {
			http.Error(w, "account workspace is unavailable", http.StatusServiceUnavailable)
			return
		}
		if current.Job.Change != nil && current.Job.Change.Status == "pending" {
			if err := s.paypal.VoidCurrent(ctx, current); err != nil {
				http.Error(w, "the existing PayPal authorization could not be released", http.StatusConflict)
				return
			}
		}
	}
	state, err := s.applyAction(ctx, principal.User.ID, string(principal.ActiveRole), input, input.WorkspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, state)
}

func actionMayCallAI(action string) bool {
	switch action {
	case "start-intake", "revise-intake", "answer-intake", "set-location", "refresh-matches", "return-to-matches":
		return true
	default:
		return false
	}
}

func (s *Server) currentState(ctx context.Context, accountID, role string, workspaceID ...string) (State, error) {
	if s.store == nil {
		return State{}, errors.New("account storage is not configured")
	}
	if len(workspaceID) > 0 && workspaceID[0] != "" {
		if workspaces, ok := s.store.(workflow.WorkspaceStore); ok {
			return workspaces.LoadWorkspace(ctx, accountID, role, workspaceID[0])
		}
	}
	if routed, ok := s.store.(RoutedStateStore); ok {
		return routed.LoadFor(ctx, accountID, role)
	}
	return s.store.Load(ctx, accountID)
}

func (s *Server) applyAction(ctx context.Context, accountID, role string, input Action, workspaceID ...string) (State, error) {
	if s.store == nil {
		return State{}, errors.New("account storage is not configured")
	}
	if len(workspaceID) > 0 && workspaceID[0] != "" {
		input.WorkspaceID = workspaceID[0]
	}
	if workspaces, ok := s.store.(workflow.WorkspaceStore); ok && input.WorkspaceID != "" {
		return workspaces.ApplyWorkspace(ctx, accountID, role, input.WorkspaceID, input)
	}
	if routed, ok := s.store.(RoutedStateStore); ok {
		return routed.ApplyFor(ctx, accountID, role, input)
	}
	return s.store.Apply(ctx, accountID, input)
}

func (s *Server) configHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, struct {
		AppName string `json:"appName"`
		ProviderConfig
	}{AppName: s.appName, ProviderConfig: s.providers})
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
