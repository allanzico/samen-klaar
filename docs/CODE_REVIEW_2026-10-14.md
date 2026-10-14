# SamenKlaar code review and implementation handoff

Review date: 14 October 2026. Scope: the complete available application in `/Users/allanakanyijuka/Documents/Github/samen-klaar`, including untracked files. This is a review and repair backlog, not an implementation change.

**17 actionable findings: 6 P1 and 11 P2.** Fix booking, payment, request-selection, and professional-eligibility defects first. Each finding below includes an affected scenario, a suggested fix, and an acceptance test. “Reproduced” means a focused probe was run; “code path confirmed” means the relevant implementation and callers were inspected, without executing that particular scenario.

## Findings

### R01 — [P1] Retain the reservation while late cancellation awaits a decision — `internal/adapters/postgres/store.go:359`

A customer cancelling less than 24 hours before the visit creates a pending cancellation review and leaves `state.Cancelled` false (`internal/workflow/cancellation.go:26`). The persistence layer nevertheless releases the booking after every successful `cancel-request`. The product still says the visit is scheduled, while another customer can reserve its time. The operator's keep-visit decision does not restore the reservation.

**Evidence:** reproduced against disposable PostgreSQL: `cancelled=false review=pending reservation=released slot=available`.

**Suggested fix:** release the reservation only when cancellation actually becomes effective. Keep the existing reservation throughout pending review and a keep-visit decision; retain the explicit release on operator approval. Make these state transitions and reservation updates one transaction.

**Acceptance test:** request late cancellation; verify the reservation remains booked and a second customer cannot claim the time. Keep the visit and check again. In a separate case, approve cancellation and verify release.

### R02 — [P1] Commit request identity and displayed state together — `src/state/create-app-controller.ts:294`

`selectRequest` changes `app.workspaceId` and the remembered URL before loading the new state. If loading request B fails, the UI still displays request A, but subsequent `act` calls target B. This can apply an address, message, or approval to the wrong request within the user's account. Overlapping selections also lack a response-generation check.

**Evidence:** a focused controller probe produced `displayedJob=job-A`, `actionSentTo=B`, `wrongRequest=true` after a failed selection.

**Suggested fix:** fetch into a local result, then commit the selected ID, state, URL, draft, and payment state together. Alternatively clear the old view and block actions until the new request is loaded. Reject stale responses using a selection generation or abort signal, including payment-status responses.

**Acceptance test:** load A, fail B's state request, then attempt an action; it must either still target and display A or remain blocked. Resolve two successful selections out of order and verify the last selection wins.

### R03 — [P1] Block scope changes until the preceding payment outcome is known — `internal/adapters/paypal/service.go:763`

`VoidCurrent` returns success for every payment status other than `authorized`, including authorization/capture/void operations that are still running or uncertain. The `approve-change` handler treats that success as permission to change the scope (`internal/httpapi/state_handlers.go:187`). After a timeout, a retry can therefore change the payable scope without releasing the existing hold or resolving an earlier capture. A captured operation can also be bypassed if job payment state has not yet caught up.

**Evidence:** a focused Go probe accepted a scope change for `authorizing`, `authorization_uncertain`, `capturing`, `capture_uncertain`, `captured`, and `void_uncertain`.

**Suggested fix:** replace the broad early return with explicit state handling. Unresolved operations must stop the change and reconcile first; an authorized amount requires a confirmed void. Captured payments need their payment/refund resolution, not a successful no-op. Coordinate this guard with R04 so a new payment operation cannot begin between the guard and scope update.

**Acceptance test:** exercise all payment states, including a failed void followed by a second approval attempt. Scope and amount must remain unchanged until the preceding payment is safely resolved.

### R04 — [P1] Recheck current workflow eligibility when reserving a capture — `internal/adapters/postgres/payments.go:82`

The capture handler loads and validates the workspace, then performs additional work before claiming the payment operation. `ClaimPaymentIntent` only compares the payment row's status. A concurrent `report-issue` can commit an open dispute after validation but before the capture claim; the capture still proceeds from the stale state even though the customer's issue says payment is paused. A later failed `confirm-payment` does not undo the external capture.

**Evidence:** code path confirmed through `internal/httpapi/payment_handlers.go:91`, `internal/adapters/paypal/service.go:562`, and `internal/workflow/actions.go:62`. No concurrent-provider test was run for this interleaving.

**Suggested fix:** use a short database transaction to lock the workspace and payment operation in a consistent order, revalidate scope, approval, amount, and dispute state, and persist the in-flight capture claim. Make conflicting workflow commands respect that claim. Release the transaction before calling PayPal; reconcile the durable operation afterward. Do not hold a database lock across network I/O.

