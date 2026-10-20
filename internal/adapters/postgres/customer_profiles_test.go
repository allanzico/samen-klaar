package postgres

import (
	"testing"

	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestCustomerProfilePersistsAndLoginKeepsEditedName(t *testing.T) {
	ctx, store := accountTestStore(t)
	user, err := store.UpsertIdentity(ctx, "profile-test", t.Name(), "profile@example.test", "Identity name")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = store.pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, user.ID) })
	location := &workflow.Location{ID: "saved-place", Label: "Amsterdam", Kind: "place", Latitude: 52.37, Longitude: 4.9, Selection: "short-lived-selection"}
	want := workflow.CustomerProfile{DisplayName: "Chosen name", Phone: "+31 6 12345678", Location: location}
	if _, err = store.SaveCustomerProfile(ctx, user.ID, want); err != nil {
		t.Fatal(err)
	}
	user, err = store.UpsertIdentity(ctx, "profile-test", t.Name(), "profile@example.test", "Identity name")
	if err != nil || user.DisplayName != "Chosen name" {
		t.Fatalf("login overwrote chosen name: %+v %v", user, err)
	}
	profile, err := store.CustomerProfile(ctx, user.ID)
	if err != nil || profile.Phone != want.Phone || profile.Location == nil || profile.Location.ID != location.ID || profile.Location.Selection != "" {
		t.Fatalf("profile did not round-trip privately: %+v %v", profile, err)
	}
	other := accountFixture(t, ctx, store)
	otherProfile, err := store.CustomerProfile(ctx, other)
	if err != nil || otherProfile.Phone != "" || otherProfile.Location != nil {
		t.Fatal("another account received the profile")
	}
}
