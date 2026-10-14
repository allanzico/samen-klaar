import { t } from './i18n.svelte';
import type { WorkspaceRole, WorkspaceState } from './types';

export type PillVariant = 'your-turn' | 'waiting' | 'scheduled' | 'done' | 'review' | 'error' | 'neutral';
export type StatusUi = { pill: PillVariant; pillLabel: string; line: string; title: string; rank: number };

const RANK: Record<PillVariant, number> = { 'your-turn': 0, scheduled: 1, waiting: 2, error: 3, review: 3, neutral: 4, done: 5 };

// Codes come from two vocabularies: the frontend requestStatus() and the Go list summaries.
// Both are mapped here so the pill, list line and next-step title never disagree (handoff section 6).
export function statusUi(code: string, label: string, role: WorkspaceRole | null, name?: string): StatusUi {
  const pro = role === 'professional';
  const waitingName = label.startsWith('Waiting for ') ? label.slice('Waiting for '.length) : name;
  const waiting = (line: string, title = line): StatusUi => ui('waiting', waitingName ? t('Waiting on {name}', { name: waitingName }) : t('Waiting'), line, title);
  const ui = (pill: PillVariant, pillLabel: string, line: string, title = line): StatusUi => ({ pill, pillLabel, line, title, rank: RANK[pill] });
  const yourTurn = (line: string, title = line, pillLabel = t('Your turn')) => ui('your-turn', pillLabel, line, title);
  const isWaiting = label.startsWith('Waiting for ');

  switch (code) {
    case 'draft': case 'describing': return ui('neutral', t('Describing'), t('Describing the job'));
    case 'matching': return yourTurn(t('Choose a professional'));
    case 'quote': return pro ? waiting(t('Waiting for the customer to confirm the plan')) : yourTurn(t('Confirm the plan'), t('To book, confirm three things'));
    case 'booking': return pro ? waiting(t('Waiting for the customer to choose a time')) : yourTurn(t('Choose a visit time'));
    case 'choose_visit': case 'choose_new_visit': return pro ? waiting(t('Waiting for the customer to choose another time')) : yourTurn(t('Choose another time'));
    case 'review_price': return yourTurn(t('Review the price offer'));
    case 'review_date': return yourTurn(t('Review the proposed visit'));
    case 'price': return isWaiting ? waiting(t('Offer sent to {name}', { name: waitingName ?? '' }), t('Waiting for the price answer')) : yourTurn(t('Review the price offer'));
    case 'date': return isWaiting ? waiting(t('Offer sent to {name}', { name: waitingName ?? '' }), t('Waiting for the visit answer')) : yourTurn(t('Review the proposed visit'));
    case 'waiting_price': case 'waiting_date': return waiting(t('Offer sent to {name}', { name: waitingName ?? '' }));
    case 'waiting_visit_confirmation': return waiting(t('Waiting for the professional to confirm'), t('Time held while the professional confirms'));
    case 'confirm_visit': return yourTurn(t('Confirm the reserved visit'), t('Confirm visit'), t('Confirm visit'));
    case 'appointment_confirmation': return pro ? yourTurn(t('Confirm the reserved visit'), t('Confirm visit'), t('Confirm visit')) : waiting(t('Waiting for the professional to confirm'), t('Time held while the professional confirms'));
    case 'review_change': return yourTurn(t('Review extra work'));
    case 'waiting_change': return waiting(t('Extra work sent to {name}', { name: waitingName ?? '' }));
    case 'change': return pro || isWaiting ? waiting(t('Extra work sent to {name}', { name: waitingName ?? '' })) : yourTurn(t('Review extra work'));
    case 'booked': return ui('scheduled', t('Scheduled'), t('Visit booked'), t('You’re booked'));
    case 'in_progress': case 'working': return ui('scheduled', t('In progress'), t('Work in progress'), t('Work in progress'));
    case 'review': return pro ? waiting(t('Waiting for the customer to check the work')) : yourTurn(t('Check the work'));
    case 'payment': return pro ? waiting(t('Waiting for the customer to pay')) : yourTurn(t('Pay for the finished work'), t('Pay'));
    case 'paid': return ui('done', t('Klaar'), t('Paid'), t('All done'));
    case 'dispute_review': return ui('review', t('Under review'), t('Payment paused'), t('We’re looking into it'));
    case 'refund_pending': return ui('review', t('Refund processing'), t('Refund processing'));
    case 'refunded': return ui('neutral', t('Refunded'), t('Refunded'));
    case 'cancellation_review': return ui('review', t('Under review'), t('Cancellation under review'));
    case 'cancelled': return ui('neutral', t('Cancelled'), t('Cancelled'));
    case 'issue_resolved': return ui('neutral', t('Resolved'), t('Issue reviewed'));
    default: return ui('neutral', t('Request'), label ? t(label) : t('Request'));
  }
}

export function canCancelRequest(workspace: WorkspaceState): boolean {
  const job = workspace.job;
  return !workspace.cancelled && workspace.cancellationReview?.status !== 'pending' && !job.workStarted && !job.evidenceSubmitted && !job.completionApproved && !job.paid && !job.paymentApprovedVersion;
}
