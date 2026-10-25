import { t } from './i18n.svelte';
import type { WorkspaceRole, WorkspaceState } from './types';

// Colour means state (docs/DESIGN_SYSTEM.md, section 2.1).
export type PillVariant = 'your-turn' | 'waiting' | 'booked' | 'progress' | 'pending' | 'warn' | 'error' | 'done' | 'neutral' | 'best' | 'smart' | 'fact';
// 1–6 is the journey position, 'done' is all six steps finished, null hides the stepper (closed requests).
export type JourneyStep = 1 | 2 | 3 | 4 | 5 | 6 | 'done' | null;
export type ListFilter = 'your-turn' | 'progress' | 'done' | 'new' | 'active' | 'finished';
export type StatusUi = {
  pill: PillVariant; pillLabel: string; icon?: 'check' | 'calendar' | 'alert';
  line: string; title: string; rank: number; step: JourneyStep;
};

export const JOURNEY = ['Describe', 'Match', 'Agree', 'Do the job', 'Check', 'Pay'] as const;

const RANK: Record<PillVariant, number> = { 'your-turn': 0, booked: 1, progress: 1, waiting: 2, pending: 3, warn: 3, error: 3, neutral: 4, done: 5, best: 4, smart: 4, fact: 4 };

// The other person's first name. Go list summaries put it in "Waiting for {name}" labels.
// Only the waiting_* codes put a person's name in the label.
export function otherFromLabel(label?: string): string {
  if (!label?.startsWith('Waiting for ') || label === 'Waiting for professional confirmation') return '';
  return firstName(label.slice('Waiting for '.length));
}
const capitalize = (text: string) => text.charAt(0).toUpperCase() + text.slice(1);

// An account without a display name falls back to its email; show only the part before the @.
export function firstName(name?: string): string {
  // A trading name written in capitals ("THE BUSINESS") is not a person's name; keep it whole.
  const whole = (name ?? '').trim();
  if (whole.includes(' ') && whole === whole.toUpperCase() && /[A-Z]/.test(whole)) return whole;
  const first = (name ?? '').trim().split(/\s+/)[0] ?? '';
  return first.includes('@') ? first.split('@')[0] : first;
}