**Acceptance test:** pause capture before its claim, commit a dispute, then resume; no provider capture may be sent. Test the inverse ordering and ensure the issue action accurately reports an already-started payment rather than promising a pause it cannot enforce.

### R05 — [P1] Repair workspace state when a captured payment is replayed — `internal/adapters/postgres/paypal_webhooks.go:159`

Capture persistence and the workspace's `confirm-payment` action are separate writes. A crash or failed second write can leave the payment operation `captured` while the job remains unpaid. The matching capture webhook then returns success immediately because the operation already has that capture ID, skipping workspace reconciliation and marking the event processed.

**Evidence:** reproduced against disposable PostgreSQL: `webhook processed=true jobPaid=false`.

**Suggested fix:** make capture reconciliation idempotent across both the operation and workspace. An existing matching capture should still ensure the workspace reflects payment, without duplicating audit events or notifications. Apply the reservation-completion update used by normal `confirm-payment` as part of the same reconciliation, too.

**Acceptance test:** persist a captured operation with an unpaid workspace, deliver a verified matching webhook, and assert payment, transaction ID, and reservation completion. Replay it and assert no duplicate business events.

### R06 — [P1] Reassess professional eligibility when the approved trade changes — `internal/adapters/postgres/professionals.go:80`

An active professional can update their category and service while the upsert preserves `active` status. Review checks reset only for pending profiles. A provider approved for one trade can therefore publish a different trade using the previous qualification and insurance approval, bypassing the eligibility checks that initial onboarding requires.

**Evidence:** reproduced against disposable PostgreSQL: changing an approved profile to `Electrical assessment` returned `status=active publicly bookable=true`.

**Suggested fix:** distinguish commercial edits such as price from changes affecting approved identity, trade, qualifications, or insurance. Invalidate the affected approval and withhold the changed offer from new matching until the existing verification policy is satisfied. Preserve already-booked service snapshots. This does not require adding manual review to ordinary customer job descriptions.

**Acceptance test:** an approved provider's trade change cannot immediately appear as an approved electrical service. A price-only edit should retain eligibility unless an explicit policy requires otherwise.

### R07 — [P2] Recover a persisted refund claim after interruption — `internal/adapters/paypal/service.go:805`

`Refund` saves `refunding` before the provider call, but subsequent calls accept only `captured` or `refund_uncertain`. `Reconcile` has no `refunding` recovery branch. A crash after the claim, or a database failure saving the result, leaves the refund blocked indefinitely through the application's normal actions. If the process stopped before contacting PayPal, no webhook can repair it.

**Evidence:** code path confirmed in `Refund`, `claimIntent`, and `Reconcile`; process-crash injection was not run.

**Suggested fix:** give the durable operation a recoverable claim/lease and reconcile interrupted refunds using the saved request identity and any saved refund ID. Reuse the original idempotency key; do not make a fresh refund request identity merely to escape the stuck state. Preserve provider results needed to reconcile pending refunds.

**Acceptance test:** interrupt after the claim but before HTTP, and after HTTP but before saving the result. Restart and retry; both cases must converge to one provider refund and one durable final result.

### R08 — [P2] Require an uploaded asset before accepting completion evidence — `internal/workflow/actions.go:173`

The domain checks only `len(job.Evidence) > 0`. `add-evidence` appends a text note without an asset ID, so a professional can submit that note as the only evidence and continue through customer completion and payment approval. The frontend's upload requirement does not protect the API.

**Evidence:** a focused workflow probe completed with `uploaded assets=0, completionApproved=true, paymentApprovedVersion=1`.

**Suggested fix:** enforce the actual upload requirement in the workflow using a validated stored asset, separately from the work-note requirement. Keep the private upload endpoint as the only route for recording assets; define how evidence applies to later scope versions.

**Acceptance test:** note-only evidence must fail submission through the API. A valid private upload plus the required note must permit the intended completion sequence. A client-supplied fake asset must remain rejected.

### R09 — [P2] Send professional appointment decisions to the customer — `internal/adapters/postgres/notifications.go:257`

`confirm-appointment` and `decline-appointment` are grouped with customer actions and always notify the professional. The professional receives their own decision while the customer receives no corresponding notification. Polling an open page does not repair the missing inbox/email event.

**Evidence:** a focused recipient probe returned `professional` for both professional actions.

**Suggested fix:** route these events to the customer. Add table-driven coverage for action, acting role, recipient, and event type so new actions cannot silently inherit the wrong audience.

