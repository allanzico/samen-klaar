# Deployment

## Render

`render.yaml` defines one Docker web service and PostgreSQL 16. Connect the repository through a Render Blueprint. Create the `samenklaar-env` environment group with the provider settings from `.env.example`; set `APP_BASE_URL` to the actual HTTPS origin and `AUTH0_REDIRECT_URI` to that origin plus `/api/auth/callback`.

The Blueprint supplies the internal `DATABASE_URL`, production environment, listening port and frontend path. It runs `/api migrate` before deployment and checks `/api/health/ready`. Keep one replica: assistant runner locks and request rate limiters are in process memory. The database access list is private by default; do not open it publicly to make the app run.

Allow the HTTPS callback/logout origin in Auth0. Register the PayPal sandbox webhook at `https://your-domain/api/paypal/webhook`, using the same sandbox platform app, and store its webhook ID. Set both marketplace identifiers. SMTP is optional; important email stays queued until configured. Live PayPal payments remain disabled even when `APP_ENV=production`.

All application settings belong in the environment group or service secrets. A Docker build contains source and static assets, never `.env`, `.secrets`, customer uploads or local media.

## Docker host

Copy `.env.example` to ignored `.env.production`, set production provider values, the public `DOMAIN`, `ACME_EMAIL` and an immutable `APP_IMAGE` from your registry.

```sh
make publish IMAGE_REPO=your-registry/samen-klaar
# Put the returned immutable image digest in .env.production, then:
make init-prod
make validate-prod
make deploy-prod
```

`make init-prod` generates database credentials and imports app settings into `.secrets/prod/app.env`. Existing generated secrets are preserved. Caddy terminates HTTPS; keep the app and database off public ports. Runtime migrations are a separate deployment step before readiness checks. Do not start a second replica without shared assistant locks and rate limits.

Use `make status-prod`, `make logs-prod`, `make backup-prod` and `make rollback-prod` for operations. Keep backups private and verify restore procedures before accepting real customer data. A code rollback does not reverse database migrations. Do not commit generated deployment settings or backup files.
