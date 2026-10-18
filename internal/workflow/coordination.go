package workflow

import (
	"strings"
	"unicode/utf8"
)

type CoordinationRequest struct {
	WorkspaceID    string   `json:"workspaceId"`
	JobID          string   `json:"jobId"`
	ScopeVersion   int      `json:"scopeVersion"`
	Round          int      `json:"round"`
	Trigger        string   `json:"trigger"`
	Locale         string   `json:"locale"`
	JobTitle       string   `json:"jobTitle"`
	AgreedScope    string   `json:"agreedScope"`
	Message        string   `json:"message,omitempty"`
	Reply          string   `json:"reply,omitempty"`
	EvidenceLabels []string `json:"evidenceLabels"`
}

type CoordinationResponse struct {
	Question       string `json:"question"`
	Summary        string `json:"summary"`
	ProposedAction string `json:"proposedAction"`
	NeedsOperator  bool   `json:"needsOperator"`
	Reason         string `json:"reason"`
}

type CoordinationTask struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspaceId"`
	JobID          string `json:"jobId"`
	ScopeVersion   int    `json:"scopeVersion"`
	Round          int    `json:"round"`
	Trigger        string `json:"trigger"`
	RecipientRole  string `json:"recipientRole"`
	Status         string `json:"status"`
	Question       string `json:"question"`
	Reply          string `json:"reply"`
	Summary        string `json:"summary"`
	ProposedAction string `json:"proposedAction"`
	Outcome        string `json:"outcome"`
	Revision       int64  `json:"revision"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func ValidCoordinationResponse(response CoordinationResponse, hasReply bool) bool {
	if !utf8.ValidString(response.Question) || !utf8.ValidString(response.Summary) || !utf8.ValidString(response.Reason) || len(response.Question) > 500 || len(response.Summary) > 2000 || len(response.Reason) > 500 {
		return false
	}
	switch response.ProposedAction {
	case "request-work-note", "review-scope", "review-completion", "operator-review":
	default:
		return false
	}
	if response.NeedsOperator {
		return response.ProposedAction == "operator-review" && len(strings.TrimSpace(response.Reason)) >= 5
	}
	if response.ProposedAction == "operator-review" {
		return false
	}
	if hasReply {
		return len(strings.TrimSpace(response.Summary)) >= 12 && response.ProposedAction != "request-work-note"
	}
	return len(strings.TrimSpace(response.Question)) >= 10 && response.ProposedAction != "review-completion"
}

func ValidCoordinationResponseForRequest(input CoordinationRequest, response CoordinationResponse) bool {
	if !ValidCoordinationResponse(response, input.Reply != "") {
		return false
	}
	if response.NeedsOperator {
		return true
	}
	if input.Trigger == "post-booking-scope" {
		return response.ProposedAction == "review-scope"
	}
	if input.Reply == "" {
		return response.ProposedAction == "request-work-note"
	}
	return response.ProposedAction == "review-completion"
}

func CoordinationReplyUncertain(reply string) bool {
	lower := strings.ToLower(strings.TrimSpace(reply))
	lower = strings.NewReplacer("’", "'", "ik weet het niet", "weet niet").Replace(lower)
	for _, phrase := range []string{"don't know", "do not know", "not sure", "unsure", "can't explain", "cannot explain", "won't explain", "refuse", "weet niet", "geen idee", "niet zeker", "kan niet uitleggen", "wil niet"} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return len(lower) < 12
}

func HasCurrentWorkNote(job Job) bool {
	if !job.DetailAdded {
		return false
	}
	for _, e := range job.Evidence {
		if e.Kind == "note" && CurrentEvidence(job, e) && len(strings.TrimSpace(e.Description)) >= 12 {
			return true
		}
	}
	return false
}
