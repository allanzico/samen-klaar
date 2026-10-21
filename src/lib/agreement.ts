import { t } from './i18n.svelte';
import { money, sameDay, time, dayTime, visitRange } from './format';
import { firstName } from './status-ui';
import type { Job, WorkspaceRole } from './types';
import type { IconName } from '../ui/icons';

// What's agreed: price, visit, address and payment, each with its own state.
export type Term = { key: 'price' | 'extra' | 'visit' | 'address' | 'payment'; icon: IconName; label: string; value: string; short: string; agreed: boolean };

export function agreementTerms(job: Job, role: WorkspaceRole | null): Term[] {
  const pro = role === 'professional';
  const other = firstName(pro ? job.customer : job.professional) || t(pro ? 'the customer' : 'the professional');
  const currency = job.quote.currency;
  const priceAgreed = job.quoteApproved;
  const terms: Term[] = [];

  // Extra work is a line the professional proposed and the customer approved; a negotiated price also bumps the scope version.
  const scopeChanged = job.quote.lines.some((line) => line.basis?.startsWith('Professional proposal'));
  const negotiated = job.scopeVersion > 1;
  terms.push({
    key: 'price', icon: priceAgreed ? 'check' : 'clock', agreed: priceAgreed,
    label: priceAgreed ? t('Price, agreed') : job.priceProposal?.status === 'pending' ? t('Price, offer open') : t('Price'),
    value: priceAgreed ? (job.workStarted && !job.paid ? t('{amount} so far', { amount: money(job.quote.totalMinor, currency) }) : scopeChanged ? t('{amount} with extra', { amount: money(job.quote.totalMinor, currency) }) : money(job.quote.totalMinor, currency)) : t(negotiated ? '{amount} negotiated' : '{amount} published', { amount: money(job.quote.totalMinor, currency) }),
    short: money(job.quote.totalMinor, currency)
  });
  if (job.change?.status === 'pending') {
    terms.push({ key: 'extra', icon: 'clock', agreed: false, label: t('Extra work, waiting'), value: `+ ${money(job.change.amountMinor, currency)}`, short: `+ ${money(job.change.amountMinor, currency)}` });
  }

  const confirming = job.appointmentConfirmation?.status === 'pending' ? job.appointment : null;
  const proposal = job.dateProposal?.status === 'pending' ? job.dateProposal.appointment : null;
  const visit = job.appointment;
  if (visit) {
    const today = sameDay(new Date(visit.startUtc), new Date());
    terms.push({
      key: 'visit', icon: confirming ? 'clock' : 'check', agreed: !confirming,
      label: confirming ? t('Visit, held') : job.workStarted ? t('Visit, started') : today ? t('Visit, today') : t('Visit, booked'),
      value: today ? t('{start} to {end}', { start: time(visit.startUtc, visit.timezone), end: time(visit.endUtc, visit.timezone) }) : dayTime(visit.startUtc, visit.timezone),
      short: today ? t('today {start} to {end}', { start: time(visit.startUtc, visit.timezone), end: time(visit.endUtc, visit.timezone) }) : dayTime(visit.startUtc, visit.timezone)
    });
  } else if (proposal) {
    terms.push({ key: 'visit', icon: 'clock', agreed: false, label: t('Visit, proposed'), value: visitRange(proposal.startUtc, proposal.endUtc, proposal.timezone), short: dayTime(proposal.startUtc, proposal.timezone) });
  } else {
    terms.push({ key: 'visit', icon: 'clock', agreed: false, label: t('Visit'), value: t('Not chosen yet'), short: '' });
  }

  terms.push({
    key: 'address', icon: 'lock', agreed: !!job.serviceAddress, label: t('Address'),
    value: pro ? job.serviceAddress || t('Not added yet') : job.serviceAddress ? (visit ? t('Shared with {name}', { name: other }) : t('Only {name} will see it', { name: other })) : t('Not added yet'),
    short: ''
  });

  const authorized = job.scopeVersion > 0 && job.paymentAuthorizedVersion === job.scopeVersion;
  terms.push({
    key: 'payment', icon: job.paid ? 'check' : 'clock', agreed: job.paid, label: t('Payment'),
    value: job.paid ? t('Paid') : authorized ? (pro ? t('Held by PayPal') : t('Held by PayPal until you approve')) : job.paymentAuthorizationTiming === 'at_booking' ? t('PayPal approval needed for the booking hold') : pro ? t('After {name} approves', { name: other }) : t('After you approve the work'),
    short: ''
  });
  return terms;
}

export function agreementSummary(job: Job, role: WorkspaceRole | null): string {
  const terms = agreementTerms(job, role);
  const price = terms.find((term) => term.key === 'price');
  const visit = terms.find((term) => term.key === 'visit');
  return [price?.value, visit?.short].filter(Boolean).join(' · ');
}
