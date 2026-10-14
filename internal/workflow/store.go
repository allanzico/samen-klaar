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
}

func NewMemoryStore(initial State) *MemoryStore {
	return &MemoryStore{initial: initial, states: make(map[string]State), owners: make(map[string]string), updated: make(map[string]time.Time)}
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
		if role == "customer" && s.owners[id] == accountID || role == "professional" && state.Job.ProfessionalAccountID == accountID {
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
	if err := Apply(&state, input); err != nil {
		return State{}, err
	}
	state.WorkspaceID = workspaceID
	s.states[workspaceID] = state
	s.updated[workspaceID] = time.Now().UTC()
	return ConversationForRole(state, role), nil
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
