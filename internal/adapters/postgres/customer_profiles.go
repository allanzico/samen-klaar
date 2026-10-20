package postgres

import (
	"context"
	"encoding/json"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func (s *Store) CustomerProfile(ctx context.Context, accountID string) (workflow.CustomerProfile, error) {
	var profile workflow.CustomerProfile
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT a.display_name,a.email,COALESCE(p.phone,''),p.location FROM accounts a LEFT JOIN customer_profiles p ON p.account_id=a.id WHERE a.id=$1`, accountID).Scan(&profile.DisplayName, &profile.Email, &profile.Phone, &raw)
	if err == nil && len(raw) > 0 {
		err = json.Unmarshal(raw, &profile.Location)
	}
	return profile, err
}

func (s *Store) SaveCustomerProfile(ctx context.Context, accountID string, profile workflow.CustomerProfile) (workflow.CustomerProfile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return profile, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var raw []byte
	if profile.Location != nil {
		profile.Location.Selection = ""
		raw, err = json.Marshal(profile.Location)
		if err != nil {
			return profile, err
		}
	}
	if err = tx.QueryRow(ctx, `UPDATE accounts SET display_name=$2,display_name_custom=true,updated_at=now() WHERE id=$1 RETURNING email`, accountID, profile.DisplayName).Scan(&profile.Email); err != nil {
		return profile, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO customer_profiles(account_id,phone,location) VALUES($1,$2,$3) ON CONFLICT(account_id) DO UPDATE SET phone=EXCLUDED.phone,location=EXCLUDED.location,updated_at=now()`, accountID, profile.Phone, raw); err != nil {
		return profile, err
	}
	return profile, tx.Commit(ctx)
}
