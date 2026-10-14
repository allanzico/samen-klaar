package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/identity"
)

func TestExpiredAuthCleanupRetainsCurrentRecords(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for auth retention test")
	}
	ctx := context.Background()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	store, err := OpenDirectory(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	account := accountFixture(t, ctx, store)
	old, current := "old-session-"+account, "current-session-"+account
	if err := store.CreateSession(ctx, old, account, identity.Customer, time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(ctx, current, account, identity.Customer, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateOIDCTransaction(ctx, identity.Transaction{StateHash: "old-oidc-" + account, Nonce: "nonce", CodeVerifier: "verifier", Role: identity.Customer, ExpiresAt: time.Now().Add(-48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateOIDCTransaction(ctx, identity.Transaction{StateHash: "current-oidc-" + account, Nonce: "nonce", CodeVerifier: "verifier", Role: identity.Customer, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	removed, err := store.CleanupExpiredAuthRecords(ctx)
	if err != nil || removed < 2 {
		t.Fatalf("removed=%d, %v", removed, err)
	}
	if _, err := store.Session(ctx, old); err == nil {
		t.Fatal("expired session survived cleanup")
	}
	if _, err := store.Session(ctx, current); err != nil {
		t.Fatalf("current session removed: %v", err)
	}
	if _, err := store.ConsumeOIDCTransaction(ctx, "old-oidc-"+account); err == nil {
		t.Fatal("expired OIDC transaction survived cleanup")
	}
	if _, err := store.ConsumeOIDCTransaction(ctx, "current-oidc-"+account); err != nil {
		t.Fatalf("current OIDC transaction removed: %v", err)
	}
}
