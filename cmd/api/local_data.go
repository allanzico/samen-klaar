package main

import (
	"context"
	"errors"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allanzico/samen-klaar/internal/seed"
)

func localDataCommand(action string) error {
	base, err := url.Parse(os.Getenv("APP_BASE_URL"))
	if err != nil || envOr("APP_ENV", "development") != "development" ||
		(base.Hostname() != "localhost" && base.Hostname() != "127.0.0.1" && base.Hostname() != "::1") {
		return errors.New("test data tools require APP_ENV=development and a loopback APP_BASE_URL")
	}
	if err := loadEnv(envOr("SEED_CONFIG_FILE", ".secrets/local/seed.env")); err != nil {
		return errors.New("cannot read local seed configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	output := envOr("SEED_OUTPUT_DIR", ".secrets/local/seed-pros")
	issuer := os.Getenv("AUTH0_ISSUER_URL")
	switch action {
	case "seed-pros":
		return seed.RunPros(ctx, os.Getenv("DATABASE_URL"), issuer, os.Getenv("SEED_PASSWORD"), output)
	case "import-seed-pros-auth0":
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
		return seed.Import(ctx, issuer, os.Getenv("AUTH0_CLIENT_ID"), os.Getenv("AUTH0_MANAGEMENT_TOKEN"), os.Getenv("AUTH0_CONNECTION_ID"), output)
	}
	return errors.New("unknown local data action")
}
