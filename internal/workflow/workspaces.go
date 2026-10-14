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
	return WorkspaceSummary{ID: state.WorkspaceID, JobID: state.Job.ID, Title: title, Status: status, StatusLabel: statusLabel, Role: role, UpdatedAt: updatedAt}
}
