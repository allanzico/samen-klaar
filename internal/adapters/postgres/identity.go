package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/jackc/pgx/v5"
)

func newAccountID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "acct_" + hex.EncodeToString(value), nil
}

func (s *Store) UpsertIdentity(ctx context.Context, issuer, subject, email, displayName string) (identity.User, error) {
	id, err := newAccountID()
	if err != nil {
		return identity.User{}, err
	}
	var user identity.User
	err = s.pool.QueryRow(ctx, `
		INSERT INTO accounts (id, issuer, subject, email, display_name)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (issuer, subject) DO UPDATE
		SET email = CASE WHEN EXCLUDED.email = '' THEN accounts.email ELSE EXCLUDED.email END,
		    display_name = CASE WHEN accounts.display_name_custom OR EXCLUDED.display_name = '' THEN accounts.display_name ELSE EXCLUDED.display_name END,
		    updated_at = now()
		RETURNING id, email, display_name, locale`, id, issuer, subject, email, displayName).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Locale)
	return user, err
}

func (s *Store) EnsureMembership(ctx context.Context, accountID string, role identity.Role) (identity.Membership, error) {
	var initialStatus string
	switch role {
	case identity.Customer:
		initialStatus = "active"
	case identity.Professional:
		if _, err := s.EnsureMembership(ctx, accountID, identity.Customer); err != nil {
			return identity.Membership{}, err
		}
		initialStatus = "pending"
	case identity.Operator:
		// Operators are provisioned by the service owner. Authentication must
		// never grant this role based on a browser-selected workspace.
	default:
		return identity.Membership{}, errors.New("unsupported workspace role")
	}
	if initialStatus != "" {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO account_memberships (account_id, role, status)
			VALUES ($1, $2, $3)
			ON CONFLICT (account_id, role) DO NOTHING`, accountID, string(role), initialStatus); err != nil {
			return identity.Membership{}, err
		}
	}
	var membership identity.Membership
	err := s.pool.QueryRow(ctx, `SELECT role, status FROM account_memberships WHERE account_id = $1 AND role = $2`, accountID, string(role)).Scan(&membership.Role, &membership.Status)
	if err != nil {
		return identity.Membership{}, err
	}
	return membership, nil
}

func (s *Store) CreateSession(ctx context.Context, tokenHash, accountID string, role identity.Role, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO account_sessions (token_hash, account_id, active_role, expires_at) VALUES ($1, $2, $3, $4)`, tokenHash, accountID, string(role), expires)
	return err
}

