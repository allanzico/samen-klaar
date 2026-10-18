package workflow

// NewDemoState builds deterministic workflow fixtures for package tests. It is
// compiled only with the test binary and is never part of the API runtime.
func NewDemoState(scenario string) State {
	state := NewState()
	state.AvailableSlots = []Appointment{
		{SlotID: "slot-tue", StartsAt: "Tue, 13 Oct · 10:00–11:30", Timezone: "Europe/Amsterdam", StartUTC: "2026-10-13T08:00:00Z", EndUTC: "2026-10-13T09:30:00Z"},
		{SlotID: "slot-wed", StartsAt: "Wed, 14 Oct · 13:00–14:30", Timezone: "Europe/Amsterdam", StartUTC: "2026-10-14T11:00:00Z", EndUTC: "2026-10-14T12:30:00Z"},
		{SlotID: "slot-thu", StartsAt: "Thu, 15 Oct · 09:00–10:30", Timezone: "Europe/Amsterdam", StartUTC: "2026-10-15T07:00:00Z", EndUTC: "2026-10-15T08:30:00Z"},
	}
	category, service, price, callout, duration := "Plumbing", "Sink leak repair", int64(11200), int64(3500), 90
	ids := map[string]string{
		"missing-docs": "job_test_missing", "quote-ready": "job_test_quote", "ready-review": "job_test_ready",
		"change-review": "job_test_change", "carpentry": "job_test_carpentry", "electrical": "job_test_electrical",
	}
	if scenario == "welcome" {
		return state
	}
	if scenario == "carpentry" {
		category, service, price, callout, duration = "Carpentry", "Cupboard door alignment", 9400, 3000, 75
	} else if scenario == "electrical" {
		category, service, price, callout, duration = "Electrical assessment", "Electrical safety assessment", 5500, 2500, 45
	}
	job := Job{
		ID: ids[scenario], Title: service, Customer: "Test customer", Professional: "Test professional",
		Description: "A test request for workflow rules.", Category: category, ScopeVersion: 1,
		Quote: Quote{
			Currency: "USD", Lines: []QuoteLine{{Label: service, AmountMinor: price, Basis: "Test catalog price"}, {Label: "Call-out", AmountMinor: callout, Basis: "Test call-out fee"}},
			TotalMinor: price + callout, MaterialsPolicy: "Parts require separate customer approval.",
			Exclusions: []string{"Replacement parts"}, DurationMinutes: duration,
		},
		Evidence: []Evidence{}, Timeline: []Event{},
	}
	job.ScopeHistory = []ScopeSnapshot{scopeSnapshot(job)}
	if scenario == "missing-docs" || scenario == "ready-review" || scenario == "change-review" {
		job.QuoteApproved = true
		job.Appointment = &state.AvailableSlots[0]
		job.WorkStarted, job.EvidenceSubmitted = true, true
		job.Evidence = []Evidence{{AssetID: "test_asset", ContentType: "image/png", Label: "Completion photo", Description: "Test evidence reference.", Kind: "photo"}}
		if scenario == "ready-review" || scenario == "change-review" {
			job.DetailAdded = true
			job.Evidence = append(job.Evidence, Evidence{Label: "Work note", Description: "Test completion note.", Kind: "note"})
		}
	}
	if scenario == "change-review" {
		job.Change = &ScopeChange{Status: "pending", Description: "Replace a test part", AmountMinor: 4800, ProposedFrom: 1}
	}
	state.Job = job
	return state
}
