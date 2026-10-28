# Product behaviour

SamenKlaar is a home-services marketplace for the Netherlands, with English and Dutch interfaces. Customers and professionals each have an assistant, a private conversation and editable permissions. The professional provides the service; SamenKlaar coordinates the agreement and payment.

## Discovery and agreement

Customers describe the work, answer clarification questions and select a Mapbox address, postcode or city. Matching uses approved professionals, published service scopes, coverage and available visits. Exact addresses stay separate from discovery labels and are checked against coverage before booking. The assistant cannot select a professional on the customer's behalf.

A customer reviews service details and alternatives, negotiates if needed, explicitly confirms the professional and chooses who will review completion/payment. Background refreshes and assistant decisions cannot skip these review screens. Before booking, the customer can revise intake, return to results or choose another professional. Unapproved requests without retained booking/payment/work history can be deleted; booked work uses cancellation instead.

Quotes use published services, integer minor-unit arithmetic, materials, exclusions and an agreed duration. New agreements snapshot the professional platform-fee policy; later price/scope changes require approval. A price offer outside the professional's assistant limits waits for that person.

## Scheduling

Professionals publish recurring weekly hours plus days off, notice, horizon and travel buffers. Free times are generated from that schedule. The professional can separately permit its assistant to accept a proposed appointment outside weekly hours; this never overrides a day off, an overlapping booking or payment guards.

A reserved visit waits for professional confirmation for two business hours on the Europe/Amsterdam business clock. Decline or expiry releases the reservation. Alternative-date requests avoid days the customer rejected. Late changes and requests affecting payment holds require the appropriate human/operator path.

## Completion and PayPal

The professional starts work, supplies the agreed photos/documents and writes a specific completion note. Check and send saves a draft, runs the documentation check and requests corrections before customer approval when evidence is missing or unclear. Uploaded files are private and scoped to the current job, scope and completion round.

Customers explicitly choose manual review/payment or delegated review/release for each booking. Delegation binds the agreed scope, professional, amount and currency, requires a buyer-approved PayPal hold and can be turned off before payment release starts. The assistant may approve documented completion and capture only inside that permission; it cannot create a fresh buyer authorization. Unclear evidence, disputes and unsafe/stale state block automatic release. Photos cannot verify workmanship or safety.

The completion report records the scope, evidence, approval actors and payment reference. Paid completed jobs can receive one customer review and one professional response. See [PAYMENTS.md](PAYMENTS.md) for the durable payment and recovery rules.

## Accounts and operations

Customers edit a private profile and usual location. Professional onboarding and operator approval govern matching eligibility. Operators have searchable user/location directories, reasoned account actions, review queues and audit history. Access changes do not erase existing job history. In-app notifications carry routine conversation updates; email is limited to important events described in [EMAIL.md](EMAIL.md).

## Current limits

- Money movement is PayPal sandbox only. Live partner approval, payment/settlement verification and applicable professional/consumer policies must be completed before a live-money release.
- Saved buyer wallets and assistant-created fresh holds are unavailable without platform vaulting enablement. The customer approves initial and replacement holds in PayPal.
- Distance ranking is approximate straight-line coverage, not driving time. Public rating aggregation, external calendars, push notifications and multi-replica coordination are not implemented.
- Dutch copy and consumer-policy wording require the appropriate human review. AI summaries do not certify the service or resolve disputes.
