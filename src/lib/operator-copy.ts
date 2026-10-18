import { t } from './i18n.svelte';
import type { OperatorCase } from './types';

// Stalled-job reasons arrive as codes so operators read them in their own language.
const REASONS: Record<string, string> = {
  'price-proposal-reminder': 'The price proposal has had no answer for 24 hours. Contact the recipient and record the outcome.',
  'date-proposal-reminder': 'The proposed visit time has had no answer for 24 hours. Contact both people and settle the open proposal.',
  'scope-change-reminder': 'The extra work offer has had no answer for 24 hours. Contact the customer. The current plan stays as agreed.',
  'missed-visit-reminder': 'The confirmed visit ended over 24 hours ago and no work was started. Find out what happened.',
  'evidence-overdue': 'Completion proof is still missing 24 hours after the visit. Contact the professional.',
  'completion-note-overdue': 'The work note has been missing for 24 hours. Contact the professional.',
  'completion-review-reminder': 'The customer hasn’t checked the completed work within 24 hours. Contact the customer.',
  'payment-approval-reminder': 'The payment decision has been waiting for 24 hours. Contact the customer.',
  'checkout-reminder': 'Payment was approved 24 hours ago but isn’t finished. Ask the customer to check their PayPal status.'
};

export function caseDescription(item: OperatorCase): string {
  if (item.kind === 'stalled-job' && item.reasonCode && REASONS[item.reasonCode]) {
    const who = item.responsible ? t(item.responsible === 'professional' ? 'Waiting on the professional.' : 'Waiting on the customer.') : '';
    return [t(REASONS[item.reasonCode]), who].filter(Boolean).join(' ');
  }
  return item.description;
}
