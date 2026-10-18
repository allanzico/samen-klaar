package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

// AgentStore persists mandates, the decision log and open escalations.
type AgentStore interface {
	AgentMandate(ctx context.Context, accountID, role string) (workflow.AgentMandate, error)
	SaveAgentMandate(ctx context.Context, accountID, role string, mandate workflow.AgentMandate) (workflow.AgentMandate, error)
	RecordAgentDecision(ctx context.Context, workspaceID, accountID, role string, mandateVersion int, code, action string, params map[string]string, outcome, failure string) error
	AgentCounters(ctx context.Context, workspaceID, accountID, role string) (prices, visits int, err error)
	AgentAvoidedProfiles(ctx context.Context, workspaceID, accountID string) ([]string, error)
	AgentActivity(ctx context.Context, workspaceID, accountID, role string, limit int) ([]workflow.AgentActivity, error)
	SyncAgentEscalation(ctx context.Context, workspaceID, accountID, role, code string, params map[string]string) (bool, error)
	OpenAgentEscalation(ctx context.Context, workspaceID, accountID, role string) (string, error)
	AgentEscalations(ctx context.Context, accountID, role string) ([]workflow.AgentEscalation, error)
	AgentParties(ctx context.Context, workspaceID string) (customer, professional workflow.AgentParty, err error)
	AgentWorkspaces(ctx context.Context, since time.Time, limit int) ([]string, error)
}

const (
	agentMaxSteps      = 10
	agentSweepInterval = 30 * time.Second
	agentCodeBlocked   = "agent_blocked"

	interpretRetryAfter = 5 * time.Minute
)

