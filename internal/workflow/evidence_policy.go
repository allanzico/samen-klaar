package workflow

import (
	"errors"
	"slices"
	"strings"
	"time"
)

type EvidenceRequirement struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}
type WorkItem struct {
	ID           string `json:"id"`
	Description  string `json:"description"`
	Kind         string `json:"kind"`
	AddedInScope int    `json:"addedInScope"`
}
type EvidenceResolution struct {
	WorkItemID      string `json:"workItemId"`
	ScopeVersion    int    `json:"scopeVersion"`
	CompletionRound int    `json:"completionRound"`
	Rationale       string `json:"rationale"`
	ResolvedAt      string `json:"resolvedAt"`
}
type ServiceEvidencePolicy struct {
	ProfileID    string                `json:"profileId"`
	Requirements []EvidenceRequirement `json:"requirements"`
	Proposed     []EvidenceRequirement `json:"proposed,omitempty"`
	Status       string                `json:"status"`
	ReviewedAt   string                `json:"reviewedAt,omitempty"`
}

func ValidateEvidenceRequirements(items []EvidenceRequirement) error {
	if len(items) < 1 || len(items) > 12 {
		return errors.New("define 1 to 12 completion evidence requirements")
	}
	seen := map[string]bool{}
	for _, item := range items {
		if len(strings.TrimSpace(item.Label)) < 5 || len(item.Label) > 200 || item.ID == "" || len(item.ID) > 80 || seen[item.ID] || (item.Kind != "photo" && item.Kind != "document" && item.Kind != "note" && item.Kind != "file") {
			return errors.New("each requirement needs a unique ID, a descriptive label and photo, document or note type")
		}
		seen[item.ID] = true
	}
	return nil
}
func EvidenceRound(job Job) int {
	if job.CompletionRound < 1 {
		return 1
	}
	return job.CompletionRound
}
func CurrentEvidence(job Job, e Evidence) bool {
	if e.CompletionRound == 0 && e.ScopeVersion == 0 {
		return len(job.WorkItems) == 0 && EvidenceRound(job) == 1
	}
	return e.ScopeVersion == job.ScopeVersion && e.CompletionRound == EvidenceRound(job)
}
func BindEvidence(job Job, e Evidence, ids []string) (Evidence, error) {
	if e.ScopeVersion != 0 && e.ScopeVersion != job.ScopeVersion || e.CompletionRound != 0 && e.CompletionRound != EvidenceRound(job) {
		return e, errors.New("the completion scope changed during the upload; upload for the current scope")
	}
	e.ScopeVersion = job.ScopeVersion
	e.CompletionRound = EvidenceRound(job)
	if len(job.WorkItems) == 0 {
		return e, nil
	}
	eligible := []string{}
	for _, item := range job.WorkItems {
		if item.Kind == e.Kind || item.Kind == "file" && (e.Kind == "photo" || e.Kind == "document") {
			eligible = append(eligible, item.ID)
		}
	}
	if len(eligible) == 0 && e.Kind == "note" {
		return e, nil
	}
	if len(ids) == 0 && len(eligible) == 1 {
		ids = eligible
	}
	if len(ids) == 0 {
		return e, errors.New("select the agreed work item this evidence documents")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		found := false
		for _, candidate := range eligible {
			if id == candidate {
				found = true
			}
		}
		if !found || seen[id] {
			return e, errors.New("evidence must reference a matching current work item")
		}
		seen[id] = true
	}
	e.WorkItemIDs = append([]string(nil), ids...)
	return e, nil
}
func EvidenceGapResolved(job Job, id string) bool {
	for _, r := range job.EvidenceResolutions {
		if r.WorkItemID == id && r.ScopeVersion == job.ScopeVersion && r.CompletionRound == EvidenceRound(job) {
			return true
		}
	}
	return false
}
func MissingEvidence(job Job, includeNotes bool) []string {
	missing := []string{}
	for _, item := range job.WorkItems {
		if item.Kind == "note" && !includeNotes {
			continue
		}
		covered := EvidenceGapResolved(job, item.ID)
		for _, e := range job.Evidence {
			if !CurrentEvidence(job, e) || (e.Kind != item.Kind && !(item.Kind == "file" && e.AssetID != "")) || e.Kind != "note" && (e.AssetID == "" || e.ContentType == "") {
				continue
			}
			for _, id := range e.WorkItemIDs {
				if id == item.ID {
					covered = true
				}
			}
		}
		if !covered {
			missing = append(missing, item.ID)
		}
	}
	if includeNotes && job.EvidenceReview != nil {
		for _, finding := range job.EvidenceReview.Coverage {
			if (finding.Coverage == "unclear" || finding.Coverage == "not_shown") && !EvidenceGapResolved(job, finding.ScopeItem) {
				missing = append(missing, finding.ScopeItem)
			}
		}
	}
	return missing
}

// MissingEvidenceLabels names what is still missing in words a person can act on: the description of
// each required work item (or the scope item an evidence check flagged).
func MissingEvidenceLabels(job Job, includeNotes bool) []string {
	names := map[string]string{}
	for _, item := range job.WorkItems {
		names[item.ID] = item.Description
	}
	labels := []string{}
	for _, id := range MissingEvidence(job, includeNotes) {
		label := strings.TrimSpace(names[id])
		if label == "" && job.EvidenceReview != nil {
			// A scope item the file check could not confirm: say what the check saw, not an internal id.
			for _, finding := range job.EvidenceReview.Coverage {
				if finding.ScopeItem == id && strings.TrimSpace(finding.Observation) != "" {
					label = "the file check says: " + strings.TrimSpace(strings.TrimRight(strings.TrimSpace(finding.Observation), "."))
					if len([]rune(label)) > 220 {
						label = string([]rune(label)[:220]) + "…"
					}
					break
				}
			}
		}
		if label == "" {
			label = id
		}
		if !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}
	return labels
}

func ValidateCompletionEvidence(job Job, includeNotes bool) error {
	hasFile := false
	for _, e := range job.Evidence {
		if CurrentEvidence(job, e) && e.AssetID != "" && e.ContentType != "" {
			hasFile = true
		}
	}
	if !hasFile {
		return errors.New("upload current-scope evidence for this completion attempt")
	}
	if missing := MissingEvidenceLabels(job, includeNotes); len(missing) > 0 {
		return errors.New("required completion documentation is missing or unresolved: " + strings.Join(missing, "; ") + ". Supply it or request human review")
	}
	return nil
}
func ResolveEvidenceGap(job *Job, id, rationale string, now time.Time) error {
	if len(strings.TrimSpace(rationale)) < 10 || len(rationale) > 1000 || job.Paid || job.CompletionApproved {
		return errors.New("record a 10 to 1000 character documentation decision before approval")
	}
	found := false
	for _, item := range job.WorkItems {
		if item.ID == id {
			found = true
		}
	}
	if job.EvidenceReview != nil {
		for _, f := range job.EvidenceReview.Coverage {
			if f.ScopeItem == id {
				found = true
			}
		}
	}
	if !found {
		return errors.New("evidence work item was not found")
	}
	job.EvidenceResolutions = append(job.EvidenceResolutions, EvidenceResolution{WorkItemID: id, ScopeVersion: job.ScopeVersion, CompletionRound: EvidenceRound(*job), Rationale: strings.TrimSpace(rationale), ResolvedAt: now.UTC().Format(time.RFC3339)})
	return nil
}
