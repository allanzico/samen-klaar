package workflow

type JobPlan struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}
type RequestJob struct {
	Workspace    WorkspaceSummary `json:"workspace"`
	ScopeVersion int              `json:"scopeVersion"`
	TotalMinor   int64            `json:"totalMinor"`
	Currency     string           `json:"currency"`
	Professional string           `json:"professional"`
}
type RequestTimelineEvent struct {
	WorkspaceID string `json:"workspaceId"`
	JobTitle    string `json:"jobTitle"`
	Event       Event  `json:"event"`
}
type ParentRequest struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	Jobs        []RequestJob           `json:"jobs"`
	Timeline    []RequestTimelineEvent `json:"timeline"`
}
