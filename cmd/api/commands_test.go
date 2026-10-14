package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheckRejectsFailuresAndRedirects(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusServiceUnavailable, http.StatusFound} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		err := healthcheck(server.URL)
		server.Close()
		if (err == nil) != (code == http.StatusOK) {
			t.Fatalf("HTTP %d: healthcheck error = %v", code, err)
		}
	}
}

func TestConfigRejectsMismatchedCallbackAndInsecureProduction(t *testing.T) {
	for _, key := range []string{"AI_PROVIDER_KEY", "AI_MODEL", "AUTH0_CLIENT_ID", "AUTH0_CLIENT_SECRET", "CLOUDFLARE_R2_ACCESS_KEY_ID", "CLOUDFLARE_R2_SECRET_ACCESS_KEY", "CLOUDFLARE_R2_BUCKET", "CLOUDFLARE_R2_ACCOUNT_ID", "PAYPAL_CLIENT_ID", "PAYPAL_CLIENT_SECRET", "PAYPAL_WEBHOOK_ID", "PAYPAL_PLATFORM_MERCHANT_ID", "PAYPAL_PARTNER_ATTRIBUTION_ID", "SMTP_ADDR", "SMTP_FROM"} {
		t.Setenv(key, "test-credential")
	}
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_PASSWORD", "")
	t.Setenv("AI_PROVIDER", "openai")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_BASE_URL", "https://demo.example.com")
	t.Setenv("AUTH0_REDIRECT_URI", "https://demo.example.com/api/auth/callback")
	t.Setenv("AUTH0_ISSUER_URL", "https://tenant.example.com/")
	t.Setenv("DATABASE_URL", "postgres://relay:test-password@db:5432/relay")
	t.Setenv("PAYPAL_ENV", "sandbox")
	if err := checkConfig(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PAYPAL_WEBHOOK_ID", "")
	if err := checkConfig(); err == nil || !strings.Contains(err.Error(), "PAYPAL_WEBHOOK_ID") {
		t.Fatalf("production accepted without a registered PayPal webhook: %v", err)
	}
	t.Setenv("PAYPAL_WEBHOOK_ID", "test-credential")
	t.Setenv("AUTH0_REDIRECT_URI", "https://other.example.com/api/auth/callback")
	if err := checkConfig(); err == nil || !strings.Contains(err.Error(), "AUTH0_REDIRECT_URI") {
		t.Fatalf("mismatched callback accepted: %v", err)
	}
	t.Setenv("APP_BASE_URL", "http://demo.example.com")
	if err := checkConfig(); err == nil || !strings.Contains(err.Error(), "APP_BASE_URL") {
		t.Fatalf("HTTP production accepted: %v", err)
	}
	t.Setenv("APP_BASE_URL", "https://demo.example.com")
	t.Setenv("AI_PROVIDER_KEY", "")
	if err := checkConfig(); err == nil || strings.Contains(err.Error(), "test-credential") || !strings.Contains(err.Error(), "AI_PROVIDER_KEY") {
		t.Fatalf("unsafe or missing preflight error: %v", err)
	}
}
