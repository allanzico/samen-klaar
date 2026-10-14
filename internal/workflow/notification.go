package workflow

import "time"

type Notification struct {
	ID          int64      `json:"id"`
	EventType   string     `json:"eventType"`
	WorkspaceID string     `json:"workspaceId"`
	CreatedAt   time.Time  `json:"createdAt"`
	ReadAt      *time.Time `json:"readAt,omitempty"`
}
