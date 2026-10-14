package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/jackc/pgx/v5"
)

func scanPayPalSeller(row pgx.Row) (paypal.SellerAccount, error) {
	var seller paypal.SellerAccount
	var scopesJSON []byte
	err := row.Scan(&seller.AccountID, &seller.TrackingID, &seller.RequestID, &seller.ReferralSelfURL,
		&seller.MerchantID, &seller.Status, &seller.PaymentsReceivable, &seller.PrimaryEmailConfirmed, &scopesJSON)
	if err != nil {
		return paypal.SellerAccount{}, err
	}
	if err := json.Unmarshal(scopesJSON, &seller.Scopes); err != nil {
		return paypal.SellerAccount{}, err
	}
	return seller, nil
}

const paypalSellerColumns = `account_id, tracking_id, request_id, referral_self_url, COALESCE(merchant_id, ''),
 status, payments_receivable, primary_email_confirmed, permissions_json`

func (s *Store) GetPayPalSeller(ctx context.Context, accountID string) (paypal.SellerAccount, bool, error) {
	seller, err := scanPayPalSeller(s.pool.QueryRow(ctx, `SELECT `+paypalSellerColumns+`
		FROM paypal_seller_accounts WHERE account_id=$1`, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return paypal.SellerAccount{}, false, nil
	}
	return seller, err == nil, err
}

func (s *Store) BeginPayPalSellerReferral(ctx context.Context, accountID, trackingID, requestID string) (paypal.SellerAccount, error) {
	if accountID == "" || trackingID == "" || requestID == "" {
		return paypal.SellerAccount{}, errors.New("PayPal seller referral identifiers are required")
	}
	seller, err := scanPayPalSeller(s.pool.QueryRow(ctx, `
		INSERT INTO paypal_seller_accounts (account_id, tracking_id, request_id, status)
		SELECT $1, $2, $3, 'creating'
		WHERE EXISTS (SELECT 1 FROM professional_profiles WHERE account_id=$1)
		ON CONFLICT (account_id) DO UPDATE SET
			tracking_id=EXCLUDED.tracking_id, request_id=EXCLUDED.request_id,
			referral_self_url='', merchant_id=NULL, status='creating', payments_receivable=false,
			primary_email_confirmed=false, permissions_json='[]'::jsonb, checked_at=NULL, updated_at=now()
		WHERE paypal_seller_accounts.status IN ('revoked', 'ineligible')
		RETURNING `+paypalSellerColumns, accountID, trackingID, requestID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return paypal.SellerAccount{}, errors.New("professional profile is missing or its PayPal setup cannot be restarted")
		}
		return paypal.SellerAccount{}, err
	}
	return seller, nil
}

func (s *Store) SavePayPalSellerReferral(ctx context.Context, accountID, trackingID, selfURL string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE paypal_seller_accounts
		SET referral_self_url=$3, status='pending', updated_at=now()
		WHERE account_id=$1 AND tracking_id=$2 AND status='creating'`, accountID, trackingID, selfURL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("PayPal seller referral record changed unexpectedly")
	}
	return nil
}

func (s *Store) UpdatePayPalSellerStatus(ctx context.Context, accountID, merchantID, status string, paymentsReceivable, emailConfirmed bool, scopes []string) error {
	if status != "pending" && status != "verified" && status != "ineligible" && status != "revoked" {
		return errors.New("invalid PayPal seller status")
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE paypal_seller_accounts
		SET merchant_id=NULLIF($2,''), status=$3, payments_receivable=$4,
			primary_email_confirmed=$5, permissions_json=$6, checked_at=now(), updated_at=now()
		WHERE account_id=$1`, accountID, merchantID, status, paymentsReceivable, emailConfirmed, scopesJSON)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("PayPal seller setup record does not exist")
	}
	checkStatus := "pending"
	if status == "verified" {
		checkStatus = "verified"
	}
	if _, err := tx.Exec(ctx, `UPDATE professional_review_checks c
		SET paypal_onboarding_status=$2, updated_at=now()
		FROM professional_profiles p
		WHERE p.id=c.professional_profile_id AND p.account_id=$1`, accountID, checkStatus); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RevokePayPalSeller(ctx context.Context, merchantID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE paypal_seller_accounts
		SET status='revoked', payments_receivable=false, updated_at=now()
		WHERE merchant_id=$1`, merchantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE professional_review_checks c
		SET paypal_onboarding_status='pending', updated_at=now()
		FROM professional_profiles p JOIN paypal_seller_accounts a ON a.account_id=p.account_id
		WHERE p.id=c.professional_profile_id AND a.merchant_id=$1`, merchantID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordPayPalSellerMerchant(ctx context.Context, trackingID, merchantID string) error {
	if trackingID == "" || merchantID == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `UPDATE paypal_seller_accounts
		SET merchant_id=$2, status='pending', payments_receivable=false,
			primary_email_confirmed=false, permissions_json='[]'::jsonb, updated_at=now()
		WHERE tracking_id=$1 AND status IN ('creating', 'pending')`, trackingID, merchantID)
	return err
}
