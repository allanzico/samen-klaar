package workflow

import "time"

// ProfessionalDashboard summarizes the signed-in professional's complete history,
// independently of the paginated jobs list. Currencies are never added together.
type ProfessionalDashboard struct {
	Balances             []ProfessionalBalance `json:"balances"`
	UpcomingVisits       int64                 `json:"upcomingVisits"`
	CompletedJobs        int64                 `json:"completedJobs"`
	AwaitingConfirmation int64                 `json:"awaitingConfirmation"`
	AsOf                 time.Time             `json:"asOf"`
}

type ProfessionalBalance struct {
	PlatformFeesMinor    int64  `json:"platformFeesMinor"`
	Currency             string `json:"currency"`
	ReceivedMinor        int64  `json:"receivedMinor"`
	AwaitingPaymentMinor int64  `json:"awaitingPaymentMinor"`
}
