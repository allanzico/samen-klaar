package postgres

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func adminOperatorFixture(t *testing.T, ctx context.Context, s *Store) string {
	t.Helper()
	id := accountFixture(t, ctx, s)
	if _, err := s.pool.Exec(ctx, `INSERT INTO account_memberships(account_id,role,status) VALUES($1,'operator','active')`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOperatorAdministrationSuspendsRestoresAndAuditsAccess(t *testing.T) {
	ctx, s := accountTestStore(t)
	op := adminOperatorFixture(t, ctx, s)
	user := accountFixture(t, ctx, s)
	if err := s.CreateSession(ctx, "admin-test-"+user, user, identity.Customer, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	input := workflow.AdminUserAction{Action: "suspend-role", Role: "customer", ExpectedStatus: "active", Reason: "Investigating a reported account issue."}
	if err := s.AdministerUser(ctx, user, user, input); !errors.Is(err, workflow.ErrAdminForbidden) {
		t.Fatalf("customer action: %v", err)
	}
	if _, err := s.OperatorUsers(ctx, user, workflow.DirectoryFilter{}); !errors.Is(err, workflow.ErrAdminForbidden) {
		t.Fatalf("customer directory: %v", err)
	}
	if err := s.AdministerUser(ctx, op, user, input); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, "admin-test-"+user); err == nil {
		t.Fatal("suspended session remained usable")
	}
	// Authentication and onboarding must not recreate a disabled grant.
	membership, err := s.EnsureMembership(ctx, user, identity.Customer)
	if err != nil || membership.Status != "disabled" {
		t.Fatalf("login restored suspended access: %+v %v", membership, err)
	}
	if err := s.AdministerUser(ctx, op, user, input); !errors.Is(err, workflow.ErrAdminConflict) {
		t.Fatalf("stale write: %v", err)
	}
	detail, err := s.OperatorUser(ctx, op, user)
	if err != nil || len(detail.Events) != 1 || detail.Events[0].ActorID != op || detail.Events[0].AffectedSessions != 1 || !detail.User.Memberships[0].Suspended {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	input.Action, input.ExpectedStatus = "restore-role", "disabled"
	if err = s.AdministerUser(ctx, op, user, input); err != nil {
		t.Fatal(err)
	}
	membership, _ = s.EnsureMembership(ctx, user, identity.Customer)
	if membership.Status != "active" {
		t.Fatal("previous access was not restored")
	}
	input = workflow.AdminUserAction{Action: "revoke-sessions", Reason: "Customer requested signing out every device."}
	if err = s.CreateSession(ctx, "admin-test-2-"+user, user, identity.Customer, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = s.AdministerUser(ctx, op, user, input); err != nil {
		t.Fatal(err)
	}
	detail, _ = s.OperatorUser(ctx, op, user)
	if len(detail.Events) != 3 || detail.User.ActiveSessions != 0 || detail.Events[0].Reason != input.Reason {
		t.Fatalf("audit or sessions: %+v", detail)
	}
	if err = s.AdministerUser(ctx, op, op, input); !errors.Is(err, workflow.ErrAdminProtected) {
		t.Fatalf("self admin operation: %v", err)
	}
	otherOperator := adminOperatorFixture(t, ctx, s)
	if err = s.AdministerUser(ctx, op, otherOperator, input); !errors.Is(err, workflow.ErrAdminProtected) {
		t.Fatalf("operator sessions changed: %v", err)
	}
	if _, err = s.OperatorUser(ctx, op, "missing-account"); !errors.Is(err, workflow.ErrAdminNotFound) {
		t.Fatalf("missing account: %v", err)
	}
}

func TestOperatorDirectoriesFilterMembershipsCoverageAndPrivateData(t *testing.T) {
	ctx, s := accountTestStore(t)
	op := adminOperatorFixture(t, ctx, s)
	pro := accountFixture(t, ctx, s)
	if _, err := s.EnsureMembership(ctx, pro, identity.Professional); err != nil {
		t.Fatal(err)
	}
	city := "Admin-" + rand.Text()
	base := &workflow.Location{ID: "mapbox-admin", Source: "mapbox", Kind: "address", Label: "1012 LG " + city, City: city, Postcode: "1012 LG", Country: "nl", Address: "Private base street 99", Latitude: 52.37, Longitude: 4.9}
	profile, err := s.SaveProfessionalProfile(ctx, pro, workflow.ProfessionalProfile{DisplayName: "Directory professional", Category: "Plumbing", ServiceName: "Sink leak repair", ServiceArea: base.Label, BaseLocation: base, TravelRadiusKM: 25, Currency: "EUR", PriceMinor: 9000, DurationMinutes: 60, MaterialsPolicy: "Agreed separately", Exclusions: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE professional_profiles SET status='active' WHERE id=$1`, profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE account_memberships SET status='active' WHERE account_id=$1 AND role='professional'`, pro); err != nil {
		t.Fatal(err)
	}
	customer := accountFixture(t, ctx, s)
	if _, err = s.pool.Exec(ctx, `UPDATE accounts SET email=$1,display_name=$2 WHERE id=ANY($3::text[])`, city+"@test.invalid", city, []string{pro, customer}); err != nil {
		t.Fatal(err)
	}
	state, err := s.CreateWorkspace(ctx, customer)
	if err != nil {
		t.Fatal(err)
	}
	state.Intake = &workflow.Intake{Step: "waitlist", Waitlisted: true, Location: base, ServiceAddress: "Customer private street 22"}
	raw, _ := json.Marshal(state)
	if _, err = s.pool.Exec(ctx, `UPDATE account_workspaces SET state_json=$2 WHERE id=$1`, state.WorkspaceID, raw); err != nil {
		t.Fatal(err)
	}
	filter := workflow.DirectoryFilter{Query: city, Role: "professional", Status: "active", Location: base.Label}
	page, err := s.OperatorUsers(ctx, op, filter)
	if err != nil || page.Total != 1 || len(page.Users) != 1 || page.Users[0].SameEmailCount != 2 {
		t.Fatalf("users=%+v err=%v", page, err)
	}
	serialized, _ := json.Marshal(page)
	if strings.Contains(string(serialized), "Private base street") || strings.Contains(string(serialized), "latitude") || strings.Contains(string(serialized), "Customer private") {
		t.Fatal("directory leaked private location data")
	}
	locations, err := s.OperatorLocations(ctx, op, city, "available", 0)
	if err != nil || locations.Total != 1 || locations.Locations[0].ActiveProfessionals != 1 || locations.Locations[0].WaitlistedRequests != 1 {
		t.Fatalf("locations=%+v err=%v", locations, err)
	}
	if err = s.AdministerUser(ctx, op, pro, workflow.AdminUserAction{Action: "suspend-role", Role: "professional", ExpectedStatus: "active", Reason: "Coverage test suspension for operator QA."}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CatalogOffer(ctx, profile.ID); err == nil {
		t.Fatal("suspended provider remained bookable")
	}
	// A customer's active role must not satisfy a professional-status filter.
	page, err = s.OperatorUsers(ctx, op, filter)
	if err != nil || page.Total != 0 {
		t.Fatalf("cross-role status leak: %+v %v", page, err)
	}
	locations, err = s.OperatorLocations(ctx, op, city, "waitlisted", 0)
	if err != nil || len(locations.Locations) != 1 || locations.Locations[0].ActiveProfessionals != 0 {
		t.Fatalf("suspended coverage: %+v %v", locations, err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	job := workflow.Job{ID: "cached-agreement-" + pro, ProfessionalAccountID: pro, Appointment: &workflow.Appointment{StartUTC: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339), EndUTC: time.Now().Add(49 * time.Hour).UTC().Format(time.RFC3339)}}
	if err = reserveAgreedDate(ctx, tx, &job); err == nil || !strings.Contains(err.Error(), "suspended") {
		t.Fatal("cached appointment could bypass suspension")
	}
	_ = tx.Rollback(ctx)
	profile.ServiceDescription = "Changed scope requires a fresh professional review."
	if _, err = s.SaveProfessionalProfile(ctx, pro, profile); err != nil {
		t.Fatal(err)
	}
	membership, _ := s.EnsureMembership(ctx, pro, identity.Professional)
	if membership.Status != "disabled" {
		t.Fatal("profile resubmission bypassed administrative suspension")
	}
}

func TestOperatorUserPaginationAndPendingRestore(t *testing.T) {
	ctx, s := accountTestStore(t)
	op := adminOperatorFixture(t, ctx, s)
	label := "Pagination-" + rand.Text()
	for i := 0; i < workflow.DirectoryPageSize+2; i++ {
		id := accountFixture(t, ctx, s)
		if _, err := s.pool.Exec(ctx, `UPDATE accounts SET display_name=$2 WHERE id=$1`, id, label); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.OperatorUsers(ctx, op, workflow.DirectoryFilter{Query: label})
	if err != nil || len(first.Users) != 25 || first.Total != 27 || !first.HasMore {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := s.OperatorUsers(ctx, op, workflow.DirectoryFilter{Query: label, Offset: 25})
	if err != nil || len(second.Users) != 2 || second.HasMore {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	seen := map[string]bool{}
	for _, user := range append(first.Users, second.Users...) {
		if seen[user.ID] {
			t.Fatal("duplicate account across pages")
		}
		seen[user.ID] = true
	}
	id := second.Users[0].ID
	if _, err = s.EnsureMembership(ctx, id, identity.Professional); err != nil {
		t.Fatal(err)
	}
	input := workflow.AdminUserAction{Action: "suspend-role", Role: "professional", ExpectedStatus: "pending", Reason: "Temporarily pause an unreviewed application."}
	if err = s.AdministerUser(ctx, op, id, input); err != nil {
		t.Fatal(err)
	}
	input.Action, input.ExpectedStatus = "restore-role", "disabled"
	if err = s.AdministerUser(ctx, op, id, input); err != nil {
		t.Fatal(err)
	}
	membership, _ := s.EnsureMembership(ctx, id, identity.Professional)
	if membership.Status != "pending" {
		t.Fatal("restoration granted professional approval")
	}
	if _, err = s.pool.Exec(ctx, `UPDATE account_memberships SET status='disabled' WHERE account_id=$1 AND role='professional'`, id); err != nil {
		t.Fatal(err)
	}
	if err = s.AdministerUser(ctx, op, id, input); !errors.Is(err, workflow.ErrAdminUnrestorable) {
		t.Fatalf("restored an unrelated disabled grant: %v", err)
	}
}
