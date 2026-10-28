# PayPal payments

## Implemented flow

SamenKlaar uses PayPal marketplace seller onboarding and the Orders authorization/capture flow. All payment operations are **sandbox only**, including on the hosted product. The server rejects a live payment provider configuration. A normal buyer checkout does not require a customer to onboard as a seller.

1. The professional connects a separate receiving wallet through a partner referral. The server verifies the merchant/tracking relationship, confirmed email, receivability and required permissions with PayPal before saving payment eligibility. Callback flags alone are insufficient.
2. The customer confirms the service, price and visit. They choose authorization at booking or after manual completion approval. Delegated review/release requires the booking-time path.
3. The server creates an `AUTHORIZE` order for the current scope and verified professional, with fixed-price, no-shipping checkout. The buyer explicitly approves the initial or replacement authorization in PayPal. This creates a hold, not a capture or completed payment.
4. Completion approval and payment approval are separate persisted gates. The customer can act personally or grant the assistant permission for both for that specific booking. Automatic release requires a current successful documentation review, matching scope/amount/currency/professional, a verified usable authorization and no blocking dispute, cancellation or payment operation.
5. The server captures the authorization against the verified seller and stores the result. A completed report identifies the approval actors and payment reference. Provider responses and webhooks reconcile durable state; a return URL alone cannot record payment.

Photographs can document visible changes but cannot certify workmanship, safety or a real water test. Missing, unclear or stale documentation blocks delegated release. The professional must improve it or use the human-review path. Customers can disable delegation before release starts.

## Configuration and wallets

Set `PAYMENT_PROVIDER=paypal_sandbox`, `PAYPAL_ENV=sandbox`, `PAYPAL_CLIENT_ID`, `PAYPAL_CLIENT_SECRET`, `PAYPAL_PLATFORM_MERCHANT_ID` and `PAYPAL_PARTNER_ATTRIBUTION_ID` on the Go server. The platform app needs the marketplace features and permissions used by its seller referrals. Keep all identifiers and credentials out of client bundles and public files.

Use a Personal sandbox buyer with sufficient balance and a distinct receiving sandbox wallet. A pre-created Business receiver makes setup predictable. The receiver must differ from the buyer and platform API merchant. Connect it through the professional profile, finish PayPal consent and email confirmation, then use Check status until the backend verifies eligibility. Partner-fee permission is required for new fee-bearing agreements. Receiving-account eligibility depends on the platform's country/account settings and PayPal permissions; do not infer readiness from account type alone.

Seller-status verification uses the platform bearer token and attribution header. Seller assertions belong on seller-scoped money operations, not the partner management status endpoint. Failed connection verification retains the returned merchant reference for Check status/reload retries. Public errors are allowlisted; diagnostic logs omit credentials, referral URLs, seller identifiers and raw provider bodies.

Register the webhook on the **same sandbox platform application** at `https://your-domain/api/paypal/webhook` and set `PAYPAL_WEBHOOK_ID`. Subscribe to the seller/onboarding and checkout/payment/dispute event types handled by `internal/adapters/paypal/webhooks.go`; the route verifies each transmission with PayPal before enqueueing it. For local webhook verification, use a reachable HTTPS origin and update `APP_BASE_URL`, Auth0 callback/logout settings and the registered webhook accordingly. Merely setting an ID does not make localhost reachable.

Saved buyer wallets and assistant-created fresh holds are not implemented: platform vaulting is not enabled. Each initial or replacement hold still requires buyer approval in PayPal. This does not prevent an assistant from capturing an existing buyer-approved hold inside the customer's explicit booking permission.

## Professional platform fee

New agreements snapshot a 5% fee on the professional's agreed total, including call-out. The customer total stays unchanged. Amounts are integer minor units: at €105, the fee is €5.25 and professional proceeds are €99.75 before PayPal processing fees. Older agreements without a fee snapshot remain zero-fee.

The capture includes PayPal partner-fee instructions for the stored scope. Capture/refund validation, payout summaries and professional earnings use the same snapshot. Full refunds include the platform-fee return; partial refunds and uncertain provider outcomes require the supported reconciliation/operator path. Receiving-wallet net proceeds can differ because PayPal processing fees are separate.

## Recovery and retained safeguards

- PostgreSQL stores request IDs, order/authorization/capture/refund identities and operation status. Retries reuse the original provider idempotency keys. An uncertain response is reconciled before another operation can claim the payment.
- Recovered authorizations must match workspace, job, scope, amount, currency, seller and request identity. A stale callback cannot approve a replacement booking or canceled request.
- Booking cancellation, decline and expiry transactionally enqueue release of the exact authorization attempt. A retry cannot void a replacement attempt. Scope changes invalidate prior approval and arrange release/reapproval as appropriate.
- Webhooks are provider-verified, deduplicated and processed through durable workers. Reversals, refunds, revoked consent and disputes update or block the applicable state.
- Authorization usability is rechecked before capture. An expired/unusable authorization requires fresh buyer approval; an assistant cannot fabricate or silently renew it.
- Operators use audited case/refund actions. Client-supplied booleans cannot assert verified authorization, completed capture or seller eligibility.

## Verification and release limits

`make check-flow` covers the provider contract and persisted HTTP journeys against disposable PostgreSQL, including lost responses, restart/recovery, stale callbacks, seller binding, fees, refunds, dispute/cancellation guards and per-booking autonomy. These provider fixtures are distinct from external wallet verification.

The recorded 28 October 2026 journey used one real sandbox buyer/seller transaction: a negotiated €105 hold, professional photos/work note, successful documentation review, delegated completion/payment approval and capture. Both application reports recorded the same completed payment. Generated photos and the work note were explicitly fictional; no real repair or live settlement was claimed.

Before live money, obtain PayPal partner approval, confirm the intended seller/country and fee capabilities, verify settlement/refunds/webhook and longer authorization windows with the approved account, and complete professional/consumer-policy and operational review. Passing sandbox tests is not live-payment approval.

Provider references: [seller onboarding](https://developer.paypal.com/docs/multiparty/seller-onboarding/before-payment/), [delayed disbursement / authorization guidance](https://developer.paypal.com/docs/checkout/standard/customize/authorization/), [sandbox accounts](https://developer.paypal.com/tools/sandbox/accounts/).
