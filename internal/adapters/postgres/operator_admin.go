package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
)

// Coverage is an inventory of published areas, not a copy of private base coordinates.
const adminCoverageCTE = `WITH coverage AS (
 SELECT p.account_id, p.category, p.status AS profile_status, m.status AS membership_status,
        trim(a.label) AS label, lower(regexp_replace(trim(a.label),'[[:space:]]+',' ','g')) AS area
 FROM professional_profiles p
 JOIN account_memberships m ON m.account_id=p.account_id AND m.role='professional'
 CROSS JOIN LATERAL (
   SELECT value AS label FROM regexp_split_to_table(p.service_area,E'[,;|\\n]+') value WHERE p.base_location IS NULL
   UNION ALL
   SELECT COALESCE(NULLIF(trim(concat_ws(' ',NULLIF(p.base_location->>'postcode',''),NULLIF(p.base_location->>'city',''))),''),p.service_area)
   WHERE p.base_location IS NOT NULL
 ) a WHERE trim(a.label)<>''
) `

const adminUserWhere = `WHERE ($1='' OR strpos(lower(concat_ws(' ',a.id,a.email,a.display_name)),lower($1))>0)
 AND ($2='' AND $3='' OR EXISTS(SELECT 1 FROM account_memberships m WHERE m.account_id=a.id
      AND ($2='' OR m.role=$2) AND ($3='' OR m.status=$3)))
 AND ($4='' OR EXISTS(SELECT 1 FROM coverage c WHERE c.account_id=a.id AND c.area=lower(regexp_replace(trim($4),'[[:space:]]+',' ','g')))) `

const adminUserColumns = `a.id,a.email,a.display_name,a.locale,a.issuer,a.subject,a.created_at,
 COALESCE((SELECT max(last_seen_at) FROM account_sessions WHERE account_id=a.id),a.updated_at),
 COALESCE((SELECT jsonb_agg(jsonb_build_object('role',m.role,'status',m.status,'suspended',s.account_id IS NOT NULL) ORDER BY m.role)
   FROM account_memberships m LEFT JOIN operator_membership_suspensions s ON s.account_id=m.account_id AND s.role=m.role WHERE m.account_id=a.id),'[]'::jsonb),
 (SELECT count(*) FROM account_workspaces w WHERE w.account_id=a.id),
 (SELECT count(*) FROM account_workspaces w WHERE w.state_json#>>'{job,professionalAccountId}'=a.id),
 (SELECT count(*) FROM account_sessions WHERE account_id=a.id AND expires_at>now()),
 CASE WHEN trim(a.email)='' THEN 0 ELSE (SELECT count(*) FROM accounts other WHERE lower(trim(other.email))=lower(trim(a.email))) END,
 (SELECT jsonb_build_object('id',p.id,'status',p.status,'category',p.category,'serviceName',p.service_name,
     'serviceArea',p.service_area,'travelRadiusKm',p.travel_radius_km) FROM professional_profiles p WHERE p.account_id=a.id) `

func scanAdminUser(row pgx.Row) (workflow.AdminUser, error) {
	var user workflow.AdminUser
	var created, seen time.Time
	var memberships, profile []byte
	err := row.Scan(&user.ID, &user.Email, &user.DisplayName, &user.Locale, &user.Issuer, &user.Subject,
		&created, &seen, &memberships, &user.CustomerJobs, &user.ProfessionalJobs, &user.ActiveSessions, &user.SameEmailCount, &profile)
	if err != nil {
		return user, err
	}
	user.CreatedAt, user.LastSeenAt = created.UTC().Format(time.RFC3339), seen.UTC().Format(time.RFC3339)
	if err = json.Unmarshal(memberships, &user.Memberships); err != nil {
		return user, err
	}
	if len(profile) > 0 {
		err = json.Unmarshal(profile, &user.Professional)
	}
	return user, err
}

func (s *Store) adminTransaction(ctx context.Context, operatorID string) (pgx.Tx, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	if err = assertOperator(ctx, tx, operatorID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, workflow.ErrAdminForbidden
	}
	return tx, nil
}

