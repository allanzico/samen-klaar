# SamenKlaar

SamenKlaar connects customers with home-service professionals. Every person gets an assistant that clarifies the job, negotiates price and visit time within limits they set, books, and asks before anything outside those limits. Quotes, scheduling, evidence, approvals and payment stay in one conversation from first message to resolution. Payment is always the customer's decision.

Licensed under the [MIT License](LICENSE).

## Run locally

Requirements: Docker with Compose, Make, OpenSSL, curl, and credentials for an AI provider (Gemini, OpenAI or Claude), Cloudflare R2, PayPal (sandbox) and Auth0. Go, Node.js and PostgreSQL run in containers.

1. `make init-local` generates the database password and local configuration.
2. Fill in the credentials in `.secrets/local/app.env`. Empty settings are also filled from a root `.env` (see [.env.example](.env.example)).
3. In Auth0 create a Regular Web Application with callback `http://localhost:8090/api/auth/callback` and logout URL `http://localhost:8090`.
4. `make deploy-local` builds and starts the stack at [http://localhost:8090](http://localhost:8090).

`make stop-local` stops it and keeps the data; rerun `make deploy-local` after changing code or credentials.

## Checks

`make check-flow` builds both halves and runs the end-to-end flow against a disposable PostgreSQL database. Go tests run with `go test ./...`, and the frontend with `npm run check`.

## Test data

Test data is loaded explicitly, never at startup. Put `SEED_PASSWORD` and the Auth0 management settings in `.secrets/local/seed.env`, then run `make seed-pros` and `make import-seed-pros-auth0`. See [docs/SEED_PROFESSIONALS.md](docs/SEED_PROFESSIONALS.md).

## Documentation

- [Product spec](docs/PRODUCT_SPEC.md) and [implementation status](docs/IMPLEMENTATION.md)
- [Architecture](docs/ARCHITECTURE.md) and [assistants](docs/AGENT_ARCHITECTURE.md)
- [Payments](docs/PAYMENTS.md) and [design system](docs/DESIGN_SYSTEM.md)

## Structure

- `src/` — Svelte frontend (`features/`, `ui/`, `lib/`, `state/`)
- `cmd/api/` — Go executable
- `internal/` — workflow, identity, HTTP API, adapters (AI, PayPal, R2, PostgreSQL), seed data
- `db/` — schema and queries; migrations in `internal/adapters/postgres/migrations/`
- `docker/` — Dockerfile, Compose, Caddy and scripts
