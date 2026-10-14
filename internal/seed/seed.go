// Package seed supplies explicit, local-only test data and performance tools.
// Application startup and HTTP handlers never call this package.
package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const Customers = 5
const Professionals = 10000

// Keep seed identities/version stable: local Auth0 users and manifests already use these IDs.
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

type service struct {
	category, name, description string
	minutes, basePrice          int
}

var services = []service{
	{"Plumbing", "Sink leak repair", "Inspect and repair a leaking sink trap or accessible pipe joint.", 60, 7500},
	{"Plumbing", "Drain clearing", "Clear a blocked household sink or drain and check water flow.", 90, 11000},
	{"Plumbing", "Industrial drain assessment", "Assess extensive drain blockage and arrange appropriate jetting equipment.", 120, 18000},
	{"Plumbing", "Tap replacement", "Replace a faulty tap and test connections for leaks.", 60, 9000},
	{"Plumbing", "Toilet repair", "Inspect a leaking or continuously running toilet and repair accessible fittings.", 90, 10000},
	{"Plumbing", "Water supply assessment", "Investigate weak or absent water flow and propose a repair plan.", 120, 14000},
	{"Carpentry", "Door adjustment", "Adjust a sticking wooden door, hinges and latch.", 60, 7000},
	{"Carpentry", "Cabinet repair", "Repair a loose cabinet door, hinges or damaged fittings.", 90, 10000},
	{"Carpentry", "Shelf installation", "Fit and secure shelves after inspecting the mounting surface.", 120, 12500},
	{"Carpentry", "Furniture repair", "Assess and repair loose wooden furniture joints.", 120, 13000},
	{"Carpentry", "Worktop fitting", "Measure and fit a replacement kitchen worktop.", 240, 24000},
	{"Carpentry", "Timber damage assessment", "Assess damaged timber and prepare a tailored repair plan.", 90, 11500},
	{"Electrical assessment", "Socket fault assessment", "Assess a faulty socket and propose work appropriate to the fault.", 60, 8500},
	{"Electrical assessment", "Lighting fault assessment", "Investigate a flickering or failed light circuit.", 90, 11000},
	{"Electrical assessment", "Circuit breaker assessment", "Investigate recurring circuit breaker trips.", 120, 14500},
	{"Electrical assessment", "Wiring safety assessment", "Inspect reported wiring faults and prepare a repair plan.", 120, 16000},
	{"Electrical assessment", "Outdoor power assessment", "Assess a fault affecting outdoor electrical power.", 90, 12500},
	{"Electrical assessment", "Appliance circuit assessment", "Investigate a fault affecting an appliance supply circuit.", 120, 15000},
}

