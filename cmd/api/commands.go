package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	emailadapter "github.com/allanzico/samen-klaar/internal/adapters/email"
	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
)

func command(args []string) error {
	if len(args) > 0 && args[0] == "healthcheck" {
		endpoint := "http://127.0.0.1:8080/api/health/ready"
		if len(args) == 2 {
			endpoint = args[1]
		} else if len(args) > 2 {
			return errors.New("usage: api healthcheck [URL]")
		}
		return healthcheck(endpoint)
	}
	path := envOr("APP_CONFIG_FILE", ".env")
	if err := loadEnv(path); err != nil && (os.Getenv("APP_CONFIG_FILE") != "" || !os.IsNotExist(err)) {
		return fmt.Errorf("read application configuration: %w", err)
	}
	if len(args) == 0 {
		if os.Getenv("APP_CONFIG_FILE") != "" || envOr("APP_ENV", "development") == "production" {
			if err := checkConfig(); err != nil {
				return err
			}
		}
		return run()
	}
	if len(args) == 1 && (args[0] == "seed-pros" || args[0] == "import-seed-pros-auth0") {
		return localDataCommand(args[0])
	}
	if len(args) > 0 && args[0] == "test-email" {
		return testEmailCommand(args[1:], os.Stdin, os.Stdout)
	}
	if len(args) != 1 {
		return errors.New("usage: api [check-config|migrate|healthcheck|test-email [recipient]]")
	}
	switch args[0] {
	case "check-config":
		return checkConfig()
	case "migrate":
		if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
			return errors.New("DATABASE_URL is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return postgres.Migrate(ctx, os.Getenv("DATABASE_URL"))
	default:
		return errors.New("usage: api [check-config|migrate|healthcheck|test-email [recipient]]")
	}
}

// Preflight reports setting names, never credential values. Provider discovery
// and database connectivity are verified separately by startup and readiness.
func checkConfig() error {
	mode := envOr("APP_ENV", "development")
	if mode != "development" && mode != "production" {
		return errors.New("APP_ENV must be development or production")
	}
	if _, err := emailadapter.NewSMTPFromEnv(); err != nil {
		return err
	}
	required := []string{"APP_BASE_URL", "DATABASE_URL", "AI_PROVIDER_KEY", "AI_MODEL", "AUTH0_ISSUER_URL", "AUTH0_CLIENT_ID", "AUTH0_CLIENT_SECRET", "AUTH0_REDIRECT_URI", "CLOUDFLARE_R2_ACCESS_KEY_ID", "CLOUDFLARE_R2_SECRET_ACCESS_KEY", "CLOUDFLARE_R2_BUCKET", "PAYPAL_CLIENT_ID", "PAYPAL_CLIENT_SECRET"}
	if mode == "production" {
		required = append(required, "PAYPAL_WEBHOOK_ID", "PAYPAL_PLATFORM_MERCHANT_ID", "PAYPAL_PARTNER_ATTRIBUTION_ID")
	}
	var missing []string
	for _, key := range required {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			missing = append(missing, key)
		}
	}
	if os.Getenv("CLOUDFLARE_R2_ACCOUNT_ID") == "" && os.Getenv("CLOUDFLARE_R2_ENDPOINT") == "" {
		missing = append(missing, "CLOUDFLARE_R2_ACCOUNT_ID or CLOUDFLARE_R2_ENDPOINT")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing configuration: %s", strings.Join(missing, ", "))
	}
	aiProvider := strings.ToLower(envOr("AI_PROVIDER", "gemini"))
	if _, supported := map[string]bool{"gemini": true, "openai": true, "anthropic": true, "claude": true}[aiProvider]; !supported {
		return errors.New("AI_PROVIDER must be gemini, openai, claude, or anthropic")
	}
	for key, want := range map[string]string{"EVIDENCE_PROVIDER": "r2", "PAYMENT_PROVIDER": "paypal_sandbox", "PAYPAL_ENV": "sandbox"} {
		if envOr(key, want) != want || key == "PAYPAL_ENV" && os.Getenv(key) == "" {
			return fmt.Errorf("%s must be %s", key, want)
		}
	}
	base, err := url.Parse(os.Getenv("APP_BASE_URL"))
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") || (base.Scheme != "http" && base.Scheme != "https") || mode == "production" && base.Scheme != "https" {
		return errors.New("APP_BASE_URL must be an HTTP origin (HTTPS in production)")
	}
	if expected := os.Getenv("EXPECTED_APP_BASE_URL"); expected != "" && strings.TrimRight(os.Getenv("APP_BASE_URL"), "/") != expected {
		return errors.New("APP_BASE_URL must match the deployment gateway origin")
	}
	callback, err := url.Parse(os.Getenv("AUTH0_REDIRECT_URI"))
	if err != nil || callback.Scheme != base.Scheme || callback.Host != base.Host || callback.Path != "/api/auth/callback" || callback.RawQuery != "" || callback.Fragment != "" || callback.User != nil {
		return errors.New("AUTH0_REDIRECT_URI must match APP_BASE_URL + /api/auth/callback")
	}
	issuer, err := url.Parse(os.Getenv("AUTH0_ISSUER_URL"))
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.User != nil {
		return errors.New("AUTH0_ISSUER_URL must be an HTTPS URL")
	}
	db, err := url.Parse(os.Getenv("DATABASE_URL"))
	if err != nil || db.Host == "" || (db.Scheme != "postgres" && db.Scheme != "postgresql") {
		return errors.New("DATABASE_URL must be a PostgreSQL URL")
	}
	return nil
}

func healthcheck(endpoint string) error {
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(endpoint)
	if err != nil {
		return errors.New("health endpoint unreachable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned HTTP %d", response.StatusCode)
	}
	return nil
}
