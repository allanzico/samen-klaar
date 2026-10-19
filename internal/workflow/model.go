package workflow

import (
	"errors"
	"strings"
	"time"
)

type Quote struct {
	Currency        string      `json:"currency"`
	Lines           []QuoteLine `json:"lines"`
	TotalMinor      int64       `json:"totalMinor"`
	MaterialsPolicy string      `json:"materialsPolicy"`
	Exclusions      []string    `json:"exclusions"`
	DurationMinutes int         `json:"durationMinutes"`
	Expires         string      `json:"expires"`
}

type QuoteLine struct {
	Label       string `json:"label"`
	AmountMinor int64  `json:"amountMinor"`
	Basis       string `json:"basis"`
}

type Appointment struct {
	SlotID   string `json:"slotId"`
	StartsAt string `json:"startsAt"`
	Timezone string `json:"timezone"`
	StartUTC string `json:"startUtc"`
	EndUTC   string `json:"endUtc"`
}

type ProfessionalProfile struct {
	BaseLocation       *Location `json:"baseLocation,omitempty"`
	TravelRadiusKM     int       `json:"travelRadiusKm,omitempty"`
	ID                 string    `json:"id"`
	AccountID          string    `json:"-"`
	DisplayName        string    `json:"displayName"`
	Category           string    `json:"category"`
	ServiceName        string    `json:"serviceName"`
	ServiceDescription string    `json:"serviceDescription"`
	ServiceArea        string    `json:"serviceArea"`
	Latitude           float64   `json:"latitude"`
	Longitude          float64   `json:"longitude"`
	Currency           string    `json:"currency"`
	PriceMinor         int64     `json:"priceMinor"`
	CalloutMinor       int64     `json:"calloutMinor"`
	DurationMinutes    int       `json:"durationMinutes"`
	MaterialsPolicy    string    `json:"materialsPolicy"`
	Exclusions         []string  `json:"exclusions"`
	Status             string    `json:"status"`
}

type AvailabilitySlot struct {
	Appointment
	Status string `json:"status"`
}

type ProfessionalApplication struct {
	Profile ProfessionalProfile      `json:"profile"`
	Email   string                   `json:"email,omitempty"`
	Checks  ProfessionalReviewChecks `json:"checks"`
}

type PayPalPayoutSetupStatus struct {
	Status                string `json:"status"`
	Connected             bool   `json:"connected"`
	PaymentsReceivable    bool   `json:"paymentsReceivable"`
	PrimaryEmailConfirmed bool   `json:"primaryEmailConfirmed"`
}

// ProfessionalReviewChecks records an operator's review outcome without
// retaining identity documents, payment identifiers, or qualification files.
type ProfessionalReviewChecks struct {
	IdentityBusiness    string `json:"identityBusiness"`
	PayPalOnboarding    string `json:"paypalOnboarding"`
	Qualification       string `json:"qualification"`
	Insurance           string `json:"insurance"`
	QualificationReason string `json:"qualificationRationale"`
	InsuranceReason     string `json:"insuranceRationale"`
}

func (checks ProfessionalReviewChecks) Validate() error {
	if checks.IdentityBusiness != "pending" && checks.IdentityBusiness != "verified" {
		return errors.New("identity and business review must be pending or verified")
	}
	if checks.PayPalOnboarding != "pending" && checks.PayPalOnboarding != "verified" {
		return errors.New("PayPal onboarding must be pending or verified")
	}
	if !validProfessionalReviewStatus(checks.Qualification) || !validProfessionalReviewStatus(checks.Insurance) {
		return errors.New("qualification and insurance review must be pending, verified, or not required")
	}
	if len(checks.QualificationReason) > 500 || len(checks.InsuranceReason) > 500 {
		return errors.New("review rationale must be 500 characters or fewer")
	}
	if checks.Qualification == "not_required" && strings.TrimSpace(checks.QualificationReason) == "" {
		return errors.New("explain why qualification checks are not required")
	}
	if checks.Insurance == "not_required" && strings.TrimSpace(checks.InsuranceReason) == "" {
		return errors.New("explain why insurance checks are not required")
	}
	return nil
}

func validProfessionalReviewStatus(status string) bool {
	return status == "pending" || status == "verified" || status == "not_required"
}

func (checks ProfessionalReviewChecks) ReadyToApprove() bool {
	return checks.IdentityBusiness == "verified" && checks.PayPalOnboarding == "verified" &&
		(checks.Qualification == "verified" || checks.Qualification == "not_required" && strings.TrimSpace(checks.QualificationReason) != "") &&
		(checks.Insurance == "verified" || checks.Insurance == "not_required" && strings.TrimSpace(checks.InsuranceReason) != "")
}

