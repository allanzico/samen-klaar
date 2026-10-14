# SamenKlaar product spec and delivery plan

Last reviewed: 13 October 2026.

## Product spec

SamenKlaar is an AI-coordinated home-services marketplace launching in Amsterdam. Customers describe a problem; AI clarifies it, finds a suitable professional, and moves routine coordination forward so customers and professionals get faster responses and resolutions. The job keeps the catalog-backed quote, appointment, approved scope and changes, completion evidence, and payment approvals together.

The professional is the service provider contracting with the customer. SamenKlaar operates the marketplace and coordination platform. AI may interpret, summarize, suggest, remind, and request missing information. It cannot invent prices, make a diagnosis, claim to verify workmanship, approve a commitment, or authorize money movement. Customers and professionals explicitly approve the match, price, schedule, scope changes, completion, and payment.

### Launch decisions

- **Market:** Amsterdam, Netherlands. Start in a focused service area and expand when supply and support can serve it reliably. Ask for neighborhood or postcode for matching; reveal the exact address only after the customer selects a professional, and only to that assigned professional.
- **Services:** plumbing and carpentry are bookable; electrical work is assessment-only. Triage unsupported or unclear work rather than fabricate a final repair quote.
- **Supply and matching:** review professionals before they become bookable. Review identity and business information, PayPal onboarding, and relevant category-specific qualifications or insurance. Show one best-fit professional with an offer derived from their published catalog, then allow customers to view alternatives.
- **Accounts:** customer access is available by default. A person may also have professional access after onboarding and approval. Operators are separately provisioned. Every request and action is scoped to the signed-in account and role.
- **Languages:** customer, professional, and AI conversations support Dutch and English; AI follows the customer's language.
- **Booking:** customer chooses a published slot. Hold it while the professional has two business hours to confirm (implemented as weekdays, 09:00–17:00 Europe/Amsterdam). On decline or timeout, release the slot and offer alternatives. A multi-trade request may become separately scoped and quoted jobs on one shared timeline.
- **Work and evidence:** each bookable service defines completion evidence. Missing or unclear documentation blocks completion approval. AI can flag evidence gaps but cannot certify work. A reported incomplete or damaged job pauses capture and opens an operator case; people decide rework or refund.
- **Payments:** use PayPal marketplace payments. Authorize the agreed total at booking and capture only after the customer separately approves completion and payment. No deposits in the first release. Professionals must complete PayPal onboarding before paid bookings. SamenKlaar charges a configurable percentage disclosed before booking; the rate remains unset until unit-economics work is complete.
- **Cancellations:** free customer cancellation or rescheduling through 24 hours before the appointment; later requests go to review, with no automatic cancellation fee in the first release. If a professional cancels, offer rebooking or a refund path. Provide a self-service statutory withdrawal path and record the customer's request and acknowledgment when work starts during the withdrawal period; encode urgent-service exceptions only through reviewed rules, never an AI guess.
- **Notifications:** in-app and email for booking updates, 24-hour appointment reminders, overdue evidence/approvals, and operator escalation when work is stalled for 24 hours. SMS and WhatsApp are later.
- **Reviews:** only for completed, paid jobs; mark them as verified jobs and allow a professional response.
- **AI configuration:** provider-agnostic, using `AI_PROVIDER`, `AI_PROVIDER_KEY`, and `AI_MODEL`. Secrets stay server-side.
- **Product experience:** production accounts and routing; no runtime demo identities, fake professionals, placeholder evidence, or simulated payment success. Test distinct people in separate authenticated browser sessions.

### Delivery phases

#### Phase 1 — Close launch dependencies

