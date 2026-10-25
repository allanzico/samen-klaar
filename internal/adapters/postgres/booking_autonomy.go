package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func customerAgentEnabledTx(ctx context.Context, tx pgx.Tx, workspaceID string) error {
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT (m.mandate->>'enabled')::boolean FROM agent_mandates m WHERE m.account_id=w.account_id AND m.role='customer'),true) AND EXISTS(SELECT 1 FROM account_memberships access WHERE access.account_id=w.account_id AND access.role='customer' AND access.status='active') FROM account_workspaces w WHERE w.id=$1`, workspaceID).Scan(&enabled)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("the customer has paused their assistant; automatic approval and release have stopped")
	}
	return nil
}
