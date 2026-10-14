# SamenKlaar

SamenKlaar connects customers with home-service professionals. AI handles tedious follow-up by quickly clarifying each job and organizing the details professionals need to respond sooner. Quotes, scheduling, updates, evidence, approvals, and payment stay together from first message to resolution, reducing back-and-forth so jobs get resolved faster.


Licensed under the [MIT License](LICENSE). [View the source repository](https://github.com/allanzico/samen-klaar).

Customer intake now asks for an Amsterdam neighbourhood or postcode area before discovery. Professionals publish the neighbourhoods or postcode areas they serve; SamenKlaar filters against those labels and does not claim geocoding, distance, or travel-time precision. The full visit address is collected only after a customer selects a professional, is required before plan approval, and is available only to that customer and the assigned professional. If no approved service covers the area, the customer can save the request to an operator-visible waitlist.

For automated application checks, run `make check-flow`. It builds the frontend and backend, starts a disposable PostgreSQL database, and checks onboarding, operator approval, intake, price negotiation, date planning, booking, evidence, scope changes and payment reconciliation. It never uses the application's persistent database. External AI, storage and PayPal responses are contract fixtures; this does not establish that Auth0 login, cloud storage credentials or a real PayPal wallet work. Manual testing uses the application at `http://localhost:8090` with actual accounts and configured integrations.


Intake does not fabricate a provider when a service or area has no match. Known trades proceed to service-area matching, including assessment requests; an unclear trade stays in the chat for another clarification. Uncovered areas can be saved for operator review. “As soon as possible” requests show the earliest real published availability, even when it is days away. The customer provides the full address after selecting a professional, then explicitly approves the plan and visit before reservation. Providers without published times remain available for bilateral date planning. Previously blocked conversations retain their answers and resume after migration.

## Run locally

Requirements: Docker with Compose, Make, OpenSSL, curl, and credentials for an AI provider (Gemini, OpenAI, or Claude through Anthropic), Cloudflare R2, PayPal, and Auth0. Go, Node.js, and PostgreSQL run inside containers.

1. Run `make init-local` to generate the database password and local configuration.
2. Fill in the provider credentials in `.secrets/local/app.env` (the active Docker configuration). Local deployment also fills any empty settings from the root `.env`; existing values in the active file take precedence.
3. In Auth0, use a Regular Web Application with callback `http://localhost:8090/api/auth/callback` and logout URL `http://localhost:8090`.
4. Start the complete stack:

```sh
make deploy-local
```

Auth0 settings use `AUTH0_ISSUER_URL`, `AUTH0_CLIENT_ID`, `AUTH0_CLIENT_SECRET`, and `AUTH0_REDIRECT_URI`. Set `AI_PROVIDER` to `gemini`, `openai`, or `claude`, then set `AI_PROVIDER_KEY` and `AI_MODEL` to the matching provider credentials and model ID. The AI key stays on the server and is never sent to browsers.

Open [http://localhost:8090](http://localhost:8090). Caddy routes all browser traffic to the app container. Go serves the built Svelte frontend and `/api` together, and PostgreSQL stores account and job state. The deployment command builds the frontend and backend, runs migrations, and waits for health checks. There is no separate frontend development server to start.

The welcome page appears immediately; checking for an existing sign-in runs in the background and does not create an account. There is one Auth0 sign-in flow. Every account has customer access and can start professional onboarding from its customer workspace. Onboarding adds a pending professional membership to the same identity; professional approval is still required before offering services. The workspace switch uses existing memberships and saves the choice on the account, so it survives sign-out and a new login. Operator access is provisioned by the service owner in PostgreSQL; login parameters and workspace switches cannot grant that privilege.

## Build and deploy

The same three containers run locally and on the production server: Caddy, the app (Go API + built Svelte frontend), and PostgreSQL with a persistent volume. [docker/Dockerfile](docker/Dockerfile) builds everything inside Docker. Docker and Caddy configuration and deployment scripts live in `docker/`; Make runs Compose with the repository root as its project and build context. `make infra-local` starts just PostgreSQL; `make stop-local` stops the stack while preserving data. After editing code or credentials, rerun `make deploy-local` to rebuild and apply the changes.

Production uses the same stack on one Linux server: `make init-prod DOMAIN=app.your-domain.com ACME_EMAIL=you@your-domain.com`, fill the server credentials, set an immutable release image, then run `make deploy-prod` on the server. Deployments validate configuration, back up production data, run migrations separately, wait for readiness, and attempt app image rollback if replacement fails.

## Product capabilities

- **Separate accounts and requests:** Auth0 identity plus PostgreSQL memberships; each customer can maintain multiple requests, and a professional can have multiple assigned jobs. Workspace IDs are checked against the signed-in customer owner or assigned professional on state, evidence and payment actions.
- **Provider-owned offers:** approved professionals publish a service, price, currency, duration, materials policy, exclusions and availability. Matching and quote arithmetic use those records. A professional cannot be booked for overlapping reserved time.
- **AI coordination:** the Go server supports Gemini, OpenAI, and Claude (Anthropic) through the same structured intake and optional customer-requested evidence review flow. Set `AI_PROVIDER`, `AI_PROVIDER_KEY`, and `AI_MODEL` for the provider you choose. AI suggestions cannot set prices, approve work or move money.
- **Private evidence:** professional uploads are stored in Cloudflare R2 and served through account- and request-authorized API routes. Completion requires a real upload; the customer separately approves completion and payment.
- **PayPal checkout:** the current adapter creates an authorization order, redirects the buyer to PayPal, authorizes on return and captures only after explicit customer approval. Local and contract verification use sandbox credentials; the application does not label the customer-facing payment flow as a sandbox.
- **Professional and operator workspaces:** professional setup includes operator approval; operators can review profiles and inspect exceptions.

## Production readiness

The application does not automatically create test identities, professionals or appointments. Local test data is an explicit development operation; payment success is never simulated in the application. This checkout is **not ready to accept live money**: marketplace seller onboarding and seller-directed settlement are implemented for PayPal sandbox, but the local PayPal platform merchant ID, partner attribution code and webhook ID still need configuration. Live marketplace approval, fee policy, production settlement and authorization recovery remain release dependencies. Webhook verification, full refunds, cancellation review, dispute and exception workflows, scheduled reminders, authenticated in-app notifications, SMTP delivery with operator-visible delivery health, per-account API throttles and per-client sign-in throttles are implemented and covered by local checks. Production email credentials, sender-domain authentication, broader edge-level abuse controls, multi-replica rate coordination, legal review, data retention and restore procedures still require launch setup. See [docs/PRODUCT_SPEC.md](docs/PRODUCT_SPEC.md) and [docs/IMPLEMENTATION.md](docs/IMPLEMENTATION.md) for the release status.

Each request shows the current agreement above a scrolling conversation. Message, price-offer and visit-proposal modes share one composer. The proposer can withdraw a pending offer or visit proposal. Customer cancellation at least 24 hours before a visit releases its time; later requests go to operator review. Cancellation after work begins raises an issue for people to resolve. Operators can record rework or close a reviewed issue and trigger an approved full refund; partial refunds and reversals require manual reconciliation. Consumer withdrawal and cancellation wording remains subject to legal review. Unsent messages, proposals and professional work notes are saved in the current browser tab, separately for each account, role and request.

Use the Customer/Professional switch to change workspaces within your own account. On mobile, the switch is inside the account menu. The selected workspace persists until you change it. An account’s own services are excluded from matching and rejected by the API and booking transaction. Use separate browser profiles when testing two different people’s accounts.

## Local test data and matching performance

Test data is loaded explicitly into the local persistent database, never during application startup or production deployment. Set `SEED_PASSWORD` in the ignored `.secrets/local/seed.env`, then run `make seed-local` after deployment. This creates five customer accounts and 10,000 professional accounts. The generated Auth0 import batches under `.secrets/local` contain their synthetic sign-in identifiers. Professionals have active memberships, approved offers across the three supported trades, varied EUR prices and durations, and four to eight future availability slots each. Seeded service-area labels exercise Amsterdam and other cities; production matching uses the professional's published area labels. Reruns preserve account preferences, edited profiles, membership status, bookings and unrelated data.

These account rows become sign-in capable only after Auth0 import. In the same file, set `AUTH0_MANAGEMENT_TOKEN` and `AUTH0_CONNECTION_ID`, then run `make import-seed-auth0`. Get the token from **Applications → APIs → Auth0 Management API → API Explorer**; it needs `create:users` and `read:connections`. Find the existing database connection under **Authentication → Database**, and enable it for the SamenKlaar login application. The importer validates the connection, sends bcrypt password hashes in bounded batches, disables user updates and completion emails, and verifies completed job counts. It saves job IDs to resume interrupted polling and uses a database session lock to prevent concurrent imports; an unknown submission outcome requires recovering the job ID before retrying. No password or Management API credential belongs in Git. See [Auth0 test tokens](https://auth0.com/docs/secure/tokens/access-tokens/management-api-access-tokens/get-management-api-access-tokens-for-testing), [connection IDs](https://auth0.com/docs/authenticate/identity-providers/locate-the-connection-id), and [bulk imports](https://auth0.com/docs/manage-users/user-migration/bulk-user-imports).

Run `make benchmark-local` to measure the configured live AI on 14 labeled requests and matching against the seeded directory. Service scope is checked against published catalog descriptions, materials policies and exclusions before ranking eligible professionals by availability. The UI shows one recommendation and up to two alternatives, with the published service, price, duration and visit time. Unmatched requests ask a focused clarification; no unrelated trade-only offer or operator intake review is substituted. Catalog input is bounded to 200 distinct scopes; this is an AI-assisted relevance check, not a diagnosis or guarantee of equipment. Budget remains a preference for negotiation, not a fabricated price.

The [service-selection result](benchmarks/matching-services-2026-10-12.json) selected suitable services in 12/12 labeled service cases and classified 14/14 trades correctly, with earliest eligible availability and self-booking checks passing. The [original baseline](benchmarks/matching-2026-10-12.json) selected suitable services in only 3/12 cases. Service-specific database reads measured 2.55 ms median and 3.12 ms p95; ten concurrent readers measured 90.32 ms median and 110.53 ms p95. Live routing plus service selection took about 8 seconds median. These are direct coordinator/repository measurements, excluding browser/login, HTTP, negotiation and booking latency; 14 prompts are a smoke evaluation, not proof of general AI reliability.

Messages remain available during planning and negotiation. Price and date proposals require explicit agreement; a chat message cannot change a quote or reserve a visit. Unsent message and proposal drafts are stored per account, role and request in the current browser tab, restored on reload, and cleared after successful submission or sign-out. Browser storage failures are shown. Dates default to the published service duration, display the timezone and validate before conversion and submission.

## Structure

- `src/features/` — focused Svelte components for authentication, customer, professional, operator and request navigation
- `src/lib/` — shared TypeScript API client and domain contracts
- `src/state/` — app state and request/action controller
- `cmd/api/` — Go executable and dependency wiring
- `internal/workflow/` — workflow state and server-side domain transitions
- `internal/identity/` — OIDC login, session cookies and membership routing
- `internal/httpapi/` — authenticated API, request-scoped authorization and static frontend delivery
- `internal/adapters/` — AI providers, PayPal, Cloudflare R2 and PostgreSQL/River adapters
- `internal/seed/` — explicit local test data, Auth0 import and matching measurements
- `internal/adapters/postgres/migrations/` — ordered runtime schema migrations
- `db/` — canonical schema and sqlc query sources
- `docker/` — Dockerfile, Compose files, Caddy configuration and deployment scripts
