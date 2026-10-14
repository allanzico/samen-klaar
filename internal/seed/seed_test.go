package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func TestPreparedUsersHaveStableSubjectsSupportedHashesAndSmallBatches(t *testing.T) {
	output := t.TempDir()
	m, err := prepare("https://tenant.example/", "test-password-only", output)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	seen := map[string]bool{}
	for _, file := range m.Files {
		data, err := os.ReadFile(filepath.Join(output, file))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 400000 {
			t.Fatalf("batch too large: %d", len(data))
		}
		var users []authUser
		if err = json.Unmarshal(data, &users); err != nil {
			t.Fatal(err)
		}
		for _, u := range users {
			if seen[u.ID] || u.ID == "" || u.Email == "" || !u.EmailVerified {
				t.Fatal("invalid or duplicate user")
			}
			seen[u.ID] = true
			if u.PasswordHash != m.PasswordHash {
				t.Fatal("hash differs")
			}
		}
		count += len(users)
	}
	if count != Customers+Professionals {
		t.Fatalf("users=%d", count)
	}
	if !seen["relay-seed-customer1"] || !seen["relay-seed-professional10000"] {
		t.Fatal("missing boundary identities")
	}
	if err = bcrypt.CompareHashAndPassword([]byte(m.PasswordHash), []byte("test-password-only")); err != nil {
		t.Fatal(err)
	}
	again, err := prepare("https://tenant.example", "test-password-only", output)
	if err != nil {
		t.Fatal(err)
	}
	if again.PasswordHash != m.PasswordHash || !again.Anchor.Equal(m.Anchor) {
		t.Fatal("rerun changed identity hash or dates")
	}
	if _, err = prepare("https://other.example/", "test-password-only", output); err == nil {
		t.Fatal("tenant mismatch accepted")
	}
	if _, err = prepare("https://tenant.example/", "different-password", output); err == nil {
		t.Fatal("password change accepted")
	}
}

func TestSeedPreservesExistingDataAndRerunsWithoutDuplicates(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := fmt.Sprintf("samen_klaar_seed_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	dsn = parsed.String()
	if err = postgres.Migrate(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `INSERT INTO accounts (id,issuer,subject,email) VALUES ('existing','https://tenant.example/','auth0|existing','existing@example.com')`); err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	if err = Run(ctx, dsn, "https://tenant.example/", "test-password-only", output); err != nil {
		t.Fatal(err)
	}
	var accounts, profiles, memberships int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM accounts),(SELECT count(*) FROM professional_profiles),(SELECT count(*) FROM account_memberships)`).Scan(&accounts, &profiles, &memberships); err != nil {
		t.Fatal(err)
	}
	if accounts != Customers+Professionals+1 || profiles != Professionals || memberships != Customers+Professionals*2 {
		t.Fatalf("unexpected counts %d/%d/%d", accounts, profiles, memberships)
	}
	var invalidSlots int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM professional_availability_slots s JOIN professional_profiles p ON p.account_id=s.professional_account_id WHERE s.starts_at<=now() OR s.ends_at-s.starts_at<>make_interval(mins=>p.duration_minutes)`).Scan(&invalidSlots); err != nil || invalidSlots != 0 {
		t.Fatalf("invalid slots=%d err=%v", invalidSlots, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE professional_profiles SET price_minor=12345,service_name='Edited by professional' WHERE id='prof_seed_1'; UPDATE accounts SET preferred_role='customer' WHERE id='acct_seed_professional1'; UPDATE account_memberships SET status='disabled' WHERE account_id='acct_seed_professional2' AND role='professional'; UPDATE professional_availability_slots SET status='booked' WHERE id='slot_seed_1_1'`); err != nil {
		t.Fatal(err)
	}
	if err = Run(ctx, dsn, "https://tenant.example/", "test-password-only", output); err != nil {
		t.Fatal(err)
	}
	var role, status, slot, service string
	var price int64
	if err = pool.QueryRow(ctx, `SELECT a.preferred_role,p.service_name,p.price_minor FROM accounts a JOIN professional_profiles p ON p.account_id=a.id WHERE p.id='prof_seed_1'`).Scan(&role, &service, &price); err != nil {
		t.Fatal(err)
	}
	if role != "customer" || service != "Edited by professional" || price != 12345 {
		t.Fatal("rerun overwrote edits")
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM account_memberships WHERE account_id='acct_seed_professional2' AND role='professional'`).Scan(&status); err != nil || status != "disabled" {
		t.Fatal("rerun reactivated membership")
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM professional_availability_slots WHERE id='slot_seed_1_1'`).Scan(&slot); err != nil || slot != "booked" {
		t.Fatal("rerun cleared booking")
	}
	data, err := os.ReadFile(filepath.Join(output, "seed-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Inserted map[string]int64 `json:"inserted"`
	}
	if err = json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	for table, n := range report.Inserted {
		if n != 0 {
			t.Fatalf("rerun duplicated %s: %d", table, n)
		}
	}
}

func TestLatencySummary(t *testing.T) {
	if summarize(nil).Samples != 0 {
		t.Fatal("empty sample")
	}
	s := summarize([]float64{10, 5, 3, 8, 1})
	if s.Samples != 5 || s.P50 != 5 || s.P95 != 10 || s.Max != 10 {
		t.Fatalf("wrong percentiles: %+v", s)
	}
}

func TestOfferRelevanceFailsDespiteCorrectTrade(t *testing.T) {
	cases := []benchmarkCase{
		{Correct: true, Service: "Toilet repair", ExpectedServices: []string{"Sink leak repair"}},
		{Correct: true, Service: "Drain clearing", ExpectedServices: []string{"Drain clearing"}},
		{Expected: "Unclear", Correct: true},
	}
	total, correct := assessServices(cases)
	if total != 2 || correct != 1 || *cases[0].ServiceSuitable || !*cases[1].ServiceSuitable || cases[2].ServiceSuitable != nil {
		t.Fatal("unsuitable offer was accepted")
	}
}
