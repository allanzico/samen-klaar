package seed

import (
	"context"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTestProfessionalsAreDistinctAndHaveFutureTimes(t *testing.T) {
	if len(testPros) != 10 {
		t.Fatalf("expected ten test professionals, got %d", len(testPros))
	}
	zone, _ := time.LoadLocation("Europe/Amsterdam")
	anchor := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	emails, trades, noTimes, paused := map[string]bool{}, map[string]bool{}, 0, 0
	for _, p := range testPros {
		email := p.slug + "@" + prosDomain
		if emails[email] || !strings.HasSuffix(email, ".test") {
			t.Fatalf("duplicate or non-test address %s", email)
		}
		emails[email] = true
		trades[p.category] = true
		if len(p.week) == 0 {
			noTimes++
		}
		if !p.mandate.Enabled {
			paused++
		}
		slots, err := workflow.GenerateProfessionalAvailability(p.schedule(), anchor, p.daysOff(anchor, zone), nil)
		if err != nil {
			t.Fatalf("%s schedule: %v", p.slug, err)
		}
		for _, slot := range slots {
			start, err := time.Parse(time.RFC3339, slot.StartUTC)
			if err != nil {
				t.Fatal(err)
			}
			local := start.In(zone)
			if p.slug == "mike-dawson" && local.Weekday() != time.Saturday && local.Weekday() != time.Sunday {
				t.Fatal("weekend professional has a weekday free time")
			}
			if p.slug == "sanne-de-vries" && local.Hour() < 17 {
				t.Fatal("evening professional has a daytime free time")
			}
			if p.daysOffAhead > 0 && local.Format("2006-01-02") < anchor.In(zone).AddDate(0, 0, p.daysOffAhead).Format("2006-01-02") {
				t.Fatal("a free time was generated on a seeded day off")
			}
		}
		if err := p.mandate.Normalize("professional"); err != nil {
			t.Errorf("%s: %v", p.slug, err)
		}
	}
	if len(trades) != 3 || noTimes != 1 || paused != 1 {
		t.Fatalf("the set should cover three trades, one professional with no times and one with a paused assistant: %v %d %d", trades, noTimes, paused)
	}
}

func TestSeedProfessionalsUseWeeklyHoursAndPreserveEditedSchedules(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("run make check-flow for PostgreSQL-backed seed verification")
	}
	ctx := context.Background()
	if err := postgres.Migrate(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	issuer := "https://seed-" + strings.ToLower(rand.Text()) + ".example.test/"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM accounts WHERE issuer=$1`, issuer)
	})
	output := t.TempDir()
	if err := RunPros(ctx, databaseURL, issuer, "disposable-seed-test", output); err != nil {
		t.Fatal(err)
	}
	var schedules, manual, generated, past int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM professional_schedules s JOIN accounts a ON a.id=s.professional_account_id WHERE a.issuer=$1`, issuer).Scan(&schedules); err != nil || schedules != 10 {
		t.Fatalf("missing weekly schedules: %d %v", schedules, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE s.source='manual'),count(*) FILTER(WHERE s.source='schedule'),count(*) FILTER(WHERE s.starts_at<=now()) FROM professional_availability_slots s JOIN accounts a ON a.id=s.professional_account_id WHERE a.issuer=$1`, issuer).Scan(&manual, &generated, &past); err != nil || manual != 0 || generated == 0 || past != 0 {
		t.Fatalf("seed did not generate future weekly times: manual=%d generated=%d past=%d err=%v", manual, generated, past, err)
	}
	for _, query := range []string{
		`UPDATE professional_working_hours SET starts_local='12:00',ends_local='14:00' WHERE professional_account_id='acct_pro_jane-price'`,
		`UPDATE professional_schedules SET gap_minutes=45 WHERE professional_account_id='acct_pro_jane-price'`,
		`UPDATE professional_profiles SET display_name='Edited name' WHERE account_id='acct_pro_jane-price'`,
	} {
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunPros(ctx, databaseURL, issuer, "disposable-seed-test", output); err != nil {
		t.Fatal(err)
	}
	var preserved bool
	if err := pool.QueryRow(ctx, `SELECT p.display_name='Edited name' AND s.gap_minutes=45 AND (SELECT count(*) FROM professional_working_hours h WHERE h.professional_account_id=p.account_id AND h.starts_local='12:00'::time AND h.ends_local='14:00'::time)=5 FROM professional_profiles p JOIN professional_schedules s ON s.professional_account_id=p.account_id WHERE p.account_id='acct_pro_jane-price'`).Scan(&preserved); err != nil || !preserved {
		t.Fatalf("repeat seed reset the professional's choices: %v", err)
	}
}
