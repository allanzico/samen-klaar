import type { WorkspaceState, WorkspaceRole } from './types';

export function requestStatus(state: WorkspaceState | null, role: WorkspaceRole | null): { code: string; label: string } {
  if (!state) return { code: 'loading', label: 'Loading your request' };
  const job = state.job;
  if (state.cancelled) return { code: 'cancelled', label: 'Cancelled' };
  if (state.cancellationReview?.status === 'pending') return { code: 'cancellation_review', label: 'Cancellation request under review' };
  if (!job.id) return { code: state.intake?.step === 'matches' ? 'matching' : 'describing', label: state.intake?.step === 'matches' ? 'Review matching services' : 'Tell us about the job' };
  const other = role === 'professional' ? job.customer : job.professional;
  if (job.dispute?.status === 'open') return { code: 'dispute_review', label: job.paid ? 'Payment dispute under review' : 'Issue under review · payment paused' };
  if (job.dispute?.status === 'refund_pending') return { code: 'refund_pending', label: 'Full refund processing' };
  if (job.dispute?.status === 'refunded') return { code: 'refunded', label: 'Full refund complete' };
  if (job.paid) return { code: 'paid', label: 'Payment complete' };
  if (job.appointmentConfirmation?.status === 'pending') return { code: 'appointment_confirmation', label: role === 'professional' ? 'Confirm this visit' : 'Waiting for visit confirmation' };
  if (job.appointmentConfirmation?.status === 'expired' || job.appointmentConfirmation?.status === 'declined') return { code: 'choose_new_visit', label: 'Choose another visit time' };
  if (job.change?.status === 'pending') return { code: 'change', label: role === 'customer' ? 'Review the additional work' : `Waiting for ${other}` };
  if (job.dateProposal?.status === 'pending') return { code: 'date', label: job.dateProposal.actor === role ? `Waiting for ${other}` : 'Review the proposed visit' };
  if (job.priceProposal?.status === 'pending') return { code: 'price', label: job.priceProposal.actor === role ? `Waiting for ${other}` : 'Review the price offer' };
  if (job.completionApproved) return { code: 'payment', label: 'Ready for payment' };
  if (job.evidenceSubmitted) return { code: 'review', label: 'Review completion' };
  if (job.workStarted) return { code: 'working', label: 'Work in progress' };
  if (job.appointment) return { code: 'booked', label: 'Visit booked' };
  if (job.quoteApproved) return { code: 'booking', label: 'Choose or propose a visit' };
  return { code: 'quote', label: role === 'customer' ? 'Review the plan and price' : 'Waiting for customer approval' };
}
