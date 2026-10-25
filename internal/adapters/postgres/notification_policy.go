package postgres

import "slices"

// Email is reserved for material booking, completion, payment and case events.
// All other events (including new event types) remain in-app only. Keep this
// list shared by enqueue, delivery, recovery and operator delivery health.
var emailNotificationEvents = []string{
	"approve-and-book", "book-appointment",
	"confirm-appointment", "decline-appointment", "appointment-expired",
	"accept-date", "cancel-request", "late-reschedule-requested", "withdrawal-requested",
	"submit-evidence", "confirm-payment", "refund-completed", "payment-review", "issue-reported",
	"approve-late-cancellation", "decline-late-cancellation",
	"approve-late-reschedule", "decline-late-reschedule",
	"approve-withdrawal", "decline-withdrawal", "refer-withdrawal-to-work-review",
	"request-work-rework", "close-job-issue", "operator-contact",
}

func notificationUsesEmail(event string) bool {
	return slices.Contains(emailNotificationEvents, event)
}
