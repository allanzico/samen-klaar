import type { WorkspaceState } from './types';

export function canReviseUnbookedRequest(state: WorkspaceState) {
  const job = state.job;
  return !state.cancelled && !job.appointment && !job.appointmentConfirmation && !job.paymentAuthorizedVersion && !job.paymentApprovedVersion
    && !job.workStarted && !job.evidenceSubmitted && !job.evidence.length && !job.completionApproved && !job.paid
    && !job.paymentTransactionId && !job.dispute && !job.review && !state.cancellationReview;
}
export function previousIntakeStep(step: string) {
  return ({ detail:'description', urgency:'detail', budget:'urgency', location:'budget', matches:'location', waitlist:'location', unmatched:'description' } as Record<string,string>)[step];
}
