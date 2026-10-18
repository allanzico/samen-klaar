package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type scheduleQuery interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// All scheduling writes share the same professional lock as reservations.
func lockProfessionalTimes(ctx context.Context, tx pgx.Tx, accountID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, accountID)
	return err
}

// One condition is used for customer results, earliest-match facts, and the
// professional's listing. An explicit agreement in chat bypasses notice, while
// its reservation still removes nearby times using the professional's gap.
func bookableAvailabilitySQL(slot, profile, excludeJob string) string {
	exclusion := ""
	if excludeJob != "" {
		exclusion = " AND b.job_id <> " + excludeJob
	}
	return fmt.Sprintf(`%[1]s.status='available'
 AND %[1]s.ends_at-%[1]s.starts_at >= make_interval(mins=>%[2]s.duration_minutes)
 AND %[1]s.starts_at >= now()+make_interval(mins=>COALESCE((SELECT notice_minutes FROM professional_schedules WHERE professional_account_id=%[1]s.professional_account_id),0))
 AND NOT EXISTS(SELECT 1 FROM booking_reservations b WHERE b.professional_id=%[1]s.professional_account_id AND b.status='booked' %[3]s
 AND b.starts_at < %[1]s.ends_at+make_interval(mins=>COALESCE((SELECT gap_minutes FROM professional_schedules WHERE professional_account_id=%[1]s.professional_account_id),0))
 AND b.ends_at > %[1]s.starts_at-make_interval(mins=>COALESCE((SELECT gap_minutes FROM professional_schedules WHERE professional_account_id=%[1]s.professional_account_id),0)))`, slot, profile, exclusion)
}

func loadProfessionalSchedule(ctx context.Context, q scheduleQuery, accountID string) (workflow.ProfessionalScheduleView, error) {
	view := workflow.ProfessionalScheduleView{DaysOff: []workflow.ProfessionalDayOff{}}
	var schedule workflow.ProfessionalSchedule
	err := q.QueryRow(ctx, `SELECT s.timezone,p.duration_minutes,s.gap_minutes,s.notice_minutes,s.horizon_days FROM professional_schedules s JOIN professional_profiles p ON p.account_id=s.professional_account_id WHERE s.professional_account_id=$1`, accountID).Scan(&schedule.Timezone, &schedule.VisitMinutes, &schedule.GapMinutes, &schedule.NoticeMinutes, &schedule.HorizonDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return view, nil
	}
	if err != nil {
		return view, err
	}
	schedule.Week = []workflow.WorkingRange{}
	rows, err := q.Query(ctx, `SELECT weekday,to_char(starts_local,'HH24:MI'),to_char(ends_local,'HH24:MI') FROM professional_working_hours WHERE professional_account_id=$1 ORDER BY weekday,starts_local`, accountID)
	if err != nil {
		return view, err
	}
	for rows.Next() {
		var day workflow.WorkingRange
		if err := rows.Scan(&day.Weekday, &day.Start, &day.End); err != nil {
			rows.Close()
			return view, err
		}
		schedule.Week = append(schedule.Week, day)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return view, err
	}
	rows, err = q.Query(ctx, `SELECT id,first_day::text,last_day::text,note FROM professional_days_off WHERE professional_account_id=$1 AND last_day >= (now() AT TIME ZONE $2)::date ORDER BY first_day,id`, accountID, schedule.Timezone)
	if err != nil {
		return view, err
	}
	for rows.Next() {
		var day workflow.ProfessionalDayOff
		if err := rows.Scan(&day.ID, &day.FirstDay, &day.LastDay, &day.Note); err != nil {
			rows.Close()
			return view, err
		}
		view.DaysOff = append(view.DaysOff, day)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return view, err
	}
	preview, err := workflow.PreviewProfessionalSchedule(schedule)
	if err != nil {
		return view, err
	}
	view.Schedule = &schedule
	view.WeeklyCount = preview.WeeklyCount
	return view, nil
}

func (s *Store) ProfessionalSchedule(ctx context.Context, accountID string) (workflow.ProfessionalScheduleView, error) {
	return loadProfessionalSchedule(ctx, s.pool, accountID)
}

