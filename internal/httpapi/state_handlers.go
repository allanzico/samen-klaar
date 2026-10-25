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
	input.Locale = principal.User.Locale
	input.CustomerAccountID = principal.User.ID
	if s.ai != nil && actionMayCallAI(input.Action) && !s.limiter.allow("ai:"+principal.User.ID, 0.2, 20, time.Now()) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "Please wait a moment before trying again", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	state, status, err := s.runAction(ctx, principal, input)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, state)
	s.TriggerAgents(state.WorkspaceID)
}

// runAction prepares and applies one workflow action for a signed-in principal: the same path whether a
// person clicked a button or their agent acted within a mandate. It returns an HTTP status for errors.
func (s *Server) runAction(ctx context.Context, principal identity.Principal, input Action) (State, int, error) {
	if (input.Action == "set-booking-autonomy" && strings.HasPrefix(input.Value, "automatic|") || input.Action == "agent-reply" && strings.HasPrefix(input.ChoiceID, "automatic|")) && (s.ai == nil || s.r2 == nil || s.paypal == nil || s.agents == nil) {
		return State{}, http.StatusServiceUnavailable, errors.New("automatic review and payment release are not available; choose to review and pay yourself")
	}
	if input.Action == "reset" {
		return State{}, http.StatusGone, errors.New("workspace resets are not supported")
	}
	if (input.Action == "set-payment-timing" || input.Action == "approve-and-book" || input.Action == "book-appointment" || input.Action == "approve-quote" || input.Action == "accept-date") && input.PaymentAuthorizationTiming == workflow.PaymentAuthorizationAtBooking && s.paypal == nil {
		return State{}, http.StatusServiceUnavailable, errors.New("booking-time PayPal authorization is not available for this environment")
	}
	if input.Action == "record-payment-authorization" {
		return State{}, http.StatusForbidden, errors.New("PayPal authorizations must be recorded by the payment callback")
	}
	if input.Action == "remove-request" {
		return State{}, http.StatusBadRequest, errors.New("remove a request with DELETE /api/workspaces/{id}")
	}
	if input.Action == "create-request" {
		return State{}, http.StatusBadRequest, errors.New("create a request through the conversation")
	}
	if input.Action == "confirm-payment" && s.paypal != nil {
		return State{}, http.StatusConflict, errors.New("use the separate PayPal capture action after authorization")
	}
	if (input.Action == "start-intake" || input.Action == "revise-intake") && s.ai != nil {
		if input.Actor != string(identity.Customer) || len(strings.TrimSpace(input.Value)) < 5 || len(strings.TrimSpace(input.Value)) > 500 {
			return State{}, http.StatusBadRequest, errors.New("describe the job in 5 to 500 characters as the customer")
		}
		suggestion, err := s.ai.Clarify(ctx, "Infer from description", strings.TrimSpace(input.Value))
		if err != nil {
			return State{}, http.StatusBadGateway, errors.New("request clarification is unavailable")
		}
		input.Suggestion = &suggestion
	}
	if input.Action == "set-location" {
		var location workflow.Location
		var err error
		if s.locations != nil || input.Location != nil && input.Location.Source == "mapbox" {
			location, err = s.selectedLocation(principal.User.ID, input.Location)
		} else {
			location, err = workflow.ResolveLocation(input.Location, input.Value)
		}
		if err != nil {
			return State{}, http.StatusBadRequest, errors.New(err.Error())
		}
		input.Location = &location
	}
	if (input.Action == "answer-intake" || input.Action == "set-location" || input.Action == "refresh-matches" || input.Action == "return-to-matches") && principal.ActiveRole == identity.Customer {
		current, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), input.WorkspaceID)
		if err != nil {
			return State{}, http.StatusConflict, errors.New("request could not be loaded")
		}
		category := ""
		if current.Intake != nil {
			category = current.Intake.Category
		}
		if current.Intake != nil && input.Action == "answer-intake" && current.Intake.Step == "detail" {
			if s.ai != nil {
				suggestion, err := s.ai.Clarify(ctx, "Continue clarifying the trade from the original request and this answer", current.Intake.Description+"\nAdditional detail: "+input.Value)
				if err != nil {
					return State{}, http.StatusBadGateway, errors.New("request clarification is unavailable")
				}
				input.Suggestion = &suggestion
			}
			category = workflow.ClarifiedCategory(*current.Intake, input.Value, input.Suggestion)
		}
		readyAfterDetail := current.Intake != nil && current.Intake.Step == "detail" && current.Intake.Urgency != "" && current.Intake.Budget != "" && current.Intake.Location != nil
		returningToSavedMatches := input.Action == "return-to-matches" && current.Intake != nil && len(current.Intake.Matches) > 0 && !current.Cancelled
		if returningToSavedMatches {
			input.Matches = append([]workflow.Match{}, current.Intake.Matches...)
		}
		searchRequested := input.Action == "set-location" || input.Action == "return-to-matches" && !returningToSavedMatches || input.Action == "refresh-matches" && current.Intake != nil && current.Intake.Location != nil
		searchAfterAnswer := input.Action == "answer-intake" && (current.Intake != nil && current.Intake.Step == "budget" && current.Intake.Location != nil || readyAfterDetail)
		if current.Intake != nil && (searchRequested || searchAfterAnswer) {
			if s.directory == nil {
				return State{}, http.StatusServiceUnavailable, errors.New("professional directory is unavailable")
			}
			if s.ai == nil {
				return State{}, http.StatusServiceUnavailable, errors.New("service matching is unavailable")
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
				return State{}, http.StatusServiceUnavailable, errors.New("professional directory could not be searched")
			}
		}
	}
	if input.Action == "set-service-address" && principal.ActiveRole == identity.Customer {
		current, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), input.WorkspaceID)
		if err != nil {
			return State{}, http.StatusConflict, errors.New("request could not be loaded")
		}
		if s.directory != nil && current.Job.ProfessionalProfileID != "" {
			offer, err := s.directory.CatalogOffer(ctx, current.Job.ProfessionalProfileID)
			if err != nil {
				return State{}, http.StatusConflict, errors.New("the selected professional is no longer available")
			}
			if offer.BaseLocation != nil || current.Job.Location != nil && current.Job.Location.Source == "mapbox" {
				location, err := s.resolveVisitAddress(ctx, principal.User.ID, input.Value, input.Location)
				if err != nil {
					return State{}, http.StatusBadRequest, err
				}
				if !workflow.OfferCoversLocation(offer, location) {
					return State{}, http.StatusConflict, errors.New("This address is outside the professional’s service area. Choose another professional.")
				}
				input.Value = location.Address
				input.Location = &location
			}
		}
	}
	if input.Action == "select-match" && principal.ActiveRole == identity.Customer {
		current, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), input.WorkspaceID)
		if err != nil || current.Intake == nil {
			return State{}, http.StatusConflict, errors.New("the request is no longer available")
		}
		if s.directory == nil {
			return State{}, http.StatusServiceUnavailable, errors.New("professional directory is unavailable")
		}
		found := false
		for _, match := range current.Intake.Matches {
			if match.ID == input.Value {
				found = true
				break
			}
		}
		if !found {
			return State{}, http.StatusConflict, errors.New("choose an active professional from these results")
		}
		offer, err := s.directory.CatalogOffer(ctx, input.Value)
		if err != nil {
			return State{}, http.StatusConflict, errors.New("the selected professional catalog is no longer available")
		}
		if offer.AccountID == principal.User.ID {
			return State{}, http.StatusConflict, errors.New("you cannot book your own services")
		}
		for _, match := range current.Intake.Matches {
			if match.ID == input.Value && match.ServiceName != "" && (match.ServiceName != offer.ServiceName || match.ServiceDescription != offer.ServiceDescription || match.TotalMinor != offer.PriceMinor+offer.CalloutMinor || match.Currency != offer.Currency || match.DurationMinutes != offer.DurationMinutes || match.MaterialsPolicy != offer.MaterialsPolicy || !sameStrings(match.Exclusions, offer.Exclusions)) {
				return State{}, http.StatusConflict, errors.New("this service or price changed; refresh the recommendations before choosing")
			}
		}
		if policies, ok := s.store.(evidencePolicyStore); ok {
			policy, policyErr := policies.EvidencePolicy(ctx, offer.ProfileID)
			if policyErr != nil {
				return State{}, http.StatusServiceUnavailable, errors.New("service evidence requirements unavailable")
			}
			offer.EvidenceRequirements = policy.Requirements
		}
		input.SelectedOffer = &offer
		input.CustomerName = principal.User.DisplayName
		input.AvailableSlots, err = s.directory.AvailableProfessionalSlots(ctx, offer.AccountID, "")
		if err != nil {
			return State{}, http.StatusServiceUnavailable, errors.New("professional availability could not be loaded")
		}
	}
	if input.Action == "record-evidence" {
		return State{}, http.StatusBadRequest, errors.New("use the private evidence upload endpoint")
	}
	if input.Action == "record-evidence-review" {
		return State{}, http.StatusBadRequest, errors.New("use the customer evidence review endpoint")
	}
	if input.Action == "approve-change" && principal.ActiveRole == identity.Customer && s.paypal != nil {
		current, err := s.currentState(ctx, principal.User.ID, string(principal.ActiveRole), input.WorkspaceID)
		if err != nil {
			return State{}, http.StatusServiceUnavailable, errors.New("account workspace is unavailable")
		}
		if current.Job.Change != nil && current.Job.Change.Status == "pending" {
			if err := s.paypal.VoidCurrent(ctx, current); err != nil {
				return State{}, http.StatusConflict, errors.New("the existing PayPal authorization could not be released")
			}
		}
	}
	state, err := s.applyAction(ctx, principal.User.ID, string(principal.ActiveRole), input, input.WorkspaceID)
	if err != nil {
		return State{}, http.StatusConflict, err
	}
	return state, http.StatusOK, nil
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
		AppName                   string `json:"appName"`
		AutomaticReleaseAvailable bool   `json:"automaticReleaseAvailable"`
		GuidedReviewAvailable     bool   `json:"guidedReviewAvailable"`
		EvidenceUploadsAvailable  bool   `json:"evidenceUploadsAvailable"`
		PaymentsAvailable         bool   `json:"paymentsAvailable"`
		TestPayments              bool   `json:"testPayments"`
		LocationSearchAvailable   bool   `json:"locationSearchAvailable"`
	}{
		AppName:                   s.appName,
		AutomaticReleaseAvailable: s.ai != nil && s.r2 != nil && s.paypal != nil && s.agents != nil,
		LocationSearchAvailable:   s.locations != nil,
		GuidedReviewAvailable:     s.providers.AIProvider != "",
		EvidenceUploadsAvailable:  s.providers.EvidenceProvider != "",
		PaymentsAvailable:         s.providers.PaymentProvider == "paypal_sandbox",
		TestPayments:              s.providers.PaymentProvider == "paypal_sandbox",
	})
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
