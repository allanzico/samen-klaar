package workflow

type CaseNote struct {
	ID            int64  `json:"id"`
	RecipientRole string `json:"recipientRole,omitempty"`
	Note          string `json:"note"`
	CreatedAt     string `json:"createdAt"`
}
type CaseDecision struct {
	Actor        string `json:"actor"`
	Action       string `json:"action"`
	ScopeVersion int    `json:"scopeVersion"`
	At           string `json:"at"`
}
type OperatorCaseDetail struct {
	Case          OperatorCase   `json:"case"`
	State         State          `json:"state"`
	AssignedTo    string         `json:"assignedTo,omitempty"`
	Status        string         `json:"status"`
	Outcome       string         `json:"outcome,omitempty"`
	Notes         []CaseNote     `json:"notes"`
	Decisions     []CaseDecision `json:"decisions"`
	PaymentStatus string         `json:"paymentStatus"`
}
