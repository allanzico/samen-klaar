package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	aiadapter "github.com/allanzico/samen-klaar/internal/adapters/ai"
	emailadapter "github.com/allanzico/samen-klaar/internal/adapters/email"
	mapboxadapter "github.com/allanzico/samen-klaar/internal/adapters/mapbox"
	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/adapters/postgres"
	"github.com/allanzico/samen-klaar/internal/adapters/r2"
	"github.com/allanzico/samen-klaar/internal/httpapi"
	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := command(os.Args[1:]); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	environment := envOr("APP_ENV", "development")
	if environment != "development" && environment != "production" {
		return fmt.Errorf("APP_ENV=%q must be development or production", environment)
	}
	if environment == "production" && (strings.TrimSpace(os.Getenv("PAYPAL_PLATFORM_MERCHANT_ID")) == "" || strings.TrimSpace(os.Getenv("PAYPAL_PARTNER_ATTRIBUTION_ID")) == "") {
		return errors.New("PAYPAL_PLATFORM_MERCHANT_ID and PAYPAL_PARTNER_ATTRIBUTION_ID are required for marketplace payouts")
	}
	appName := envOr("APP_NAME", "SamenKlaar")
	defaultAddress := "127.0.0.1:8080"
	if environment == "production" {
		defaultAddress = "0.0.0.0:8080"
	}
	address := envOr("APP_ADDR", defaultAddress)
	webDirectory := strings.TrimSpace(os.Getenv("WEB_DIR"))
	if webDirectory == "" {
		webDirectory = "dist"
	}
	if environment == "production" {
		if info, err := os.Stat(webDirectory); err != nil || !info.IsDir() {
			return fmt.Errorf("WEB_DIR %q must contain the built frontend in production", webDirectory)
		}
	}
	appBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")
	emailSender, err := emailadapter.NewSMTPFromEnv()
	if err != nil {
		return err
	}
	if emailSender == nil {
		slog.Warn("email delivery is not configured; important notification emails stay queued until SMTP_ADDR and SMTP_FROM are set")
	}

	providers := httpapi.ProviderConfig{
		AIProvider:       envOr("AI_PROVIDER", "gemini"),
		AIModel:          strings.TrimSpace(os.Getenv("AI_MODEL")),
		EvidenceProvider: envOr("EVIDENCE_PROVIDER", "r2"),
		PaymentProvider:  envOr("PAYMENT_PROVIDER", "paypal_sandbox"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coordinator, err := aiadapter.New(ctx, providers.AIProvider, os.Getenv("AI_PROVIDER_KEY"), providers.AIModel, os.Getenv("AI_EFFORT"))
	if err != nil {
		return err
	}
	var ai httpapi.IntakeAI = coordinator
	providers.AIProvider, providers.AIModel = coordinator.Provider(), coordinator.Model()

	var evidence httpapi.EvidenceStorage
	switch providers.EvidenceProvider {
	case "r2":
		storage, err := r2.New(ctx)
		if err != nil {
			return err
		}
		evidence = storage
	default:
		return fmt.Errorf("unsupported EVIDENCE_PROVIDER %q; use r2", providers.EvidenceProvider)
	}

	var payments httpapi.PaymentGateway
	var paypalService *paypal.Service
	switch providers.PaymentProvider {
	case "paypal_sandbox":
		service, err := paypal.New(appName)
		if err != nil {
			return err
		}
		payments = service
		paypalService = service
	default:
		return fmt.Errorf("unsupported PAYMENT_PROVIDER %q; use paypal_sandbox", providers.PaymentProvider)
	}

	var store workflow.StateStore
	var persistent *postgres.Store
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL != "" {
		opened, err := postgres.OpenWithNotifications(ctx, databaseURL, emailSender, appBaseURL)
		if err != nil {
			return fmt.Errorf("initialize PostgreSQL store: %w", err)
		}
		persistent = opened
		persistent.SetCoordinator(coordinator)
		defer persistent.Close()
		store = persistent
		if paypalService != nil {
			paypalService.SetIntentStore(persistent)
			paypalService.SetWebhookSink(persistent)
			paypalService.SetMarketplaceStore(persistent)
			persistent.SetAuthorizationReleaser(paypalService)
		}
		slog.Info("persistence initialized", "database", "PostgreSQL")
	} else {
		if environment == "production" {
			return errors.New("DATABASE_URL is required when APP_ENV=production")
		}
		slog.Info("account persistence unavailable", "setting", "DATABASE_URL")
	}

	auth0Issuer := strings.TrimSpace(os.Getenv("AUTH0_ISSUER_URL"))
	auth0ClientID := strings.TrimSpace(os.Getenv("AUTH0_CLIENT_ID"))
	auth0ClientSecret := strings.TrimSpace(os.Getenv("AUTH0_CLIENT_SECRET"))
	auth0Redirect := strings.TrimSpace(os.Getenv("AUTH0_REDIRECT_URI"))
	hasAuth0Settings := auth0Issuer != "" || auth0ClientID != "" || auth0ClientSecret != "" || auth0Redirect != ""
	var auth *identity.Auth
	if hasAuth0Settings {
		if persistent == nil {
			return errors.New("DATABASE_URL is required to enable account sign-in")
		}
		var err error
		auth, err = identity.New(ctx, identity.Config{
			IssuerURL: auth0Issuer, ClientID: auth0ClientID, ClientSecret: auth0ClientSecret,
			RedirectURL: auth0Redirect, AppBaseURL: appBaseURL, SecureCookie: environment == "production",
		}, persistent)
		if err != nil {
			return err
		}
	} else if environment == "production" {
		return errors.New("AUTH0_ISSUER_URL, AUTH0_CLIENT_ID, AUTH0_CLIENT_SECRET and AUTH0_REDIRECT_URI are required when APP_ENV=production")
	}

	slog.Info("providers initialized", "ai", providers.AIProvider, "model", providers.AIModel, "effort", strings.TrimSpace(os.Getenv("AI_EFFORT")), "evidence", providers.EvidenceProvider, "payments", providers.PaymentProvider)
	var professionals httpapi.ProfessionalService
	var directory httpapi.ProfessionalDirectory
	var operators httpapi.OperatorService
	if persistent != nil {
		professionals, directory, operators = persistent, persistent, persistent
	}
	var readiness func(context.Context) error
	if persistent != nil {
		readiness = persistent.Ping
	}
	var refunds httpapi.RefundGateway
	var webhookReceiver httpapi.PaymentWebhookReceiver
	if paypalService != nil {
		refunds = paypalService
		if persistent != nil {
			webhookReceiver = paypalService
		}
	}
	var agents httpapi.AgentStore
	if persistent != nil {
		agents = persistent
	}
	var locationSearch httpapi.LocationSearch
	if geocoder := mapboxadapter.New(os.Getenv("MAPBOX_ACCESS_TOKEN")); geocoder != nil {
		locationSearch = geocoder
	}
	app := httpapi.New(httpapi.Dependencies{Locations: locationSearch, AppName: appName, WebDirectory: webDirectory, Store: store, Auth: auth, AppBaseURL: appBaseURL, Providers: providers, AI: ai, Evidence: evidence, Payments: payments, Refunds: refunds, Marketplace: paypalService, Webhook: webhookReceiver, Professionals: professionals, Directory: directory, Operators: operators, Agents: agents, Readiness: readiness, ClientIPHeader: os.Getenv("CLIENT_IP_HEADER")})
	server := &http.Server{
		Addr: address, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 20 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	slog.Info("HTTP server listening", "address", address)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()

	stopSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.StartAgents(stopSignal)
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stopSignal.Done():
		app.BeginShutdown()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		err := <-serverErrors
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func loadEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == value[len(value)-1] && (value[0] == '"' || value[0] == '\'') {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
