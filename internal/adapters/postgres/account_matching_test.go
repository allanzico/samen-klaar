package postgres

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func accountTestStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("run make check-flow for PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return ctx, store
}

func accountFixture(t *testing.T, ctx context.Context, s *Store) string {
	t.Helper()
	user, err := s.UpsertIdentity(ctx, "account-test", rand.Text(), "", "Test account")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.stopBackgroundLoops()
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM accounts WHERE id=$1`, user.ID)
	})
	if _, err = s.EnsureMembership(ctx, user.ID, identity.Customer); err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func TestServiceMatchesIncludesCityWideProviderForNeighborhood(t *testing.T) {
	ctx, s := accountTestStore(t)
	accountID := accountFixture(t, ctx, s)
	category := "area-test-" + rand.Text()
	if _, err := s.EnsureMembership(ctx, accountID, identity.Professional); err != nil {
		t.Fatal(err)
	}
	profile, err := s.SaveProfessionalProfile(ctx, accountID, workflow.ProfessionalProfile{
		DisplayName: "Amsterdam coverage test", Category: category, ServiceName: "Sink leak repair",
		ServiceDescription: "Inspect and repair a leaking sink trap.", ServiceArea: "Amsterdam",
		Currency: "EUR", PriceMinor: 9000, CalloutMinor: 2000, DurationMinutes: 60,
		MaterialsPolicy: "Materials quoted separately.", Exclusions: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE professional_profiles SET status='active' WHERE account_id=$1`, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, accountID); err != nil {
		t.Fatal(err)
	}

	service := matching.Service{
		Name: profile.ServiceName, Description: profile.ServiceDescription,
		Materials: profile.MaterialsPolicy, Exclusions: profile.Exclusions,
	}
	for _, area := range []string{"Amsterdam-Centrum", "Amsterdam/Centrum", "Amsterdam Centrum"} {
		matches, err := s.ServiceMatches(ctx, category, "", service, workflow.Location{Label: area})
		if err != nil {
			t.Fatalf("ServiceMatches(%q): %v", area, err)
		}
		if len(matches) != 1 || matches[0].AccountID != accountID {
			t.Fatalf("ServiceMatches(%q) = %+v, want the Amsterdam-wide profile", area, matches)
		}
	}
}

