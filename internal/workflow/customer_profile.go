package workflow

// CustomerProfile is private account information, separate from an agreed job.
// Editing it never changes an existing visit address or agreement.
type CustomerProfile struct {
	DisplayName string    `json:"displayName"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Location    *Location `json:"location,omitempty"`
}
