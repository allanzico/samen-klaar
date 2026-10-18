# Agent-managed requests

Every customer and every professional has an assistant from their first message (no opt-in; it can be paused).
The two assistants handle a request between them (choose, negotiate, book, confirm, reschedule) and talk to
their own person in the chat, stopping to ask only when a step needs a human. People and assistants use the
same named workflow actions and the same validation, so an assistant can never do something a button could not.

**Status, 18 October 2026.** Delivered: decision engine, runner, mandates, audit log, escalations with email,
visit and price negotiation, the customer's and the professional's chat dialogues (before and after booking),
AI reading of free-form replies, bot names. Live-verified with the real AI provider for the customer's booking
dialogue and the professional's confirm question. Blocked: agent-made payment holds (PayPal vaulting). Open:
see "Known limits" at the end.

## What an agent may do (negotiate, book, confirm, reschedule, recover)

| Agent | Acts alone when | Stops and asks when |
| --- | --- | --- |
| Customer | a match is inside the price limit; the service address is saved; the price is inside the limit, then books the earliest time inside the allowed days and hours; accepts an offer at or under the limit; approves extra work up to the allowed amount | price is above the limit after one counter-offer; no published free time fits and its one visit proposal was not met; address missing; a clarification question; finished work (it asks in the chat and approves only on a yes, unless the person delegated approval); payment (always: PayPal approval is the person's) |
| Professional | a price offer is at or above the floor (a percentage of the listed price); a reserved visit is inside the allowed days and hours (if auto-confirm is on); a proposed time is inside the hours (if auto-accept is on); a proposed time outside the hours gets one counter at the closest time inside them (a free published slot first) | an offer is below the floor after one counter-offer; a visit is outside the hours; anything about work, disputes or money |

### Reschedules, declines and cancellations

| Event | Customer agent | Professional agent |
| --- | --- | --- |
| The other side proposes a new time for a booked visit | accepts when the new time is inside its days, hours and horizon | accepts when auto-accept is on and the new time is inside its days and hours |
| The professional declines a reserved visit (or the reservation lapses) | books another free time inside its window, never the declined one; asks its person when none fits | n/a |
| The professional cancels the request | goes back to the matches, picks another professional inside the limits and never the one who cancelled; asks its person when nobody fits | n/a |
| The customer cancels more than 24 hours ahead | n/a (cancellation is immediate and final) | nothing to decide; any open question is closed |

When the customer asks their assistant for a move or cancellation the assistant sends the same
`propose-date` or `cancel-request` after the customer confirms in the chat (see "The assistants after
booking").

Both agents hand a visit change to their person (`late_change_needs_you`) when the booked visit is less
than 24 hours away, because the workflow sends those to operator review, and
(`visit_change_involves_payment`) when the visit uses pay-at-booking, so a reschedule never changes
payment timing. Late cancellations, withdrawals and anything under operator review are nobody's agent
to decide: both agents wait. Agents never cancel a request themselves.

**Visit-time negotiation.** When none of the professional's published free times fits the customer's days
and hours, the customer agent approves the plan and proposes the preferred hour (a mandate field) on the
first allowed day more than 24 hours out. The professional agent accepts it when it is inside its hours,
or counters once with the closest time inside them: a free published slot if there is one, otherwise the
proposed day clamped into its hours, then the next allowed days. The customer agent accepts that counter
when it is inside its own window. A time neither side's mandate allows ends in a question to a person.
`TestAgentsNegotiateBookAndEscalate` runs both the accepted proposal and the accepted counter.

An agent sends at most one counter-offer (and one visit proposal) per request. If the other side does not meet it, the person
decides. Releasing payment, cancellations with money consequences, refunds and disputes stay with people or the
operator team (approving finished work only moves to the agent if the customer hands it over). Nothing in
this slice moves money.

## On from the start

There is no opt-in. Every customer and professional has an agent from their first message, with a
default mandate (`workflow.DefaultMandate`) until they change anything. "Assistant preferences" in the
account menu lets them tighten or widen it, pause the agent entirely, or hand over work approval.

- **Customer defaults:** visits Monday to Saturday, 08:00 to 20:00, up to 14 days out, proposing 10:00
  when none of the professional's listed times fit. The price limit comes from the budget named in the
  chat (`workflow.BudgetMinor` reads "My budget is EUR 120"), capped by an optional ceiling in the
  preferences. With no budget, the agent still chooses a professional and negotiates, but the person
  approves the price (`approve_price`). The service address is the one thing only the person has, so the
  agent asks for it (`address_needed`).
- **Professional defaults:** auto-confirm and auto-accept on, a 90 percent price floor, and days and hours
  taken from their working-hours schedule (Monday to Friday 08:00 to 18:00 without one).
- **Always the person's:** payment. Approving finished work is theirs: the assistant asks in the chat and
  acts on the answer, or approves alone if the customer ticks "Approve finished work for me"
  (`autoApproveWork`); the workflow still refuses unless documentation is present. Delegating payment is not possible yet (see Known limits).
- **Pause.** A paused agent does nothing. Saved preferences override the defaults, including the pause.

## The assistant talks (scheduling dialogue)

The customer's agent is a chat partner. After a professional is chosen it carries on in the same thread,
in the customer's language (English or Dutch), and only the customer sees these messages:

1. **Address.** "What is the full address where Marieke should come?" The reply is saved as the service address.
2. **Price.** Inside the budget the customer named in the chat it just proceeds (and counters once if the price is a little over). With no budget it asks "shall I go ahead at that price?" before approving.
3. **Availability.** "When are you free for Marieke to visit?" with quick replies (this week, next week, weekday evenings, any time). The reply is read by `ParseAvailability` (English and Dutch: "Tuesday afternoon or Thursday after 5", "dinsdagavond", "21 oct 10-12", "next week", "any time").
4. **Compare.** The windows are checked against the professional's free published times. One fit: "Marieke has Tue 21 Oct, 14:00-15:00 open. Does it suit you?" Several: up to three on different days as quick replies. None: the next open times plus "Ask Marieke for another time", which approves the plan and sends a visit proposal inside the customer's windows; the professional's agent accepts it or counters once (see above).
5. **Book only after yes.** The visit is reserved when the customer picks a time, unless they turned on "Book a visit time without asking me first".
6. **Updates.** It tells the customer once when the visit is reserved and once when the professional confirmed, offers new times if the professional declined, and asks before accepting a time the professional suggests.

Negotiation steps between the two agents (price counters, visit proposals, agreements) leave a short
line in the shared thread, localized for each reader, so both people can follow the exchange. Neither
side can see the other's private chat with their assistant, and the professional never sees the
customer's stated availability. A waiting question also shows as an open question (banner and email).

Implementation: `agent_converse.go` (dialogue, pure), `availability.go` (parser and matching), the
`agent-say` (server only) and `agent-reply` actions in `agent_dialog.go`, `State.AgentDialog`.

**The professional's assistant talks too.** In its own private chat (`State.ProDialog`, messages with
audience `professional`) it asks before doing something outside the professional's settings: confirming a
reserved visit when auto-confirm is off or the time is outside their hours, accepting a visit time they
did not pre-approve, or accepting a price below their floor after its one counter. A yes acts, a no
declines. Inside the settings it acts alone, then reports what it agreed ("Confirmed Tue 21 Oct, 14:00
with Alex."). The customer never sees this chat. Chat messages are ordered by `Seq`, so each viewer's
filtered copy of the thread still lines up with the dialogue state.

**Reading free-form replies.** Quick-reply buttons, plain yes/no and numbers are read locally. Anything
else typed ("I'm usually back around six", "the later one", "not Wednesdays") goes once to the configured AI
provider (`Coordinator.InterpretReply`, one structured call per reply, cached) together with the
question, the offered options, the date and the time zone. The reply is treated as untrusted data: the
model can only return an intent for the open question (`yes`, `no`, `choose` n, `availability` windows,
`ask_pro`, `address`, `reschedule`, `cancel`, `unclear`), and `ReplyIntent.Sanitize` drops anything outside the stage, out of
range or impossible before the assistant uses it. The model never acts: the assistant still only does
what its mandate and the workflow allow. If no provider is configured, the call fails or the model is
unsure, the built-in parser reads the reply, and after two failed tries the person is asked on the page.
The call counts against the same per-account AI rate limit as intake. A failed call is not repeated by the
periodic sweep for five minutes. The reply to the address question is never sent to the provider: the
service address is read locally and shared only with the chosen professional.

## The assistants after booking

The conversation does not stop at the booking. Everything below is private to the person it concerns
(audience `customer` or `professional`), ordered by `Seq` like the rest of the chat, and uses the same
actions a button would, so every workflow rule still applies.

**Asking the assistant to move or cancel the visit (customer).** Once a visit is booked the composer has
a "Message {pro} / Ask your assistant" switch (`app.messageTo`). Text sent to the assistant with no
question open is an `agent-reply` that the server marks `Code: "ask"` (only a customer with a booked
visit, never a quick reply). The assistant reads it with the built-in parser (`localIntent`, stage `ask`:
cancel and move words in English and Dutch plus `ParseAvailability` for the days) and, when that cannot
settle it, with the AI interpreter. The new intents are `reschedule` (days optional) and `cancel`, and
`ReplyIntent.Sanitize` keeps the model inside them (stages `ask` and `move`). Then:

1. **Move.** Without days it asks "when would you like to move the visit to?" (stage `move`). With days
   it looks for one of the professional's free published times on them, otherwise a time to ask for
   (`candidateFromWindows`, at least 24 hours out), and asks "Shall I ask {pro}?" Answering with other days
   ("No, Saturday") replaces them. Only a yes sends the existing `propose-date`; the professional's
   assistant then accepts, asks its person or leaves it to them exactly as for any proposal. The customer
   hears once that the request is with the professional, and again when the new time is confirmed, when the
   professional declines (the visit stays), or when it went to operator review.
2. **Cancel.** It asks "Do you want me to cancel the visit on ...?" and only a yes sends the existing
   `cancel-request`. Within 24 hours of the visit the workflow turns that into an operator review; the
   question says so beforehand and the assistant reports "sent to our operator team, the visit stays
   booked".
3. **Never decided by the assistant:** work already started, a request already under operator review, and
   any visit with PayPal authorization at booking (`visit_hold`): it says so plainly and leaves it to the
   person. A late move is proposed only after the customer is told that the operator team must approve it
   once the professional agrees.
4. Anything else ("the key is under the mat") gets one reply saying what it can do and that nothing was
   sent to the professional; the customer switches the box to the professional to write to them.

**Approving finished work and payment (customer).** When the professional submits the completion
(status `review`, work note present, evidence the workflow would accept) the assistant asks in the chat,
summarising what was handed in (number of files, the professional's note, the file check if there was one):
"Shall I approve it? Approving doesn't pay anything." A yes sends `approve-completion`; a no approves
nothing, leaves "check the work" with the person and is asked again only for a new submission. If the work
cannot be approved yet it stays the old escalation `check_the_work`; with "Approve finished work for me"
on, the assistant approves without asking. After approval it says once that payment is the one step it
cannot do, because PayPal needs the customer's own approval, with a button (`Code: "link:payment"`) that
focuses the payment step. It never calls `approve-payment` or any PayPal action. When the payment
completes it says so once.

**The professional's assistant mirrors it.** In its private chat it reports, once each and in order, that
the customer approved the finished work (payment next), that they approved the payment in PayPal, and
that the payment arrived. PayPal capture and reconcile now wake the agents, and the sweep keeps looking at
a paid request for a day, so these reports do not depend on a person's next click.

Reported milestones are remembered in `AgentDialog.Notified` as a short list of keys, so several kinds
of report cannot repeat each other. Pure code: `agent_followup.go`; texts in English and Dutch are in
the same file (`dialogText`) and the page's own strings in `src/lib/agent-copy.ts`.

Limits: with no AI provider configured the assistants rely on the built-in parser, which covers common
phrases in English and Dutch but not every way of saying a time. Replies are read one at a time; there
is no memory of earlier answers beyond the availability the customer stated.

**Names.** Assistants go by short bot names (Pip, Juno, Kit, ...) shown with a small bot icon, not "<person>'s
assistant". The name is stable for the same person (hash of their name) and the two assistants on one
request never share one (`AssistantNames`). Each assistant introduces itself once, in its first message
("Hi, I'm Pip, your assistant for this job."). Messages carry the name in `Message.Agent`.

## Mandate

A mandate is the explicit authority a person grants (`agent_mandates`, one row per account and role,
versioned on every change); without a row the default applies. The product bounds it: price ceiling up
to 10,000, floor 50 to 100 percent, horizon up to 90 days, extra-work allowance no higher than the
ceiling. Only the signed-in person can change their mandate. Every logged decision records the mandate
version in force.

## Pieces

- `internal/workflow/agent.go`: the pure decision engine (`DecideAgent`). It reads a snapshot and
  returns act, escalate or wait. It has no side effects and is unit tested.
- `internal/httpapi/agent_runtime.go`: `RunAgents` loops (at most 10 rounds) over both people's agents
  and applies each step with `runAction`, the same code path as `POST /api/actions`. It is triggered
  after every action a person takes and by a 30-second sweep of recently active requests. A per-request
  in-process lock serializes runs (a second API replica would need a shared lock; every state change
  still takes a row lock in PostgreSQL and is re-validated by the workflow). A step the workflow rejects is logged, becomes an `agent_blocked` question and
  is not retried by the sweep (it is retried after the next person action).
- `agent_decisions` is the audit log shown as the activity list. `agent_escalations` holds the one open
  question per request and person; it resolves itself when the agent can act again or the request
  changes.
- **Escalation notices.** Opening an escalation (`SyncAgentEscalation`) enqueues an `agent-question`
  notification in the same transaction, through the normal outbox and River pipeline
  (`internal/adapters/postgres/notifications.go`): one email per question, in the recipient's saved
  language (Dutch or English), plus a bell entry. The event key is `agent-question:<escalation id>`; a
  question that has already been resolved when the email is about to go out is dropped as obsolete. Before a
  professional is chosen there is no job yet, so the request id (the workspace) stands in for the job id.
  Delivery health shows up in the operator queue like any other notification.
- `GET/PUT /api/agent/mandate`, `GET /api/agent/activity?workspaceId=`, `GET /api/agent/escalations`.
- UI: "Assistant preferences" in the account menu (pause, ceiling, hours, address, hand over work approval),
  the assistant's open question as the first card on the request page with its quick replies, the thread with
  bot names and a "replying" line that expires after 25 seconds, an activity panel, and a banner when an
  assistant needs its person.
- When the workflow refuses a step an assistant tried, `BlockedNotice` posts the plain reason in the chat so a
  reply never hangs. A refused work approval names exactly which documentation is missing (`MissingEvidenceLabels`).

## Known limits

- **Payment timing.** Agent bookings always use pay-after-completion. A PayPal hold at booking needs
  the buyer to approve in PayPal's own window; an agent cannot do that without a saved (vaulted)
  payment method and a spending authorization. That is the next money slice (hold and pay with limits).
  Design and blockers: [AGENT_PAYMENT_HOLDS.md](AGENT_PAYMENT_HOLDS.md); PayPal must enable vaulting first.
- **Payment delegation.** Customers can delegate approving finished work but not paying: capturing after
  completion needs the buyer in PayPal's window, and a hold at booking needs vaulting (see above). The
  assistant only tells the customer where that step is.
- **Changing a visit that has a PayPal hold** (payment at booking) stays with the person, because it
  touches an authorization; the assistant says so and does nothing.
- **Cancelling after work started** and disputes are not the assistant's. A professional's own counter
  for a move outside their hours is not automatic (only for a first visit); their assistant asks them.
- **Push notifications** for an agent question are not wired (email and the in-app bell are).
- **Intake** (describing the problem) is still a conversation the customer has with the assistant; the
  customer agent starts when the match list appears.
- Agents act on the earliest matching time and the first match inside the limit. There is no ranking
  by rating or distance yet.
