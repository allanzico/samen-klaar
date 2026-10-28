# Assistants and delegated decisions

Every customer and professional has an assistant with a versioned mandate. It can clarify, negotiate, suggest visits and coordinate within that person's limits. Human choices remain explicit at professional selection, price/options review and initial PayPal authorization. Completion/payment can be delegated only through the per-booking permission described below.

## Decision engine and runner

`internal/workflow/agent.go` contains `DecideAgent`, a pure decision over a request snapshot, role and mandate. It returns act, wait or escalate without side effects. `internal/httpapi/agent_runtime.go` runs bounded turns for both assistants through the same validated action path as a human button. The runner executes after actions and in a periodic sweep; it does not bypass ownership, scope, booking or payment validation.

PostgreSQL stores `agent_mandates`, `agent_decisions` and `agent_escalations`. Decision records identify the mandate version. A per-request in-process lock prevents overlapping runner turns; the API currently runs one replica. Persisted actions still use database locking. An action rejected by the workflow becomes an actionable question rather than an endless retry loop.

Typed replies are read by a bounded built-in English/Dutch parser, with the configured AI provider as a sanitized intent reader when needed. Free text is data; it cannot grant itself payment permission or invent a verified provider response. Address replies use canonical location resolution instead of sending the private address to a general reply-reading model.

## Defaults and private conversations

The default customer mandate uses the budget named in the conversation, with editable price ceiling, allowed days/hours and horizon. Without an approved price, the assistant asks. Professional defaults allow bounded price negotiation and visit confirmation within published working hours; saved preferences can narrow/pause that authority. Permissions outside weekly hours are separate from published availability and never override days off or overlapping reservations.

Each assistant speaks its person's selected language and sees the applicable private dialogue. Shared price/date offers use stable structured values and are localized for the reader. Stable short assistant names come from `internal/workflow/agent_names.go` and `src/lib/agent-names.ts`.

An open question creates an in-app notification in the same transaction. Routine assistant questions do not create email intents. Assistant preferences, the active question, conversation and decision history expose what the assistant is doing and when it needs a person.

## Review stages cannot be skipped

1. Matching leaves the request on results. Only the customer can select a professional; `bookWithoutAsking` does not delegate that selection.
2. `professional_review` shows the service, price, materials and exclusions. Both assistants wait for the customer's explicit confirmation.
3. `plan_review` allows a lower offer and the completion/payment choice. The professional assistant may answer a submitted offer; customer planning cannot replace this screen with an address question.
4. `start-planning` requires the current job ID and current review choice. Before a visit/work/payment commitment, the customer can return to review, results or intake. Old active questions are closed on rewind.

## Price and visit negotiation

Offers have durable identities, amounts, actors and statuses. Assistant acceptance must satisfy its person's mandate. A price below the professional's floor waits for that professional; the customer can revise or withdraw the offer through the supported actions. Bounded proposal attempts prevent autonomous haggling loops.

The customer assistant asks for availability, offers matching times and waits for approval unless visit booking was separately delegated. A request for another day records the rejected dates, avoids presenting them again and proposes within the customer's new preferences. The other assistant may accept within its authority, counter within its limits, or ask its person. A proposal is not a booking until accepted through the reservation workflow.

Booking decline, expiry or professional cancellation can return the customer to alternatives. Late moves/cancellations, changes involving a payment hold, started work and operator review use the human/operator path. Assistants do not resolve disputes or independently issue refunds.

## Per-booking completion and payment authority

The customer explicitly selects manual review/payment or automatic review/release before the booking. No automatic payment choice is preselected. Delegation snapshots job/scope, professional, amount and currency, requires an initial buyer-approved PayPal authorization, and can be disabled before release starts. Legacy work-approval preferences alone do not confer payment authority.

The professional submits photos/documents and a specific work note for the current scope and completion round. Check and send saves the draft and validates documentation before sending it for customer review. Missing or unclear evidence asks the professional to improve the record or request the supported human review. It must not leave the customer trying to repair an incomplete professional submission.

Once documentation passes, the customer's assistant may approve completion and payment only inside the saved booking permission. The payment adapter/store then revalidate authorization, scope, amount, seller, current agreement, dispute/cancellation state and operation identity before capture. Stale or uncertain state blocks release and is reconciled. A provider-model response cannot itself declare a paid job.

The assistant cannot create a fresh or replacement PayPal hold: the buyer approves it in PayPal. Saved-wallet vaulting is unavailable in the current platform configuration. [PAYMENTS.md](PAYMENTS.md) documents the separate authorization and capture operations.

## Limits

Photos and notes document a claim; they do not certify workmanship or safety. Ambiguous evidence returns to people. Public ratings, routed driving-time ranking, external calendar integration, browser push, cross-request availability memory and multi-replica agent coordination are not implemented. Reconciliation and operator escalation preserve financial history when a request cannot continue automatically.
