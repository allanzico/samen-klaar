package workflow

import "time"

type WorkspaceSummary struct {
	ID          string    `json:"id"`
	JobID       string    `json:"jobId,omitempty"`
	Title       string    `json:"title"`
	StatusLabel string    `json:"statusLabel"`
	Status      string    `json:"status"`
	Role        string    `json:"role"`
	UpdatedAt   time.Time `json:"updatedAt"`
	// Display fields for request cards, job rows and the rail. They repeat what the
	// signed-in account can already read in the full workspace state.
	Counterpart   string `json:"counterpart,omitempty"`
	Category      string `json:"category,omitempty"`
	TotalMinor    int64  `json:"totalMinor,omitempty"`
	Currency      string `json:"currency,omitempty"`
	VisitStartUTC string `json:"visitStartUtc,omitempty"`
	VisitEndUTC   string `json:"visitEndUtc,omitempty"`
	VisitTimezone string `json:"visitTimezone,omitempty"`
	ConfirmDueAt  string `json:"confirmDueAt,omitempty"`
}

func SummarizeWorkspace(state State, role string, updatedAt time.Time) WorkspaceSummary {
	title := "New request"
	if state.Job.ID != "" {
		title = state.Job.Title
		if title == "" {
			title = state.Job.Description
		}
	} else if state.Intake != nil && state.Intake.Description != "" {
		title = state.Intake.Description
	}
	status, statusLabel := RequestStatus(state, role)
	summary := WorkspaceSummary{ID: state.WorkspaceID, JobID: state.Job.ID, Title: title, Status: status, StatusLabel: statusLabel, Role: role, UpdatedAt: updatedAt}
	job := state.Job
	if job.ID == "" {
		if state.Intake != nil {
			summary.Category = state.Intake.Category
		}
		return summary
	}
	summary.Category = job.Category
	summary.Counterpart = job.Professional
	if role == "professional" {
		summary.Counterpart = job.Customer
	}
	summary.TotalMinor, summary.Currency = job.Quote.TotalMinor, job.Quote.Currency
	if job.Appointment != nil {
		summary.VisitStartUTC, summary.VisitEndUTC, summary.VisitTimezone = job.Appointment.StartUTC, job.Appointment.EndUTC, job.Appointment.Timezone
	}
	if job.AppointmentConfirmation != nil && job.AppointmentConfirmation.Status == "pending" {
		summary.ConfirmDueAt = job.AppointmentConfirmation.DueAt
	}
	return summary
}