func identity(role string, n int) (id, subject, email, name string) {
	label := fmt.Sprintf("%s%d", role, n)
	return "acct_seed_" + label, "auth0|relay-seed-" + label, label + "@relay.com", fmt.Sprintf("%s %d", strings.ToUpper(role[:1])+role[1:], n)
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

func prepare(issuer, password, output string) (manifest, error) {
	issuer = strings.TrimRight(issuer, "/") + "/"
	if !strings.HasPrefix(issuer, "https://") || password == "" {
		return manifest{}, errors.New("AUTH0_ISSUER_URL and SEED_PASSWORD are required")
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		return manifest{}, err
	}
	path := filepath.Join(output, "manifest.json")
	var m manifest
	data, err := os.ReadFile(path)
	if err == nil {
		if json.Unmarshal(data, &m) != nil || m.Version != version || m.Issuer != issuer || m.Anchor.IsZero() {
			return m, errors.New("seed manifest does not match this tenant or dataset")
		}
		if bcrypt.CompareHashAndPassword([]byte(m.PasswordHash), []byte(password)) != nil {
			return m, errors.New("SEED_PASSWORD differs from the prepared Auth0 import; existing users are never reset")
		}
	} else if os.IsNotExist(err) {
		// These fixtures deliberately share one user-specified test password. Hash
		// it once at Auth0's supported bcrypt cost instead of hashing it 10,005 times.
		hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
		if err != nil {
			return m, errors.New("could not hash SEED_PASSWORD")
		}
		m = manifest{Version: version, Issuer: issuer, Anchor: time.Now().UTC().Truncate(24 * time.Hour), PasswordHash: string(hash)}
	} else {
		return m, err
	}
	var batch []authUser
	size := 2
	flush := func() error {
		file := fmt.Sprintf("auth0-users-%03d.json", len(m.Files)+1)
		if err := writeJSON(filepath.Join(output, file), batch); err != nil {
			return err
		}
		m.Files = append(m.Files, file)
		batch = nil
		size = 2
		return nil
	}
	m.Files = nil
	for _, group := range []struct {
		role  string
		count int
	}{{"customer", Customers}, {"professional", Professionals}} {
		for n := 1; n <= group.count; n++ {
			_, subject, email, name := identity(group.role, n)
			u := authUser{ID: strings.TrimPrefix(subject, "auth0|"), Email: email, Name: name, EmailVerified: true, PasswordHash: m.PasswordHash, Metadata: map[string]string{"relay_seed": version}}
			encoded, _ := json.MarshalIndent(u, "  ", "  ")
			if size+len(encoded)+4 > 400000 && len(batch) > 0 {
				if err := flush(); err != nil {
					return m, err
				}
			}
			batch = append(batch, u)
			size += len(encoded) + 4
		}
	}
	if len(batch) > 0 {
		if err := flush(); err != nil {
			return m, err
		}
	}
	return m, writeJSON(path, m)
}

// Run uses COPY into temporary staging tables, then inserts the dataset in one
// transaction. Stable OIDC subjects and IDs make reruns preserve edited profiles,
// memberships, preferences, booked slots and all unrelated accounts.
func Run(ctx context.Context, databaseURL, issuer, password, output string) error {
	m, err := prepare(issuer, password, output)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	defer pool.Close()
	rng := rand.New(rand.NewSource(20261012))
	var accounts, profiles, slots [][]any
	for _, group := range []struct {
		role  string
		count int
	}{{"customer", Customers}, {"professional", Professionals}} {
		for n := 1; n <= group.count; n++ {
			id, subject, email, name := identity(group.role, n)
			accounts = append(accounts, []any{id, m.Issuer, subject, email, name, group.role})
			if group.role == "customer" {
				continue
			}
			s := services[rng.Intn(len(services))]
			areas := []string{"Amsterdam", "Utrecht", "Rotterdam", "The Hague", "Eindhoven"}
			coords := [][2]float64{{52.37, 4.90}, {52.09, 5.12}, {51.92, 4.48}, {52.07, 4.30}, {51.44, 5.48}}
			area := rng.Intn(len(areas))
			price := s.basePrice/2 + rng.Intn(s.basePrice+1)
			price = (price / 100) * 100
			callout := rng.Intn(6) * 1000
			profiles = append(profiles, []any{fmt.Sprintf("prof_seed_%d", n), subject, name, s.category, s.name, s.description, areas[area], coords[area][0] + (rng.Float64()-.5)*.1, coords[area][1] + (rng.Float64()-.5)*.1, "EUR", int64(price), int64(callout), s.minutes, "Materials quoted separately after customer approval."})
			count := 4 + rng.Intn(5)
			days := rng.Perm(30)
			for j := 0; j < count; j++ {
				start := m.Anchor.AddDate(0, 0, days[j]+1).Add(time.Duration(8+rng.Intn(9))*time.Hour + time.Duration(rng.Intn(4)*15)*time.Minute)
				slots = append(slots, []any{fmt.Sprintf("slot_seed_%d_%d", n, j+1), subject, start, start.Add(time.Duration(s.minutes) * time.Minute), "Europe/Amsterdam"})
			}
		}
	}
	start := time.Now()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('relay_local_seed_v1',0));
CREATE TEMP TABLE seed_accounts (id text,issuer text,subject text,email text,display_name text,preferred_role text) ON COMMIT DROP;
CREATE TEMP TABLE seed_profiles (id text,subject text,display_name text,category text,service_name text,service_description text,service_area text,latitude double precision,longitude double precision,currency text,price_minor bigint,callout_minor bigint,duration_minutes integer,materials_policy text) ON COMMIT DROP;
CREATE TEMP TABLE seed_slots (id text,subject text,starts_at timestamptz,ends_at timestamptz,timezone text) ON COMMIT DROP;`); err != nil {
		return err
	}
	for _, table := range []struct {
		name    string
		columns []string
		rows    [][]any
	}{
		{"seed_accounts", []string{"id", "issuer", "subject", "email", "display_name", "preferred_role"}, accounts},
		{"seed_profiles", []string{"id", "subject", "display_name", "category", "service_name", "service_description", "service_area", "latitude", "longitude", "currency", "price_minor", "callout_minor", "duration_minutes", "materials_policy"}, profiles},
		{"seed_slots", []string{"id", "subject", "starts_at", "ends_at", "timezone"}, slots},
	} {
		if _, err = tx.CopyFrom(ctx, pgx.Identifier{table.name}, table.columns, pgx.CopyFromRows(table.rows)); err != nil {
			return err
		}
	}
	statements := []string{
		`INSERT INTO accounts (id,issuer,subject,email,display_name,preferred_role) SELECT * FROM seed_accounts ON CONFLICT (issuer,subject) DO NOTHING`,
		`INSERT INTO account_memberships (account_id,role,status) SELECT a.id,'customer','active' FROM accounts a JOIN seed_accounts s USING(issuer,subject) ON CONFLICT DO NOTHING`,
		`INSERT INTO account_memberships (account_id,role,status) SELECT a.id,'professional','active' FROM accounts a JOIN seed_accounts s USING(issuer,subject) WHERE s.preferred_role='professional' ON CONFLICT DO NOTHING`,
		`INSERT INTO professional_profiles (id,account_id,display_name,category,service_name,service_description,service_area,latitude,longitude,currency,price_minor,callout_minor,duration_minutes,materials_policy,status)
SELECT p.id,a.id,p.display_name,p.category,p.service_name,p.service_description,p.service_area,p.latitude,p.longitude,p.currency,p.price_minor,p.callout_minor,p.duration_minutes,p.materials_policy,'active'
FROM seed_profiles p JOIN seed_accounts s USING(subject) JOIN accounts a USING(issuer,subject) ON CONFLICT (account_id) DO NOTHING`,
		`INSERT INTO professional_availability_slots (id,professional_account_id,starts_at,ends_at,timezone)
SELECT s.id,a.id,s.starts_at,s.ends_at,s.timezone FROM seed_slots s JOIN seed_accounts t USING(subject) JOIN accounts a USING(issuer,subject) ON CONFLICT DO NOTHING`,
	}
	counts := map[string]int64{}
	for i, sql := range statements {
		tag, err := tx.Exec(ctx, sql)
		if err != nil {
			return err
		}
		counts[[]string{"accounts_inserted", "customer_memberships_inserted", "professional_memberships_inserted", "profiles_inserted", "slots_inserted"}[i]] = tag.RowsAffected()
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	// Fresh statistics prevent a benchmark of stale pre-seed query plans.
	for _, table := range []string{"accounts", "account_memberships", "professional_profiles", "professional_availability_slots"} {
		if _, err = pool.Exec(ctx, "ANALYZE "+table); err != nil {
			return err
		}
	}
	report := map[string]any{"dataset": version, "customers": Customers, "professionals": Professionals, "generated_slots": len(slots), "inserted": counts, "database_ms": float64(time.Since(start).Microseconds()) / 1000, "auth0_import_files": len(m.Files), "auth0_status": "prepared; import required before password login", "availability_anchor": m.Anchor}
	if err = writeJSON(filepath.Join(output, "seed-report.json"), report); err != nil {
		return err
	}
	encoded, _ := json.Marshal(report)
	fmt.Println(string(encoded))
	return nil
}
