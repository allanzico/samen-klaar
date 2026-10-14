# Payments

## PayPal marketplace sandbox path

PayPal's Complete Payments Platform supports multiparty seller onboarding and marketplace checkout in sandbox. The implementation in this repository now uses that platform flow, but this local sandbox is not configured for it yet: `PAYPAL_PLATFORM_MERCHANT_ID`, `PAYPAL_PARTNER_ATTRIBUTION_ID`, and `PAYPAL_WEBHOOK_ID` are currently blank. A normal sandbox REST app alone does not provide the platform identity and partner attribution needed to test seller routing. PayPal allows the platform APIs to be tested with sandbox credentials before live partner approval; live use still requires PayPal approval.

Each professional completes PayPal's before-payment onboarding before accepting a paid booking. The server verifies the returned seller merchant ID against the referral tracking ID, confirms the seller can receive payments and has granted the required payment/refund permissions, and stores that verified seller against the professional account. Callback query parameters alone never mark a seller eligible. Revoking consent disables new payment actions.

After the customer approves the agreed booking amount, Go creates an Orders v2 order with `intent=AUTHORIZE` and the verified professional as `purchase_units[].payee`. The customer approves at PayPal; Go authorizes the order. After work is complete, a separate customer action captures the exact approved amount with `payment_instruction.disbursement_mode=INSTANT`, routing the captured payment to that professional. PayPal settles the seller payment; SamenKlaar does not hold funds as escrow. This uses PayPal's marketplace checkout and seller disbursement behavior, not PayPal's separate batch Payouts API.

No platform fee is sent yet. The percentage and PayPal `PARTNER_FEE` entitlement have not been confirmed; do not invent a rate.

The amount, authorization ID and capture state are server-owned. Go validates the current job and scope before each operation and uses stable `PayPal-Request-Id` values. Order, authorization and capture records are stored in PostgreSQL with conditional state claims; API instances coordinate through persisted state instead of relying on process memory. If an operation is uncertain after a timeout or restart, the customer can ask SamenKlaar to reconcile with PayPal before another action. An active authorization is voided before a customer-approved scope change proceeds.

Every payment action is associated with a job and request workspace. The PayPal return URL carries that workspace ID so multi-request accounts return to the correct job. A workspace ID is not treated as authorization: Go checks customer ownership on each route.

## Why this is not live-payment ready

The PayPal adapter is still sandbox-only. A browser redirect is not proof of capture: the return path checks with PayPal, while the public webhook route verifies each transmission with PayPal before durably queueing a deduplicated event. PostgreSQL/River process capture, denial, refund, reversal and customer-dispute events asynchronously. A capture can succeed at PayPal while a database outage delays local state; stable operation records and reconciliation remain available for recovery.

The application now has marketplace seller onboarding, seller-bound orders, instant professional settlement on capture, seller-scoped refunds, and onboarding/revocation webhook handling. Those routes cannot be exercised against the configured sandbox until PayPal provides the platform merchant ID and partner attribution ID and the app's webhook ID is registered. The operator tool supports full refunds with a stable idempotency key. A partial refund or payment reversal reported by PayPal is held in a reconciliation-only operator case; it cannot be mistaken for a completed full refund or trigger another automatic refund. Partner fees, deposits, live authorization expiry and reauthorization remain unimplemented or undecided.

Keep `PAYPAL_ENV=sandbox`. Do not connect live credentials or accept customer money until the external marketplace, policy and production-readiness requirements below are satisfied.

## Remaining marketplace work

PayPal says an authorization can remain valid for up to 29 days but recommends capture within the first 3-day honor period. Longer appointment-to-completion windows need tested reauthorization behavior; if authorization cannot be used, request fresh customer approval. Netherlands seller eligibility and the exact approved platform configuration, live payout/currency availability, partner-fee entitlement, and seller settlement behavior still need confirmation. These constraints are recorded in [PRODUCT_SPEC.md](PRODUCT_SPEC.md).

Required external confirmation before live marketplace payments are enabled:

1. PayPal approves SamenKlaar as a platform partner and confirms the marketplace product, Netherlands seller features, partner-fee entitlement, and target currency/settlement behavior.
2. PayPal confirms seller onboarding and authorization/capture behavior for the approved merchant configuration.
3. The business selects the percentage after unit-economics work; the application must disclose that configured rate before booking.
4. Legal review confirms the marketplace contract model and customer-facing cancellation, withdrawal, refund, and dispute terms.

The current application remains sandbox-only while those approvals are pending. See [PRODUCT_SPEC.md](PRODUCT_SPEC.md) for the phase plan and launch dependency register.

## Configuration

Set these values in the ignored project `.env` or the production secret manager:

```dotenv
PAYMENT_PROVIDER=paypal_sandbox
PAYPAL_ENV=sandbox
PAYPAL_CLIENT_ID=...
PAYPAL_CLIENT_SECRET=...
PAYPAL_WEBHOOK_ID=...
PAYPAL_PLATFORM_MERCHANT_ID=...
PAYPAL_PARTNER_ATTRIBUTION_ID=...
```

Use the sandbox Platform REST app credentials, the platform business account's PayPal merchant ID, and the PayPal partner attribution (BN) code associated with the platform. Configure the webhook on that same sandbox app, then set its ID in `PAYPAL_WEBHOOK_ID`. Subscribe to `MERCHANT.ONBOARDING.COMPLETED`, `MERCHANT.PARTNER-CONSENT.REVOKED`, `PAYMENT.CAPTURE.COMPLETED`, `PAYMENT.CAPTURE.DENIED`, `PAYMENT.CAPTURE.REFUNDED`, `PAYMENT.CAPTURE.REVERSED`, `CUSTOMER.DISPUTE.CREATED`, `CUSTOMER.DISPUTE.UPDATED`, and `CUSTOMER.DISPUTE.RESOLVED`. Set the callback URL to `https://<app-domain>/api/paypal/webhook`; the endpoint acknowledges only after verification and queue persistence. Keep the webhook ID in `PAYPAL_WEBHOOK_ID`.

## Required before a live payment release

1. Register the webhook against the deployed HTTPS callback with the sandbox or later approved live app, and verify provider delivery, retries and event ordering in that environment.
2. PayPal approves SamenKlaar as a marketplace partner and confirms seller onboarding, seller payment routing, partner-fee entitlement, currency and settlement behavior.
3. Define and validate authorization expiry and reauthorization, seller onboarding, scope-change, cancellation, void/refund, dispute and failure behavior for the approved marketplace product.
4. The business sets and discloses a fee after unit-economics work; legal review confirms marketplace, withdrawal, cancellation, refund and dispute terms.
5. Validate the live merchant account and supported customer/provider markets. Do not promise coverage based on sandbox behavior.

Completion approval and financial payment approval remain distinct decisions. AI receives no money-moving tool and cannot authorize a payment.
