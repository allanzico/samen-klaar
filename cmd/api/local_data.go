package main

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allanzico/samen-klaar/internal/seed"
)

func localDataCommand(action string) error {
	if err := validateSeedTarget(action); err != nil {
		return err
	}
	if err := loadEnv(envOr("SEED_CONFIG_FILE", ".secrets/local/seed.env")); err != nil {
		return errors.New("cannot read local seed configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	output := envOr("SEED_OUTPUT_DIR", ".secrets/local/seed-pros")
	if action == "seed-demo" || action == "import-seed-demo-auth0" {
		output = envOr("SEED_OUTPUT_DIR", ".secrets/local/seed-demo")
	}
	issuer := os.Getenv("AUTH0_ISSUER_URL")
	switch action {
	case "seed-pros":
		return seed.RunPros(ctx, os.Getenv("DATABASE_URL"), issuer, os.Getenv("SEED_PASSWORD"), output)
	case "seed-demo":
		return seed.RunDemo(ctx, os.Getenv("DATABASE_URL"), issuer, os.Getenv("SEED_PASSWORD"), output)
	case "import-seed-pros-auth0", "import-seed-demo-auth0":
		// A session lock survives long-running polling and is released by
		// PostgreSQL when the tool exits or its container is interrupted.
		// Strip pool tuning parameters before opening a dedicated connection;
		// they are valid in DATABASE_URL but are not PostgreSQL settings.
		config, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
		if err != nil {
			return errors.New("invalid DATABASE_URL for the import lock")
		}
		conn, err := pgx.ConnectConfig(ctx, config.ConnConfig)
		if err != nil {
			return errors.New("cannot connect to the local database for the import lock")
		}
		defer func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = conn.Close(closeCtx)
		}()
		var acquired bool
		if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('relay_auth0_seed_v1',0))`).Scan(&acquired); err != nil {
			return err
		}
		if !acquired {
			return errors.New("another Auth0 seed import is already running")
		}
		return seed.Import(ctx, issuer, os.Getenv("AUTH0_CLIENT_ID"), os.Getenv("AUTH0_MANAGEMENT_TOKEN"), os.Getenv("AUTH0_CONNECTION_ID"), output, os.Getenv("AUTH0_MANAGEMENT_URL"))
	}
	return errors.New("unknown local data action")
}

func validateSeedTarget(action string) error {
	base, err := url.Parse(os.Getenv("APP_BASE_URL"))
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Path != "" && base.Path != "/" {
		return errors.New("test data tools require a valid application origin")
	}
	loopback := base.Hostname() == "localhost" || base.Hostname() == "127.0.0.1" || base.Hostname() == "::1"
	demo := action == "seed-demo" || action == "import-seed-demo-auth0"
	if demo {
		// Hosted demo preparation is an explicit, origin-pinned CLI operation.
		// The presentation can use the production site when its owner opts in.
		hostedDemo := !loopback && envOr("APP_ENV", "development") == "production" && base.Scheme == "https" &&
			os.Getenv("ENABLE_DEMO_SEED") == "1" && strings.TrimRight(os.Getenv("EXPECTED_DEMO_ORIGIN"), "/") == strings.TrimRight(base.String(), "/")
		if hostedDemo {
			return nil
		}
	}
	if envOr("APP_ENV", "development") != "development" || !loopback || base.Scheme != "http" && base.Scheme != "https" {
		return errors.New("test data tools require a local development origin, or explicitly enabled demo target")
	}
	return nil
}