**Acceptance test:** professional confirmation and decline each create the expected customer notification and no redundant self-notification.

### R10 — [P2] Advance reminder batches beyond already-notified jobs — `internal/adapters/postgres/scheduling.go:69`

The scheduler selects the oldest 100 eligible workspaces. Enqueuing an idempotent notice changes neither their eligibility nor their ordering. If those jobs remain overdue, every pass selects the same 100, leaving later eligible jobs without reminders indefinitely.

**Evidence:** a disposable-database probe created 101 eligible jobs and ran three passes; the 101st still had zero reminder notifications.

**Suggested fix:** exclude already-enqueued notices for the relevant lifecycle timestamp before applying the limit, or persist a due/processed cursor that advances fairly. Keep unique event keys and transactional insertion; account for parallel workers.

**Acceptance test:** more than 100 persistent eligible jobs must all receive their applicable notices within bounded passes, with no duplicates on subsequent passes or concurrent workers.

### R11 — [P2] Continue draining notifications after delivery becomes configured — `internal/adapters/postgres/notifications.go:200`

Startup recovery includes `adapter_unconfigured` and failed notifications but queues at most 100. The recurring recovery calls the function with `includeAll=false`, which admits only `delivery_interrupted` failures. A backlog larger than 100 accumulated before SMTP was configured therefore remains undelivered after one startup, requiring repeated restarts to drain it.

**Evidence:** code path confirmed through startup recovery at line 179 and recurring recovery at line 236.

**Suggested fix:** repeatedly drain bounded batches of notifications that become eligible when delivery is configured. Define retry/backoff and terminal-failure policy instead of indiscriminately retrying permanent failures. Preserve outbox/job deduplication.

**Acceptance test:** create 250 `adapter_unconfigured` notifications, configure a fake sender, start one worker process, and verify the whole eligible backlog drains without further restarts.

### R12 — [P2] Preserve mutation success when refreshing the request list fails — `src/state/create-app-controller.ts:85`

`act` handles the persisted mutation and the subsequent workspace-list refresh in one `try` block. If the mutation succeeds but the list request fails, the function reports an action failure and returns no success value. `sendMessage` consequently keeps the draft, encouraging the customer to repeat an already-saved message or proposal.

**Evidence:** a focused controller probe saved the action once, then rejected the list refresh; the draft remained and the UI reported that the action could not be completed.

**Suggested fix:** treat the mutation's returned state as the success boundary. Clear or commit the relevant draft immediately; retry secondary refresh independently and show a distinct refresh warning when useful. Use request idempotency for writes that must survive response loss, rather than treating a sidebar refresh failure as a failed command.

**Acceptance test:** successful message submission followed by a failed list request must show the saved message, clear the submitted draft, and avoid prompting a duplicate submission.

### R13 — [P2] Preserve an unsaved address across background polling — `src/features/customer/CustomerJobPanel.svelte:24`

The effect copies `job.serviceAddress` into the editable field whenever the `job` object changes. The five-second refresh in `src/App.svelte:60` replaces that object, so a customer typing an address can have their input repeatedly replaced by the last saved value.

**Evidence:** code path confirmed through the effect, controller refresh, and polling timer. No browser automation was run.

**Suggested fix:** track which request the form belongs to and whether it is dirty. Initialize on request selection, update pristine fields from the server, and reset dirty state after a successful save. Do not overwrite current input on an unrelated refresh.

**Acceptance test:** type an address across at least two poll intervals; it must remain intact and save successfully. Switching requests must initialize the new request's address rather than leak the prior draft.

### R14 — [P2] Make accepted evidence uploads usable by the review endpoint — `internal/httpapi/evidence_handlers.go:157`

Uploads accept individual files up to 10 MB without an equivalent aggregate constraint, but AI review always reads all uploaded assets and rejects more than five files or more than 12 MB total. Six small uploads or two 7 MB uploads are accepted, then make review fail with HTTP 413. There is no selection or removal flow to get that job back within the review limit. This blocks AI review, not the entire manual approval flow.

**Evidence:** upload and review code paths confirmed, including the aggregate check at line 173.

**Suggested fix:** support explicit bounded review batches or a current-scope selection, and allow replacement/removal where workflow rules permit. If the intended product limit is global instead, enforce and explain that limit before accepting the uploads. Include prior-scope evidence in the design decision.

**Acceptance test:** cover six small files, two individually valid files totalling over 12 MB, and evidence retained after a scope change. Users must have a working path to review the permitted evidence.

