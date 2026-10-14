package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
	"github.com/allanzico/samen-klaar/internal/adapters/r2"
	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func TestReadinessChecksDatabaseAndDrainingWithoutBreakingLiveness(t *testing.T) {
	databaseUp := true
	checks := 0
	persistent := &postgres.Store{}
	server := New(Dependencies{
		Store: workflow.NewMemoryStore(workflow.NewState()), Auth: &identity.Auth{},
		AI: &intakeStub{}, Evidence: &r2.Store{}, Payments: &paypal.Service{},
		Professionals: persistent, Directory: persistent, Operators: persistent,
		Readiness: func(ctx context.Context) error {
			checks++
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 2*time.Second {
				t.Error("database probe must have a bounded deadline")
			}
			if !databaseUp {
				return errors.New("credential-that-must-not-be-returned")
			}
			return nil
		},
	})
	handler := server.Handler()
	check := func(path string, want int) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != want || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s: status %d, type %s", path, response.Code, response.Header().Get("Content-Type"))
		}
		if strings.Contains(response.Body.String(), "credential") {
			t.Fatal("health response leaked a database error")
		}
	}
	check("/api/health/ready", http.StatusOK)
	databaseUp = false
	check("/api/health/ready", http.StatusServiceUnavailable)
	check("/api/health/live", http.StatusOK)
	databaseUp = true
	server.BeginShutdown()
	check("/api/health/ready", http.StatusServiceUnavailable)
	check("/api/health/live", http.StatusOK)
	if checks != 2 {
		t.Fatalf("database probes = %d, want 2 (none while draining)", checks)
	}
}
