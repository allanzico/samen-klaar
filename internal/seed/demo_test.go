package seed

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDemoCandidatesCompareTheSameServiceWithRealScheduleDifferences(t *testing.T) {
	pros := demoProfessionals()
	if len(pros) != 10 || pros[0].slug+"@"+prosDomain != DemoProfessionalEmail {
		t.Fatal("the demo must have ten candidates and one stable receiving professional")
	}
	emails, totals, conditions, schedules := map[string]bool{}, map[int64]bool{}, map[string]bool{}, map[string]bool{}
	anchor := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	zone, _ := time.LoadLocation("Europe/Amsterdam")
	for _, p := range pros {
		email := p.slug + "@" + prosDomain
		if emails[email] || email == DemoCustomerEmail || !strings.HasSuffix(email, ".test") {
			t.Fatal("demo identities must be distinct test addresses")
		}
		for _, name := range []string{p.name, p.person, p.slug} {
			if name == "" || strings.Contains(strings.ToLower(name), "demo") || strings.Contains(strings.ToLower(name), "test") {
				t.Fatal("presentation people and businesses must have normal names")
			}
		}
		emails[email], totals[p.price+p.callout], conditions[p.materials] = true, true, true
		if p.category != "Plumbing" || p.service != pros[0].service || p.description != pros[0].description || p.area != "Amsterdam" {
			t.Fatal("the comparison accidentally includes another service or market")
		}
		if !strings.Contains(p.description, "replace") || !strings.Contains(p.materials, "PVC sink trap section") || !strings.Contains(p.materials, "standard seals included") && !strings.Contains(p.materials, "standard seals and") {
			t.Fatal("each comparable offer must cover the broken trap replacement and its standard parts")
		}
		raw, _ := json.Marshal(p.week)
		schedules[string(raw)] = true
		slots, err := workflow.GenerateProfessionalAvailability(p.schedule(), anchor, p.daysOff(anchor, zone), nil)
		if err != nil || len(slots) == 0 {
			t.Fatalf("%s has no bookable future schedule: %v", p.slug, err)
		}
		future := 0
		for _, slot := range slots {
			start, err := time.Parse(time.RFC3339, slot.StartUTC)
			if err != nil || p.daysOffAhead > 0 && start.In(zone).Format("2006-01-02") < anchor.In(zone).AddDate(0, 0, p.daysOffAhead).Format("2006-01-02") {
				t.Fatal("demo dates must respect days off")
			}
			if start.After(anchor) {
				future++
			}
		}
		if future == 0 {
			t.Fatalf("%s has no future availability", p.slug)
		}
	}
	if len(totals) < 7 || len(conditions) < 7 || len(schedules) < 8 {
		t.Fatal("demo candidates do not exercise enough price, condition and schedule differences")
	}
}

func TestDemoSeedIsRepeatableAndDoesNotForgePayPalReadiness(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("run with disposable PostgreSQL")
	}
	ctx := context.Background()
	if err := postgres.Migrate(ctx, raw); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	issuer := "https://demo-" + strings.ToLower(rand.Text()) + ".example.test/"
	defer pool.Exec(ctx, `DELETE FROM accounts WHERE issuer=$1`, issuer)
	output := t.TempDir()
	if err := RunDemo(ctx, raw, issuer, "disposable-demo-test", output); err != nil {
		t.Fatal(err)
	}
	var accounts, profiles, professionalBuyer, wallets int
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM accounts WHERE issuer=$1), (SELECT count(*) FROM professional_profiles p JOIN accounts a ON a.id=p.account_id WHERE a.issuer=$1), (SELECT count(*) FROM account_memberships WHERE account_id='acct_alex-de-vries' AND role='professional'), (SELECT count(*) FROM paypal_seller_accounts p JOIN accounts a ON a.id=p.account_id WHERE a.issuer=$1)`, issuer).Scan(&accounts, &profiles, &professionalBuyer, &wallets)
	if err != nil || accounts != 11 || profiles != 10 || professionalBuyer != 0 || wallets != 0 {
		t.Fatalf("unexpected demo identities or fabricated payment setup: accounts=%d profiles=%d customerPro=%d wallets=%d err=%v", accounts, profiles, professionalBuyer, wallets, err)
	}
	store, err := postgres.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, location := range []workflow.Location{
		{Label: "Amsterdam"},
		{Label: "1071 XX, Amsterdam", Kind: "postcode", Postcode: "1071 XX", City: "Amsterdam", Country: "NL", Source: "mapbox", Latitude: 52.36, Longitude: 4.89},
	} {
		matches, err := store.ListProfessionalMatches(ctx, "Plumbing", location, demoCustomerID)
		if err != nil || len(matches) != 10 {
			t.Fatalf("the buyer cannot compare all ten candidates for %s: count=%d err=%v", location.Kind, len(matches), err)
		}
	}
	var users []authUser
	data, err := os.ReadFile(filepath.Join(output, "auth0-users-001.json"))
	if err != nil || json.Unmarshal(data, &users) != nil || len(users) != 11 {
		t.Fatal("Auth0 preparation must include the buyer and all professionals")
	}
	for _, query := range []string{
		`UPDATE professional_profiles SET price_minor=9900 WHERE account_id='acct_pro_noor-van-dijk'`,
		`UPDATE professional_working_hours SET starts_local='10:00' WHERE professional_account_id='acct_pro_noor-van-dijk'`,
	} {
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunDemo(ctx, raw, issuer, "disposable-demo-test", output); err != nil {
		t.Fatal(err)
	}
	var preserved bool
	if err := pool.QueryRow(ctx, `SELECT price_minor=9900 AND (SELECT count(*) FROM professional_working_hours WHERE professional_account_id='acct_pro_noor-van-dijk' AND starts_local='10:00'::time)=5 FROM professional_profiles WHERE account_id='acct_pro_noor-van-dijk'`).Scan(&preserved); err != nil || !preserved {
		t.Fatal("repeat seeding reset a changed profile or weekly hours")
	}
}