func (s *Store) PreviewProfessionalSchedule(ctx context.Context, accountID string, schedule workflow.ProfessionalSchedule) (workflow.SchedulePreview, error) {
	if err := s.pool.QueryRow(ctx, `SELECT duration_minutes FROM professional_profiles WHERE account_id=$1`, accountID).Scan(&schedule.VisitMinutes); err != nil {
		return workflow.SchedulePreview{}, errors.New("Save your service profile before setting working hours.")
	}
	if err := workflow.ValidateProfessionalSchedule(&schedule); err != nil {
		return workflow.SchedulePreview{}, err
	}
	return workflow.PreviewProfessionalSchedule(schedule)
}

func (s *Store) SaveProfessionalSchedule(ctx context.Context, accountID string, schedule workflow.ProfessionalSchedule) (workflow.ProfessionalScheduleView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return workflow.ProfessionalScheduleView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = lockProfessionalTimes(ctx, tx, accountID); err != nil {
		return workflow.ProfessionalScheduleView{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT duration_minutes FROM professional_profiles WHERE account_id=$1`, accountID).Scan(&schedule.VisitMinutes); err != nil {
		return workflow.ProfessionalScheduleView{}, errors.New("Save your service profile before setting working hours.")
	}
	if err = workflow.ValidateProfessionalSchedule(&schedule); err != nil {
		return workflow.ProfessionalScheduleView{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO professional_schedules(professional_account_id,timezone,gap_minutes,notice_minutes,horizon_days) VALUES($1,$2,$3,$4,$5) ON CONFLICT(professional_account_id) DO UPDATE SET timezone=EXCLUDED.timezone,gap_minutes=EXCLUDED.gap_minutes,notice_minutes=EXCLUDED.notice_minutes,horizon_days=EXCLUDED.horizon_days,updated_at=now()`, accountID, schedule.Timezone, schedule.GapMinutes, schedule.NoticeMinutes, schedule.HorizonDays); err != nil {
		return workflow.ProfessionalScheduleView{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM professional_working_hours WHERE professional_account_id=$1`, accountID); err != nil {
		return workflow.ProfessionalScheduleView{}, err
	}
	for _, day := range schedule.Week {
		if _, err = tx.Exec(ctx, `INSERT INTO professional_working_hours(professional_account_id,weekday,starts_local,ends_local) VALUES($1,$2,$3::time,$4::time)`, accountID, day.Weekday, day.Start, day.End); err != nil {
			return workflow.ProfessionalScheduleView{}, err
		}
	}
	added, removed, err := s.materializeSchedule(ctx, tx, accountID, time.Now())
	if err != nil {
		return workflow.ProfessionalScheduleView{}, err
	}
	view, err := loadProfessionalSchedule(ctx, tx, accountID)
	if err != nil {
		return view, err
	}
	view.Added, view.Removed = added, removed
	return view, tx.Commit(ctx)
}

func (s *Store) materializeSchedule(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (int, int, error) {
	view, err := loadProfessionalSchedule(ctx, tx, accountID)
	if err != nil || view.Schedule == nil {
		return 0, 0, err
	}
	exclusions := map[string]bool{}
	rows, err := tx.Query(ctx, `SELECT starts_at FROM professional_slot_exclusions WHERE professional_account_id=$1`, accountID)
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			rows.Close()
			return 0, 0, err
		}
		exclusions[at.UTC().Format(time.RFC3339)] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, 0, err
	}
	desired, err := workflow.GenerateProfessionalAvailability(*view.Schedule, now, view.DaysOff, exclusions)
	if err != nil {
		return 0, 0, err
	}
	type interval struct{ start, end time.Time }
	blocked := []interval{}
	rows, err = tx.Query(ctx, `SELECT starts_at,ends_at FROM professional_availability_slots WHERE professional_account_id=$1 AND ends_at>$2 AND (status='booked' OR source='manual' AND status='available') UNION ALL SELECT starts_at,ends_at FROM booking_reservations WHERE professional_id=$1 AND status='booked' AND ends_at>$2`, accountID, now)
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var b interval
		if err := rows.Scan(&b.start, &b.end); err != nil {
			rows.Close()
			return 0, 0, err
		}
		blocked = append(blocked, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, 0, err
	}
	wanted := map[string]workflow.Appointment{}
	gap := time.Duration(view.Schedule.GapMinutes) * time.Minute
	for _, slot := range desired {
		start, end := mustParseUTC(slot.StartUTC), mustParseUTC(slot.EndUTC)
		if start.Before(now) {
			continue
		}
		overlaps := false
		for _, b := range blocked {
			if start.Before(b.end.Add(gap)) && end.After(b.start.Add(-gap)) {
				overlaps = true
				break
			}
		}
		if !overlaps {
			wanted[slot.StartUTC+"/"+slot.EndUTC] = slot
		}
	}
	rows, err = tx.Query(ctx, `SELECT id,starts_at,ends_at FROM professional_availability_slots WHERE professional_account_id=$1 AND source='schedule' AND status='available' AND starts_at>=$2`, accountID, now)
	if err != nil {
		return 0, 0, err
	}
	remove := []string{}
	for rows.Next() {
		var id string
		var start, end time.Time
		if err := rows.Scan(&id, &start, &end); err != nil {
			rows.Close()
			return 0, 0, err
		}
		key := start.UTC().Format(time.RFC3339) + "/" + end.UTC().Format(time.RFC3339)
		if _, ok := wanted[key]; ok {
			delete(wanted, key)
		} else {
			remove = append(remove, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, 0, err
	}
	if len(remove) > 0 {
		if _, err = tx.Exec(ctx, `DELETE FROM professional_availability_slots WHERE professional_account_id=$1 AND id=ANY($2::text[]) AND status='available' AND source='schedule'`, accountID, remove); err != nil {
			return 0, 0, err
		}
	}
	batch := &pgx.Batch{}
	for _, slot := range wanted {
		id, err := randomID("slot_")
		if err != nil {
			return 0, 0, err
		}
		batch.Queue(`INSERT INTO professional_availability_slots(id,professional_account_id,starts_at,ends_at,timezone,source) VALUES($1,$2,$3,$4,$5,'schedule') ON CONFLICT(professional_account_id,starts_at,ends_at) DO NOTHING`, id, accountID, mustParseUTC(slot.StartUTC), mustParseUTC(slot.EndUTC), slot.Timezone)
	}
	added := 0
	result := tx.SendBatch(ctx, batch)
	for range wanted {
		tag, err := result.Exec()
		if err != nil {
			_ = result.Close()
			return 0, 0, err
		}
		added += int(tag.RowsAffected())
	}
	if err = result.Close(); err != nil {
		return 0, 0, err
	}
	zone := loadTimezone(view.Schedule.Timezone)
	through := now.In(zone).AddDate(0, 0, view.Schedule.HorizonDays-1).Format("2006-01-02")
	if _, err = tx.Exec(ctx, `UPDATE professional_schedules SET materialized_through=$2::date WHERE professional_account_id=$1`, accountID, through); err != nil {
		return 0, 0, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM professional_availability_slots WHERE professional_account_id=$1 AND source='schedule' AND status='available' AND ends_at<$2::timestamptz-interval '7 days'`, accountID, now); err != nil {
		return 0, 0, err
	}
	return added, len(remove), nil
}

func (s *Store) materializeRecurringAvailability(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT professional_account_id FROM professional_schedules WHERE materialized_through IS NULL OR materialized_through < (now() AT TIME ZONE timezone)::date+horizon_days-1 ORDER BY materialized_through NULLS FIRST LIMIT 100`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if err = lockProfessionalTimes(ctx, tx, id); err == nil {
			_, _, err = s.materializeSchedule(ctx, tx, id, time.Now())
		}
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func scheduleWindow(schedule *workflow.ProfessionalSchedule, from string, days int, now time.Time) (time.Time, time.Time, error) {
	timezone := "Europe/Amsterdam"
	horizon := 90
	if schedule != nil {
		timezone = schedule.Timezone
		horizon = schedule.HorizonDays
	}
	zone := loadTimezone(timezone)
	if days < 1 || days > 90 {
		return time.Time{}, time.Time{}, errors.New("Choose from 1 to 90 days of availability.")
	}
	if from == "" {
		from = now.In(zone).Format("2006-01-02")
	}
	start, err := time.ParseInLocation("2006-01-02", from, zone)
	if err != nil {
		return start, start, errors.New("Choose a valid availability date.")
	}
	today := now.In(zone).Format("2006-01-02")
	if from < today || from >= now.In(zone).AddDate(0, 0, horizon).Format("2006-01-02") {
		return start, start, errors.New("Choose a future time inside your booking window.")
	}
	end := start.AddDate(0, 0, days)
	limitLocal := now.In(zone).AddDate(0, 0, horizon)
	limit := time.Date(limitLocal.Year(), limitLocal.Month(), limitLocal.Day(), 0, 0, 0, 0, zone)
	if end.After(limit) {
		end = limit
	}
	return start, end, nil
}

func (s *Store) ProfessionalAvailabilityWindow(ctx context.Context, accountID, from string, days int) (workflow.ProfessionalAvailabilityWindow, error) {
	result := workflow.ProfessionalAvailabilityWindow{Slots: []workflow.ScheduledAvailabilitySlot{}, Removed: []workflow.RemovedAvailabilitySlot{}, DaysOff: []workflow.ProfessionalDayOff{}}
	view, err := s.ProfessionalSchedule(ctx, accountID)
	if err != nil {
		return result, err
	}
	start, end, err := scheduleWindow(view.Schedule, from, days, time.Now())
	if err != nil {
		return result, err
	}
	result.DaysOff = view.DaysOff
	rows, err := s.pool.Query(ctx, `SELECT a.id,a.starts_at,a.ends_at,a.timezone,a.status,a.source FROM professional_availability_slots a JOIN professional_profiles p ON p.account_id=a.professional_account_id WHERE a.professional_account_id=$1 AND a.starts_at>=$2 AND a.starts_at<$3 AND (a.status='booked' OR (`+bookableAvailabilitySQL("a", "p", "")+`)) ORDER BY a.starts_at,a.id`, accountID, start, end)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var slot workflow.ScheduledAvailabilitySlot
		var at, to time.Time
		if err := rows.Scan(&slot.SlotID, &at, &to, &slot.Timezone, &slot.Status, &slot.Source); err != nil {
			rows.Close()
			return result, err
		}
		slot.StartUTC, slot.EndUTC = at.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)
		slot.StartsAt = at.In(loadTimezone(slot.Timezone)).Format("Mon, 2 Jan · 15:04")
		result.Slots = append(result.Slots, slot)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if view.Schedule != nil {
		generated, generateErr := workflow.GenerateProfessionalAvailability(*view.Schedule, time.Now(), view.DaysOff, nil)
		if generateErr != nil {
			return result, generateErr
		}
		current := map[string]string{}
		for _, slot := range generated {
			current[slot.StartUTC] = slot.EndUTC
		}
		rows, err = s.pool.Query(ctx, `SELECT starts_at FROM professional_slot_exclusions WHERE professional_account_id=$1 AND starts_at>=$2 AND starts_at<$3 ORDER BY starts_at`, accountID, start, end)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var at time.Time
			if err := rows.Scan(&at); err != nil {
				rows.Close()
				return result, err
			}
			if endUTC, exists := current[at.UTC().Format(time.RFC3339)]; exists {
				result.Removed = append(result.Removed, workflow.RemovedAvailabilitySlot{StartUTC: at.UTC().Format(time.RFC3339), EndUTC: endUTC})
			}
		}
		err = rows.Err()
		rows.Close()
	}
	return result, err
}

func (s *Store) PreviewProfessionalDaysOff(ctx context.Context, accountID string, day workflow.ProfessionalDayOff) (workflow.DaysOffPreview, error) {
	view, err := s.ProfessionalSchedule(ctx, accountID)
	if err != nil {
		return workflow.DaysOffPreview{}, err
	}
	if view.Schedule == nil {
		return workflow.DaysOffPreview{}, errors.New("Save working hours before adding days off.")
	}
	if err = workflow.ValidateProfessionalDayOff(&day, view.Schedule.Timezone, time.Now()); err != nil {
		return workflow.DaysOffPreview{}, err
	}
	return previewDaysOff(ctx, s.pool, accountID, day, view.Schedule.Timezone)
}

func previewDaysOff(ctx context.Context, q scheduleQuery, accountID string, day workflow.ProfessionalDayOff, timezone string) (workflow.DaysOffPreview, error) {
	preview := workflow.DaysOffPreview{BookedVisits: []workflow.ScheduledBookedVisit{}}
	if err := q.QueryRow(ctx, `SELECT count(*) FROM professional_availability_slots WHERE professional_account_id=$1 AND status='available' AND (starts_at AT TIME ZONE $4)::date BETWEEN $2::date AND $3::date`, accountID, day.FirstDay, day.LastDay, timezone).Scan(&preview.RemovedCount); err != nil {
		return preview, err
	}
	rows, err := q.Query(ctx, `SELECT b.job_id,w.id,COALESCE(w.state_json #>> '{job,title}','Booked visit'),b.starts_at,b.ends_at FROM booking_reservations b JOIN account_workspaces w ON w.state_json #>> '{job,id}'=b.job_id WHERE b.professional_id=$1 AND b.status='booked' AND (b.starts_at AT TIME ZONE $4)::date BETWEEN $2::date AND $3::date ORDER BY b.starts_at`, accountID, day.FirstDay, day.LastDay, timezone)
	if err != nil {
		return preview, err
	}
	defer rows.Close()
	for rows.Next() {
		var visit workflow.ScheduledBookedVisit
		var start, end time.Time
		if err := rows.Scan(&visit.JobID, &visit.WorkspaceID, &visit.Title, &start, &end); err != nil {
			return preview, err
		}
		visit.StartUTC, visit.EndUTC = start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)
		preview.BookedVisits = append(preview.BookedVisits, visit)
	}
	return preview, rows.Err()
}

func (s *Store) AddProfessionalDaysOff(ctx context.Context, accountID string, day workflow.ProfessionalDayOff) (workflow.ProfessionalDayOff, int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return day, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = lockProfessionalTimes(ctx, tx, accountID); err != nil {
		return day, 0, err
	}
	view, err := loadProfessionalSchedule(ctx, tx, accountID)
	if err != nil {
		return day, 0, err
	}
	if view.Schedule == nil {
		return day, 0, errors.New("Save working hours before adding days off.")
	}
	if err = workflow.ValidateProfessionalDayOff(&day, view.Schedule.Timezone, time.Now()); err != nil {
		return day, 0, err
	}
	day.ID, err = randomID("off_")
	if err != nil {
		return day, 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO professional_days_off(id,professional_account_id,first_day,last_day,note) VALUES($1,$2,$3::date,$4::date,$5)`, day.ID, accountID, day.FirstDay, day.LastDay, day.Note); err != nil {
		return day, 0, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM professional_availability_slots WHERE professional_account_id=$1 AND status='available' AND (starts_at AT TIME ZONE $4)::date BETWEEN $2::date AND $3::date AND NOT EXISTS(SELECT 1 FROM booking_reservations b WHERE b.professional_id=$1 AND b.status='booked' AND b.slot_id=professional_availability_slots.id)`, accountID, day.FirstDay, day.LastDay, view.Schedule.Timezone)
	if err != nil {
		return day, 0, err
	}
	_, _, err = s.materializeSchedule(ctx, tx, accountID, time.Now())
	if err != nil {
		return day, 0, err
	}
	return day, int(tag.RowsAffected()), tx.Commit(ctx)
}

func (s *Store) RemoveProfessionalDaysOff(ctx context.Context, accountID, id string) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = lockProfessionalTimes(ctx, tx, accountID); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM professional_days_off WHERE professional_account_id=$1 AND id=$2`, accountID, id)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() != 1 {
		return 0, errors.New("Days off were not found.")
	}
	added, _, err := s.materializeSchedule(ctx, tx, accountID, time.Now())
	if err != nil {
		return 0, err
	}
	return added, tx.Commit(ctx)
}

func (s *Store) RemoveProfessionalAvailability(ctx context.Context, accountID, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = lockProfessionalTimes(ctx, tx, accountID); err != nil {
		return err
	}
	var start time.Time
	var source, status string
	var booked bool
	err = tx.QueryRow(ctx, `SELECT a.starts_at,a.source,a.status,EXISTS(SELECT 1 FROM booking_reservations b WHERE b.professional_id=$1 AND b.status='booked' AND b.slot_id=a.id) FROM professional_availability_slots a WHERE a.professional_account_id=$1 AND a.id=$2 FOR UPDATE`, accountID, id).Scan(&start, &source, &status, &booked)
	if err != nil {
		return errors.New("That free time was not found.")
	}
	if status == "booked" || booked {
		return errors.New("That time is booked. Open the job to change it.")
	}
	view, err := loadProfessionalSchedule(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if start.Before(time.Now()) {
		return errors.New("Choose a future time inside your booking window.")
	}
	if view.Schedule != nil {
		if _, _, err = scheduleWindow(view.Schedule, start.In(loadTimezone(view.Schedule.Timezone)).Format("2006-01-02"), 1, time.Now()); err != nil {
			return err
		}
	}
	if source == "schedule" {
		if _, err = tx.Exec(ctx, `INSERT INTO professional_slot_exclusions(professional_account_id,starts_at) VALUES($1,$2) ON CONFLICT DO NOTHING`, accountID, start); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM professional_availability_slots WHERE professional_account_id=$1 AND id=$2 AND status='available'`, accountID, id); err != nil {
		return err
	}
	if _, _, err = s.materializeSchedule(ctx, tx, accountID, time.Now()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RestoreProfessionalAvailability(ctx context.Context, accountID, startUTC string) error {
	start, err := time.Parse(time.RFC3339, startUTC)
	if err != nil {
		return errors.New("Choose a valid availability time.")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = lockProfessionalTimes(ctx, tx, accountID); err != nil {
		return err
	}
	view, err := loadProfessionalSchedule(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if view.Schedule == nil || start.Before(time.Now()) {
		return errors.New("Choose a future time inside your booking window.")
	}
	if _, _, err = scheduleWindow(view.Schedule, start.In(loadTimezone(view.Schedule.Timezone)).Format("2006-01-02"), 1, time.Now()); err != nil {
		return err
	}
	var booked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM booking_reservations WHERE professional_id=$1 AND status='booked' AND starts_at<=$2 AND ends_at>$2)`, accountID, start).Scan(&booked); err != nil {
		return err
	}
	if booked {
		return errors.New("That time is booked. Open the job to change it.")
	}
	if _, err = tx.Exec(ctx, `DELETE FROM professional_slot_exclusions WHERE professional_account_id=$1 AND starts_at=$2`, accountID, start); err != nil {
		return err
	}
	if _, _, err = s.materializeSchedule(ctx, tx, accountID, time.Now()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateScheduledBookingTx(ctx context.Context, tx pgx.Tx, accountID string, start, end time.Time, jobID string) error {
	var tooSoon bool
	err := tx.QueryRow(ctx, `SELECT $2 < now()+make_interval(mins=>COALESCE((SELECT notice_minutes FROM professional_schedules WHERE professional_account_id=$1),0))`, accountID, start).Scan(&tooSoon)
	if err != nil {
		return err
	}
	if tooSoon {
		return errors.New("That time is too soon to book. Choose a later time.")
	}
	var overlaps bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM booking_reservations b WHERE b.professional_id=$1 AND b.status='booked' AND b.job_id<>$4 AND b.starts_at<$3::timestamptz+make_interval(mins=>COALESCE((SELECT gap_minutes FROM professional_schedules WHERE professional_account_id=$1),0)) AND b.ends_at>$2::timestamptz-make_interval(mins=>COALESCE((SELECT gap_minutes FROM professional_schedules WHERE professional_account_id=$1),0)))`, accountID, start, end, jobID).Scan(&overlaps); err != nil {
		return err
	}
	if overlaps {
		return errors.New("That time is too close to another booked visit. Choose a later time.")
	}
	return nil
}

func validateManualAvailabilityTx(ctx context.Context, tx pgx.Tx, accountID string, start, end time.Time) error {
	var blocked bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM professional_days_off d JOIN professional_schedules s ON s.professional_account_id=d.professional_account_id WHERE d.professional_account_id=$1 AND ($2::timestamptz AT TIME ZONE s.timezone)::date BETWEEN d.first_day AND d.last_day) OR EXISTS(SELECT 1 FROM booking_reservations b WHERE b.professional_id=$1 AND b.status='booked' AND b.starts_at<$3::timestamptz+make_interval(mins=>COALESCE((SELECT gap_minutes FROM professional_schedules WHERE professional_account_id=$1),0)) AND b.ends_at>$2::timestamptz-make_interval(mins=>COALESCE((SELECT gap_minutes FROM professional_schedules WHERE professional_account_id=$1),0)))`, accountID, start, end).Scan(&blocked)
	if err != nil {
		return err
	}
	if blocked {
		return errors.New("This time overlaps a booked visit or a day off.")
	}
	return nil
}