type OperatorCase struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"accountId"`
	WorkspaceID string    `json:"workspaceId"`
	Kind        string    `json:"kind"`
	Description string    `json:"description"`
	ReportedAt  string    `json:"reportedAt,omitempty"`
	Status      string    `json:"status,omitempty"`
	Category    string    `json:"category,omitempty"`
	Detail      string    `json:"detail,omitempty"`
	Urgency     string    `json:"urgency,omitempty"`
	Location    *Location `json:"location,omitempty"`
	JobID       string    `json:"jobId,omitempty"`
	JobTitle    string    `json:"jobTitle,omitempty"`
	Paid        bool      `json:"paid,omitempty"`
	// ReasonCode and Responsible let a client show a stalled-job reason in the viewer's language.
	ReasonCode  string `json:"reasonCode,omitempty"`
	Responsible string `json:"responsible,omitempty"`
}

// NotificationDeliveryHealth contains aggregate operational counts only; it
// deliberately omits recipient addresses and notification contents.
type NotificationDeliveryHealth struct {
	Failed       int64 `json:"failed"`
	Unconfigured int64 `json:"unconfigured"`
	Stalled      int64 `json:"stalled"`
}

type CatalogOffer struct {
	BaseLocation         *Location
	TravelRadiusKM       int
	EvidenceRequirements []EvidenceRequirement
	ProfileID            string
	AccountID            string
	DisplayName          string
	Category             string
	ServiceName          string
	ServiceDescription   string
	ServiceArea          string
	Currency             string
	PriceMinor           int64
	CalloutMinor         int64
	DurationMinutes      int
	MaterialsPolicy      string
	Exclusions           []string
}

type Evidence struct {
	ScopeVersion    int      `json:"scopeVersion,omitempty"`
	CompletionRound int      `json:"completionRound,omitempty"`
	WorkItemIDs     []string `json:"workItemIds,omitempty"`
	AssetID         string   `json:"assetId,omitempty"`
	Label           string   `json:"label"`
	Description     string   `json:"description"`
	Kind            string   `json:"kind"`
	ContentType     string   `json:"contentType,omitempty"`
}

type EvidenceFinding struct {
	ScopeItem   string   `json:"scopeItem"`
	EvidenceIDs []string `json:"evidenceIds"`
	Coverage    string   `json:"coverage"`
	Observation string   `json:"observation"`
}

type EvidenceReview struct {
	Summary     string            `json:"summary"`
	Coverage    []EvidenceFinding `json:"coverage"`
	Limitations string            `json:"limitations"`
	Model       string            `json:"-"`
	ReviewedAt  string            `json:"reviewedAt"`
}

// JobReview is created only for a completed job whose payment was captured.
// It stays attached to the request so its verified-job provenance is explicit.
type JobReview struct {
	Rating               int    `json:"rating"`
	Comment              string `json:"comment,omitempty"`
	CreatedAt            string `json:"createdAt"`
	ProfessionalResponse string `json:"professionalResponse,omitempty"`
	RespondedAt          string `json:"respondedAt,omitempty"`
}

type ScopeChange struct {
	CreatedAt    string `json:"createdAt,omitempty"`
	Status       string `json:"status"`
	Description  string `json:"description"`
	AmountMinor  int64  `json:"amountMinor"`
	ProposedFrom int    `json:"proposedFrom"`
}

type ScopeSnapshot struct {
	WorkItems  []WorkItem  `json:"workItems,omitempty"`
	Version    int         `json:"version"`
	TotalMinor int64       `json:"totalMinor"`
	Lines      []QuoteLine `json:"lines"`
}

type CancellationRequest struct {
	Kind                string       `json:"kind,omitempty"`
	ProposalID          string       `json:"proposalId,omitempty"`
	ProposedAppointment *Appointment `json:"proposedAppointment,omitempty"`
	Status              string       `json:"status"`
	RequestedBy         string       `json:"requestedBy"`
	Reason              string       `json:"reason,omitempty"`
	RequestedAt         string       `json:"requestedAt"`
	Resolution          string       `json:"resolution,omitempty"`
	ResolvedAt          string       `json:"resolvedAt,omitempty"`
}

type DisputeCase struct {
	Status     string `json:"status"`
	ReportedBy string `json:"reportedBy"`
	Reason     string `json:"reason"`
	ReportedAt string `json:"reportedAt"`
	Resolution string `json:"resolution,omitempty"`
	ResolvedAt string `json:"resolvedAt,omitempty"`
}

// AppointmentConfirmation records the professional response window for a
// customer-selected published slot. The prior appointment stays in this
// record after a decline or expiry so the request timeline remains auditable.
type AppointmentConfirmation struct {
	Status      string      `json:"status"`
	DueAt       string      `json:"dueAt,omitempty"`
	RespondedAt string      `json:"respondedAt,omitempty"`
	Appointment Appointment `json:"appointment"`
}

type Event struct {
	At   string `json:"at,omitempty"`
	Text string `json:"text"`
	Kind string `json:"kind"`
	When string `json:"when"`
}

type PriceProposal struct {
	CreatedAt    string `json:"createdAt,omitempty"`
	ID           string `json:"id"`
	Status       string `json:"status"`
	Actor        string `json:"actor"`
	AmountMinor  int64  `json:"amountMinor"`
	Note         string `json:"note"`
	ScopeVersion int    `json:"scopeVersion"`
}

type DateProposal struct {
	CreatedAt   string      `json:"createdAt,omitempty"`
	ID          string      `json:"id"`
	Status      string      `json:"status"`
	Actor       string      `json:"actor"`
	Note        string      `json:"note"`
	Appointment Appointment `json:"appointment"`
}

type Job struct {
	ServiceLocation            *Location                `json:"serviceLocation,omitempty"`
	PolicyRequirements         []PolicyRequirement      `json:"policyRequirements,omitempty"`
	PolicyAcknowledgments      []PolicyAcknowledgment   `json:"policyAcknowledgments,omitempty"`
	WorkItems                  []WorkItem               `json:"workItems,omitempty"`
	CompletionRound            int                      `json:"completionRound,omitempty"`
	EvidenceResolutions        []EvidenceResolution     `json:"evidenceResolutions,omitempty"`
	ServiceDescription         string                   `json:"serviceDescription,omitempty"`
	ServiceAddress             string                   `json:"serviceAddress,omitempty"`
	AppointmentConfirmation    *AppointmentConfirmation `json:"appointmentConfirmation,omitempty"`
	PriceProposal              *PriceProposal           `json:"priceProposal,omitempty"`
	DateProposal               *DateProposal            `json:"dateProposal,omitempty"`
	Dispute                    *DisputeCase             `json:"dispute,omitempty"`
	Location                   *Location                `json:"location,omitempty"`
	ID                         string                   `json:"id"`
	ProfessionalProfileID      string                   `json:"professionalProfileId,omitempty"`
	ProfessionalAccountID      string                   `json:"professionalAccountId,omitempty"`
	Title                      string                   `json:"title"`
	Customer                   string                   `json:"customer"`
	Professional               string                   `json:"professional"`
	Description                string                   `json:"description"`
	Category                   string                   `json:"category"`
	ScopeVersion               int                      `json:"scopeVersion"`
	ScopeHistory               []ScopeSnapshot          `json:"scopeHistory"`
	Quote                      Quote                    `json:"quote"`
	QuoteApproved              bool                     `json:"quoteApproved"`
	NeedsClarification         bool                     `json:"needsClarification"`
	ClarificationQuestion      string                   `json:"clarificationQuestion"`
	ClarificationAnswer        string                   `json:"clarificationAnswer"`
	IntakeRoute                string                   `json:"intakeRoute,omitempty"`
	Appointment                *Appointment             `json:"appointment"`
	WorkStarted                bool                     `json:"workStarted"`
	WorkStartedAt              string                   `json:"workStartedAt,omitempty"`
	EvidenceSubmitted          bool                     `json:"evidenceSubmitted"`
	EvidenceSubmittedAt        string                   `json:"evidenceSubmittedAt,omitempty"`
	DetailsRequested           bool                     `json:"detailsRequested"`
	DetailAdded                bool                     `json:"detailAdded"`
	CompletionApproved         bool                     `json:"completionApproved"`
	CompletionApprovedAt       string                   `json:"completionApprovedAt,omitempty"`
	PaymentApprovedVersion     int                      `json:"paymentApprovedVersion"`
	PaymentApprovedAt          string                   `json:"paymentApprovedAt,omitempty"`
	DetailAddedAt              string                   `json:"detailAddedAt,omitempty"`
	PaymentAuthorizedVersion   int                      `json:"paymentAuthorizedVersion"`
	PaymentAuthorizationTiming string                   `json:"paymentAuthorizationTiming,omitempty"`
	Paid                       bool                     `json:"paid"`
	PaymentRefunded            bool                     `json:"paymentRefunded,omitempty"`
	PaymentTransactionID       string                   `json:"paymentTransactionId,omitempty"`
	RefundTransactionID        string                   `json:"refundTransactionId,omitempty"`
	Evidence                   []Evidence               `json:"evidence"`
	EvidenceReview             *EvidenceReview          `json:"evidenceReview,omitempty"`
	Review                     *JobReview               `json:"review,omitempty"`
	Change                     *ScopeChange             `json:"change"`
	Timeline                   []Event                  `json:"timeline"`
}

type State struct {
	ParentRequestID    string               `json:"parentRequestId,omitempty"`
	Cancelled          bool                 `json:"cancelled"`
	CancelledBy        string               `json:"cancelledBy,omitempty"`
	CancellationReview *CancellationRequest `json:"cancellationReview,omitempty"`
	WorkspaceID        string               `json:"workspaceId"`
	Intake             *Intake              `json:"intake,omitempty"`
	RequestPhotos      []RequestPhoto       `json:"requestPhotos,omitempty"`
	Job                Job                  `json:"job"`
	AvailableSlots     []Appointment        `json:"availableSlots"`
	// AgentDialog is the customer's conversation with their assistant about scheduling. It holds the
	// customer's stated availability, so the professional's view omits it.
	AgentDialog *AgentDialog `json:"agentDialog,omitempty"`
	// ProDialog is the professional's private conversation with their assistant.
	ProDialog *AgentDialog `json:"proDialog,omitempty"`
}

type Action struct {
	ChoiceID                         string            `json:"choiceId,omitempty"`
	ByAgent                          bool              `json:"-"`
	Audience                         string            `json:"-"`
	Dialog                           *AgentDialog      `json:"-"`
	NoteCode                         string            `json:"-"`
	NoteParams                       map[string]string `json:"-"`
	PhotoAddedAt                     string            `json:"-"`
	PolicyKind                       string            `json:"policyKind,omitempty"`
	PolicyVersion                    string            `json:"policyVersion,omitempty"`
	PublishedPolicy                  *ConsumerPolicy   `json:"-"`
	Locale                           string            `json:"-"`
	WorkItemIDs                      []string          `json:"workItemIds,omitempty"`
	ExpectedEvidenceRound            int               `json:"-"`
	Rating                           int               `json:"rating,omitempty"`
	PriceAmountMinor                 int64             `json:"priceAmountMinor,omitempty"`
	ProposedAppointment              *Appointment      `json:"proposedAppointment,omitempty"`
	ProposalID                       string            `json:"proposalId,omitempty"`
	WorkspaceID                      string            `json:"workspaceId,omitempty"`
	ChangeAmountMinor                int64             `json:"changeAmountMinor,omitempty"`
	ExpectedScope                    int               `json:"-"`
	Location                         *Location         `json:"location,omitempty"`
	Matches                          []Match           `json:"-"`
	SelectedOffer                    *CatalogOffer     `json:"-"`
	AvailableSlots                   []Appointment     `json:"-"`
	CustomerName                     string            `json:"-"`
	CustomerAccountID                string            `json:"-"`
	Suggestion                       *IntakeSuggestion `json:"-"`
	Action                           string            `json:"action"`
	Actor                            string            `json:"actor"`
	Value                            string            `json:"value"`
	Category                         string            `json:"category"`
	Evidence                         Evidence          `json:"evidence,omitempty"`
	EvidenceReview                   *EvidenceReview   `json:"evidenceReview,omitempty"`
	SuggestedQuestion                string            `json:"-"`
	PayPalCaptured                   bool              `json:"-"`
	PaymentCaptureID                 string            `json:"-"`
	PaymentAuthorizationTiming       string            `json:"paymentAuthorizationTiming,omitempty"`
	PayPalAuthorized                 bool              `json:"-"`
	PaymentAuthorizationID           string            `json:"-"`
	PaymentAuthorizationRequestID    string            `json:"-"`
	PaymentAuthorizationScopeVersion int               `json:"-"`
}

func NewState() State {
	return State{
		Job:            Job{Evidence: []Evidence{}, Timeline: []Event{}},
		AvailableSlots: []Appointment{},
	}
}

func addEvent(job *Job, text, kind string) {
	job.Timeline = append(job.Timeline, Event{Text: text, Kind: kind, When: time.Now().Format("15:04"), At: time.Now().UTC().Format(time.RFC3339Nano)})
}

func scopeSnapshot(job Job) ScopeSnapshot {
	lines := append([]QuoteLine(nil), job.Quote.Lines...)
	return ScopeSnapshot{Version: job.ScopeVersion, TotalMinor: job.Quote.TotalMinor, Lines: lines, WorkItems: append([]WorkItem(nil), job.WorkItems...)}
}

// EvidenceBlob is the in-memory input passed to the optional evidence reviewer.
type EvidenceBlob struct {
	ID          string
	ContentType string
	Label       string
	Data        []byte
}

type IntakeSuggestion struct {
	Category string   `json:"category"`
	Options  []string `json:"options"`
	Question string   `json:"question"`
	Route    string   `json:"route"`
}
