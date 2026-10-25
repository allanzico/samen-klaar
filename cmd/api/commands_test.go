package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvPreservesMailValuesAndOnlyRemovesPairedQuotes(t *testing.T) {
	expected := map[string]string{
		"SMTP_FROM_NAME": "SamenKlaar Updates",
		"SMTP_PASSWORD":  "example-password",
		"SMTP_USERNAME":  "literal\"",
		"SMTP_FROM":      `"Quoted Sender" <notifications@example.test>`,
	}
	for key := range expected {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("SMTP_FROM_NAME='SamenKlaar Updates'\nSMTP_PASSWORD=\"example-password\"\nSMTP_USERNAME=literal\"\nSMTP_FROM=\"Quoted Sender\" <notifications@example.test>\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := loadEnv(path); err != nil {
		t.Fatal(err)
	}
	for key, want := range expected {
		if os.Getenv(key) != want {
			t.Fatalf("%s was not preserved by the environment loader", key)
		}
	}
}

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
	for _, key := range []string{"AI_PROVIDER_KEY", "AI_MODEL", "AUTH0_CLIENT_ID", "AUTH0_CLIENT_SECRET", "CLOUDFLARE_R2_ACCESS_KEY_ID", "CLOUDFLARE_R2_SECRET_ACCESS_KEY", "CLOUDFLARE_R2_BUCKET", "CLOUDFLARE_R2_ACCOUNT_ID", "PAYPAL_CLIENT_ID", "PAYPAL_CLIENT_SECRET", "PAYPAL_WEBHOOK_ID", "PAYPAL_PLATFORM_MERCHANT_ID", "PAYPAL_PARTNER_ATTRIBUTION_ID"} {
		t.Setenv(key, "test-credential")
	}
	for _, key := range []string{"SMTP_ADDR", "SMTP_FROM", "SMTP_FROM_NAME", "SMTP_USERNAME", "SMTP_PASSWORD"} {
		t.Setenv(key, "")
	}
	t.Setenv("AI_PROVIDER", "openai")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_BASE_URL", "https://demo.example.com")
	t.Setenv("AUTH0_REDIRECT_URI", "https://demo.example.com/api/auth/callback")
	t.Setenv("AUTH0_ISSUER_URL", "https://tenant.example.com/")
	t.Setenv("DATABASE_URL", "postgres://samenklaar:test-password@db:5432/samenklaar")
	t.Setenv("PAYPAL_ENV", "sandbox")
	if err := checkConfig(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SMTP_ADDR", "")
	t.Setenv("SMTP_FROM", "")
	if err := checkConfig(); err != nil {
		t.Fatalf("production without SMTP should still start: %v", err)
	}
	t.Setenv("SMTP_USERNAME", "user")
	if err := checkConfig(); err == nil || !strings.Contains(err.Error(), "SMTP_USERNAME") {
		t.Fatalf("unpaired SMTP credentials accepted: %v", err)
	}
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_ADDR", "mail.example.test:invalid")
	t.Setenv("SMTP_FROM", "notifications@example.test")
	if err := checkConfig(); err == nil || !strings.Contains(err.Error(), "SMTP_ADDR") {
		t.Fatalf("invalid SMTP endpoint passed preflight: %v", err)
	}
	t.Setenv("SMTP_ADDR", "mail.example.test:2525")
	t.Setenv("SMTP_FROM_NAME", "SamenKlaar Updates")
	if err := checkConfig(); err != nil {
		t.Fatalf("valid SMTP configuration rejected by preflight: %v", err)
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
