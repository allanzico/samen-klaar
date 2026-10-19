package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// StateStore persists and applies marketplace workflow state.
type StateStore interface {
	Load(context.Context, string) (State, error)
	Apply(context.Context, string, Action) (State, error)
}

type WorkspaceStore interface {
	ListWorkspaces(context.Context, string, string) ([]WorkspaceSummary, error)
	CreateWorkspace(context.Context, string) (State, error)
	LoadWorkspace(context.Context, string, string, string) (State, error)
	ApplyWorkspace(context.Context, string, string, string, Action) (State, error)
}

// MemoryStore is used by isolated unit tests. The API process never selects it.
type MemoryStore struct {
	mu      sync.Mutex
	initial State
	states  map[string]State
	owners  map[string]string
	updated map[string]time.Time
	// removed holds workspace|job|professional keys for jobs a professional took off their list.
	removed map[string]bool
}

func NewMemoryStore(initial State) *MemoryStore {
	return &MemoryStore{initial: initial, states: make(map[string]State), owners: make(map[string]string), updated: make(map[string]time.Time), removed: make(map[string]bool)}
}

func (s *MemoryStore) CreateWorkspace(_ context.Context, accountID string) (State, error) {
	if accountID == "" {
		return State{}, errors.New("account is required to create a workspace")
	}
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return State{}, err
	}
	id := hex.EncodeToString(idBytes[:])
	state := NewState()
	state.WorkspaceID = id
	s.mu.Lock()
	if err := CheckOpenRequestLimit(s.openRequestCount(accountID, ""), 1); err != nil {
		s.mu.Unlock()
		return State{}, err
	}
	s.states[id] = state
	s.owners[id] = accountID
	s.updated[id] = time.Now().UTC()
	s.mu.Unlock()
	return state, nil
}

func (s *MemoryStore) ListWorkspaces(_ context.Context, accountID, role string) ([]WorkspaceSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]WorkspaceSummary, 0)
	for id, state := range s.states {
		if role == "customer" && s.owners[id] == accountID || role == "professional" && state.Job.ProfessionalAccountID == accountID && !s.removed[id+"|"+state.Job.ID+"|"+accountID] {
			result = append(result, SummarizeWorkspace(state, role, s.updated[id]))
		}
	}
	return result, nil
}

func (s *MemoryStore) LoadWorkspace(_ context.Context, accountID, role, workspaceID string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[workspaceID]
	if !ok || role == "customer" && s.owners[workspaceID] != accountID || role == "professional" && state.Job.ProfessionalAccountID != accountID {
		return State{}, errors.New("workspace was not found for this account")
	}
	return ConversationForRole(state, role), nil
}

func (s *MemoryStore) ApplyWorkspace(_ context.Context, accountID, role, workspaceID string, input Action) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[workspaceID]
	if !ok || role == "customer" && s.owners[workspaceID] != accountID || role == "professional" && state.Job.ProfessionalAccountID != accountID {
		return State{}, errors.New("workspace was not found for this account")
	}
	input.WorkspaceID = workspaceID
	if input.Action == "return-to-matches" && role == "customer" && state.Cancelled {
		if err := CheckOpenRequestLimit(s.openRequestCount(accountID, workspaceID), 1); err != nil {
			return State{}, err
		}
	}
	if err := Apply(&state, input); err != nil {
		return State{}, err
	}
	state.WorkspaceID = workspaceID
	if input.Action == "remove-request" {
		s.removed[workspaceID+"|"+state.Job.ID+"|"+accountID] = true
	}
	s.states[workspaceID] = state
	s.updated[workspaceID] = time.Now().UTC()
	return ConversationForRole(state, role), nil
}

// OpenRequestCount reports how many of the customer's requests count towards MaxOpenRequests.
func (s *MemoryStore) OpenRequestCount(_ context.Context, accountID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openRequestCount(accountID, ""), nil
}

func (s *MemoryStore) openRequestCount(accountID, excludeWorkspaceID string) int {
	states := []State{}
	for id, state := range s.states {
		if s.owners[id] == accountID && id != excludeWorkspaceID {
			states = append(states, state)
		}
	}
	return CountOpenRequests(states)
}

// RemoveRequest deletes a customer's unapproved request, or takes an unapproved job off the
// assigned professional's list after declining it. It returns "deleted" or "removed".
func (s *MemoryStore) RemoveRequest(ctx context.Context, accountID, role, workspaceID string) (string, error) {
	if role == "professional" {
		if _, err := s.ApplyWorkspace(ctx, accountID, role, workspaceID, Action{Actor: role, Action: "remove-request"}); err != nil {
			if !errors.Is(err, ErrRequestNotRemovable) {
				return "", ErrRequestNotFound
			}
			return "", err
		}
		return "removed", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[workspaceID]
	if !ok || role != "customer" || s.owners[workspaceID] != accountID {
		return "", ErrRequestNotFound
	}
	if !CustomerCanDelete(state) {
		return "", ErrRequestNotDeletable
	}
	delete(s.states, workspaceID)
	delete(s.owners, workspaceID)
	delete(s.updated, workspaceID)
	return "deleted", nil
}

func (s *MemoryStore) Load(_ context.Context, accountID string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[accountID]
	if !ok {
		state = s.initial
		s.states[accountID] = state
	}
	return state, nil
}

func (s *MemoryStore) Apply(_ context.Context, accountID string, input Action) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[accountID]
	if !ok {
		state = s.initial
	}
	if err := Apply(&state, input); err != nil {
		return State{}, err
	}
	s.states[accountID] = state
	return state, nil
}
