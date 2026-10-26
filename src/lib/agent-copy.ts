import type { AgentActivity, AgentEscalation } from './types';
import { dayTime, decimalMoney } from './format';
import { t } from './i18n.svelte';

// Agent parameters arrive as strings: amounts as decimal euros, times as ISO timestamps.
function fill(params: Record<string, string> = {}): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(params)) {
    if (['start', 'end'].includes(key)) out[key] = dayTime(value);
    else if (['amount', 'price', 'limit', 'floor', 'offered', 'published'].includes(key)) out[key] = decimalMoney(value);
    else out[key] = value;
  }
  return out;
}

export function activityText(item: AgentActivity): string {
  const params = fill(item.params);
  if (!item.action) return t('Stopped and asked you: {question}', { question: questionText(item) });
  if (item.outcome === 'rejected') return t('Tried “{step}”, but it was not allowed: {reason}', { step: item.action ?? item.code, reason: item.error ?? '' });
  switch (item.code) {
    case 'ask-booking-autonomy': case 'ask-address': case 'ask-availability': case 'ask-offer': case 'ask-confirm': case 'ask-move': case 'ask-cancel': case 'ask-work': return t('Asked you a question in the chat');
    case 'tell-ask': return t('Answered your message in the chat');
    case 'tell-moved': return t('Told you your request to move the visit is with the professional');
    case 'tell-refused': return t('Told you the professional could not take the new time');
    case 'tell-move-review': case 'tell-cancel-review': return t('Told you our operator team will decide');
    case 'tell-cancelled': return t('Told you the request is cancelled');
    case 'tell-kept': return t('Left the visit as it is, as you said');
    case 'tell-work-hold': return t('Left the finished work for you to decide, as you said');
    case 'tell-payment': return t('Told you the payment step is yours in PayPal');
    case 'tell-paid': return item.role === 'professional' ? t('Told you the payment arrived') : t('Told you the payment is complete');
    case 'tell-work-approved': return t('Told you the customer approved the finished work');
    case 'tell-payment-approved': return t('Told you the customer approved the payment in PayPal');
    case 'reschedule-visit': return t('Asked the professional to move the visit to {start}', params);
    case 'cancel-visit': return t('Cancelled the visit, as you asked');
    case 'tell-booked': return t('Told you the visit was reserved');
    case 'tell-confirmed': return item.role === 'professional' ? t('Told you the visit is confirmed') : t('Told you the professional confirmed the visit');
    case 'tell-nothing': return t('Told you no open time fits');
    case 'tell-manual': return t('Left the price for you to approve');
    case 'decline-visit': return t('Declined the suggested visit time, as you said');
    case 'decline-price': return t('Declined the price offer, as you said');
    case 'select-match': return t('Chose {name} at {price}', params);
    case 'set-address': return t('Shared your service address with the professional');
    case 'book-visit': return t('Approved the plan and booked {start} at {amount}', params);
    case 'counter-price': return t('Proposed a total of {amount}', params);
    case 'accept-price': return t('Accepted the price of {amount}', params);
    case 'approve-plan': return t('Approved the plan and price of {amount}', params);
    case 'remember-rejected-days': return t('Remembered the visit days you rejected');
    case 'propose-visit': return t('Proposed a visit on {start}', params);
    case 'accept-visit': return t('Accepted the visit on {start}', params);
    case 'approve-extra': return t('Approved extra work of {amount}', params);
    case 'check-completion-photos': return t('Checked the completion photos for this booking');
    case 'approve-delegated-payment': return t('Approved release of the agreed amount under your booking permission');
    case 'release-delegated-payment': return t('Released the authorized payment under your booking permission');
    case 'approve-work': return t('Approved the finished work for you');
    case 'request-clearer-completion': return t('Asked the professional to improve the completion photos or work note');
    case 'confirm-visit': return t('Confirmed the visit on {start}', params);
    case 'find-another': return t('Went back to the matches to find another professional');
    default: return t('Took a step for you');
  }
}

// A line an assistant left in the shared thread when it negotiated for its person, in the viewer's language.
export function noteText(message: { code?: string; text: string; params?: Record<string, string> }): string {
  const params = fill(message.params);
  switch (message.code) {
    case 'customer:counter-price': return t('Could you do this for {amount}? That is the most my customer can agree to.', params);
    case 'professional:counter-price': return t('The lowest I can do is {amount}.', params);
    case 'customer:accept-price': case 'professional:accept-price': return t('Agreed on {amount}.', params);
    case 'customer:propose-visit': return t('Would {start} work? None of your listed times fit my customer.', params);
    case 'professional:propose-visit': return t('How about {start}? It is the closest time I have.', params);
    case 'customer:reschedule-visit': return t('Could we move the visit to {start}? My customer asked for it.', params);
    case 'customer:accept-visit': case 'professional:accept-visit': return t('Agreed: {start}.', params);
    default: return message.text;
  }
}

export function questionText(item: Pick<AgentEscalation, 'code' | 'params'>): string {
  const params = fill(item.params);
  switch (item.code) {
    case 'review_plan': return t('Review the price and payment options. Your assistant waits until you choose Continue.');
    case 'review_professional': return t('Review the selected professional and choose Continue when you are ready.');
    case 'choose_professional': return t('Review the professionals and choose who you want to continue with.');
    case 'approve_price': return t('Approve the price of {price}, or ask the professional for a different one.', params);
    case 'awaiting_reply': return t('Your assistant asked you a question in the chat. Answer it there.');
    case 'agent_misunderstood': return t('Your assistant could not understand your answer. Please use the buttons on this page.');
    case 'address_needed': return t('Your agent needs your service address before it can book.');
    case 'price_above_limit': return t('The price of {price} is above your limit of {limit}. Your agent has stopped.', params);
    case 'no_time_in_window': return t('None of the free times fit the days and hours you allowed.');
    case 'visit_outside_window': return t('A visit on {start} is outside the days and hours you allowed.', params);
    case 'extra_work_needs_approval': return t('Extra work of {amount} needs your approval.', params);
    case 'booking_autonomy_choice': return t('Choose who reviews completion and releases payment');
    case 'autonomy_review_needed': return t('The completion record needs your review. Your assistant has not released payment.');
    case 'check_the_work': return t('The work is ready. Only you can approve it.');
    case 'payment_needs_you': return t('The work is approved. Payment is yours to confirm.');
    case 'no_match_within_limits': return t('No professional fits your price limit and time window.');
    case 'clarification_needed': return t('The professional asked a question only you can answer.');
    case 'confirm_needed': return t('A visit is waiting for your confirmation.');
    case 'price_below_floor': return t('The customer offered {offered}, below your lowest price of {floor}.', params);
    case 'visit_proposal_needs_you': return t('A visit on {start} was proposed and needs your decision.', params);
    case 'late_change_needs_you': return t('The visit on {start} is less than 24 hours away. Changes this late need operator review, so your agent leaves it to you.', params);
    case 'visit_change_involves_payment': return t('A new visit time of {start} touches your payment. Your agent leaves it to you.', params);
    case 'agent_blocked': return t('Your agent tried a step that was not allowed: {reason}', params);
    default: return t('Your agent needs you here.');
  }
}