func (s *Store) OperatorUsers(ctx context.Context, operatorID string, filter workflow.DirectoryFilter) (workflow.AdminUserPage, error) {
	page := workflow.AdminUserPage{Users: []workflow.AdminUser{}, Offset: filter.Offset}
	if err := filter.Validate(); err != nil {
		return page, err
	}
	tx, err := s.adminTransaction(ctx, operatorID)
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	args := []any{strings.TrimSpace(filter.Query), filter.Role, filter.Status, filter.Location}
	if err = tx.QueryRow(ctx, adminCoverageCTE+`SELECT count(*) FROM accounts a `+adminUserWhere, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	args = append(args, workflow.DirectoryPageSize, filter.Offset)
	rows, err := tx.Query(ctx, adminCoverageCTE+`SELECT `+adminUserColumns+` FROM accounts a `+adminUserWhere+`ORDER BY a.created_at DESC,a.id LIMIT $5 OFFSET $6`, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		user, err := scanAdminUser(rows)
		if err != nil {
			return page, err
		}
		page.Users = append(page.Users, user)
	}
	page.HasMore = filter.Offset+len(page.Users) < page.Total
	return page, rows.Err()
}

func (s *Store) OperatorUser(ctx context.Context, operatorID, accountID string) (workflow.AdminUserDetail, error) {
	detail := workflow.AdminUserDetail{Events: []workflow.AdminEvent{}}
	tx, err := s.adminTransaction(ctx, operatorID)
	if err != nil {
		return detail, err
	}
	defer tx.Rollback(ctx)
	detail.User, err = scanAdminUser(tx.QueryRow(ctx, `SELECT `+adminUserColumns+` FROM accounts a WHERE a.id=$1`, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return detail, workflow.ErrAdminNotFound
	}
	if err != nil {
		return detail, err
	}
	rows, err := tx.Query(ctx, `SELECT e.id,COALESCE(e.actor_account_id,''),COALESCE(NULLIF(a.display_name,''),a.email,''),
 e.action,e.role,e.previous_status,e.next_status,e.reason,e.affected_sessions,e.created_at
 FROM operator_admin_events e LEFT JOIN accounts a ON a.id=e.actor_account_id WHERE e.account_id=$1 ORDER BY e.id DESC LIMIT 50`, accountID)
	if err != nil {
		return detail, err
	}
	defer rows.Close()
	for rows.Next() {
		var event workflow.AdminEvent
		var at time.Time
		if err = rows.Scan(&event.ID, &event.ActorID, &event.ActorName, &event.Action, &event.Role, &event.PreviousStatus, &event.NextStatus, &event.Reason, &event.AffectedSessions, &at); err != nil {
			return detail, err
		}
		event.CreatedAt = at.UTC().Format(time.RFC3339)
		detail.Events = append(detail.Events, event)
	}
	return detail, rows.Err()
}

func (s *Store) AdministerUser(ctx context.Context, operatorID, accountID string, input workflow.AdminUserAction) error {
	if err := input.Validate(); err != nil {
		return err
	}
	// ReadCommitted lets a second administrator see the latest status after waiting on the lock.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = assertOperator(ctx, tx, operatorID); err != nil {
		return workflow.ErrAdminForbidden
	}
	if input.Role == "professional" {
		if err = lockProfessionalTimes(ctx, tx, accountID); err != nil {
			return err
		}
	}
	var target string
	if err = tx.QueryRow(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, accountID).Scan(&target); errors.Is(err, pgx.ErrNoRows) {
		return workflow.ErrAdminNotFound
	} else if err != nil {
		return err
	}
	var operatorAccount bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_memberships WHERE account_id=$1 AND role='operator')`, accountID).Scan(&operatorAccount); err != nil {
		return err
	}
	if accountID == operatorID || operatorAccount {
		return workflow.ErrAdminProtected
	}
	previous, next := "", ""
	if input.Action != "revoke-sessions" {
		if err = tx.QueryRow(ctx, `SELECT status FROM account_memberships WHERE account_id=$1 AND role=$2 FOR UPDATE`, accountID, input.Role).Scan(&previous); errors.Is(err, pgx.ErrNoRows) {
			return workflow.ErrAdminConflict
		} else if err != nil {
			return err
		}
		if previous != input.ExpectedStatus {
			return workflow.ErrAdminConflict
		}
		if input.Action == "suspend-role" {
			if previous != "active" && previous != "pending" {
				return workflow.ErrAdminConflict
			}
			if _, err = tx.Exec(ctx, `INSERT INTO operator_membership_suspensions(account_id,role,previous_status) VALUES($1,$2,$3)`, accountID, input.Role, previous); err != nil {
				return err
			}
			next = "disabled"
		} else {
			if previous != "disabled" {
				return workflow.ErrAdminConflict
			}
			if err = tx.QueryRow(ctx, `DELETE FROM operator_membership_suspensions WHERE account_id=$1 AND role=$2 RETURNING previous_status`, accountID, input.Role).Scan(&next); errors.Is(err, pgx.ErrNoRows) {
				return workflow.ErrAdminUnrestorable
			} else if err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE account_memberships SET status=$3,updated_at=now() WHERE account_id=$1 AND role=$2`, accountID, input.Role, next); err != nil {
			return err
		}
	}
	var affected int64
	if input.Action != "restore-role" {
		result, err := tx.Exec(ctx, `DELETE FROM account_sessions WHERE account_id=$1 AND ($2='' OR active_role=$2)`, accountID, input.Role)
		if err != nil {
			return err
		}
		affected = result.RowsAffected()
	}
	_, err = tx.Exec(ctx, `INSERT INTO operator_admin_events(account_id,actor_account_id,action,role,previous_status,next_status,reason,affected_sessions)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, accountID, operatorID, input.Action, input.Role, previous, next, strings.TrimSpace(input.Reason), affected)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const adminLocationsCTE = adminCoverageCTE + `, demand AS (
 SELECT lower(regexp_replace(trim(l.label),'[[:space:]]+',' ','g')) AS area,trim(l.label) AS label,
        COALESCE((w.state_json#>>'{intake,waitlisted}')::boolean,false) AND NOT COALESCE((w.state_json->>'cancelled')::boolean,false) AS waitlisted
 FROM account_workspaces w CROSS JOIN LATERAL (
   SELECT COALESCE(NULLIF(w.state_json#>'{intake,location}','null'::jsonb),w.state_json#>'{job,location}') AS location
 ) j CROSS JOIN LATERAL (
   SELECT CASE WHEN j.location->>'kind'='address' THEN concat_ws(' ',NULLIF(j.location->>'postcode',''),NULLIF(j.location->>'city','')) ELSE j.location->>'label' END AS label
 ) l WHERE COALESCE(trim(l.label),'')<>''
), areas AS (
 SELECT area,min(label) AS label FROM (SELECT area,label FROM coverage UNION ALL SELECT area,label FROM demand) all_areas GROUP BY area
), supply_totals AS (
 SELECT area,count(DISTINCT account_id) FILTER(WHERE profile_status='active' AND membership_status='active') AS active,
 count(DISTINCT account_id) FILTER(WHERE profile_status='pending' AND membership_status IN ('active','pending')) AS pending,
 count(DISTINCT account_id) AS professionals,jsonb_agg(DISTINCT category ORDER BY category) AS categories
 FROM coverage GROUP BY area
), demand_totals AS (
 SELECT area,count(*) AS requests,count(*) FILTER(WHERE waitlisted) AS waitlisted FROM demand GROUP BY area
), locations AS (
 SELECT a.label,a.area,COALESCE(s.active,0) AS active,COALESCE(s.pending,0) AS pending,COALESCE(s.professionals,0) AS professionals,
 COALESCE(d.requests,0) AS requests,COALESCE(d.waitlisted,0) AS waitlisted,COALESCE(s.categories,'[]'::jsonb) AS categories
 FROM areas a LEFT JOIN supply_totals s ON s.area=a.area LEFT JOIN demand_totals d ON d.area=a.area
) `

func (s *Store) OperatorLocations(ctx context.Context, operatorID, query, status string, offset int) (workflow.AdminLocationPage, error) {
	page := workflow.AdminLocationPage{Locations: []workflow.AdminLocation{}, Offset: offset}
	if err := (workflow.DirectoryFilter{Query: query, Offset: offset}).Validate(); err != nil {
		return page, err
	}
	if status != "" && status != "available" && status != "uncovered" && status != "waitlisted" {
		return page, errors.New("invalid coverage status")
	}
	tx, err := s.adminTransaction(ctx, operatorID)
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	where := `WHERE ($1='' OR strpos(lower(label),lower($1))>0) AND ($2='' OR $2='available' AND active>0 OR $2='uncovered' AND active=0 OR $2='waitlisted' AND waitlisted>0) `
	if err = tx.QueryRow(ctx, adminLocationsCTE+`SELECT count(*) FROM locations `+where, strings.TrimSpace(query), status).Scan(&page.Total); err != nil {
		return page, err
	}
	rows, err := tx.Query(ctx, adminLocationsCTE+`SELECT label,active,pending,professionals,requests,waitlisted,categories FROM locations `+where+`ORDER BY active DESC,area LIMIT $3 OFFSET $4`, strings.TrimSpace(query), status, workflow.DirectoryPageSize, offset)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var location workflow.AdminLocation
		var categories []byte
		if err = rows.Scan(&location.Label, &location.ActiveProfessionals, &location.PendingProfessionals, &location.TotalProfessionals, &location.Requests, &location.WaitlistedRequests, &categories); err != nil {
			return page, err
		}
		if err = json.Unmarshal(categories, &location.Categories); err != nil {
			return page, err
		}
		page.Locations = append(page.Locations, location)
	}
	page.HasMore = offset+len(page.Locations) < page.Total
	return page, rows.Err()
}

func validateProfessionalSuspensionTx(ctx context.Context, tx pgx.Tx, accountID string) error {
	var suspended bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM operator_membership_suspensions WHERE account_id=$1 AND role='professional')`, accountID).Scan(&suspended); err != nil {
		return err
	}
	if suspended {
		return errors.New("this professional is suspended; choose another professional")
	}
	return nil
}
