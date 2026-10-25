import { needsAutonomyChoice, automaticReleaseOn } from './booking-autonomy';
import { t } from './i18n.svelte';
import { money, sameDay, shortVisit, time, visitRange } from './format';
import { downloadVisitCalendar } from './calendar';
import { firstName } from './status-ui';
import { shortName } from './trade';
import { missingItems } from './evidence';
import { pendingPolicies } from './policy';
import type { RequestCode } from './request-status';
import type { StepView } from './step-view';
import type { PaymentTiming } from './request-ui.svelte';
import type { AppActions, AppState, Appointment, WorkspaceState } from './types';

// One next step per request state. Every action and guard here is the
// same API call the previous UI made; only the presentation changed.

const UNCERTAIN = ['creating', 'authorizing', 'authorization_uncertain', 'order_uncertain', 'voiding', 'void_uncertain', 'release_pending', 'authorized'];
const RELEASING = ['voiding', 'void_uncertain', 'release_pending'];
const CAPTURE_UNCERTAIN = ['capture_uncertain', 'authorization_uncertain', 'order_uncertain', 'void_uncertain', 'captured'];

export type CustomerContext = {
  app: AppState; actions: AppActions; workspace: WorkspaceState; code: RequestCode;
  timing: PaymentTiming; slot?: Appointment; openReport: () => void; openPolicy: () => void;
};

export function showTimingChoice(app: AppState, workspace: WorkspaceState, code: RequestCode): boolean {
  const job = workspace.job;
  return !!app.providerConfig?.paymentsAvailable && job.paymentAuthorizedVersion !== job.scopeVersion
    && (['quote', 'booking', 'choose_visit'].includes(code) || (code === 'review_date' && job.dateProposal?.actor === 'professional'));
}

