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

type slotRule struct {
	daysAhead int // first offset in days
	step      int // days between slots
	count     int
	hour, min int // local start time
	weekdays  []time.Weekday
}

type testPro struct {
	slug, name, locale             string
	category, service, description string
	area                           string
	lat, lon                       float64
	price, callout                 int64
	minutes                        int
	slots                          []slotRule
	mandate                        workflow.AgentMandate
}

var weekdays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
var weekend = []time.Weekday{time.Saturday, time.Sunday}

func mandate(floor int, confirm, accept bool, from, to int, days []int, enabled bool) workflow.AgentMandate {
	return workflow.AgentMandate{Enabled: enabled, FloorPercent: floor, AutoConfirmVisits: confirm, AutoAcceptDates: accept, StartHour: from, EndHour: to, Weekdays: days}
}

var testPros = []testPro{
	{slug: "jane-price", name: "Jane Price", locale: "en", category: "Plumbing", service: "Sink leak repair", description: "Inspect and repair a leaking sink trap or accessible pipe joint.", area: "Amsterdam", lat: 52.37, lon: 4.90, price: 8500, callout: 2500, minutes: 60,
		slots: []slotRule{{daysAhead: 1, step: 1, count: 6, hour: 9}, {daysAhead: 2, step: 2, count: 3, hour: 13}}, mandate: mandate(90, true, true, 8, 18, []int{1, 2, 3, 4, 5}, true)},
	{slug: "mike-dawson", name: "Mike Dawson", locale: "en", category: "Carpentry", service: "Door adjustment", description: "Adjust a sticking wooden door, hinges and latch.", area: "Amsterdam", lat: 52.36, lon: 4.92, price: 7000, callout: 0, minutes: 60,
		slots: []slotRule{{daysAhead: 3, step: 1, count: 14, hour: 10, weekdays: weekend}}, mandate: mandate(90, true, false, 9, 17, []int{6, 7}, true)},
	{slug: "sanne-de-vries", name: "Sanne de Vries", locale: "nl", category: "Electrical assessment", service: "Socket fault assessment", description: "Assess a faulty socket and propose work appropriate to the fault.", area: "Amsterdam", lat: 52.38, lon: 4.88, price: 9000, callout: 2000, minutes: 60,
		slots: []slotRule{{daysAhead: 2, step: 1, count: 8, hour: 18, weekdays: weekdays}}, mandate: mandate(90, true, true, 17, 21, []int{1, 2, 3, 4, 5}, true)},
	{slug: "omar-haddad", name: "Omar Haddad", locale: "en", category: "Plumbing", service: "Toilet repair", description: "Inspect a leaking or continuously running toilet and repair accessible fittings.", area: "Rotterdam", lat: 51.92, lon: 4.48, price: 9500, callout: 4000, minutes: 90,
		slots: []slotRule{{daysAhead: 26, step: 3, count: 2, hour: 11}}, mandate: mandate(80, true, true, 8, 18, []int{1, 2, 3, 4, 5}, true)},
	{slug: "lotte-jansen", name: "Lotte Jansen", locale: "nl", category: "Carpentry", service: "Shelf installation", description: "Fit and secure shelves after inspecting the mounting surface.", area: "Amsterdam", lat: 52.35, lon: 4.87, price: 13500, callout: 1500, minutes: 120,
		slots: []slotRule{{daysAhead: 1, step: 1, count: 6, hour: 7}, {daysAhead: 4, step: 2, count: 4, hour: 15}}, mandate: mandate(90, false, true, 7, 19, []int{1, 2, 3, 4, 5, 6}, true)},
	{slug: "tom-whitaker", name: "Tom Whitaker", locale: "en", category: "Electrical assessment", service: "Circuit breaker assessment", description: "Investigate recurring circuit breaker trips.", area: "Utrecht", lat: 52.09, lon: 5.12, price: 15500, callout: 2500, minutes: 120,
		slots: []slotRule{{daysAhead: 1, step: 1, count: 1, hour: 14}}, mandate: mandate(90, true, true, 8, 18, []int{1, 2, 3, 4, 5}, true)},
	{slug: "priya-nair", name: "Priya Nair", locale: "en", category: "Plumbing", service: "Tap replacement", description: "Replace a faulty tap and test connections for leaks.", area: "Amsterdam", lat: 52.39, lon: 4.91, price: 4500, callout: 0, minutes: 60,
		slots: []slotRule{{daysAhead: 1, step: 1, count: 10, hour: 16, weekdays: weekdays}}, mandate: mandate(75, true, true, 8, 20, []int{1, 2, 3, 4, 5, 6}, true)},
	{slug: "daan-visser", name: "Daan Visser", locale: "nl", category: "Carpentry", service: "Worktop fitting", description: "Measure and fit a replacement kitchen worktop.", area: "Eindhoven", lat: 51.44, lon: 5.48, price: 26000, callout: 5000, minutes: 240,
		slots: []slotRule{{daysAhead: 5, step: 4, count: 5, hour: 10, weekdays: weekdays}}, mandate: mandate(90, true, true, 10, 16, []int{1, 2, 3, 4, 5}, true)},
	{slug: "chloe-martin", name: "Chloe Martin", locale: "en", category: "Electrical assessment", service: "Wiring safety assessment", description: "Inspect reported wiring faults and prepare a repair plan.", area: "Amsterdam", lat: 52.37, lon: 4.85, price: 24000, callout: 4000, minutes: 120,
		slots: []slotRule{{daysAhead: 3, step: 3, count: 5, hour: 11}}, mandate: mandate(100, true, true, 8, 18, []int{1, 2, 3, 4, 5}, true)},
	{slug: "kwame-boateng", name: "Kwame Boateng", locale: "en", category: "Plumbing", service: "Drain clearing", description: "Clear a blocked household sink or drain and check water flow.", area: "Amsterdam", lat: 52.34, lon: 4.94, price: 11000, callout: 3000, minutes: 90,
		slots: nil, mandate: mandate(90, true, true, 8, 18, []int{1, 2, 3, 4, 5}, false)},
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

func (p testPro) slotTimes(anchor time.Time, zone *time.Location) []time.Time {
	var out []time.Time
	local := anchor.In(zone)
	for _, rule := range p.slots {
		for i := 0; i < rule.count; i++ {
			day := rule.daysAhead + i*rule.step
			if len(rule.weekdays) > 0 {
				// Walk forward to the next allowed weekday, one slot per allowed day.
				day = rule.daysAhead
				found := 0
				for ; found <= i; day++ {
					if slicesContainsWeekday(rule.weekdays, local.AddDate(0, 0, day).Weekday()) {
						found++
					}
				}
				day--
			}
			start := time.Date(local.Year(), local.Month(), local.Day()+day, rule.hour, rule.min, 0, 0, zone)
			out = append(out, start.UTC())
		}
	}
	return out
}

func slicesContainsWeekday(days []time.Weekday, d time.Weekday) bool {
	for _, x := range days {
		if x == d {
			return true
		}
	}
	return false
}

// RunPros writes the Auth0 import file for the test professionals and inserts their accounts, profiles,
// free times and assistant settings. Existing rows are left alone, so it can be run again.
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
		for i, start := range p.slotTimes(anchor, zone) {
			if !start.After(anchor.Add(30 * time.Minute)) {
				continue
			}
			if _, err = tx.Exec(ctx, `INSERT INTO professional_availability_slots (id,professional_account_id,starts_at,ends_at,timezone) VALUES ($1,$2,$3,$4,'Europe/Amsterdam') ON CONFLICT DO NOTHING`,
				fmt.Sprintf("slot_pro_%s_%d", p.slug, i+1), accountID, start, start.Add(time.Duration(p.minutes)*time.Minute)); err != nil {
				return err
			}
			slots++
		}
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