1. Confirm PayPal marketplace product, partner approval process, Netherlands seller onboarding, supported currencies, settlement, platform fees, refunds, and disputes.
2. Set the initial Amsterdam service coverage from actual professional supply; do not assume the entire city is covered.
3. Turn the agreed professional review into an operator checklist, with category-specific qualification and insurance requirements defined by policy owners.
4. Have the professional-as-provider contract model and Dutch/EU withdrawal, cancellation, and refund wording reviewed by qualified counsel.
5. Keep PayPal live payments disabled until required marketplace approvals and integration gates are satisfied.

**Research completed:** the best-fit public PayPal product is the Complete Payments Platform for marketplace checkout and partner fees, with seller onboarding before payment. This is a technical direction, not confirmation of SamenKlaar's approval. The Netherlands eligibility table includes business sellers onboarding before payment for Checkout and Expanded Checkout. PayPal requires platform approval for live Complete Payments Platform calls; sandbox testing is available before approval. Authorization/capture is a supported checkout model, but seller onboarding after payment does not support it. Confirm the exact seller features and settlement behavior with PayPal before treating the direction as final.

#### Phase 2 — Complete trusted supply and booking

Finish professional onboarding and approval, catalog-backed matching, location privacy, Dutch/English interaction, alternatives, quote detail, service checklists, multi-trade requests, temporary appointment holds, two-business-hour confirmation, and release/rebooking on decline or timeout.

#### Phase 3 — Build the resolution workflow

Add post-booking AI follow-up, reminder and escalation timing, overdue evidence/approval handling, scope-change approvals, missing-evidence blocks, cancellation/rescheduling lifecycle, and operator dispute resolution. AI gathers and summarizes facts; humans decide disputed outcomes.

#### Phase 4 — Make money movement and operations launch-ready

Integrate approved PayPal marketplace seller onboarding and payouts, platform fees, verified and idempotent webhooks, delayed/out-of-order reconciliation, authorization expiry, voids/refunds, disputes, notification delivery and production operational controls. Verify full payment recovery before allowing live money.

#### Phase 5 — Prove readiness with real account boundaries

Run account-isolation and end-to-end acceptance across separate customer, professional, and operator sessions. Verify booking timeout, provider decline, cancellation, missing evidence, dispute pause, notification retries, webhook replay, refund, and uncertain payment recovery. Keep external-service contract tests distinct from live provider verification.

## Implementation status

Updated 14 October 2026. This status describes the current repository; it does not certify a live-money release.

- **Phase 1:** the operator professional-review checklist, public PayPal dependency register, and sandbox marketplace path are implemented. Professionals can onboard and verify seller eligibility; checkout binds authorization and customer-approved instant capture to that seller, with seller-scoped refunds and onboarding/revocation webhooks. The local PayPal platform merchant ID, partner attribution code, and webhook ID are unset, so provider sandbox onboarding and settlement have not been exercised. PayPal partner approval, fee and settlement setup, initial neighborhood coverage, category-specific vetting policy, and legal review remain open external decisions.
- **Phase 2:** account-scoped Amsterdam matching, address privacy, provider offers, quote and visit negotiation, appointment reservation, two-business-hour professional confirmation, expiry, decline, and rebooking after professional cancellation are implemented. A persisted Dutch/English preference now covers customer and professional task flows, with intake/evidence AI responding in the customer's language; native Dutch editorial QA remains. Multi-trade splitting, recurring availability, and service-specific evidence policy remain open.
- **Phase 3:** late-cancellation review, customer issue reporting, operator rework/close/refund decisions, evidence and payment gates, 24-hour reminders, and stalled-job escalation are implemented. Statutory withdrawal handling and category-specific escalation rules still require reviewed policy.
- **Phase 4:** verified and deduplicated webhooks, capture and dispute reconciliation, full-refund retry, partial-refund/reversal review, a persisted account-scoped in-app notification inbox, SMTP delivery with aggregate operator delivery-health counts, payment recovery, per-account authenticated API throttles, and per-client sign-in start/callback throttles are implemented. Sandbox marketplace seller onboarding and seller-directed instant settlement are implemented in code but await platform credentials and provider verification. Platform fees, confirmed production settlement, live payment, broader edge-level abuse controls, and multi-replica rate coordination are not.
- **Phase 5:** PostgreSQL-backed workflow and account-isolation checks run through `make check-flow`. A completed, paid, non-refunded job supports one verified customer review and one professional response. Browser verification has covered separate authenticated customer, professional, and operator sessions and safe routing; it has not completed real wallet approval or a live marketplace payment. Public profile review aggregation and review moderation/reporting await visibility and policy decisions.

