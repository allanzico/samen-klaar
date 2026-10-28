# Local setup

## Start the application

Install Docker with Compose, Make, OpenSSL and curl. The Dockerfile pins the Go and Node major versions and includes all build steps; PostgreSQL 16 is started by Compose.

1. Clone the repository and copy `.env.example` to ignored `.env`.
2. Run `make init-local`. This creates `.deploy/local.env`, a database password and `.secrets/local/app.env`. Fill provider settings in the latter. Existing generated database/origin settings take precedence over the root template.
3. Configure the providers below, then run `make deploy-local`.
4. Visit `http://localhost:8090`, sign in and create a customer request. A professional uses the same sign-in system, then completes onboarding and operator review before being matched.

`make validate-local` validates configuration; `make status-local` and `make logs-local` show the running stack. `make migrate-local` runs migrations explicitly. `make stop-local` retains the database. After editing app settings or code, rerun `make deploy-local`.

## Provider settings

| Provider | Required settings and preparation |
| --- | --- |
| AI | Choose `AI_PROVIDER=gemini`, `openai` or `claude`; set `AI_PROVIDER_KEY` and a model supported by that account in `AI_MODEL`. `AI_EFFORT` is optional for Claude. Keys stay on the server. |
| Auth0 | Create a Regular Web Application. Set `AUTH0_ISSUER_URL`, `AUTH0_CLIENT_ID`, `AUTH0_CLIENT_SECRET` and `AUTH0_REDIRECT_URI=http://localhost:8090/api/auth/callback`. Allow that callback and logout URL `http://localhost:8090`. |
| Cloudflare R2 | Create a private bucket with read/write object credentials. Fill `CLOUDFLARE_R2_ACCESS_KEY_ID`, `CLOUDFLARE_R2_SECRET_ACCESS_KEY`, `CLOUDFLARE_R2_BUCKET` and `CLOUDFLARE_R2_ACCOUNT_ID` or the explicit endpoint. Keep `EVIDENCE_PROVIDER=r2`. Do not expose evidence through a public bucket. |
| Mapbox | Set server-side `MAPBOX_ACCESS_TOKEN`. Permanent Geocoding v6 requires the appropriate account billing/entitlement. `python3 scripts/probe-mapbox.py` checks three public location examples without printing the token. |
| PayPal | Keep `PAYMENT_PROVIDER=paypal_sandbox` and `PAYPAL_ENV=sandbox`. Supply the platform app's client ID/secret, platform merchant ID and partner attribution ID. See [PAYMENTS.md](PAYMENTS.md) for separate buyer/seller wallets, permissions and webhooks. |
| SMTP | Optional. Set `SMTP_ADDR`, `SMTP_FROM` and paired username/password when the relay requires authentication. See [EMAIL.md](EMAIL.md). Routine conversation updates remain in-app. |

The root `.env.example` documents all supported settings. Secrets, generated database files and imported identities remain ignored. Never paste credentials into tracked files.

## Reproducible sample accounts

For ten comparable Amsterdam sink-repair professionals plus a customer, create ignored `.secrets/local/seed.env`:

```dotenv
SEED_PASSWORD=choose-a-local-password
AUTH0_MANAGEMENT_TOKEN=
AUTH0_CONNECTION_ID=
# Optional when sign-in uses a custom domain:
AUTH0_MANAGEMENT_URL=
```

Use a short-lived Management API token for the intended Auth0 tenant. The importer needs `read:connections`, `create:users` and `read:users`; the connection must allow the regular web application. Keep this token out of the running web service.

```sh
make seed-demo
make import-seed-demo-auth0
```

The customer sign-in is `alex-de-vries@samenklaar.test`; the receiving professional is `noor-van-dijk@samenklaar.test` (Helder Kraanwerk). Use the password you chose. These are reserved login addresses, not mailboxes. The seed creates normal profiles, weekly hours, days off and offers; it never creates a payment connection, booking, authorization or capture. Connect a distinct sandbox seller through the professional's normal PayPal UI.

The separate `make seed-pros` / `make import-seed-pros-auth0` commands create the mixed-trade cohort defined in `internal/seed/pros.go`. Import reports and credentials stay in `.secrets/local/seed-pros/` or `seed-demo/`. Repeating the seeds preserves existing edited profiles, schedules and preferences.

These Make targets are local development tools. Hosted presentation preparation requires the separate explicit origin-pinned opt-in described in [PRESENTATION.md](PRESENTATION.md); seed switches do not belong on the running web service.

## Development checks

`make check-flow` needs Docker and no external-provider credentials. For faster frontend work, use Node.js 22, `npm ci`, `npm run check` and `npm run build`; the Go API serves `dist/`. `go test ./...` skips database/provider opt-in tests without their dedicated settings. Use the disposable full-flow gate before changing persistence or payment code.
