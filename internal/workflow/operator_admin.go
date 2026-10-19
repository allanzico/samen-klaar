package workflow

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrAdminNotFound     = errors.New("account was not found")
	ErrAdminConflict     = errors.New("account access changed; refresh the account and try again")
	ErrAdminForbidden    = errors.New("active operator access is required")
	ErrAdminProtected    = errors.New("operator accounts are managed by the service owner")
	ErrAdminUnrestorable = errors.New("only an operator suspension can be restored here; professional applications use the review queue")
)

// DirectoryFilter bounds operator reads; role and status refer to the same membership.
type DirectoryFilter struct {
	Query    string
	Role     string
	Status   string
	Location string
	Offset   int
}

const DirectoryPageSize = 25

func (f DirectoryFilter) Validate() error {
	if utf8.RuneCountInString(f.Query) > 160 || utf8.RuneCountInString(f.Location) > 160 || f.Offset < 0 || f.Offset > 1000000 {
		return errors.New("invalid directory search")
	}
	if f.Role != "" && f.Role != "customer" && f.Role != "professional" && f.Role != "operator" {
		return errors.New("invalid directory role")
	}
	if f.Status != "" && f.Status != "active" && f.Status != "pending" && f.Status != "disabled" {
		return errors.New("invalid directory status")
	}
	return nil
}

type AdminMembership struct {
	Role      string `json:"role"`
	Status    string `json:"status"`
	Suspended bool   `json:"suspended"`
}

// AdminProfessional contains published service information, never the private base address.
type AdminProfessional struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Category       string `json:"category"`
	ServiceName    string `json:"serviceName"`
	ServiceArea    string `json:"serviceArea"`
	TravelRadiusKM int    `json:"travelRadiusKm"`
}

type AdminUser struct {
	ID               string             `json:"id"`
	Email            string             `json:"email"`
	DisplayName      string             `json:"displayName"`
	Locale           string             `json:"locale"`
	Issuer           string             `json:"issuer"`
	Subject          string             `json:"subject"`
	CreatedAt        string             `json:"createdAt"`
	LastSeenAt       string             `json:"lastSeenAt"`
	Memberships      []AdminMembership  `json:"memberships"`
	Professional     *AdminProfessional `json:"professional,omitempty"`
	CustomerJobs     int                `json:"customerJobs"`
	ProfessionalJobs int                `json:"professionalJobs"`
	ActiveSessions   int                `json:"activeSessions"`
	SameEmailCount   int                `json:"sameEmailCount"`
}

type AdminUserPage struct {
	Users   []AdminUser `json:"users"`
	Total   int         `json:"total"`
	Offset  int         `json:"offset"`
	HasMore bool        `json:"hasMore"`
}

type AdminEvent struct {
	ID               int64  `json:"id"`
	ActorID          string `json:"actorId"`
	ActorName        string `json:"actorName"`
	Action           string `json:"action"`
	Role             string `json:"role"`
	PreviousStatus   string `json:"previousStatus"`
	NextStatus       string `json:"nextStatus"`
	Reason           string `json:"reason"`
	AffectedSessions int    `json:"affectedSessions"`
	CreatedAt        string `json:"createdAt"`
}

type AdminUserDetail struct {
	User   AdminUser    `json:"user"`
	Events []AdminEvent `json:"events"`
}

type AdminUserAction struct {
	Action         string `json:"action"`
	Role           string `json:"role"`
	ExpectedStatus string `json:"expectedStatus"`
	Reason         string `json:"reason"`
}

func (a AdminUserAction) Validate() error {
	n := utf8.RuneCountInString(strings.TrimSpace(a.Reason))
	if n < 10 || n > 1000 {
		return errors.New("record a reason of 10 to 1000 characters")
	}
	if a.Action == "revoke-sessions" && a.Role == "" && a.ExpectedStatus == "" {
		return nil
	}
	if a.Action != "suspend-role" && a.Action != "restore-role" || a.Role != "customer" && a.Role != "professional" {
		return errors.New("choose a customer or professional access action")
	}
	if a.ExpectedStatus != "active" && a.ExpectedStatus != "pending" && a.ExpectedStatus != "disabled" {
		return errors.New("the current access status is required")
	}
	return nil
}

type AdminLocation struct {
	Label                string   `json:"label"`
	ActiveProfessionals  int      `json:"activeProfessionals"`
	PendingProfessionals int      `json:"pendingProfessionals"`
	TotalProfessionals   int      `json:"totalProfessionals"`
	Requests             int      `json:"requests"`
	WaitlistedRequests   int      `json:"waitlistedRequests"`
	Categories           []string `json:"categories"`
}

type AdminLocationPage struct {
	Locations []AdminLocation `json:"locations"`
	Total     int             `json:"total"`
	Offset    int             `json:"offset"`
	HasMore   bool            `json:"hasMore"`
}
