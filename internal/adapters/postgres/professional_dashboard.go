package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func (s *Store) ProfessionalDashboard(ctx context.Context, accountID string) (workflow.ProfessionalDashboard, error) {
	if accountID == "" {
		return workflow.ProfessionalDashboard{}, errors.New("professional account is required")
	}
	now := time.Now().UTC()
	var raw []byte
	err := s.pool.QueryRow(ctx, `WITH jobs AS (
		SELECT id,state_json->'job' AS job,COALESCE((state_json->>'cancelled')::boolean,false) AS cancelled
		FROM account_workspaces WHERE state_json #>> '{job,professionalAccountId}'=$1
	), money AS (
		SELECT p.currency, sum(p.amount_minor-p.platform_fee_minor) AS received, 0::bigint AS awaiting, sum(p.platform_fee_minor) AS platform_fees
		FROM paypal_payment_operations p JOIN jobs j ON j.id=p.workspace_id AND j.job->>'id'=p.job_id
		WHERE p.status='captured' AND NULLIF(p.capture_id,'') IS NOT NULL
        AND COALESCE(j.job->>'paymentRefunded','false')!='true'
        AND COALESCE(j.job #>> '{dispute,status}','') NOT IN ('refund_pending','partial_refund_review','payment_reversal_review','refunded')
        GROUP BY p.currency
		UNION ALL
		SELECT job #>> '{quote,currency}',0::bigint,sum((job #>> '{quote,totalMinor}')::bigint - (((job #>> '{quote,totalMinor}')::bigint * COALESCE((job->>'platformFeeBasisPoints')::int,0)+5000)/10000)),0::bigint
		FROM jobs WHERE NOT cancelled AND job->>'completionApproved'='true'
		AND COALESCE(job->>'paid','false')!='true' AND COALESCE(job->>'paymentRefunded','false')!='true'
		AND COALESCE(job #>> '{dispute,status}','') IN ('','resolved','rework')
		AND job #>> '{quote,currency}' ~ '^[A-Z]{3}$'
		GROUP BY job #>> '{quote,currency}'
	), balances AS (
		SELECT currency,sum(received)::bigint AS received,sum(awaiting)::bigint AS awaiting,sum(platform_fees)::bigint AS platform_fees FROM money GROUP BY currency
	)
	SELECT jsonb_build_object(
		'balances',COALESCE((SELECT jsonb_agg(jsonb_build_object('currency',currency,'receivedMinor',received,'awaitingPaymentMinor',awaiting,'platformFeesMinor',platform_fees) ORDER BY currency) FROM balances),'[]'::jsonb),
		'upcomingVisits',count(*) FILTER (WHERE NOT cancelled AND COALESCE(job->>'workStarted','false')!='true' AND COALESCE(job->>'paid','false')!='true' AND job #>> '{appointmentConfirmation,status}'='confirmed' AND (job #>> '{appointment,startUtc}')::timestamptz > $2),
		'completedJobs',count(*) FILTER (WHERE NOT cancelled AND job->>'completionApproved'='true'),
		'awaitingConfirmation',count(*) FILTER (WHERE NOT cancelled AND job #>> '{appointmentConfirmation,status}'='pending' AND (job #>> '{appointmentConfirmation,dueAt}')::timestamptz > $2)
	) FROM jobs`, accountID, now).Scan(&raw)
	if err != nil {
		return workflow.ProfessionalDashboard{}, err
	}
	var result workflow.ProfessionalDashboard
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	result.AsOf = now
	return result, nil
}