func TestPublishedPostcodeInsideAreaLabelMatchesCustomerPostcode(t *testing.T) {
	ctx, s := accountTestStore(t)
	accountID := accountFixture(t, ctx, s)
	category := "postcode-area-test-" + rand.Text()
	if _, err := s.EnsureMembership(ctx, accountID, identity.Professional); err != nil {
		t.Fatal(err)
	}
	profile, err := s.SaveProfessionalProfile(ctx, accountID, workflow.ProfessionalProfile{
		DisplayName: "Postcode coverage test", Category: category, ServiceName: "Sink leak repair",
		ServiceDescription: "Inspect and repair a leaking sink trap.", ServiceArea: "AMSTERDAM CENTRUM 1020EN",
		Currency: "EUR", PriceMinor: 9000, CalloutMinor: 2000, DurationMinutes: 60,
		MaterialsPolicy: "Materials quoted separately.", Exclusions: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE professional_profiles SET status='active' WHERE account_id=$1`, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, accountID); err != nil {
		t.Fatal(err)
	}

	location := workflow.Location{Label: "1020EN"}
	services, err := s.ServiceCatalog(ctx, category, "leaking sink", "", location)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0].Name != profile.ServiceName {
		t.Fatalf("ServiceCatalog(1020EN) = %+v, want the published service", services)
	}
	matches, err := s.ServiceMatches(ctx, category, "", services[0], location)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].AccountID != accountID {
		t.Fatalf("ServiceMatches(1020EN) = %+v, want the professional", matches)
	}
}

func TestServiceCatalogFiltersAreaBeforeVariantLimit(t *testing.T) {
	ctx, s := accountTestStore(t)
	prefix := "catalog-area-" + rand.Text()
	category := prefix + "-trade"
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), `DELETE FROM accounts WHERE id LIKE $1`, prefix+"%") })
	if _, err := s.pool.Exec(ctx, `INSERT INTO accounts(id,issuer,subject)
		SELECT $1||'-acct-'||g::text,'test',$1||'-acct-'||g::text FROM generate_series(1,202) g`, prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO account_memberships(account_id,role,status)
		SELECT $1||'-acct-'||g::text,'professional','active' FROM generate_series(1,202) g`, prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO professional_profiles(id,account_id,display_name,category,service_name,service_description,service_area,currency,price_minor,callout_minor,duration_minutes,materials_policy,status)
		SELECT $1||'-prof-'||g::text,$1||'-acct-'||g::text,'Area provider',$2,
		CASE WHEN g=202 THEN 'Local sink repair' ELSE 'Sink repair variant '||g::text END,
		'Sink repair',CASE WHEN g=202 THEN 'Amsterdam West' ELSE 'Amsterdam Noord' END,
		'EUR',10000,0,60,'Parts agreed separately','active' FROM generate_series(1,202) g`, prefix, category); err != nil {
		t.Fatal(err)
	}
	services, err := s.ServiceCatalog(ctx, category, "sink repair", "", workflow.Location{Label: "Amsterdam West"})
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0].Name != "Local sink repair" {
		t.Fatalf("local service omitted after catalog limit: %+v", services)
	}
}

func TestSqlAreaCandidateFilterCoversGoEligibility(t *testing.T) {
	ctx, store := accountTestStore(t)
	cases := []struct{ provider, requested string }{
		{"Amsterdam", "Amsterdam West"}, {"Amsterdam-Centrum", "Amsterdam/Centrum"},
		{"Amsterdam West", "Amsterdam West"}, {"1015", "1015 Amsterdam"},
		{"AMSTERDAM CENTRUM 1020EN", "1020EN"},
		{"Amsterdam Noord", "Amsterdam West"},
	}
	for _, item := range cases {
		var candidate bool
		query := `SELECT ` + areaCoverageSQL("$1") + ` FROM (VALUES ($2::text)) AS p(service_area)`
		if err := store.pool.QueryRow(ctx, query, item.requested, item.provider).Scan(&candidate); err != nil {
			t.Fatal(err)
		}
		if serviceAreaCovers(item.provider, item.requested) && !candidate {
			t.Fatalf("SQL omitted Go-eligible area %q -> %q", item.provider, item.requested)
		}
	}
}

func TestAccountPreferenceSurvivesNewSessionsAndRoleEscalationIsDenied(t *testing.T) {
	ctx, s := accountTestStore(t)
	id := accountFixture(t, ctx, s)
	token := rand.Text()
	if err := s.CreateSession(ctx, token, id, identity.Customer, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivateRole(ctx, token, identity.Professional); err == nil {
		t.Fatal("customer acquired professional access without onboarding")
	}
	if _, err := s.EnsureMembership(ctx, id, identity.Professional); err != nil {
		t.Fatal(err)
	}
	principal, err := s.ActivateRole(ctx, token, identity.Professional)
	if err != nil {
		t.Fatal(err)
	}
	customer, ok := principal.Membership(identity.Customer)
	professional, hasProfessional := principal.Membership(identity.Professional)
	if !ok || customer.Status != "active" || !hasProfessional || professional.Status != "pending" {
		t.Fatal("one account did not retain customer access during onboarding")
	}
	if err := s.DeleteSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	preferred, err := s.PreferredRole(ctx, id)
	if err != nil || preferred != identity.Professional {
		t.Fatal("sign-out discarded professional preference")
	}
	// This is the role the unified callback supplies to its new session.
	token = rand.Text()
	if err := s.CreateSession(ctx, token, id, preferred, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	principal, err = s.Session(ctx, token)
	if err != nil || principal.ActiveRole != identity.Professional || principal.User.ID != id {
		t.Fatal("new session did not restore the same professional identity")
	}
	if _, err := s.ActivateRole(ctx, token, identity.Operator); err == nil {
		t.Fatal("self-service role switch granted operator access")
	}
	if _, err := s.ActivateRole(ctx, token, identity.Customer); err != nil {
		t.Fatal(err)
	}
	if preferred, err = s.PreferredRole(ctx, id); err != nil || preferred != identity.Customer {
		t.Fatal("switching back was not persisted")
	}
	if _, err := s.ActivateRole(ctx, token, identity.Professional); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE account_memberships SET status='disabled' WHERE account_id=$1 AND role='professional'`, id); err != nil {
		t.Fatal(err)
	}
	if preferred, err = s.PreferredRole(ctx, id); err != nil || preferred != identity.Customer {
		t.Fatal("login would restore revoked professional access")
	}
	if _, err := s.ActivateRole(ctx, token, identity.Professional); err == nil {
		t.Fatal("revoked membership could be reactivated")
	}
}

func TestEarliestMatchingAndAtomicApprovalExcludeOwnServices(t *testing.T) {
	ctx, s := accountTestStore(t)
	category := "matching-test-" + rand.Text()
	self := accountFixture(t, ctx, s)
	earlyAccount := accountFixture(t, ctx, s)
	laterAccount := accountFixture(t, ctx, s)
	noTimeAccount := accountFixture(t, ctx, s)
	profiles := map[string]workflow.ProfessionalProfile{}
	for id, name := range map[string]string{self: "Own service", earlyAccount: "Z early professional", laterAccount: "A later professional", noTimeAccount: "No published time"} {
		if _, err := s.EnsureMembership(ctx, id, identity.Professional); err != nil {
			t.Fatal(err)
		}
		profile, err := s.SaveProfessionalProfile(ctx, id, workflow.ProfessionalProfile{DisplayName: name, Category: category, ServiceName: "Service visit", Currency: "EUR", PriceMinor: 11000, CalloutMinor: 2000, DurationMinutes: 60, MaterialsPolicy: "Agree replacement parts"})
		if err != nil {
			t.Fatal(err)
		}
		profiles[id] = profile
		if _, err = s.pool.Exec(ctx, `UPDATE professional_profiles SET status='active' WHERE account_id=$1`, id); err != nil {
			t.Fatal(err)
		}
		if _, err = s.pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, id); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	addSlot := func(id string, at time.Time, duration time.Duration) string {
		t.Helper()
		slotID, err := s.AddProfessionalAvailability(ctx, id, workflow.Appointment{StartUTC: at.Format(time.RFC3339), EndUTC: at.Add(duration).Format(time.RFC3339), Timezone: "Europe/Amsterdam"})
		if err != nil {
			t.Fatal(err)
		}
		return slotID
	}
	addSlot(self, start.Add(-24*time.Hour), time.Hour)
	short := addSlot(earlyAccount, start.Add(-12*time.Hour), 15*time.Minute)
	blocked := addSlot(earlyAccount, start.Add(-6*time.Hour), time.Hour)
	earlySlot := addSlot(earlyAccount, start, time.Hour)
	addSlot(laterAccount, start.Add(24*time.Hour), time.Hour)
	blockerID := rand.Text()
	if _, err := s.pool.Exec(ctx, `INSERT INTO booking_reservations(job_id,professional_id,slot_id,starts_at,ends_at,timezone,status) VALUES($1,$2,$3,$4,$5,'UTC','booked')`, blockerID, earlyAccount, blocked, start.Add(-6*time.Hour), start.Add(-5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.pool.Exec(context.Background(), `DELETE FROM booking_reservations WHERE professional_id=$1`, earlyAccount)
	})
	matches, err := s.ListProfessionalMatches(ctx, category, workflow.Location{}, self)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 3 || matches[0].AccountID != earlyAccount || matches[0].EarliestAppointment == nil || matches[0].EarliestAppointment.SlotID != earlySlot || matches[1].AccountID != laterAccount || matches[2].EarliestAppointment != nil {
		t.Fatalf("earliest matching or self exclusion failed: %+v", matches)
	}
	slots, err := s.AvailableProfessionalSlots(ctx, earlyAccount, "")
	if err != nil || len(slots) != 1 || slots[0].SlotID == short || slots[0].SlotID == blocked {
		t.Fatal("short or conflicting time was offered")
	}
	own, err := s.CreateWorkspace(ctx, self)
	if err != nil {
		t.Fatal(err)
	}
	offer, err := s.CatalogOffer(ctx, profiles[self].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyWorkspace(ctx, self, "customer", own.WorkspaceID, workflow.Action{Actor: "customer", Action: "select-match", SelectedOffer: &offer}); err == nil || !strings.Contains(err.Error(), "own services") {
		t.Fatal("database transaction allowed self selection")
	}
	// Two customers approve the same earliest visit concurrently. Exactly one
	// succeeds; the losing transaction must not leave its plan approved.
	customers := []string{self, accountFixture(t, ctx, s)}
	workspaces := make([]string, 2)
	for i, id := range customers {
		state, err := s.CreateWorkspace(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		state.Job = workflow.Job{ID: "approval-" + rand.Text(), ProfessionalAccountID: earlyAccount, ProfessionalProfileID: profiles[earlyAccount].ID, Title: "Service visit", Description: "Repair request", ScopeVersion: 1, Quote: workflow.Quote{Currency: "EUR", TotalMinor: 13000, DurationMinutes: 60, Lines: []workflow.QuoteLine{{Label: "Visit", AmountMinor: 13000}}}, Timeline: []workflow.Event{}}
		raw, _ := json.Marshal(state)
		if _, err = s.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
			t.Fatal(err)
		}
		workspaces[i] = state.WorkspaceID
	}
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range workspaces {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-barrier
			_, errs[index] = s.ApplyWorkspace(ctx, customers[index], "customer", workspaces[index], workflow.Action{Actor: "customer", Action: "approve-and-book", Value: earlySlot})
		}(i)
	}
	close(barrier)
	wg.Wait()
	successes := 0
	for i, err := range errs {
		state, loadErr := s.LoadWorkspace(ctx, customers[i], "customer", workspaces[i])
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if err == nil {
			successes++
			if !state.Job.QuoteApproved || state.Job.Appointment == nil {
				t.Fatal("successful approval did not reserve its visit")
			}
		} else if state.Job.QuoteApproved || state.Job.Appointment != nil {
			t.Fatal("failed reservation left a partial plan approval")
		}
	}
	if successes != 1 {
		t.Fatalf("same-slot approvals: got %d successes", successes)
	}
}
