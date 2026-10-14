# Booking flow verification

Verification was run on 14 October 2026. `make check-flow` passed with disposable PostgreSQL and deterministic provider responses; it applies all 15 current SQL migrations. The run includes marketplace seller eligibility, seller-bound authorization, customer-approved instant seller settlement, onboarding/revocation webhooks, and the full booking lifecycle. The local app is healthy. A separate backup-restore check from 13 October covered the 14 migrations present at that time. No real PayPal seller onboarding, wallet approval, capture, refund, or payout was triggered.

## Automated journey

1. A professional submits a service profile through the HTTP API. An operator approves it; customer identities cannot grant professional access.
2. A customer describes a leaking sink. AI clarification, timing, and budget narrow the request to appropriate Amsterdam-area professionals without asking for an exact address or a trade selector.
3. Customer and professional negotiate the price, then choose and reserve an appointment together. Counteroffers, rescheduling, competing reservations, and professional confirmation expiry are checked.
4. The professional starts the job, proposes a scope change, uploads evidence, adds a completion note, and submits work. The customer reviews evidence and separately approves completion and payment.
5. Payment tests cover authorization, a lost capture response, reconciliation, webhook deduplication, and persistence of the resulting state.
6. Cancellation tests cover both roles, late-cancellation review, release of a professional-cancelled reservation, and prevention of a customer-cancelled request re-entering matching.
7. Scheduling tests verify one-time reminders for confirmed visits, overdue evidence, completion review, and payment approval, plus operator escalation for stalled work.
8. Customer and professional receive only their own request notifications. The recipient can mark an inbox item read, that state survives a later fetch, and another account cannot mark it read.
9. After capture, the customer submits a verified rating and the assigned professional responds. The reopened PostgreSQL workspace retains both; unpaid, incomplete, refunded, duplicate, and wrong-role attempts are rejected.
10. PayPal contract tests require platform identity and permissions for seller referral, verify that operator input cannot forge seller eligibility, bind an order to the verified professional, and request instant seller disbursement only after explicit customer capture approval.

## Results

| Boundary | Result | Scope |
|---|---|---|
| Frontend | Passed | Svelte diagnostics and production build |
| Go backend | Passed | Unit tests, `go vet`, and production binary build |
| API and PostgreSQL | Passed | Full request lifecycle, permissions, persistence, and competing booking checks against disposable PostgreSQL |
| Request throttling | Passed | Token-bucket burst/refill behavior, account isolation, public sign-in client isolation, validated gateway client address parsing, and AI-backed action classification |
| Account authorization | Passed | Customer/professional onboarding and membership checks; role escalation and cross-account access are denied |
| Scheduling and cancellation | Passed | Appointment confirmation deadlines, cancellation paths, reminders, deduplication, and stalled-job queue cases |
| Payment and webhook handling | Passed with provider stubs | Seller referral headers/permissions, seller eligibility, seller-bound authorization and instant capture, capture recovery, webhook deduplication, refund persistence, partial-refund review, reversal/dispute reconciliation. Real PayPal seller onboarding and settlement were not exercised because the platform ID, partner attribution code and webhook ID are unset. |
| Verified job reviews | Passed | One customer rating after completion and payment, followed by one assigned-professional response; persisted across store reopen |
| Notification queue | Passed | Durable outbox/River queue behavior and retry recovery paths; external SMTP delivery was not exercised in this run |
| Operator delivery monitoring | Passed | Operator-only queue returns aggregate failed, SMTP-unconfigured, and stale-send counts without recipient addresses or message contents |
| In-app notification inbox | Passed | Account and role scoping, request deep links, recipient-only read actions, and persisted read state in the HTTP/PostgreSQL journey |
| Local database recovery | Passed on 13 October | `make backup-local` dump restored into an isolated database; all 14 SQL migrations present at the time and notification tables were present. The temporary restore database was removed; production recovery was not touched. The current flow run applies all 15 migrations to a new disposable database. |
| AI and private file storage | Previously verified against configured development services | Structured AI intake/evidence responses and private R2 upload/download; not part of the disposable flow's provider-stub checks |
| Browser: customer route after local deploy | Passed | Authenticated seeded customer loaded the saved request, published price, and Amsterdam appointment; address stayed blank and booking approvals stayed disabled |
| Browser: language preference | Passed | Switched the deployed professional setup between Dutch and English; labels, dates, customer workspace navigation and shared header updated, and Dutch remained selected after reload. The 631px browser view kept the header and inbox usable. |
| Browser: notification inbox | Passed | Opened the professional and customer inboxes in their authenticated workspaces; the customer saw two unread request updates. No notification was opened or marked read in this browser pass. API/PostgreSQL checks separately cover recipient-only read actions and persistence. |
| Browser: unauthorized deep links | Passed | Opening `/professional` or `/operator` from the customer session left the active Customer role and customer workspace intact |
| Browser: separate professional/operator sessions | Passed in the earlier local session | Synthetic accounts were used in separate sessions to check professional access and operator queue boundaries; no application review decision was submitted |
| Browser: payment | Not exercised | No wallet approval, capture, refund, or payout was triggered in the browser |
| Browser: current local landing and protected route | Passed | A fresh Chrome profile loaded the deployed landing page; opening `/professional` stayed unauthenticated. A professional account session was not available to inspect the payout setup card. |

## Remaining release work

- **Sandbox marketplace routing:** the seller onboarding and seller-directed payment flow is implemented and passes provider-contract tests; configure the sandbox Platform REST app's platform merchant ID, partner attribution code, and webhook ID, then exercise real seller onboarding and settlement in PayPal sandbox.
- **Live marketplace payouts:** production PayPal platform approval, seller onboarding, fee/settlement configuration, and live credentials must be completed and validated with PayPal.
- **Live checkout:** only the configured development PayPal environment was used. Production wallet approval/capture and refund behavior still need provider-approved live testing before launch.
- **Notification delivery:** the durable queue and SMTP adapter are implemented, but production SMTP credentials, sender-domain authentication, delivery monitoring, and bounce handling need environment setup and live delivery checks. SMS delivery is not implemented.
- **Production launch settings:** production requires real AI, PayPal webhook, and SMTP configuration plus a publicly reachable HTTPS callback registered with PayPal. Local browser testing does not validate DNS, TLS, or production secrets.
- **Product policy:** cancellation/refund terms and operator playbooks need the business/legal decisions documented in the release checklist. The system routes ambiguous partial refunds, reversals, and disputes to manual reconciliation rather than claiming they are resolved.
- **Coverage boundaries:** matching uses provider-published service areas and postcode prefixes; it does not geocode or rank by travel time. Emergency escalation, native Dutch editorial review, and public review visibility/moderation are not covered here.

## Repeat the checks

- `make check-flow` runs Svelte checks/build, Go tests/vet/build, and the HTTP/PostgreSQL workflow tests against a disposable database.
- `RUN_LIVE_PROVIDER_CHECKS=1` opts into configured development-provider checks. They must not authorize or capture a payment.
