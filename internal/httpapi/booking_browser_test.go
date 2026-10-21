package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
	"github.com/allanzico/samen-klaar/internal/httpapi"
	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

type bookingBrowserSession struct{ principal identity.Principal }

func (s bookingBrowserSession) Principal(*http.Request) (identity.Principal, bool) {
	return s.principal, true
}

type bookingBrowserLocations struct{}

func (bookingBrowserLocations) Search(_ context.Context, query, _ string, _ bool) ([]workflow.Location, error) {
	base := workflow.Location{ID: "browser-amsterdam", Source: "mapbox", Kind: "place", Country: "nl", City: "Amsterdam", Label: "Amsterdam", Latitude: 52.37, Longitude: 4.9, Bounds: []float64{4.7, 52.2, 5.1, 52.5}}
	if !strings.Contains(strings.ToLower(query), "damrak") {
		return []workflow.Location{base}, nil
	}
	base.ID, base.Kind, base.Postcode, base.Label, base.Address = "browser-damrak-1", "address", "1012 LG", "1012 LG Amsterdam", "Damrak 1, 1012 LG Amsterdam"
	other := base
	other.ID = "browser-damrak-10"
	other.Address = "Damrak 10, 1012 LG Amsterdam"
	return []workflow.Location{base, other}, nil
}

// TestBookingBrowser serves the real UI/API/workflow against an explicitly disposable local DB.
// It never loads .env or contacts Mapbox, PayPal, email, or any real professional.
func TestBookingBrowser(t *testing.T) {
	if os.Getenv("RUN_BOOKING_BROWSER") != "1" {
		t.Skip("opt in for a manual local browser journey; see tests/browser/README.md")
	}
	raw := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/browser_booking" {
		t.Fatal("browser harness requires a loopback database named browser_booking")
	}
	ctx := context.Background()
	if err = postgres.Migrate(ctx, raw); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pool, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	customer, err := store.UpsertIdentity(ctx, "booking-browser", "customer", "customer@example.test", "Booking customer")
	if err != nil {
		t.Fatal(err)
	}
	pro, err := store.UpsertIdentity(ctx, "booking-browser", "professional", "professional@example.test", "Fixture plumber")
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range []struct {
		id   string
		role identity.Role
	}{{customer.ID, identity.Customer}, {pro.ID, identity.Professional}} {
		if _, err = store.EnsureMembership(ctx, member.id, member.role); err != nil {
			t.Fatal(err)
		}
	}
	base := workflow.Location{ID: "browser-base", Source: "mapbox", Kind: "place", Country: "nl", City: "Amsterdam", Label: "Amsterdam", Latitude: 52.37, Longitude: 4.9}
	_, err = store.SaveProfessionalProfile(ctx, pro.ID, workflow.ProfessionalProfile{DisplayName: "Fixture plumber", Category: "Plumbing", ServiceName: "Sink leak repair", ServiceDescription: "Repair the leaking sink connection", ServiceArea: "Amsterdam", BaseLocation: &base, TravelRadiusKM: 25, Currency: "EUR", PriceMinor: 10000, DurationMinutes: 60, MaterialsPolicy: "Replacement parts need approval", Exclusions: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE professional_profiles SET status='active' WHERE account_id=$1`, pro.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, pro.ID); err != nil {
		t.Fatal(err)
	}
	schedule := workflow.ProfessionalSchedule{Timezone: "Europe/Amsterdam", GapMinutes: 30, NoticeMinutes: 60, HorizonDays: 14}
	for day := 1; day <= 7; day++ {
		schedule.Week = append(schedule.Week, workflow.WorkingRange{Weekday: day, Start: "08:00", End: "17:00"})
	}
	if _, err = store.SaveProfessionalSchedule(ctx, pro.ID, schedule); err != nil {
		t.Fatal(err)
	}
	principal := identity.Principal{User: customer, ActiveRole: identity.Customer, Memberships: []identity.Membership{{Role: identity.Customer, Status: "active"}}}
	server := httpapi.New(httpapi.Dependencies{AppName: "SamenKlaar", AppBaseURL: "http://127.0.0.1:8098", WebDirectory: "../../dist", Store: store, Sessions: bookingBrowserSession{principal}, Directory: store, Professionals: store, Agents: store, AI: journeyAI{}, Locations: bookingBrowserLocations{}})
	handler := server.Handler()
	local := &http.Server{Addr: "127.0.0.1:8098", ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/session" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"configured": true, "authenticated": true, "account": principal.User, "activeRole": principal.ActiveRole, "memberships": principal.Memberships})
			return
		}
		handler.ServeHTTP(w, r)
	})}
	defer local.Close()
	t.Log("Disposable booking UI: http://127.0.0.1:8098 — stop this test with Ctrl-C")
	if err = local.ListenAndServe(); err != http.ErrServerClosed {
		t.Fatal(err)
	}
}
