# Agent payment holds ("hold and pay with limits")

Status: design and blocker analysis, 18 October 2026. **Not implemented**: PayPal has not enabled
vaulting on our sandbox platform app (see [Probe result](#probe-result)). Nothing in the codebase
changed except the probe script.

Goal: a customer's agent books a visit with `at_booking` payment timing, so the professional sees a
funded hold before confirming, without the customer approving in a PayPal window each time. The person
keeps approval of finished work and the capture. Agent bookings are forced to `after_completion` today
([AGENT_ARCHITECTURE.md](AGENT_ARCHITECTURE.md), "Known limits").

## Answers

### 1. Can PayPal marketplace vault a buyer's method for a merchant-initiated authorization?

Mostly yes, with two unverified parts.

- **Vaulting a PayPal wallet** is supported for platforms (Vault v3 Payment Method Tokens API, and
  `payment_source.paypal.attributes.vault` on Orders v2). The Netherlands is a supported country.
  The payer approves once in PayPal (a billing-agreement style approval). Use
  `usage_type=PLATFORM`: the token belongs to SamenKlaar and can be used with any onboarded seller,
  which is what a marketplace needs. `MERCHANT` usage ties the token to one seller and would force a
  new consent per professional.
- **Using the token with no payer present** is documented: `POST /v2/checkout/orders` with
  `payment_source.paypal.vault_id`, the platform `PayPal-Partner-Attribution-Id`, and the seller as
  `purchase_units[].payee`. This keeps our existing seller routing.
- **Unverified: `intent=AUTHORIZE` with a `vault_id`.** Every PayPal sample for this path uses
  `intent=CAPTURE`. The Orders v2 reference allows `vault_id` on the wallet source, and PayPal's
  authorize/capture guide has no vault exception, but nothing states that a token order authorizes
  without a payer redirect. This has to be proven in sandbox before building. If it is not supported,
  an authorization hold cannot exist without the buyer present, and the fallback is a vaulted charge at
  approval time (see [Fallback](#fallback-if-authorize-with-vault_id-is-not-supported)).
- **Unverified: seller-routed capture of a vault-backed authorization** with
  `disbursement_mode=INSTANT` (our current capture) behaves the same as a redirect-approved one.
- Holds keep the existing PayPal limits: 29 days valid, 3-day honor period, `reauthorize` allowed inside
  the 29 days (a new authorization ID, no buyer action). Reauthorization of the same amount does not
  count as new spend against the caps below.

### 2. What onboarding and approval does it need?

1. **PayPal must enable PayPal-wallet vaulting on our platform account.** PayPal's docs say you must
   be approved for billing agreements / reference transactions, via the account manager. It is not
   self-service, and is a precondition for any code.
2. **Sellers**: platform-level vaulting uses the same `partner-referrals` onboarding we already run
   (`EXPRESS_CHECKOUT`, `PAYMENT` + `REFUND`). Merchant-level vaulting would add `VAULT` and
   `BILLING_AGREEMENT` features and `ADVANCED_VAULTING` product, which we do not want. Confirm with
   PayPal that already-onboarded sellers need no re-onboarding for platform-level tokens.
3. **Risk data**: PayPal requires FraudNet/Magnes risk data on customer-initiated transactions that
   use tokens. The consent step (customer present) must collect it. Agent-initiated orders are
   merchant-initiated; ask PayPal what metadata they need (`PayPal-Client-Metadata-Id` appears in
   the payer-not-present sample).
4. **Webhooks**: add `VAULT.PAYMENT-TOKEN.CREATED`, `VAULT.PAYMENT-TOKEN.DELETED`,
   `VAULT.PAYMENT-TOKEN.DELETION-INITIATED` to the existing subscription list.
5. **Legal/consumer terms**: the consent text, PSD2/SCA treatment of merchant-initiated holds, and
   cancellation/refund wording need the same legal review already listed in
   [PAYMENTS.md](PAYMENTS.md). Do not promise coverage from sandbox behavior.

### 3. How does a customer set a spending authorization?

PayPal enforces no per-job or total limit on a vaulted token. **The caps are ours to enforce**, and the
consent screen must say so. Caps are recorded with the consent, are never wider than the product
allows, and are checked on the server immediately before every hold (the mandate is not trusted from
the snapshot).

## Probe result

Sandbox, using the platform app already in the local ignored `.env`. The app's token carries the
`vault/payment-tokens/readwrite` scope, but both vault entry points are refused:

| Call | Result |
| --- | --- |
| `POST /v3/vault/setup-tokens` (PayPal, `usage_type=PLATFORM`) | `403 NOT_AUTHORIZED`, issue `NOT_ENABLED_TO_VAULT_PAYMENT_SOURCE` |
| `POST /v2/checkout/orders` (`AUTHORIZE`, `vault.store_in_vault=ON_SUCCESS`) | `422`, issue `NOT_ENABLED_TO_VAULT_PAYMENT_SOURCE` |

So the first blocker is account enablement, not code. [scripts/probe-paypal-vault.sh](../scripts/probe-paypal-vault.sh)
repeats these two calls (sandbox only, reads the local `.env`, prints no secrets). Run it after PayPal
confirms enablement; it should return `201`/`PAYER_ACTION_REQUIRED` for the setup token.

A sandbox buyer approval also needs someone to sign in to the sandbox buyer account in a browser; the
rest of the proof can be scripted.

## Proposed minimal change

Keep the workflow untouched where possible: `at_booking` already blocks professional confirmation until
`record-payment-authorization` is applied, and approval of work (`approve-completion`) and capture
(`confirm-payment` after `ValidatePaymentApproval`) already belong to the customer.

### Data

New table `agent_payment_consents` (one active row per customer account; history kept):

| Column | Meaning |
| --- | --- |
| `id`, `account_id`, `created_at`, `revoked_at` | identity and lifecycle |
| `paypal_customer_id`, `paypal_payment_token_id` | vault references (never shown to the client) |
| `payer_label` | masked PayPal email for the settings screen |
| `per_job_cap_minor` | most one hold may be. At most the mandate price limit, at most 10,000 |
| `total_cap_minor` | most that may be held or captured under this consent in total |
| `expires_at` | consent expiry. Default 90 days, at most 365 |
| `mandate_version` | mandate version when granted |
| `consent_text_version` | which wording the person accepted |

New table `agent_payment_ledger`: `consent_id`, `workspace_id`, `job_id`, `scope_version`,
`amount_minor`, `status` (`reserved`, `held`, `captured`, `released`), unique on
`(job_id, scope_version)`. **Spent against `total_cap_minor` = sum of `reserved`, `held`, `captured`.**
Voids, expiry and refunds move a row to `released`; a capture is never released by a hold worker.
Reserve the row inside the same transaction that decides to hold, before calling PayPal, so two
concurrent runs cannot both fit the cap.

Mandate gains one field, `payWithHold bool` (customer). Caps live on the consent, not the mandate, so
the mandate JSON keeps its current shape. Raising a cap, or the expiry, requires a new consent screen
(the person confirms again); lowering or revoking does not need PayPal except to delete the token.

### Decision engine (`internal/workflow/agent.go`)

`AgentSnapshot` gains a small read-only `PaymentConsent` view (active, per-job cap, remaining total,
expires at). In `book-visit` and `accept-visit`, the timing becomes `at_booking` only when all of these
hold, otherwise `after_completion` exactly as today:

- `mandate.PayWithHold` and the consent is active and not expired
- the quote total (after any accepted price) is at most the per-job cap and at most the remaining total
- the consent outlives the visit date plus the 3-day honor period margin (else reauthorize is likely)
- the seller is `verified` for PayPal

The decision log records which condition chose `after_completion`, so the person sees why the agent
asked for a hold or not. An agent never chooses `at_booking` for scope increases, which already use
`after_completion`.

### Runtime (`internal/httpapi/agent_runtime.go`) and adapter

After a successful booking with `at_booking`, a new runner step `hold-payment`:

1. re-check consent, caps, expiry and mandate version on the server and reserve the ledger row;
2. adapter `HoldWithVault(ctx, state, consent)`: create the order (`AUTHORIZE`, seller payee,
   `vault_id`, same `PayPal-Request-Id` scheme as today) and authorize it server-side;
3. on success call the existing `recordBookingAuthorization` (same `record-payment-authorization`
   action, same recovery and reconciliation paths as a redirect authorization);
4. on any failure, release the ledger row, set timing back via `defer-payment-authorization`, and
   escalate with `payment_hold_failed` so the person can authorize in PayPal as today.

Nothing else changes: the agent never calls capture, approval of completed work stays an escalation
for the person, and `ValidatePaymentApproval` is still required before `Capture`. This moves the line in
[PAYMENTS.md](PAYMENTS.md) ("AI receives no money-moving tool"): the agent can now create a hold within
the consent, but cannot capture or approve work.

### UI (one-time consent)

"Your agent" settings gets a "Let my agent hold payment" card: per-job cap, total cap, expiry (defaults
from the price limit), plain statement that PayPal does not enforce the caps, that holds are not
charges, that capture still needs approval of finished work, and a revoke button. "Connect PayPal"
calls `POST /api/agent/payment-consent`, which creates the setup token, collects risk data, and returns
the approve URL; the return route exchanges the setup token for a payment token and activates the
consent. After that the agent can hold with no PayPal window. The request page shows the hold exactly
as a booking-time authorization today, plus "held by your agent within your limit".

### Fallback if `AUTHORIZE` with `vault_id` is not supported

Without a hold, an agent cannot prove funds at booking. Options, none of which is "hold and pay":
(a) keep pay-after-completion, but let the person's approval of finished work trigger a vaulted
authorize-and-capture with no PayPal window (removes the redirect, not the risk of a failed charge
after the work is done); (b) a vaulted CAPTURE at booking, which moves money before the work and is a
different refund and consumer-law model. Recommend (a) and decide only after the sandbox proof.

## Plan once unblocked

1. PayPal enables wallet vaulting (and answers questions 1 to 3 above). Run the probe script.
2. Sandbox proof by hand: setup token, buyer approval, payment token, vaulted `AUTHORIZE` order for a
   seller-routed payee, `authorize`, `reauthorize`, `capture` with `INSTANT`. Record request/response
   shapes in this note.
3. Adapter: setup-token, payment-token exchange, token delete, `HoldWithVault`, webhook handling for
   token deletion; contract tests in `internal/adapters/paypal` in the style of the existing ones.
4. Schema, ledger and consent API; decision-engine changes with table tests (cap edges, expiry, ledger
   races, revoked consent); runtime step with failure fallbacks.
5. Settings UI and request-page copy (English and Dutch).
6. Legal review of consent wording; update [PAYMENTS.md](PAYMENTS.md) and
   [AGENT_ARCHITECTURE.md](AGENT_ARCHITECTURE.md).

## Sources

- [Save PayPal with the Payment Method Tokens API (platforms)](https://developer.paypal.com/docs/multiparty/checkout/save-payment-methods/purchase-later/payment-tokens-api/paypal/)
- [Save PayPal with the Orders API (platforms)](https://developer.paypal.com/docs/multiparty/checkout/save-payment-methods/during-purchase/orders-api/paypal/)
- [Platform-level onboarding](https://developer.paypal.com/docs/multiparty/checkout/save-payment-methods/onboarding/platform) and [merchant-level onboarding](https://developer.paypal.com/docs/multiparty/checkout/save-payment-methods/onboarding/merchant/)
- [Authorize a payment and capture funds later](https://developer.paypal.com/docs/multiparty/checkout/standard/customize/auth-capture/)
