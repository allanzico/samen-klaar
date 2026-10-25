package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type BookingAutonomy struct {
	Mode                  string `json:"mode"`
	Version               int    `json:"version"`
	ScopeVersion          int    `json:"scopeVersion"`
	AmountMinor           int64  `json:"amountMinor"`
	Currency              string `json:"currency"`
	ProfessionalAccountID string `json:"professionalAccountId"`
	ProfessionalProfileID string `json:"professionalProfileId"`
	DecidedAt             string `json:"decidedAt"`
}

func BookingAutonomyKey(job Job) string {
	return fmt.Sprintf("%s|%d|%d|%s|%s", job.ID, job.ScopeVersion, job.Quote.TotalMinor, job.Quote.Currency, job.ProfessionalProfileID)
}
func BookingAutonomyCurrent(job Job) bool {
	a := job.BookingAutonomy
	return a != nil && a.Mode == "automatic" && a.ScopeVersion == job.ScopeVersion && a.AmountMinor == job.Quote.TotalMinor && a.Currency == job.Quote.Currency && a.ProfessionalAccountID == job.ProfessionalAccountID && a.ProfessionalProfileID == job.ProfessionalProfileID && a.DecidedAt != ""
}
func BookingAutonomyChoiceNeeded(job Job) bool {
	return job.BookingAutonomyRequired && (job.BookingAutonomy == nil || job.BookingAutonomy.Mode != "manual" && !BookingAutonomyCurrent(job))
}
func setBookingAutonomy(state *State, input Action) error {
	job := &state.Job
	if input.Actor != "customer" || input.ByAgent || job.ID == "" || job.Paid {
		return errors.New("only the customer can choose booking autonomy")
	}
	parts := strings.SplitN(input.Value, "|", 2)
	mode := parts[0]
	if mode != "manual" && mode != "automatic" {
		return errors.New("choose automatic review and release, or review and pay yourself")
	}
	if mode == "automatic" {
		if state.Cancelled || state.CancellationReview != nil && state.CancellationReview.Status == "pending" {
			return errors.New("this booking requires customer review")
		}
		if job.Appointment != nil || job.WorkStarted || job.CompletionApproved || len(parts) != 2 || parts[1] != BookingAutonomyKey(*job) || pendingPrice(*job) || job.Change != nil && job.Change.Status == "pending" {
			return errors.New("review the current price and choose autonomy before booking")
		}
		job.PaymentAuthorizationTiming = PaymentAuthorizationAtBooking
	}
	version := 1
	if job.BookingAutonomy != nil {
		version = job.BookingAutonomy.Version + 1
	}
	job.BookingAutonomy = &BookingAutonomy{Mode: mode, Version: version, ScopeVersion: job.ScopeVersion, AmountMinor: job.Quote.TotalMinor, Currency: job.Quote.Currency, ProfessionalAccountID: job.ProfessionalAccountID, ProfessionalProfileID: job.ProfessionalProfileID, DecidedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if mode == "manual" {
		if job.PaymentApprovedByAgent {
			job.PaymentApprovedVersion = 0
			job.PaymentApprovedAt = ""
			job.PaymentApprovedByAgent = false
		}
		if job.CompletionApprovedByAgent {
			job.CompletionApproved = false
			job.CompletionApprovedAt = ""
			job.CompletionApprovedByAgent = false
		}
	}
	if state.AgentDialog != nil && state.AgentDialog.Stage == "booking-autonomy" {
		state.AgentDialog.Awaiting = false
		state.AgentDialog.Stage = "idle"
	}
	addEvent(job, "Customer chose "+mode+" completion review and payment release for this booking", "done")
	return nil
}

// ReviewableWorkItems gives evidence review and payment delegation the same IDs,
// including older requests that predate explicit work items.
func ReviewableWorkItems(job Job) []WorkItem {
	if len(job.WorkItems) > 0 {
		return job.WorkItems
	}
	items := []WorkItem{}
	for i, line := range job.Quote.Lines {
		if line.Basis == "Published provider call-out fee" || strings.HasPrefix(line.Basis, "Customer and professional agreed:") {
			continue
		}
		items = append(items, WorkItem{ID: fmt.Sprintf("work-%d", i+1), Description: line.Label, Kind: "file"})
	}
	return items
}

func CompletionEvidenceFingerprint(job Job) string {
	files := []Evidence{}
	for _, e := range job.Evidence {
		if CurrentEvidence(job, e) {
			files = append(files, e)
		}
	}
	data, _ := json.Marshal(struct {
		Scope, Round int
		Items        []WorkItem
		Files        []Evidence
	}{job.ScopeVersion, EvidenceRound(job), ReviewableWorkItems(job), files})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func BindEvidenceReview(job Job, review *EvidenceReview) {
	if review == nil {
		return
	}
	review.ScopeVersion = job.ScopeVersion
	review.CompletionRound = EvidenceRound(job)
	review.EvidenceFingerprint = CompletionEvidenceFingerprint(job)
}

// ValidateAutonomousCompletion requires current documented coverage, rather than
// treating the mere existence of an upload or professional's note as approval.
func ValidateAutonomousCompletion(job Job) error {
	if !BookingAutonomyCurrent(job) {
		return errors.New("booking autonomy is not enabled for the current agreement")
	}
	if !job.QuoteApproved || !job.WorkStarted || !job.EvidenceSubmitted || !job.DetailAdded || job.PaymentRefunded || job.Change != nil && job.Change.Status == "pending" || job.Dispute != nil && job.Dispute.Status != "resolved" {
		return errors.New("completion needs customer review")
	}
	if err := ValidateCompletionEvidence(job, true); err != nil {
		return err
	}
	review := job.EvidenceReview
	items := ReviewableWorkItems(job)
	if review == nil || review.ReviewedAt == "" || strings.TrimSpace(review.Summary) == "" || review.ScopeVersion != job.ScopeVersion || review.CompletionRound != EvidenceRound(job) || review.EvidenceFingerprint != CompletionEvidenceFingerprint(job) || len(items) == 0 || len(review.Coverage) != len(items) {
		return errors.New("current completion photos must be checked before automatic approval")
	}
	seen := map[string]bool{}
	for _, finding := range review.Coverage {
		if finding.Coverage != "documented" || strings.TrimSpace(finding.Observation) == "" || seen[finding.ScopeItem] {
			return errors.New("unclear or missing evidence needs customer review")
		}
		seen[finding.ScopeItem] = true
		var item *WorkItem
		for i := range items {
			if items[i].ID == finding.ScopeItem {
				item = &items[i]
				break
			}
		}
		if item == nil {
			return errors.New("evidence review referenced an unknown work item")
		}
		if item.Kind != "note" && len(finding.EvidenceIDs) == 0 {
			return errors.New("documented work needs a current uploaded file reference")
		}
		for _, id := range finding.EvidenceIDs {
			found := false
			for _, e := range job.Evidence {
				if e.AssetID == id && CurrentEvidence(job, e) && (len(job.WorkItems) == 0 || item.Kind == "note" || slices.Contains(e.WorkItemIDs, item.ID)) {
					found = true
					break
				}
			}
			if !found {
				return errors.New("evidence review did not cite the current work item's files")
			}
		}
	}
	return nil
}

func (c AgentContext) autonomyAction(code, action string) AgentDecision {
	job := c.State.Job
	return act(code, Action{Action: action, AutonomyVersion: job.BookingAutonomy.Version, ExpectedScope: job.ScopeVersion, ExpectedEvidenceRound: EvidenceRound(job), ExpectedEvidenceFingerprint: CompletionEvidenceFingerprint(job)}, nil)
}
func (c AgentContext) dialogBookingAutonomy() AgentDecision {
	d := c.dialog()
	key := BookingAutonomyKey(c.State.Job)
	if d.Stage == "booking-autonomy" && d.PendingID == key && d.Awaiting {
		return c.waiting()
	}
	return c.say("ask-booking-autonomy", true, c.text("booking_autonomy", map[string]string{"amount": c.money(c.State.Job.Quote.TotalMinor)}), []Choice{{ID: "automatic|" + key, Label: c.text("autonomy_automatic", nil)}, {ID: "manual", Label: c.text("autonomy_manual", nil)}}, func(d *AgentDialog) { d.Stage = "booking-autonomy"; d.PendingID = key; d.Pending = "" })
}
