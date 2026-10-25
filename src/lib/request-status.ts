import type { WorkspaceState, WorkspaceRole } from './types';

// Mirrors RequestStatus() in internal/workflow/request_status.go so an open request and its list
// summary use the same codes. A few client-only codes refine states the summary cannot see:
// waitlist, authorize_booking and note_requested.
export type RequestCode =
  | 'loading' | 'draft' | 'describing' | 'matching' | 'waitlist'
  | 'unmatched' | 'review_professional' | 'quote' | 'booking' | 'review_price' | 'waiting_price' | 'review_date' | 'waiting_date'
  | 'authorize_booking' | 'confirm_visit' | 'waiting_visit_confirmation' | 'choose_visit'
  | 'booked' | 'in_progress' | 'review_change' | 'waiting_change'
  | 'review' | 'note_requested' | 'payment' | 'paid'
  | 'dispute_review' | 'refund_pending' | 'refunded' | 'issue_resolved'
  | 'cancellation_review' | 'cancelled';

export function requestStatus(state: WorkspaceState | null, role: WorkspaceRole | null): RequestCode {
  if (!state) return 'loading';
  const job = state.job;
  if (state.cancelled) return 'cancelled';
  if (state.cancellationReview?.status === 'pending') return 'cancellation_review';
  if (!job.id) {
    if (!state.intake) return 'draft';
    if (state.intake.step === 'matches') return 'matching';
    if (state.intake.step === 'waitlist') return 'waitlist';
    if (state.intake.step === 'unmatched') return 'unmatched';
    return 'describing';
  }
  if (state.intake?.step === 'professional_review') return 'review_professional';
  switch (job.dispute?.status) {
    case 'open': case 'partial_refund_review': case 'payment_reversal_review': return 'dispute_review';
    case 'refund_pending': return 'refund_pending';
    case 'refunded': return 'refunded';
  }
  if (job.paid) return 'paid';
  if (job.appointmentConfirmation?.status === 'pending') {
    if (job.paymentAuthorizationTiming === 'at_booking' && job.paymentAuthorizedVersion !== job.scopeVersion) return 'authorize_booking';
    return role === 'professional' ? 'confirm_visit' : 'waiting_visit_confirmation';
  }
  if (job.appointmentConfirmation?.status === 'declined' || job.appointmentConfirmation?.status === 'expired') return 'choose_visit';
  if (job.change?.status === 'pending') return role === 'customer' ? 'review_change' : 'waiting_change';
  if (job.dateProposal?.status === 'pending') return job.dateProposal.actor === role ? 'waiting_date' : 'review_date';
  if (job.priceProposal?.status === 'pending') return job.priceProposal.actor === role ? 'waiting_price' : 'review_price';
  if (job.completionApproved) return 'payment';
  if (job.evidenceSubmitted) return role === 'professional' && job.detailsRequested && !job.detailAdded ? 'note_requested' : 'review';
  if (job.workStarted) return 'in_progress';
  if (job.appointment) return 'booked';
  if (job.quoteApproved) return 'booking';
  return 'quote';
}
