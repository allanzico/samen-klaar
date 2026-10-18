package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

type consumerPolicyStore interface {
	CurrentConsumerPolicies(context.Context) ([]workflow.ConsumerPolicy, error)
	PublishConsumerPolicy(context.Context, string, workflow.ConsumerPolicy) error
}

// consumerPoliciesHandler lists the currently published, externally reviewed consumer policies.
// Signed-in participants read them to see the exact text they are asked to acknowledge.
func (s *Server) consumerPoliciesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, ok := s.store.(consumerPolicyStore)
	if !ok {
		http.Error(w, "consumer policies unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	policies, err := store.CurrentConsumerPolicies(ctx)
	if err != nil {
		http.Error(w, "consumer policies unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"policies": policies})
}

// operatorPolicyPublishHandler records an externally reviewed policy version. The publishing
// operator is the named reviewer; the review reference points at the outside decision.
func (s *Server) operatorPolicyPublishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, ok := s.store.(consumerPolicyStore)
	if !ok {
		http.Error(w, "consumer policies unavailable", http.StatusServiceUnavailable)
		return
	}
	var policy workflow.ConsumerPolicy
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if json.NewDecoder(r.Body).Decode(&policy) != nil {
		http.Error(w, "invalid policy", http.StatusBadRequest)
		return
	}
	operator := principalForRequest(r).User.ID
	policy.ReviewerID = operator
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := store.PublishConsumerPolicy(ctx, operator, policy); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	policies, err := store.CurrentConsumerPolicies(ctx)
	if err != nil {
		http.Error(w, "consumer policies unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"policies": policies})
}
