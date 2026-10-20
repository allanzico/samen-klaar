package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// A small, hand-designed set of professionals for testing every scenario: different trades, cities, prices,
// call-out fees, availability patterns (none, a few days out, evenings only, weekends only, many) and
// assistant settings (default, pausing, asking first, narrow hours, no discounts). They share one test
// password and use the reserved .test domain, so no real mailbox is ever addressed.

// version marks every user the seed tools create, so the Auth0 import can recognise its own batches.
const version = "relay-seed-v1"

type manifest struct {
	Version      string    `json:"version"`
	Issuer       string    `json:"issuer"`
	Anchor       time.Time `json:"availability_anchor"`
	PasswordHash string    `json:"password_hash"`
	Files        []string  `json:"import_files"`
}

type authUser struct {
	ID            string            `json:"user_id"`
	Email         string            `json:"email"`
	Name          string            `json:"name"`
	EmailVerified bool              `json:"email_verified"`
	PasswordHash  string            `json:"password_hash"`
	Metadata      map[string]string `json:"app_metadata"`
	Username      string            `json:"username,omitempty"`
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".seed-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

const prosDomain = "samenklaar.test"

type testPro struct {
	slug, name, locale             string
	category, service, description string
	area                           string
	lat, lon                       float64
	price, callout                 int64
	minutes                        int
	week                           []workflow.WorkingRange
	daysOffAhead                   int
	mandate                        workflow.AgentMandate
}

func weeklyHours(start, end string, weekdays ...int) []workflow.WorkingRange {
	week := make([]workflow.WorkingRange, 0, len(weekdays))
	for _, day := range weekdays {
		week = append(week, workflow.WorkingRange{Weekday: day, Start: start, End: end})
	}
	return week
}

func mandate(floor int, confirm, accept bool, from, to int, days []int, enabled bool) workflow.AgentMandate {
	return workflow.AgentMandate{Enabled: enabled, FloorPercent: floor, AutoConfirmVisits: confirm, AutoAcceptDates: accept, StartHour: from, EndHour: to, Weekdays: days}
}

var testPros = []testPro{
	{slug: "jane-price", name: "Jane Price", locale: "en", category: "Plumbing", service: "Sink leak repair", description: "Inspect and repair a leaking sink trap or accessible pipe joint.", area: "Amsterdam", lat: 52.37, lon: 4.90, price: 8500, callout: 2500, minutes: 60,
		week: weeklyHours("09:00", "17:00", 1, 2, 3, 4, 5), mandate: mandate(90, true, true, 8, 18, []int{1, 2, 3, 4, 5}, true)},
	{slug: "mike-dawson", name: "Mike Dawson", locale: "en", category: "Carpentry", service: "Door adjustment", description: "Adjust a sticking wooden door, hinges and latch.", area: "Amsterdam", lat: 52.36, lon: 4.92, price: 7000, callout: 0, minutes: 60,
		week: weeklyHours("10:00", "16:00", 6, 7), mandate: mandate(90, true, false, 9, 17, []int{6, 7}, true)},
	{slug: "sanne-de-vries", name: "Sanne de Vries", locale: "nl", category: "Electrical assessment", service: "Socket fault assessment", description: "Assess a faulty socket and propose work appropriate to the fault.", area: "Amsterdam", lat: 52.38, lon: 4.88, price: 9000, callout: 2000, minutes: 60,
		week: weeklyHours("17:00", "21:00", 1, 2, 3, 4, 5), mandate: mandate(90, true, true, 17, 21, []int{1, 2, 3, 4, 5}, true)},
	{slug: "omar-haddad", name: "Omar Haddad", locale: "en", category: "Plumbing", service: "Toilet repair", description: "Inspect a leaking or continuously running toilet and repair accessible fittings.", area: "Rotterdam", lat: 51.92, lon: 4.48, price: 9500, callout: 4000, minutes: 90,
		week: weeklyHours("09:00", "17:00", 1, 2, 3, 4, 5), daysOffAhead: 25, mandate: mandate(80, true, true, 8, 18, []int{1, 2, 3, 4, 5}, true)},
	{slug: "lotte-jansen", name: "Lotte Jansen", locale: "nl", category: "Carpentry", service: "Shelf installation", description: "Fit and secure shelves after inspecting the mounting surface.", area: "Amsterdam", lat: 52.35, lon: 4.87, price: 13500, callout: 1500, minutes: 120,
		week: weeklyHours("07:00", "19:00", 1, 2, 3, 4, 5, 6), mandate: mandate(90, false, true, 7, 19, []int{1, 2, 3, 4, 5, 6}, true)},
	{slug: "tom-whitaker", name: "Tom Whitaker", locale: "en", category: "Electrical assessment", service: "Circuit breaker assessment", description: "Investigate recurring circuit breaker trips.", area: "Utrecht", lat: 52.09, lon: 5.12, price: 15500, callout: 2500, minutes: 120,
		week: weeklyHours("14:00", "16:00", 2), mandate: mandate(90, true, true, 8, 18, []int{1, 2, 3, 4, 5}, true)},
	{slug: "priya-nair", name: "Priya Nair", locale: "en", category: "Plumbing", service: "Tap replacement", description: "Replace a faulty tap and test connections for leaks.", area: "Amsterdam", lat: 52.39, lon: 4.91, price: 4500, callout: 0, minutes: 60,
		week: weeklyHours("16:00", "18:00", 1, 2, 3, 4, 5), mandate: mandate(75, true, true, 8, 20, []int{1, 2, 3, 4, 5, 6}, true)},
	{slug: "daan-visser", name: "Daan Visser", locale: "nl", category: "Carpentry", service: "Worktop fitting", description: "Measure and fit a replacement kitchen worktop.", area: "Eindhoven", lat: 51.44, lon: 5.48, price: 26000, callout: 5000, minutes: 240,
		week: weeklyHours("10:00", "16:00", 3, 5), mandate: mandate(90, true, true, 10, 16, []int{1, 2, 3, 4, 5}, true)},
	{slug: "chloe-martin", name: "Chloe Martin", locale: "en", category: "Electrical assessment", service: "Wiring safety assessment", description: "Inspect reported wiring faults and prepare a repair plan.", area: "Amsterdam", lat: 52.37, lon: 4.85, price: 24000, callout: 4000, minutes: 120,
		week: weeklyHours("11:00", "16:00", 1, 2, 3, 4, 5), mandate: mandate(100, true, true, 8, 18, []int{1, 2, 3, 4, 5}, true)},
	{slug: "kwame-boateng", name: "Kwame Boateng", locale: "en", category: "Plumbing", service: "Drain clearing", description: "Clear a blocked household sink or drain and check water flow.", area: "Amsterdam", lat: 52.34, lon: 4.94, price: 11000, callout: 3000, minutes: 90,
		week: []workflow.WorkingRange{}, mandate: mandate(90, true, true, 8, 18, []int{1, 2, 3, 4, 5}, false)},
}

// ProEmails lists the test professionals' sign-in addresses.
func ProEmails() []string {
	emails := make([]string, 0, len(testPros))
	for _, p := range testPros {
		emails = append(emails, p.slug+"@"+prosDomain)
	}
	return emails
}

func (p testPro) subject() string { return "relay-pro-" + p.slug }

func (p testPro) schedule() workflow.ProfessionalSchedule {
	return workflow.ProfessionalSchedule{Timezone: "Europe/Amsterdam", VisitMinutes: p.minutes, GapMinutes: 30, NoticeMinutes: 1440, HorizonDays: 56, Week: append([]workflow.WorkingRange{}, p.week...)}
}

func (p testPro) daysOff(anchor time.Time, zone *time.Location) []workflow.ProfessionalDayOff {
	if p.daysOffAhead == 0 {
		return nil
	}
	return []workflow.ProfessionalDayOff{{ID: "off_pro_" + p.slug, FirstDay: anchor.In(zone).Format("2006-01-02"), LastDay: anchor.In(zone).AddDate(0, 0, p.daysOffAhead-1).Format("2006-01-02"), Note: "Unavailable"}}
}

// Seed only a missing schedule. An existing schedule, including an empty week,
// is the professional's choice and must survive repeated seed runs.
func seedSchedule(ctx context.Context, tx pgx.Tx, p testPro, accountID string, anchor time.Time, zone *time.Location) (int, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, accountID); err != nil {
		return 0, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM professional_schedules WHERE professional_account_id=$1)`, accountID).Scan(&exists); err != nil || exists {
		return 0, err
	}
	schedule := p.schedule()
	if err := tx.QueryRow(ctx, `SELECT duration_minutes FROM professional_profiles WHERE account_id=$1`, accountID).Scan(&schedule.VisitMinutes); err != nil {
		return 0, err
	}
	if err := workflow.ValidateProfessionalSchedule(&schedule); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO professional_schedules(professional_account_id,timezone,gap_minutes,notice_minutes,horizon_days) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, accountID, schedule.Timezone, schedule.GapMinutes, schedule.NoticeMinutes, schedule.HorizonDays)
	if err != nil || tag.RowsAffected() == 0 {
		return 0, err
	}
	for _, day := range schedule.Week {
		if _, err := tx.Exec(ctx, `INSERT INTO professional_working_hours(professional_account_id,weekday,starts_local,ends_local) VALUES($1,$2,$3::time,$4::time)`, accountID, day.Weekday, day.Start, day.End); err != nil {
			return 0, err
		}
	}
	daysOff := p.daysOff(anchor, zone)
	for _, day := range daysOff {
		if _, err := tx.Exec(ctx, `INSERT INTO professional_days_off(id,professional_account_id,first_day,last_day,note) VALUES($1,$2,$3::date,$4::date,$5)`, day.ID, accountID, day.FirstDay, day.LastDay, day.Note); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM professional_availability_slots a WHERE a.professional_account_id=$1 AND a.source='manual' AND a.status='available' AND NOT EXISTS(SELECT 1 FROM booking_reservations b WHERE b.slot_id=a.id AND b.status='booked')`, accountID); err != nil {
		return 0, err
	}
	times, err := workflow.GenerateProfessionalAvailability(schedule, anchor, daysOff, nil)
	if err != nil {
		return 0, err
	}
	added := 0
	for _, slot := range times {
		start, err := time.Parse(time.RFC3339, slot.StartUTC)
		if err != nil {
			return 0, err
		}
		if !start.After(anchor) {
			continue
		}
		tag, err := tx.Exec(ctx, `INSERT INTO professional_availability_slots(id,professional_account_id,starts_at,ends_at,timezone,source) VALUES($1,$2,$3,$4,$5,'schedule') ON CONFLICT DO NOTHING`, fmt.Sprintf("slot_pro_%s_%d", p.slug, start.Unix()), accountID, start, slot.EndUTC, slot.Timezone)
		if err != nil {
			return 0, err
		}
		added += int(tag.RowsAffected())
	}
	_, err = tx.Exec(ctx, `UPDATE professional_schedules SET materialized_through=$2::date WHERE professional_account_id=$1`, accountID, anchor.In(zone).AddDate(0, 0, schedule.HorizonDays-1).Format("2006-01-02"))
	return added, err
}