// RunAgents lets both people's agents act on one request until neither has anything left to do inside
// its mandate. Steps are applied through runAction, so the workflow engine re-validates each one. The
// loop is bounded, and a rejected step stops that agent and asks its person instead of retrying.
// retryBlocked is true after a person acted (a retry may now succeed) and false for the periodic sweep.
func (s *Server) RunAgents(ctx context.Context, workspaceID string, retryBlocked bool) error {
	if s.agents == nil || workspaceID == "" {
		return nil
	}
	lock, _ := s.agentLocks.LoadOrStore(workspaceID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	for range agentMaxSteps {
		// The professional is only known once the customer's agent has chosen one, so reload each round.
		customer, professional, err := s.agents.AgentParties(ctx, workspaceID)
		if err != nil {
			return err
		}
		progressed := false
		for _, party := range []struct {
			role string
			who  workflow.AgentParty
		}{{"professional", professional}, {"customer", customer}} {
			// The API refuses a suspended or pending account, so its agent must not act in its name either.
			if party.who.AccountID == "" || !party.who.Active {
				continue
			}
			did, err := s.agentStep(ctx, workspaceID, party.role, party.who, retryBlocked)
			if err != nil {
				return err
			}
			progressed = progressed || did
		}
		if !progressed {
			return nil
		}
	}
	return nil
}

func (s *Server) agentStep(ctx context.Context, workspaceID, role string, who workflow.AgentParty, retryBlocked bool) (bool, error) {
	mandate, err := s.agents.AgentMandate(ctx, who.AccountID, role)
	if err != nil {
		return false, err
	}
	if !mandate.Enabled {
		_, err := s.agents.SyncAgentEscalation(ctx, workspaceID, who.AccountID, role, "", nil)
		return false, err
	}
	state, err := s.currentState(ctx, who.AccountID, role, workspaceID)
	if err != nil {
		return false, err
	}
	counters, visitProposals, err := s.agents.AgentCounters(ctx, workspaceID, who.AccountID, role)
	if err != nil {
		return false, err
	}
	var avoid []string
	if role == "customer" && state.Job.ID == "" {
		// After a professional cancelled, the matches list contains them again; do not pick them twice.
		if avoid, err = s.agents.AgentAvoidedProfiles(ctx, workspaceID, who.AccountID); err != nil {
			return false, err
		}
	}
	agentCtx := workflow.AgentContext{State: state, Role: role, Locale: who.Locale, Mandate: mandate, Now: time.Now(), Counters: counters, DateProposals: visitProposals, Avoid: avoid}
	s.interpretReply(ctx, &agentCtx, workspaceID, who.AccountID)
	decision := workflow.DecideAgent(agentCtx)
	switch decision.Kind {
	case workflow.AgentEscalate:
		opened, err := s.agents.SyncAgentEscalation(ctx, workspaceID, who.AccountID, role, decision.Code, decision.Params)
		if err == nil && opened {
			err = s.agents.RecordAgentDecision(ctx, workspaceID, who.AccountID, role, mandate.Version, decision.Code, "", decision.Params, "applied", "")
		}
		return false, err
	case workflow.AgentAct:
		// A step that was rejected before stays blocked until the person acts; the sweeper must not hammer it.
		if open, err := s.agents.OpenAgentEscalation(ctx, workspaceID, who.AccountID, role); err != nil {
			return false, err
		} else if open == agentCodeBlocked && !retryBlocked {
			return false, nil
		}
		input := *decision.Action
		input.WorkspaceID = workspaceID
		input.Actor = role
		input.Locale = who.Locale
		input.CustomerAccountID = who.AccountID
		input.ByAgent = true
		principal := identity.Principal{User: identity.User{ID: who.AccountID, DisplayName: who.DisplayName, Locale: who.Locale}, ActiveRole: identity.Role(role)}
		stepCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
		_, _, applyErr := s.runAction(stepCtx, principal, input)
		if applyErr != nil {
			// Tell the person what went wrong in the chat, so a reply they gave never hangs unanswered.
			if state.Intake != nil && input.Action != "agent-say" {
				notice := workflow.BlockedNotice(agentCtx, decision.Code, applyErr.Error())
				notice.WorkspaceID, notice.Actor, notice.Locale, notice.CustomerAccountID = workspaceID, role, who.Locale, who.AccountID
				if _, _, noticeErr := s.runAction(stepCtx, principal, notice); noticeErr != nil {
					slog.Warn("agent notice was not posted", "workspace", workspaceID, "error", noticeErr)
				}
			}
			params := map[string]string{"reason": applyErr.Error()}
			opened, err := s.agents.SyncAgentEscalation(ctx, workspaceID, who.AccountID, role, agentCodeBlocked, params)
			if err == nil && opened {
				err = s.agents.RecordAgentDecision(ctx, workspaceID, who.AccountID, role, mandate.Version, decision.Code, input.Action, decision.Params, "rejected", applyErr.Error())
			}
			return false, err
		}
		// Negotiation steps leave a line in the shared thread so both people can read the exchange.
		if text, ok := workflow.AgentNote(role, decision.Code, decision.Params); ok {
			note := workflow.Action{Action: "agent-say", Value: text, Audience: "both", NoteCode: role + ":" + decision.Code, NoteParams: decision.Params, WorkspaceID: workspaceID, Actor: role, Locale: who.Locale, CustomerAccountID: who.AccountID, ByAgent: true}
			if _, _, noteErr := s.runAction(stepCtx, principal, note); noteErr != nil {
				slog.Warn("agent note was not posted", "workspace", workspaceID, "error", noteErr)
			}
		}
		if _, err := s.agents.SyncAgentEscalation(ctx, workspaceID, who.AccountID, role, "", nil); err != nil {
			return true, err
		}
		return true, s.agents.RecordAgentDecision(ctx, workspaceID, who.AccountID, role, mandate.Version, decision.Code, input.Action, decision.Params, "applied", "")
	}
	_, err = s.agents.SyncAgentEscalation(ctx, workspaceID, who.AccountID, role, "", nil)
	return false, err
}

// ReplyInterpreter reads a free-form reply with a language model. The AI provider implements it when
// one is configured; without it the assistants use their built-in reading.
type ReplyInterpreter interface {
	InterpretReply(context.Context, workflow.ReplyQuery) (workflow.ReplyIntent, error)
}

// interpretReply asks the model what a typed reply means when the built-in reading cannot settle it.
// Any failure leaves the built-in reading in charge, and the result is sanitized before it is used.
func (s *Server) interpretReply(ctx context.Context, agentCtx *workflow.AgentContext, workspaceID, accountID string) {
	interpreter, ok := s.ai.(ReplyInterpreter)
	if !ok || interpreter == nil {
		return
	}
	query, offered, seq, pending := workflow.PendingReply(*agentCtx)
	if !pending {
		return
	}
	// Each reply is read once, however often the sweep revisits a request that is still waiting on a person.
	key := fmt.Sprintf("%s|%s|%d", workspaceID, agentCtx.Role, seq)
	if cached, ok := s.interpretations.Load(key); ok {
		agentCtx.Interpreted, agentCtx.InterpretedSeq = cached.(*workflow.ReplyIntent), seq
		return
	}
	// A failed call is not retried by every 30-second sweep; the built-in reading serves meanwhile.
	if failed, ok := s.interpretFailed.Load(key); ok && time.Since(failed.(time.Time)) < interpretRetryAfter {
		return
	}
	if !s.limiter.allow("ai:"+accountID, 0.2, 20, time.Now()) {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	intent, err := interpreter.InterpretReply(callCtx, query)
	if err != nil {
		slog.Warn("reply interpretation failed; using the built-in reading", "error", err)
		s.interpretFailed.Store(key, time.Now())
		return
	}
	clean := intent.Sanitize(query.Stage, offered)
	s.interpretations.Store(key, &clean)
	agentCtx.Interpreted, agentCtx.InterpretedSeq = &clean, seq
}

// TriggerAgents runs the agents for a request in the background after a person changed it.
func (s *Server) TriggerAgents(workspaceID string) {
	if s.agents == nil || workspaceID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := s.RunAgents(ctx, workspaceID, true); err != nil {
			slog.Error("agent run failed", "workspace", workspaceID, "error", err)
		}
	}()
}

// StartAgents sweeps recently active requests until ctx ends, catching steps no user action triggers.
func (s *Server) StartAgents(ctx context.Context) {
	if s.agents == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(agentSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.SweepAgents(ctx)
			}
		}
	}()
}

func (s *Server) SweepAgents(ctx context.Context) {
	ids, err := s.agents.AgentWorkspaces(ctx, time.Now().AddDate(0, 0, -30), 200)
	if err != nil {
		slog.Error("agent sweep failed", "error", err)
		return
	}
	for _, id := range ids {
		runCtx, cancel := context.WithTimeout(ctx, time.Minute)
		if err := s.RunAgents(runCtx, id, false); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("agent run failed", "workspace", id, "error", err)
		}
		cancel()
	}
}