export function customerStep(ctx: CustomerContext): StepView | null {
  const { app, actions, workspace, code, timing, slot } = ctx;
  const job = workspace.job;
  if (!job.id && !workspace.cancelled) return null;
  const pro = firstName(job.professional) || t('the professional');
  if (workspace.intake?.step === 'professional_review') return {
    kind: 'your-turn', tag:{variant:'your-turn',label:t('Your turn')}, title: t('Review {name} before continuing', {name:pro}),
    text: t('Check the service, price and exclusions below. You can choose someone else. No visit is booked.'),
    primary: {label:t('Continue with {name}', {name:pro}),run:()=>actions.act('confirm-professional',job.id)},
    secondary: {label:t('Back to professionals'),run:()=>actions.act('return-to-matches')}
  };

  const total = money(job.quote.totalMinor, job.quote.currency);
  const slots = workspace.availableSlots;
  const needsAddress = !!job.location && !job.serviceAddress;
  const yourTurn = { variant: 'your-turn' as const, label: t('Your turn') };
  const waitingTag = { variant: 'waiting' as const, label: t('Waiting on {name}', { name: pro }) };
  const visit = job.appointment ? visitRange(job.appointment.startUtc, job.appointment.endUtc, job.appointment.timezone) : undefined;
  const after = (from: number) => [t('{name} comes at the agreed time', { name: pro }), t('{name} sends photos and a note when done', { name: pro }), automaticReleaseOn(job) ? t('Your assistant checks completion and releases the authorized payment') : t('You approve the work, then pay')].slice(from);
  const releasing = RELEASING.includes(app.paypalStatus);

  async function book(action: 'approve-and-book' | 'book-appointment', slotID?: string) {
    const booked = await actions.act(action, slotID, undefined, undefined, undefined, undefined, timing);
    if (booked && timing === 'at_booking') {
      const current = app.serverState?.job;
      if (!(current?.paymentAuthorizedVersion === current?.scopeVersion && app.paypalStatus === 'authorized')) await actions.beginPayPalCheckout();
    }
  }
  async function acceptVisit(proposalId: string) {
    if (await actions.act('accept-date', '', undefined, undefined, { proposalId }, undefined, timing)) {
      const current = app.serverState?.job;
      if (current?.paymentAuthorizationTiming === 'at_booking' && current.paymentAuthorizedVersion !== current.scopeVersion) await actions.beginPayPalCheckout();
    }
  }
  async function pay() {
    if (job.paymentApprovedVersion !== job.scopeVersion && !(await actions.act('approve-payment'))) return;
    if (app.paypalStatus === 'authorized') await actions.capturePayPal();
    else await actions.beginPayPalCheckout();
  }

  switch (code) {
    case 'quote': {
      const reason = needsAutonomyChoice(job) ? t('Choose who reviews completion and releases payment') : job.needsClarification ? t('Answer the question to continue') : needsAddress ? t('Add your address to continue') : slots.length && !slot ? t('Choose a time to continue') : releasing ? t('Waiting for the previous PayPal hold to be released') : '';
      return {
        kind: 'your-turn', tag: yourTurn, title: t('Book your visit'), amount: total,
        primary: slots.length
          ? { label: slot ? t('Book {time} · {amount}', { time: shortVisit(slot.startUtc, slot.timezone), amount: total }) : t('Approve {amount} and book', { amount: total }), run: () => book('approve-and-book', slot?.slotId), disabled: !!reason, reason }
          : { label: t('Approve {amount}', { amount: total }), run: async () => { if (await actions.act('approve-quote', '', undefined, undefined, undefined, undefined, timing)) actions.openProposal('date'); }, disabled: !!reason, reason },
        // With times to choose, the payment options above already say what happens; do not repeat it.
        helper: slots.length ? undefined : t('Then suggest a visit time. Nothing is reserved until you both agree.'),
        secondary: releasing ? { label: t('Check payment status'), run: () => actions.reconcilePayPal() } : undefined,
        quiet: [{ label: t('Make a price offer'), run: () => actions.openProposal('price') }, { label: t('Choose another professional'), run: () => actions.act('return-to-matches') }]
      };
    }
    case 'booking': case 'choose_visit': {
      const declined = code === 'choose_visit';
      const earlier = job.appointmentConfirmation?.appointment;
      const reason = needsAutonomyChoice(job) ? t('Choose who reviews completion and releases payment') : needsAddress ? t('Add your address to continue') : !slot ? t('Choose a time to continue') : releasing ? t('Waiting for the previous PayPal hold to be released') : '';
      return {
        kind: 'your-turn', tag: yourTurn, title: declined ? t('Choose another time') : slots.length ? t('Book your visit') : t('Suggest a visit time'),
        banner: declined ? { kind: 'error', title: earlier ? t('{name} couldn’t confirm {time}', { name: pro, time: shortVisit(earlier.startUtc, earlier.timezone) }) : t('The visit wasn’t confirmed'), text: t('Nothing charged. The time was released, and any PayPal hold is being released. Pick another time.') } : undefined,
        text: slots.length ? undefined : t('{name} hasn’t published free times. Nothing is reserved until you both agree.', { name: pro }),
        primary: slots.length
          ? { label: slot ? t('Book {time} · {amount}', { time: shortVisit(slot.startUtc, slot.timezone), amount: total }) : t('Book this visit'), run: () => book('book-appointment', slot?.slotId), disabled: !!reason, reason }
          : { label: t('Suggest a visit time'), run: () => actions.openProposal('date') },
        secondary: releasing ? { label: t('Check payment status'), run: () => actions.reconcilePayPal() } : undefined,
        helper: undefined,
        quiet: slots.length ? [{ label: t('Suggest my own time'), run: () => actions.openProposal('date') }] : []
      };
    }
    case 'review_price': {
      const offer = job.priceProposal!;
      return {
        kind: 'your-turn', tag: yourTurn, title: t('{name} proposed {amount}', { name: pro, amount: money(offer.amountMinor, job.quote.currency) }), text: offer.note || undefined,
        primary: { label: t('Accept {amount}', { amount: money(offer.amountMinor, job.quote.currency) }), run: () => actions.act('accept-price', '', undefined, undefined, { proposalId: offer.id }) },
        secondary: { label: t('Counter with a price'), run: () => actions.openProposal('price') },
        quiet: [{ label: t('Decline offer'), run: () => actions.act('decline-price', '', undefined, undefined, { proposalId: offer.id }) }]
      };
    }
    case 'waiting_price': {
      const offer = job.priceProposal!;
      return {
        kind: 'waiting', tag: waitingTag, title: t('Waiting for {name}', { name: pro }), amount: money(offer.amountMinor, job.quote.currency),
        text: t('The price stays {amount} until {name} accepts.', { amount: total, name: pro }),
        primary: job.quoteApproved ? undefined : { label: t('Book for {amount}', { amount: total }), disabled: true, reason: t('Waiting for {name} to answer your {amount} offer', { name: pro, amount: money(offer.amountMinor, job.quote.currency) }) },
        quiet: [{ label: t('Change my offer'), run: () => actions.openProposal('price') }, { label: t('Withdraw offer'), run: () => actions.act('withdraw-price', '', undefined, undefined, { proposalId: offer.id }) }]
      };
    }
    case 'review_date': {
      const offer = job.dateProposal!;
      return {
        kind: 'your-turn', tag: yourTurn, title: t('{name} proposed a visit', { name: pro }), amount: visitRange(offer.appointment.startUtc, offer.appointment.endUtc, offer.appointment.timezone), text: offer.note || undefined,
        primary: { label: t('Accept and book'), run: () => acceptVisit(offer.id), disabled: needsAutonomyChoice(job), reason: needsAutonomyChoice(job) ? t('Choose who reviews completion and releases payment') : undefined },
        secondary: { label: t('Suggest another time'), run: () => actions.openProposal('date') },
        helper: t('Accepting reserves this visit at the agreed price.'),
        quiet: [{ label: t('Decline visit'), run: () => actions.act('decline-date', '', undefined, undefined, { proposalId: offer.id }) }]
      };
    }
    case 'waiting_date': {
      const offer = job.dateProposal!;
      return {
        kind: 'waiting', tag: waitingTag, title: t('Waiting for {name}', { name: pro }), amount: visitRange(offer.appointment.startUtc, offer.appointment.endUtc, offer.appointment.timezone),
        text: t('This time isn’t reserved until {name} accepts.', { name: pro }),
        quiet: [{ label: t('Change my proposed visit'), run: () => actions.openProposal('date') }, { label: t('Withdraw proposal'), run: () => actions.act('withdraw-date', '', undefined, undefined, { proposalId: offer.id }) }]
      };
    }
    case 'authorize_booking': {
      const uncertain = UNCERTAIN.includes(app.paypalStatus);
      return {
        kind: 'your-turn', tag: yourTurn, title: t('Authorize {amount} for this booking', { amount: total }), amount: visit,
        text: automaticReleaseOn(job) ? t('Authorize the agreed amount in PayPal at booking. Your assistant releases it only after checking the completion record.') : t('PayPal holds the amount for up to 29 days. Nothing is taken until you approve the work and the payment.'),
        primary: uncertain ? { label: t('Check payment status'), run: () => actions.reconcilePayPal() } : { label: t('Continue with PayPal'), run: () => actions.beginPayPalCheckout() },
        secondary: automaticReleaseOn(job) ? undefined : { label: t('Pay after the work'), run: () => actions.act('defer-payment-authorization'), disabled: uncertain, reason: uncertain ? t('Check payment status before changing when you pay.') : undefined }
      };
    }
    case 'waiting_visit_confirmation':
      return { kind: 'waiting', tag: waitingTag, title: t('Time held. {name} confirms next', { name: pro }), amount: visit, text: t('{name} has two business hours to confirm this time.', { name: pro }), after: after(0) };
    case 'booked': {
      const terms = pendingPolicies(job).length > 0;
      return {
        kind: terms ? 'your-turn' : 'info', tag: terms ? yourTurn : { variant: 'booked', label: t('Scheduled') }, title: terms ? t('Acknowledge the terms before work starts') : t('You’re booked'), amount: visit, after: after(0),
        text: terms ? t('{name} can’t start until you’ve read and acknowledged the reviewed terms for this booking.', { name: pro }) : undefined,
        primary: terms ? { label: t('Review and acknowledge'), run: ctx.openPolicy } : undefined,
        secondary: job.appointment ? { label: t('Add to calendar'), run: () => downloadVisitCalendar(job.id, `${job.title} · ${job.professional}`, job.appointment!.startUtc, job.appointment!.endUtc) } : undefined,
        quiet: app.providerConfig?.paymentsAvailable && job.paymentAuthorizationTiming !== 'at_booking' ? [{ label: t('Authorize the amount in PayPal now'), run: async () => { if (await actions.act('set-payment-timing', '', undefined, undefined, undefined, undefined, 'at_booking')) await actions.beginPayPalCheckout(); } }] : []
      };
    }
    case 'in_progress':
      return { kind: 'info', tag: { variant: 'progress', label: t('In progress') }, title: t('{name} is working on it', { name: pro }), banner: { kind: 'info', text: t('Nothing extra without your yes. Extras come as offers.') }, after: after(1) };
    case 'review_change': {
      const change = job.change!;
      const newTotal = money(job.quote.totalMinor + change.amountMinor, job.quote.currency);
      return {
        kind: 'your-turn', tag: yourTurn, title: t('{name} found more to fix', { name: pro }), text: change.description,
        ledger: [{ label: t('Agreed so far'), value: total }, { label: t('Extra work'), value: `+ ${money(change.amountMinor, job.quote.currency)}` }, { label: t('New total'), value: newTotal, total: true }],
        primary: { label: t('Approve new total'), run: () => actions.act('approve-change') },
        secondary: { label: t('Keep plan'), run: () => actions.act('decline-change') },
        helper: job.paymentAuthorizedVersion ? t('The current PayPal hold is released. You pay the new total after approving the work.') : t('You pay the new total after approving the work.')
      };
    }
    case 'review': {
      if (!job.detailAdded) {
        return job.detailsRequested
          ? { kind: 'waiting', tag: { variant: 'waiting', label: t('Requested') }, title: t('Waiting for {name} to add the work note', { name: pro }), primary: { label: t('Approve the work'), disabled: true, reason: t('Available once the work note is added') } }
          : { kind: 'your-turn', tag: yourTurn, title: t('A work note is missing'), text: t('Ask {name} to describe the work before you approve it.', { name: pro }), primary: { label: t('Ask {name} for a note', { name: pro }), run: () => actions.act('request-details') }, secondary: { label: t('Approve the work'), disabled: true, reason: t('Available once the work note is added') } };
      }
      return {
        kind: 'your-turn', tag: yourTurn, title: t('{name} says it’s done. Check the work.', { name: pro }),
        primary: { label: t('Approve the work'), run: () => actions.act('approve-completion') },
        secondary: { label: t('Something’s not right'), run: ctx.openReport },
        helper: t('You pay next.'), after: [t('You approve the work'), t('You pay {name} with PayPal', { name: pro })]
      };
    }
    case 'payment': {
      const expired = app.paypalStatus === 'expired';
      const banner = expired ? { kind: 'warn' as const, title: t('Your PayPal hold expired'), text: t('Nothing was charged. Approve a new hold for the same amount and professional to pay.') } : undefined;
      if (!app.providerConfig?.paymentsAvailable) {
        return job.paymentApprovedVersion !== job.scopeVersion
          ? { kind: 'your-turn', tag: yourTurn, title: t('Pay {name}', { name: pro }), amount: total, primary: { label: t('Approve {amount}', { amount: total }), run: () => actions.act('approve-payment') } }
          : { kind: 'your-turn', tag: yourTurn, title: t('Pay {name}', { name: pro }), amount: total, primary: { label: t('PayPal · {amount}', { amount: total }), disabled: true, reason: t('Payments are not enabled here.') } };
      }
      if (job.paymentApprovedVersion === job.scopeVersion && CAPTURE_UNCERTAIN.includes(app.paypalStatus)) {
        return { kind: 'info', tag: { variant: 'pending', label: t('Checking payment') }, title: t('We’re checking your payment with PayPal'), amount: total, banner, secondary: { label: t('Check payment status'), run: () => actions.reconcilePayPal() } };
      }
      return {
        kind: 'your-turn', tag: yourTurn, title: t('Pay {name}', { name: pro }), amount: total, banner,
        text: app.paypalStatus === 'authorized' ? t('PayPal is holding the amount. Confirm to pay.') : undefined,
        primary: { label: t('PayPal · {amount}', { amount: total }), run: pay },
        helper: app.paypalStatus === 'authorized' ? t('No new PayPal approval needed. Paying captures the amount PayPal is holding.') : t('You confirm the amount on PayPal, then come back here.')
      };
    }
    case 'paid':
      return { kind: 'info', tag: { variant: 'done', label: t('Klaar') }, title: t('All done'), text: t('Paid {amount}.', { amount: total }), secondary: { label: t('Start a new request'), run: () => actions.createRequest() } };
    case 'dispute_review':
      return { kind: 'info', tag: { variant: 'pending', label: t('Under review') }, title: t('We’re looking into it'), text: t(job.paid ? 'A payment dispute is under review by our team.' : 'Payment is paused while our team reviews your report.') };
    case 'refund_pending':
      return { kind: 'info', tag: { variant: 'pending', label: t('Refund processing') }, title: t('Full refund processing'), text: t('Our team approved a full refund. PayPal is processing it.') };
    case 'refunded':
      return { kind: 'info', tag: { variant: 'neutral', label: t('Refunded') }, title: t('Full refund complete'), banner: { kind: 'success', text: t('Your refund is complete. {amount} goes back to your PayPal.', { amount: total }) } };
    case 'cancellation_review':
      return workspace.cancellationReview?.kind === 'withdrawal'
        ? { kind: 'info', tag: { variant: 'pending', label: t('Under review') }, title: t('Your withdrawal request is being reviewed'), text: t('Nothing is cancelled or refunded until a person on our team decides.') }
        : { kind: 'info', tag: { variant: 'pending', label: t('Under review') }, title: t('Your cancellation is being reviewed'), text: t('The visit stays booked until our team decides.') };
    case 'cancelled':
      return workspace.cancelledBy === 'professional'
        ? { kind: 'info', tag: { variant: 'neutral', label: t('Cancelled') }, title: t('This request is closed'), text: t('{name} couldn’t take this job. Nothing was charged. Your description is saved.', { name: pro }), primary: { label: t('Find another pro'), run: () => actions.act('return-to-matches') }, secondary: { label: t('Start a new request'), run: () => actions.createRequest() } }
        : { kind: 'info', tag: { variant: 'neutral', label: t('Cancelled') }, title: t('This request is closed'), text: t('Any reserved visit was released. Your history stays here.'), secondary: { label: t('Start a new request'), run: () => actions.createRequest() } };
    default:
      return null;
  }
}