### R15 — [P2] Identify evidence coverage by stable work items rather than quote labels — `internal/ai/coordinator.go:117`

Evidence review requires one coverage entry per quote line while rejecting duplicate normalized labels. Each accepted price negotiation appends another line labelled `Agreed price adjustment` (`internal/workflow/negotiation.go:57`). After two negotiated adjustments, no response can satisfy both the required length and the unique-label validation. Price adjustments also are not physical work for a photo to demonstrate.

**Evidence:** code path confirmed; the constraints are mutually unsatisfiable when quote labels repeat. No live AI call was needed.

**Suggested fix:** separate work-scope items from the price ledger and give work items stable IDs. Request and validate evidence coverage against those IDs. Keep financial adjustments out of physical evidence coverage.

**Acceptance test:** negotiate and accept two price adjustments, then review evidence with a deterministic provider fixture. Coverage must validate for the actual work items while unknown or duplicated work-item IDs remain rejected.

### R16 — [P2] Filter local eligibility before truncating the service catalog — `internal/adapters/postgres/services.go:14`

`ServiceCatalog` returns up to 200 service variants without an area filter. The AI selects up to four variants, and only then does `ServiceMatches` filter professionals by location. A valid local service can be omitted by the global limit or lose all selector positions to services available elsewhere, producing a false local waitlist result despite an eligible provider.

**Evidence:** code path confirmed in `internal/matching/matching.go:38` and `:62`, and the later area filter in `services.go:62`. This concerns the current location-enabled implementation; it does not request maps or a larger location subsystem.

**Suggested fix:** apply the same area, status, and self-booking eligibility before grouping and limiting the catalog. Keep the bounded model input, but build it from providers who can actually serve the request. Avoid duplicating area-normalization rules across the two queries.

**Acceptance test:** populate more than 200 nonlocal variants plus a valid local service and verify the local service reaches selection. Also test a local service when all higher-ranked global variants are nonlocal.

### R17 — [P2] Stop retaining an unused payment history cache in persisted mode — `internal/adapters/paypal/service.go:269`

With the database intent store configured, every intent load reads the database and also copies the record into `p.intents`. Future persisted loads never use that cache, while create/update paths keep adding entries keyed by workspace, job, and scope. There is no eviction. A long-lived process therefore retains payment history in memory with no read benefit; growth follows distinct payment scopes, not merely concurrent activity.

**Evidence:** code path confirmed in `loadIntent`, `createIntent`, `updateIntent`, and `claimIntent`. No production memory-growth rate is claimed.

**Suggested fix:** use the map only for the explicit in-memory/test implementation. Persisted mode should rely on its store without mirroring every record. If caching becomes necessary, introduce it only with a measured purpose, a bound, and a clear invalidation policy.

**Acceptance test:** loading many distinct persisted payment operations must not grow the in-memory fallback map. Existing in-memory service tests must retain their behavior.

## Smaller cleanup and optimization backlog

These are separate from the 17 functional findings and should follow correctness repairs.

| Area | Concrete observation | Suggested next step |
| --- | --- | --- |
| Schema drift | `db/schema.sql`, used by `sqlc.yaml`, omits the migrated notification delivery fields/status, payment refund/seller fields, and newer PayPal tables. It is not a reliable current-schema reference. | Use ordered migrations as the generation source or generate a canonical snapshot from a migrated empty database; check drift in CI and regenerate models. Runtime deployment should continue using migrations. |
| Request-list cost | `ListWorkspaces` in `internal/adapters/postgres/store.go:187` loads and unmarshals full state JSON for every historical request to return summaries. The frontend repeatedly refreshes this list. | Add bounded pagination and summary projection, then measure with realistic conversation histories. Consider revisions/conditional polling before introducing another datastore or messaging layer. |
| Repeated history work | `persistScopeHistory` revisits all scope snapshots during ordinary state persistence, relying on conflict handling for old entries. | Persist only newly introduced versions, retaining transactional history integrity. Benchmark first if histories are normally small. |
| Dead or misleading API surface | Repository search found no callers for `ValidateProfileCurrency`; `SuggestedRoute` has no active use. The former's presence can misleadingly suggest currency validation is enforced. | Remove unused elements after a final caller check, or wire the required validation into the actual boundary and test it. Do not remove the older matching function blindly: seed benchmarks/tests still call it. |
| Authentication-record retention | Expired sessions and abandoned `oidc_transactions` are excluded from reads, but there is no periodic expiry cleanup. The OIDC consume delete only matches unexpired records. | Add bounded indexed cleanup and a retention test. This is storage maintenance, not evidence that expired credentials are accepted. |
| Area normalization | Eligibility rules are repeated in Go and SQL, including hard-coded Amsterdam aliases. | Give normalization one tested contract and use canonical values in queries. Preserve the current launch-area scope. |