func (s *Store) Session(ctx context.Context, tokenHash string) (identity.Principal, error) {
	var principal identity.Principal
	err := s.pool.QueryRow(ctx, `
		SELECT a.id, a.email, a.display_name, a.locale, s.active_role, s.expires_at
		FROM account_sessions s
		JOIN accounts a ON a.id = s.account_id
		JOIN account_memberships active ON active.account_id = s.account_id AND active.role = s.active_role
		WHERE s.token_hash = $1 AND s.expires_at > now() AND active.status <> 'disabled'`, tokenHash).Scan(&principal.User.ID, &principal.User.Email, &principal.User.DisplayName, &principal.User.Locale, &principal.ActiveRole, &principal.ExpiresAt)
	if err != nil {
		return identity.Principal{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT role, status FROM account_memberships WHERE account_id = $1 ORDER BY role`, principal.User.ID)
	if err != nil {
		return identity.Principal{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var membership identity.Membership
		if err := rows.Scan(&membership.Role, &membership.Status); err != nil {
			return identity.Principal{}, err
		}
		principal.Memberships = append(principal.Memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return identity.Principal{}, err
	}
	return principal, nil
}

func (s *Store) ActivateRole(ctx context.Context, tokenHash string, role identity.Role) (identity.Principal, error) {
	if role != identity.Customer && role != identity.Professional && role != identity.Operator {
		return identity.Principal{}, errors.New("unsupported workspace role")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return identity.Principal{}, err
	}
	defer tx.Rollback(ctx)
	var accountID string
	err = tx.QueryRow(ctx, `
		UPDATE account_sessions SET active_role = $2, last_seen_at = now()
		WHERE token_hash = $1 AND expires_at > now() AND EXISTS (
			SELECT 1 FROM account_memberships m
			WHERE m.account_id = account_sessions.account_id AND m.role = $2 AND (m.status = 'active' OR m.role = 'professional' AND m.status = 'pending')
		) RETURNING account_id`, tokenHash, string(role)).Scan(&accountID)
	if err != nil {
		return identity.Principal{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE accounts SET preferred_role = $2, updated_at = now() WHERE id = $1`, accountID, string(role)); err != nil {
		return identity.Principal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return identity.Principal{}, err
	}
	return s.Session(ctx, tokenHash)
}

func (s *Store) PreferredRole(ctx context.Context, accountID string) (identity.Role, error) {
	var role identity.Role
	err := s.pool.QueryRow(ctx, `SELECT CASE WHEN EXISTS (
		SELECT 1 FROM account_memberships m WHERE m.account_id = a.id
		AND m.role = a.preferred_role AND (m.status = 'active' OR m.role = 'professional' AND m.status = 'pending')
	) THEN a.preferred_role ELSE 'customer' END FROM accounts a WHERE a.id = $1`, accountID).Scan(&role)
	return role, err
}

func (s *Store) AccountLocale(ctx context.Context, accountID string) (string, error) {
	var locale string
	err := s.pool.QueryRow(ctx, `SELECT locale FROM accounts WHERE id=$1`, accountID).Scan(&locale)
	return locale, err
}

func (s *Store) SetAccountLocale(ctx context.Context, accountID, locale string) error {
	if locale != "en" && locale != "nl" {
		return errors.New("choose Dutch or English")
	}
	tag, err := s.pool.Exec(ctx, `UPDATE accounts SET locale=$2,updated_at=now() WHERE id=$1`, accountID, locale)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("account was not found")
	}
	return nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM account_sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *Store) CreateOIDCTransaction(ctx context.Context, transaction identity.Transaction) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oidc_transactions (state_hash, nonce, code_verifier, requested_role, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, transaction.StateHash, transaction.Nonce, transaction.CodeVerifier, string(transaction.Role), transaction.ExpiresAt)
	return err
}

func (s *Store) ConsumeOIDCTransaction(ctx context.Context, stateHash string) (identity.Transaction, error) {
	var transaction identity.Transaction
	err := s.pool.QueryRow(ctx, `
		DELETE FROM oidc_transactions WHERE state_hash = $1 AND expires_at > now()
		RETURNING state_hash, nonce, code_verifier, requested_role, expires_at`, stateHash).Scan(&transaction.StateHash, &transaction.Nonce, &transaction.CodeVerifier, &transaction.Role, &transaction.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Transaction{}, fmt.Errorf("OIDC transaction is missing or expired")
	}
	return transaction, err
}

// CleanupExpiredAuthRecords removes only records that have been unusable for
// at least a day. The expiry indexes keep each batch bounded.
func (s *Store) CleanupExpiredAuthRecords(ctx context.Context) (int64, error) {
	sessions, err := s.pool.Exec(ctx, `DELETE FROM account_sessions WHERE token_hash IN (
		SELECT token_hash FROM account_sessions WHERE expires_at < now()-interval '1 day'
		ORDER BY expires_at LIMIT 500)`)
	if err != nil {
		return 0, err
	}
	transactions, err := s.pool.Exec(ctx, `DELETE FROM oidc_transactions WHERE state_hash IN (
		SELECT state_hash FROM oidc_transactions WHERE expires_at < now()-interval '1 day'
		ORDER BY expires_at LIMIT 500)`)
	if err != nil {
		return sessions.RowsAffected(), err
	}
	return sessions.RowsAffected() + transactions.RowsAffected(), nil
}