export type ProContext = {
  app: AppState; actions: AppActions; workspace: WorkspaceState; code: RequestCode;
  noteReady: boolean; saveNote: () => unknown; openExtra: () => void;
};

export function proStep(ctx: ProContext): StepView | null {
  const { app, actions, workspace, code, noteReady, saveNote, openExtra } = ctx;
  const job = workspace.job;
  if (workspace.intake?.step === 'professional_review') return {kind:'waiting',tag:{variant:'waiting',label:t('Waiting')},title:t('The customer is reviewing your service'),text:t('No visit has been booked yet.')};

  if (!job.id) return null;
  const customer = shortName(job.customer || t('Customer'));
  const first = firstName(job.customer) || t('the customer');
  const total = money(job.quote.totalMinor, job.quote.currency);
  const visit = job.appointment ? visitRange(job.appointment.startUtc, job.appointment.endUtc, job.appointment.timezone) : undefined;
  const yourTurn = { variant: 'your-turn' as const, label: t('Your turn') };
  const waitingTag = { variant: 'waiting' as const, label: t('Waiting on {name}', { name: first }) };
  const message = { label: t('Message {name}', { name: first }), run: actions.focusComposer };
  const canExtra = job.quoteApproved && !job.paid && job.change?.status !== 'pending';
  const after = (from: number) => [t('You start the job on the day'), t('You send photos and a note when done'), t('{name} approves the work, then pays', { name: first })].slice(from);

  switch (code) {
    case 'quote':
      return { kind: 'waiting', tag: waitingTag, title: t('Waiting for {name} to approve the plan', { name: first }), text: t('{name} sees your published price. A different total goes as a price offer.', { name: first }), quiet: [{ label: t('Make a price offer'), run: () => actions.openProposal('price') }] };
    case 'booking': case 'choose_visit':
      return { kind: 'waiting', tag: waitingTag, title: code === 'choose_visit' ? t('Waiting for {name} to choose another time', { name: first }) : t('Waiting for {name} to choose a time', { name: first }), quiet: [{ label: t('Suggest a visit time'), run: () => actions.openProposal('date') }] };
    case 'review_price': {
      const offer = job.priceProposal!;
      return {
        kind: 'your-turn', tag: yourTurn, title: t('{name} proposed {amount}', { name: first, amount: money(offer.amountMinor, job.quote.currency) }), text: offer.note || undefined,
        primary: { label: t('Accept {amount}', { amount: money(offer.amountMinor, job.quote.currency) }), run: () => actions.act('accept-price', '', undefined, undefined, { proposalId: offer.id }) },
        secondary: { label: t('Counter with a price'), run: () => actions.openProposal('price') },
        quiet: [{ label: t('Decline offer'), run: () => actions.act('decline-price', '', undefined, undefined, { proposalId: offer.id }) }]
      };
    }
    case 'waiting_price': {
      const offer = job.priceProposal!;
      return { kind: 'waiting', tag: waitingTag, title: t('Waiting for {name}', { name: first }), amount: money(offer.amountMinor, job.quote.currency), text: t('The price stays {amount} until {name} accepts.', { amount: total, name: first }), quiet: [{ label: t('Change my offer'), run: () => actions.openProposal('price') }, { label: t('Withdraw offer'), run: () => actions.act('withdraw-price', '', undefined, undefined, { proposalId: offer.id }) }] };
    }
    case 'review_date': {
      const offer = job.dateProposal!;
      return {
        kind: 'your-turn', tag: yourTurn, title: t('{name} proposed a visit', { name: first }), amount: visitRange(offer.appointment.startUtc, offer.appointment.endUtc, offer.appointment.timezone), text: offer.note || undefined,
        primary: { label: t('Accept and book'), run: () => actions.act('accept-date', '', undefined, undefined, { proposalId: offer.id }) },
        secondary: { label: t('Suggest another time'), run: () => actions.openProposal('date') },
        helper: t('Accepting reserves this visit at the agreed price.'),
        quiet: [{ label: t('Decline visit'), run: () => actions.act('decline-date', '', undefined, undefined, { proposalId: offer.id }) }]
      };
    }
    case 'waiting_date': {
      const offer = job.dateProposal!;
      return { kind: 'waiting', tag: waitingTag, title: t('Waiting for {name}', { name: first }), amount: visitRange(offer.appointment.startUtc, offer.appointment.endUtc, offer.appointment.timezone), text: t('This time isn’t reserved until {name} accepts.', { name: first }), quiet: [{ label: t('Change my proposed visit'), run: () => actions.openProposal('date') }, { label: t('Withdraw proposal'), run: () => actions.act('withdraw-date', '', undefined, undefined, { proposalId: offer.id }) }] };
    }
    case 'authorize_booking':
      return {
        kind: 'waiting', tag: waitingTag, title: t('Waiting for {name} to authorize payment', { name: first }), amount: visit,
        text: t('{name} chose to authorize with PayPal at booking. You can confirm once that succeeds.', { name: first }),
        secondary: message, danger: { label: t('Decline and release the time'), run: () => actions.act('decline-appointment') }
      };
    case 'confirm_visit': {
      const due = job.appointmentConfirmation?.dueAt;
      return {
        kind: 'your-turn', tag: { variant: 'your-turn', label: due && job.appointment ? t('Confirm by {time}', { time: time(due, job.appointment.timezone) }) : t('Your turn') }, title: t('Confirm visit'), amount: visit,
        text: t('The time is held for you. Confirm it, or decline so {name} can choose another time.', { name: first }),
        primary: { label: t('Confirm visit'), run: () => actions.act('confirm-appointment') },
        secondary: { label: t('Decline'), run: () => actions.act('decline-appointment') },
        quiet: [message]
      };
    }
    case 'booked': {
      const today = !!job.appointment && sameDay(new Date(job.appointment.startUtc), new Date());
      return {
        kind: 'info', tag: today ? { variant: 'progress', label: t('Today') } : { variant: 'booked', label: t('Booked') }, title: today ? t('Visit day') : t('Visit booked'), amount: visit,
        text: job.serviceAddress || t('The customer adds the full address before approving the plan.'),
        list: { title: t('Proof needed when you’re done'), items: [t('Photos of the finished work'), t('A short work note')] },
        primary: { label: t('Start job'), run: () => actions.act('start-job'), disabled: pendingPolicies(job).length > 0, reason: pendingPolicies(job).length ? t('Waiting for {name} to acknowledge the booking terms', { name: first }) : undefined },
        secondary: job.serviceAddress ? { label: t('Directions'), href: `https://maps.google.com/?q=${encodeURIComponent(job.serviceAddress)}` } : undefined,
        helper: t('Tells {name} you’ve started.', { name: first }),
        quiet: [message, ...(canExtra ? [{ label: t('Propose extra work'), run: openExtra }] : [])]
      };
    }
    case 'in_progress': {
      const photos = job.evidence.filter((item) => item.assetId).length;
      const missing = missingItems(job);
      const reason = !photos ? t('Add at least one photo to continue') : !job.detailAdded ? t('Add a work note to continue') : missing.length ? t('Still needed: {list}', { list: missing.map((entry) => entry.item.description).join(', ') }) : '';
      return {
        kind: 'your-turn', tag: yourTurn, title: t('Finish and send proof'),
        text: reason ? undefined : t(photos === 1 ? '1 photo and a work note are ready.' : '{count} photos and a work note are ready.', { count: photos }),
        primary: { label: t('Send to {name}', { name: first }), run: () => actions.act('submit-evidence'), disabled: !!reason, reason },
        secondary: canExtra ? { label: t('Propose extra work'), run: openExtra } : undefined,
        helper: t('{name} approves the work, then pays {amount}.', { name: first, amount: total })
      };
    }
    case 'waiting_change':
      return { kind: 'waiting', tag: waitingTag, title: t('Extra work sent'), text: job.change?.description, primary: { label: t('Send to {name}', { name: first }), disabled: true, reason: t('Waiting for {name} to answer your extra work offer', { name: first }) } };
    case 'note_requested':
      return { kind: 'your-turn', tag: yourTurn, title: t('{name} asked for the work note', { name: first }), primary: { label: t('Add work note'), run: saveNote, disabled: !noteReady, reason: noteReady ? '' : t('Write at least 12 characters to continue') } };
    case 'review':
      return { kind: 'waiting', tag: { variant: 'pending', label: t('Customer checking') }, title: t('Sent for approval'), text: t('{name} checks your proof, then pays.', { name: first }), after: [t('{name} approves the work', { name: first }), t('{name} pays {amount}', { name: first, amount: total })] };
    case 'payment':
      return { kind: 'waiting', tag: waitingTag, title: t('Waiting for {name} to pay', { name: first }), amount: total, text: t('{name} approved the work.', { name: first }) };
    case 'paid':
      return { kind: 'info', tag: { variant: 'done', label: t('Klaar') }, title: t('Payment complete'), amount: total };
    case 'dispute_review':
      return { kind: 'info', tag: { variant: 'pending', label: t('Under review') }, title: t('Issue under review'), text: t(job.paid ? 'A payment dispute is under review by our team.' : 'Payment is paused while our team reviews the report.') };
    case 'refund_pending':
      return { kind: 'info', tag: { variant: 'pending', label: t('Refund processing') }, title: t('Full refund processing'), text: t('Our team approved a full refund. PayPal is processing it.') };
    case 'refunded':
      return { kind: 'info', tag: { variant: 'neutral', label: t('Refunded') }, title: t('Full refund complete') };
    case 'cancellation_review':
      return workspace.cancellationReview?.kind === 'withdrawal'
        ? { kind: 'info', tag: { variant: 'pending', label: t('Under review') }, title: t('Withdrawal request under review'), text: t('{name} asked to withdraw from the service. Our team decides; nothing changes until then.', { name: first }) }
        : { kind: 'info', tag: { variant: 'pending', label: t('Under review') }, title: t('Cancellation under review'), text: t('The visit stays booked until our team decides.') };
    case 'cancelled':
      return { kind: 'info', tag: { variant: 'neutral', label: t('Cancelled') }, title: t('This job is closed'), text: t('Any reserved visit was released. {customer}’s request stays in their account.', { customer }) };
    default:
      return null;
  }
}
