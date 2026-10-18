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
		Providers:     ProviderConfig{AIProvider: "gemini", AIModel: "private-model", EvidenceProvider: "r2", PaymentProvider: "paypal_sandbox"},
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
		for _, detail := range []string{"credential", "gemini", "private-model", "r2", "paypal_sandbox"} {
			if strings.Contains(response.Body.String(), detail) {
				t.Fatalf("%s leaked implementation detail %q", path, detail)
			}
		}
	}
	check("/api/health/ready", http.StatusOK)
	config := httptest.NewRecorder()
	handler.ServeHTTP(config, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if config.Code != http.StatusOK || !strings.Contains(config.Body.String(), `"guidedReviewAvailable":true`) || !strings.Contains(config.Body.String(), `"testPayments":true`) {
		t.Fatalf("public config did not expose the required capabilities: %s", config.Body.String())
	}
	for _, detail := range []string{"gemini", "private-model", "r2", "paypal_sandbox"} {
		if strings.Contains(config.Body.String(), detail) {
			t.Fatalf("public config leaked implementation detail %q", detail)
		}
	}
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
