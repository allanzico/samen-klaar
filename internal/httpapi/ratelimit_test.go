package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterEnforcesBurstAndRefills(t *testing.T) {
	limiter := newRateLimiter()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	if !limiter.allow("customer-a", 2, 2, now) || !limiter.allow("customer-a", 2, 2, now) {
		t.Fatal("initial burst was not available")
	}
	if limiter.allow("customer-a", 2, 2, now) {
		t.Fatal("request beyond the burst was allowed")
	}
	if !limiter.allow("customer-a", 2, 2, now.Add(500*time.Millisecond)) {
		t.Fatal("token was not restored at the configured rate")
	}
}

func TestRateLimiterKeepsAccountsIndependent(t *testing.T) {
	limiter := newRateLimiter()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	if !limiter.allow("customer-a", 1, 1, now) || !limiter.allow("customer-b", 1, 1, now) {
		t.Fatal("one account consumed another account's allowance")
	}
}

func TestClientIPUsesValidatedGatewayHeaderAndFallsBackToPeer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.8:43120"
	r.Header.Set("X-Real-IP", "2001:db8::5")
	if got := clientIP(r); got != "2001:db8::5" {
		t.Fatalf("clientIP() = %q, want normalized IPv6 address", got)
	}
	r.Header.Set("X-Real-IP", "not-an-ip")
	if got := clientIP(r); got != "192.0.2.8" {
		t.Fatalf("clientIP() with invalid gateway header = %q, want peer address", got)
	}
}

func TestClientIPLimitSeparatesClientsAndReturnsRetryAfter(t *testing.T) {
	server := New(Dependencies{})
	server.limiter = newRateLimiter()
	called := 0
	handler := server.limitByClientIP("test", 0.1, 1, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))
	request := func(ip string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Real-IP", ip)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := request("192.0.2.1").Code; got != http.StatusNoContent {
		t.Fatalf("first client status = %d", got)
	}
	limited := request("192.0.2.1")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("repeated client status = %d, Retry-After = %q", limited.Code, limited.Header().Get("Retry-After"))
	}
	if got := request("192.0.2.2").Code; got != http.StatusNoContent {
		t.Fatalf("another client shared the first client's limit: status %d", got)
	}
	if called != 2 {
		t.Fatalf("downstream calls = %d, want 2", called)
	}
}

func TestClientIPLimitSeparatesAuthenticationRoutes(t *testing.T) {
	server := New(Dependencies{})
	server.limiter = newRateLimiter()
	called := 0
	endpoint := func(bucket string) http.Handler {
		return server.limitByClientIP(bucket, 0.1, 1, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called++
			w.WriteHeader(http.StatusNoContent)
		}))
	}
	request := func(handler http.Handler) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Real-IP", "192.0.2.1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	login := endpoint("login")
	callback := endpoint("callback")
	if request(login) != http.StatusNoContent || request(callback) != http.StatusNoContent || request(login) != http.StatusTooManyRequests {
		t.Fatal("authentication routes did not retain independent per-client limits")
	}
	if called != 2 {
		t.Fatalf("downstream calls = %d, want 2", called)
	}
}

func TestAICostActionClassification(t *testing.T) {
	for _, action := range []string{"start-intake", "revise-intake", "answer-intake", "set-location", "refresh-matches", "return-to-matches"} {
		if !actionMayCallAI(action) {
			t.Errorf("AI-backed action %q was not classified", action)
		}
	}
	for _, action := range []string{"send-message", "approve-payment", "submit-review"} {
		if actionMayCallAI(action) {
			t.Errorf("non-AI action %q was classified as AI-backed", action)
		}
	}
}