// RunPros writes the Auth0 import file for the test professionals and inserts their accounts, profiles,
// weekly hours, generated free times and assistant settings. Existing schedules and account settings
// are left alone, so it can be run again without resetting a professional’s choices.
func RunPros(ctx context.Context, databaseURL, issuer, password, output string) error {
	issuer = strings.TrimRight(issuer, "/") + "/"
	if !strings.HasPrefix(issuer, "https://") || password == "" {
		return errors.New("AUTH0_ISSUER_URL and SEED_PASSWORD are required")
	}
	zone, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return errors.New("could not hash SEED_PASSWORD")
	}
	anchor := time.Now().UTC()
	users := make([]authUser, 0, len(testPros))
	for _, p := range testPros {
		users = append(users, authUser{ID: p.subject(), Email: p.slug + "@" + prosDomain, Name: p.name, EmailVerified: true, PasswordHash: string(hash), Metadata: map[string]string{"relay_seed": version}})
	}
	const file = "auth0-users-001.json"
	if err := writeJSON(filepath.Join(output, file), users); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(output, "manifest.json"), manifest{Version: version, Issuer: issuer, Anchor: anchor, PasswordHash: string(hash), Files: []string{file}}); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	slots := 0
	for _, p := range testPros {
		accountID, profileID, subject := "acct_pro_"+p.slug, "prof_pro_"+p.slug, "auth0|"+p.subject()
		if _, err = tx.Exec(ctx, `INSERT INTO accounts (id,issuer,subject,email,display_name,preferred_role,locale) VALUES ($1,$2,$3,$4,$5,'professional',$6) ON CONFLICT (issuer,subject) DO NOTHING`, accountID, issuer, subject, p.slug+"@"+prosDomain, p.name, p.locale); err != nil {
			return err
		}
		for _, role := range []string{"customer", "professional"} {
			if _, err = tx.Exec(ctx, `INSERT INTO account_memberships (account_id,role,status) VALUES ($1,$2,'active') ON CONFLICT DO NOTHING`, accountID, role); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO professional_profiles (id,account_id,display_name,category,service_name,service_description,service_area,latitude,longitude,currency,price_minor,callout_minor,duration_minutes,materials_policy,status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'EUR',$10,$11,$12,'Materials quoted separately after customer approval.','active') ON CONFLICT (account_id) DO NOTHING`,
			profileID, accountID, p.name, p.category, p.service, p.description, p.area, p.lat, p.lon, p.price, p.callout, p.minutes); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO professional_review_checks (professional_profile_id,identity_business_status,paypal_onboarding_status,qualification_status,insurance_status) VALUES ($1,'verified','pending','verified','verified') ON CONFLICT DO NOTHING`, profileID); err != nil {
			return err
		}
		added, err := seedSchedule(ctx, tx, p, accountID, anchor, zone)
		if err != nil {
			return fmt.Errorf("%s schedule: %w", p.slug, err)
		}
		slots += added
		m := p.mandate
		if err := m.Normalize("professional"); err != nil {
			return fmt.Errorf("%s: %w", p.slug, err)
		}
		raw, _ := json.Marshal(m)
		if _, err = tx.Exec(ctx, `INSERT INTO agent_mandates (account_id,role,mandate) VALUES ($1,'professional',$2) ON CONFLICT DO NOTHING`, accountID, raw); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("{\"professionals\":%d,\"slots\":%d,\"auth0_import_files\":1,\"auth0_status\":\"prepared; import required before password login\"}\n", len(testPros), slots)
	return nil
}