export function statusUi(code: string, role: WorkspaceRole | null, other = '', stepOverride?: JourneyStep): StatusUi {
  const pro = role === 'professional';
  const name = other || t(pro ? 'the customer' : 'the professional');
  const ui = (pill: PillVariant, pillLabel: string, step: JourneyStep, line: string, title = line, icon?: StatusUi['icon']): StatusUi =>
    ({ pill, pillLabel, icon, line: capitalize(line), title: capitalize(title), rank: RANK[pill], step: stepOverride !== undefined ? stepOverride : step });
  const yourTurn = (step: JourneyStep, line: string, title = line, label = t('Your turn')) => ui('your-turn', label, step, line, title);
  const waiting = (step: JourneyStep, line: string, title = line) => ui('waiting', other ? t('Waiting on {name}', { name: other }) : t('Waiting'), step, line, title);

  switch (code) {
    case 'draft': case 'describing': return ui('neutral', t('Draft'), 1, t('Describing the job'), t('Your job so far'));
    case 'matching': return yourTurn(2, t('Choose a professional'));
    case 'waitlist': return ui('neutral', t('No coverage'), 2, t('On the area waitlist'));
    case 'unmatched': return ui('neutral', t('No suitable service identified'), 2, t('Edit request'));
    case 'review_professional': return pro ? waiting(3,t('The customer is reviewing your service')) : yourTurn(3,t('Review the professional’s service'));
    case 'quote': return pro ? waiting(3, t('Waiting for plan approval'), t('Waiting for {name} to approve the plan', { name })) : yourTurn(3, t('Confirm the plan'), t('Book your visit'));
    case 'booking': return pro ? waiting(3, t('Waiting for a visit time'), t('Waiting for {name} to choose a time', { name })) : yourTurn(3, t('Choose a visit time'), t('Book your visit'));
    case 'review_price': return yourTurn(3, t('Review the price offer'), t('{name} proposed a price', { name }));
    case 'waiting_price': return waiting(3, t('Offer sent to {name}', { name }), t('Waiting for {name}', { name }));
    case 'review_date': return yourTurn(3, t('Review the proposed visit'), t('{name} proposed a visit', { name }));
    case 'waiting_date': return waiting(3, t('Visit sent to {name}', { name }), t('Waiting for {name}', { name }));
    case 'authorize_booking': return pro ? waiting(3, t('Waiting for payment authorization'), t('Waiting for {name} to authorize payment', { name })) : yourTurn(3, t('Authorize the payment'));
    case 'confirm_visit': return yourTurn(4, t('Confirm the reserved visit'), t('Confirm visit'), t('Confirm visit'));
    case 'waiting_visit_confirmation': return waiting(4, t('{name} confirms the visit', { name }), t('Time held. {name} confirms next', { name }));
    case 'choose_visit': return pro ? waiting(3, t('Waiting for another time'), t('Waiting for {name} to choose another time', { name })) : yourTurn(3, t('Choose another time'));
    case 'booked': return pro ? ui('booked', t('Booked'), 4, t('Visit booked'), t('Visit booked'), 'calendar') : ui('booked', t('Scheduled'), 4, t('Visit booked'), t('You’re booked'), 'calendar');
    case 'in_progress': return pro ? ui('progress', t('In progress'), 4, t('Working on it'), t('Finish and send proof')) : ui('progress', t('In progress'), 4, t('{name} is working on it', { name }));
    case 'review_change': return yourTurn(4, t('Approve extra work'), t('{name} found more to fix', { name }));
    case 'waiting_change': return waiting(4, t('Extra work sent'), t('Waiting for {name}', { name }));
    case 'review': return pro ? ui('pending', t('Customer checking'), 5, t('Sent for approval')) : yourTurn(5, t('Check the work'), t('{name} says it’s done. Check the work.', { name }));
    case 'note_requested': return yourTurn(5, t('Add the work note'), t('{name} asked for the work note', { name }));
    case 'payment': return pro ? waiting(6, t('Waiting for payment'), t('Waiting for {name} to pay', { name })) : yourTurn(6, t('Pay {name}', { name }));
    case 'paid': return pro ? ui('neutral', t('Paid'), 'done', t('Paid'), t('Payment complete'), 'check') : ui('done', t('Klaar'), 'done', t('Klaar, paid'), t('All done'), 'check');
    case 'dispute_review': return ui('pending', t('Under review'), 5, t('Payment paused'), pro ? t('Issue under review') : t('We’re looking into it'));
    case 'refund_pending': return ui('pending', t('Refund processing'), null, t('Refund processing'), t('Full refund processing'));
    case 'refunded': return ui('neutral', t('Refunded'), null, t('Refunded'), t('Full refund complete'));
    case 'issue_resolved': return ui('progress', t('Reviewed'), 5, t('Issue reviewed'));
    case 'cancellation_review': return ui('pending', t('Under review'), 4, t('Cancellation under review'), pro ? t('Cancellation under review') : t('Your cancellation is being reviewed'));
    case 'cancelled': return ui('neutral', t('Cancelled'), null, t('Cancelled'), pro ? t('This job is closed') : t('This request is closed'));
    default: return ui('neutral', t('Request'), null, t('Request'));
  }
}

// Customer tabs: All · Your turn · In progress · Done. Professional tabs: All · New · Active · Finished.
export function listFilter(code: string, role: WorkspaceRole | null): ListFilter {
  if (role === 'professional') {
    if (['paid', 'refunded', 'cancelled', 'issue_resolved'].includes(code)) return 'finished';
    if (['booked', 'in_progress', 'review_change', 'waiting_change', 'review', 'note_requested', 'payment', 'dispute_review', 'refund_pending', 'cancellation_review'].includes(code)) return 'active';
    return 'new';
  }
  if (['paid', 'refunded', 'cancelled'].includes(code)) return 'done';
  return statusUi(code, role).pill === 'your-turn' ? 'your-turn' : 'progress';
}

export function canCancelRequest(workspace: WorkspaceState): boolean {
  const job = workspace.job;
  return !workspace.cancelled && workspace.cancellationReview?.status !== 'pending' && !job.workStarted && !job.evidenceSubmitted && !job.completionApproved && !job.paid && !job.paymentApprovedVersion;
}