## Suggested repair order and handoff rules

1. **Booking and trust:** R01, R02, R06. These protect the selected request, promised time, and approved professional identity/trade.
2. **Payment integrity:** implement R03–R05 together, then R07. Agree transaction/claim ordering and reconciliation invariants before editing individual handlers. Preserve saved provider request identities.
3. **Completion and notifications:** R08–R11, R14–R15. Add domain and database tests at the boundary where each defect occurs.
4. **Usability and scale:** R12–R13, R16–R17, then the smaller cleanup items.

Keep the existing Go service, PostgreSQL, and deployment model. These findings do not justify microservices, a replacement workflow framework, or a larger orchestration system. Prefer a small shared payment-reconciliation operation and explicit domain invariants to more scattered status checks.

Before modifying a cited file, re-read it: another task was actively changing marketplace integration and its fixtures during this audit. Preserve that work and unrelated untracked files. Add focused regression tests alongside each repair. No additional walkthrough application or Make target is needed; use the existing tests and product flow.

## Scope, evidence, and verification

The repository's HEAD was `020a11c` (`initial commit`), and only the original README was tracked. Consequently the review covered the working application, not a conventional small committed diff. Coverage included authentication and role checks; booking, negotiation, cancellation, completion, and payments; database persistence and jobs; AI selection and evidence validation; customer/professional state handling; seeding; and Docker/Make deployment. Not every generated file, CSS rule, or test line was individually audited.

Focused probes were placed in temporary source snapshots outside the checkout. Database probes used a disposable PostgreSQL 16 container with in-memory storage. No Auth0 users were created, no real PayPal orders/refunds were sent, and no customer application database was modified. The frontend probe used mocked API responses; it was not a browser end-to-end test. Live Auth0, AI, object-storage, email, and PayPal marketplace contracts remain integration risks that local fixtures alone cannot certify.

| Check | Result |
| --- | --- |
| `TZ=UTC TEST_DATABASE_URL=<disposable-db> go test -p 1 -count=1 -json ./...` | Final run: **119 tests/subtests passed, 1 skipped, 0 failed**. Uses local fixtures for external providers. |
| `TZ=Europe/Amsterdam ... go test ./internal/httpapi -run '^TestFullJourney$' -count=1` | Fails at `journey_test.go:566` because the preceding assertion compares formatted timestamps instead of instants. See below. |
| `npm run check` | Final run: **0 errors, 0 warnings**. |
| `go vet ./...` | Passed during the review. |
| `python3 docker/deploy_test.py` | **4 tests passed**. |

The timezone failure is a **test portability defect**, not demonstrated reservation loss: `internal/httpapi/journey_test.go:565` compares `persistedStart.Format(time.RFC3339)` with the UTC string `bookedStart`. The same instant has a different textual offset in Europe/Amsterdam. Parse the expected timestamp and compare with `Time.Equal`; run the journey under both UTC and Europe/Amsterdam in CI. The full UTC journey passed after the concurrent marketplace fixture updates.

An earlier run had three payment-store failures from empty seller IDs being written as NULL, plus the journey fixture mismatch. The concurrently edited payment code and fixture resolved those failures in the final UTC run; they are **not listed as outstanding application findings**. A separate rerun lost its disposable database and failed to connect; it was superseded by the successful fresh-database run above.

Source references were checked again before delivery. The profile upsert, payment claim, and address-reset defects remained after the concurrent edits. [Source fingerprints](CODE_REVIEW_2026-10-14.sources.json) identify the reviewed files at handoff; compare them before applying repairs if the checkout has moved on.

Temporary review probes demonstrated R01, R02, R03, R05, R06, R08, R09, R10, and R12. Other findings are marked as code-path findings rather than being presented as executed end-to-end scenarios. Existing tests passing does not negate these uncovered boundary cases.

The application has useful safeguards already: server-side role checks, self-booking rejection, private evidence access, session/CSRF protections, transactional booking, and durable provider operation identities. The urgent gaps are inconsistencies between those mechanisms. No universal P0 failure was established, and this review does not claim a complete penetration test, dependency-vulnerability audit, or measured 10,000-provider load test.

Only this handoff document and its source fingerprint manifest are review deliverables. Application fixes are intentionally left for the receiving agent.
