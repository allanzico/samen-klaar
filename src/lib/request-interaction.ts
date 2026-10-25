import type { WorkspaceRole, WorkspaceState } from './types';

function requiresManualReview(workspace: WorkspaceState) {
  return workspace.intake?.step === 'professional_review' || workspace.cancelled || workspace.job.paid || workspace.cancellationReview?.status === 'pending'
    || ['open', 'partial_refund_review', 'payment_reversal_review', 'refund_pending', 'refunded'].includes(workspace.job.dispute?.status ?? '')
    || (workspace.job.paymentAuthorizationTiming === 'at_booking' && workspace.job.appointmentConfirmation?.status === 'pending' && workspace.job.paymentAuthorizedVersion !== workspace.job.scopeVersion);
}

// The saved dialog owns the next interaction. A history message alone must never reopen a step.
export function requestInteraction(workspace: WorkspaceState, role: WorkspaceRole) {
  if (!workspace.job.id) return undefined;
  const dialog = role === 'customer' ? workspace.agentDialog : role === 'professional' ? workspace.proDialog : undefined;
  const job = workspace.job;
  if (!dialog?.awaiting || requiresManualReview(workspace)) return undefined;
  if (dialog.stage === 'address' && job.serviceAddress) return undefined;
  if (dialog.pending === 'approve-quote' && job.quoteApproved) return undefined;
  if (dialog.pending === 'accept-price' && (job.priceProposal?.status !== 'pending' || job.priceProposal.id !== dialog.pendingId)) return undefined;
  if (dialog.pending === 'accept-date' && (job.dateProposal?.status !== 'pending' || job.dateProposal.id !== dialog.pendingId)) return undefined;
  const messages = workspace.intake?.messages ?? [];
  const question = dialog.asked
    ? messages.find((message) => message.seq === dialog.asked && message.role === 'agent' && message.audience === role)
    : [...messages].reverse().find((message) => message.role === 'agent' && message.audience === role);
  if (!question) return undefined;
  return { dialog, question, address: role === 'customer' && dialog.stage === 'address' };
}

// A saved answer can arrive before the background assistant's next message.
// Keep that brief interval in the same interaction instead of flashing every manual control.
export function requestReplyKey(workspace: WorkspaceState, role: WorkspaceRole): string {
  if (!workspace.job.id) return '';
  if (requiresManualReview(workspace)) return '';
  const dialog = role === 'customer' ? workspace.agentDialog : workspace.proDialog;
  if (!dialog) return '';
  if (dialog.awaiting) return dialog.stage === 'address' && !!workspace.job.serviceAddress ? `${workspace.workspaceId}:address:${dialog.asked}` : '';
  const messages = (workspace.intake?.messages ?? []).filter((message) => !message.audience || message.audience === 'both' || message.audience === role);
  const last = messages[messages.length - 1];
  return last?.role === role && last.audience === role ? `${workspace.workspaceId}:reply:${last.seq}` : '';
}