## Phase 1 dependency register

| Dependency | Current status | Release consequence |
| --- | --- | --- |
| PayPal marketplace partner approval | Not confirmed; application and PayPal approval are external actions | No live marketplace traffic or funds before approval |
| Netherlands seller onboarding | PayPal's public table lists Netherlands support for business sellers onboarding before payment with PayPal Checkout and Expanded Checkout; confirm the exact SamenKlaar configuration with PayPal | Onboard before payment to retain authorization support |
| Authorization then capture | Product decision confirmed; marketplace seller onboarding and sandbox Orders authorization/capture flow are implemented, but account-level multiparty routing is not configured or provider-verified. PayPal says an authorization lasts up to 29 days and recommends capture within the 3-day honor period; reauthorization and issuer behavior need sandbox validation for longer jobs | No paid booking until platform/seller routing, authorization, capture, reauthorization, and settlement behavior are validated; request fresh customer approval when the authorization cannot be used |
| Partner fee | Percentage model confirmed; exact percentage is undecided. PayPal requires the seller's `PARTNER_FEE` feature for platform fees on authorization capture | Select and disclose a rate only after unit-economics work and PayPal confirms entitlement |
| PayPal currencies and settlement | Must be confirmed for the approved platform and merchant configuration | Do not promise currency or payout availability from sandbox behavior |
| Amsterdam coverage | City confirmed; launch neighborhoods depend on vetted supply | Matching must only expose actually covered areas |
| Professional review | Identity/business, PayPal onboarding, and relevant qualifications/insurance are confirmed criteria; category-specific evidence thresholds remain to be written | No profile becomes bookable until required checks are recorded and approved |
| Contract and consumer policy | Professional is the service provider; local legal review remains outstanding | Review marketplace terms, withdrawal, cancellation, refunds, and urgent-service handling before launch |

Phase 1's implementation now includes the sandbox marketplace path: professional onboarding, eligibility verification, seller-bound authorization, customer-approved capture with instant seller disbursement, seller-bound refunds, and seller onboarding/revocation webhook handling. The local PayPal platform merchant ID, attribution code, and webhook ID are still blank, so the external sandbox account flow has not been exercised. Public PayPal documentation and the product dependency register are complete, and the operator review checklist is implemented with approval blocked until all checks are complete. The checklist records statuses and brief rationales only; no identity or qualification documents are stored. External partner approval, final merchant/currency settlement confirmation, fee rate, supply-led neighborhood coverage, category-specific vetting thresholds, and legal review remain open; none is assumed in code or customer promises.

## External references

- [PayPal Complete Payments Platform overview](https://developer.paypal.com/platforms/overview)
- [PayPal marketplace onboarding and go-live](https://developer.paypal.com/platforms/get-started)
- [PayPal seller onboarding and country eligibility](https://developer.paypal.com/platforms/seller-onboarding)
- [PayPal authorization and capture](https://developer.paypal.com/platforms/checkout/standard/customize/auth-capture/)
- [PayPal partner fees and marketplace settlement](https://developer.paypal.com/platforms/overview)
- [Dutch online cancellation button requirement](https://business.gov.nl/amendments/online-shops-must-have-cancellation-button/)
- [EU consumer withdrawal rules for service contracts](https://europa.eu/youreurope/business/selling-in-eu/selling-goods-services/ecommerce-distance-selling/index_en.htm)
