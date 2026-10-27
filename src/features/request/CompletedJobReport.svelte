<script lang="ts">
  import type { Job } from '../../lib/types';
  import { currentLocale, t } from '../../lib/i18n.svelte';
  import { money, visitRange } from '../../lib/format';
  import { isCurrentEvidence } from '../../lib/evidence';
  import AgreedLedger from './AgreedLedger.svelte';
  import EvidenceRecord from './EvidenceRecord.svelte';
  import ProfessionalFeeBreakdown from '../professional/ProfessionalFeeBreakdown.svelte';

  let { job, workspaceId, viewer, status = 'paid' }: {
    job: Job; workspaceId: string | null; viewer: 'customer' | 'professional'; status?: 'paid' | 'refund_pending' | 'refunded';
  } = $props();
  const headingId = $props.id();
  const refunded = $derived(status === 'refunded' || job.paymentRefunded);
  const current = $derived(job.evidence.some(item => isCurrentEvidence(job, item)));
  const earlier = $derived(job.evidence.some(item => !isCurrentEvidence(job, item)));
  const visit = $derived(job.appointment && Number.isFinite(Date.parse(job.appointment.startUtc)) && Number.isFinite(Date.parse(job.appointment.endUtc))
    ? visitRange(job.appointment.startUtc, job.appointment.endUtc, job.appointment.timezone || 'Europe/Amsterdam') : job.appointment?.startsAt);
  const agreedWork = $derived(job.serviceDescription || job.description || job.title);
  function recordedAt(value?: string): string {
    if (!value || !Number.isFinite(Date.parse(value))) return '';
    return new Intl.DateTimeFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', {
      year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', timeZone: job.appointment?.timezone || 'Europe/Amsterdam'
    }).format(new Date(value));
  }
</script>

<div class="completion-report">
  <section class="box report" aria-labelledby={headingId}>
    <h2 id={headingId} class="section-title">{t('Job completion report')}</h2>
    <p class="job-title">{job.title}</p>
    <dl class="facts">
      <div><dt>{t('Customer')}</dt><dd>{job.customer}</dd></div>
      <div><dt>{t('Professional')}</dt><dd>{job.professional}</dd></div>
      {#if visit}<div><dt>{t('Visit')}</dt><dd>{visit}</dd></div>{/if}
      <div><dt>{t('Payment status')}</dt><dd>{refunded ? t('Refunded') : status === 'refund_pending' ? t('Refund pending') : job.paid ? t('Paid') : t('Payment not captured')}</dd></div>
      <div><dt>{t('Agreed total')}</dt><dd class="amount">{money(job.quote.totalMinor, job.quote.currency)}</dd></div>
      {#if job.completionApproved}
        <div><dt>{t('Completion approved by')}</dt><dd>{job.completionApprovedByAgent ? t('Customer’s assistant') : t('Customer')}{#if recordedAt(job.completionApprovedAt)}<span class="recorded-at">{recordedAt(job.completionApprovedAt)}</span>{/if}</dd></div>
      {/if}
      {#if job.paymentApprovedVersion === job.scopeVersion}
        <div><dt>{t('Payment release approved by')}</dt><dd>{job.paymentApprovedByAgent ? t('Customer’s assistant') : t('Customer')}{#if recordedAt(job.paymentApprovedAt)}<span class="recorded-at">{recordedAt(job.paymentApprovedAt)}</span>{/if}</dd></div>
      {/if}
      {#if job.paymentTransactionId}<div><dt>{t('PayPal payment reference')}</dt><dd class="reference">{job.paymentTransactionId}</dd></div>{/if}
      {#if job.refundTransactionId}<div><dt>{t('PayPal refund reference')}</dt><dd class="reference">{job.refundTransactionId}</dd></div>{/if}
    </dl>
    <div class="scope">
      <h3 class="small-title">{t('Agreed work')}</h3>
      <p>{agreedWork}</p>
      {#if job.quote.materialsPolicy}<p><b>{t('Materials')}</b> · {job.quote.materialsPolicy}</p>{/if}
      {#if job.quote.exclusions.length}<p><b>{t('Not included')}</b> · {job.quote.exclusions.join(' · ')}</p>{/if}
    </div>
  </section>
  <AgreedLedger {job}/>
  {#if viewer === 'professional' && !refunded && status === 'paid'}
    <ProfessionalFeeBreakdown totalMinor={job.quote.totalMinor} currency={job.quote.currency} basisPoints={job.platformFeeBasisPoints}/>
  {:else if viewer === 'professional' && job.platformFeeBasisPoints}
    <p class="box documentation secondary-text">{t('Full refunds include the platform fee.')}</p>
  {/if}
  {#if current}
    <div class="box documentation"><EvidenceRecord {job} {workspaceId} {viewer} scope="current" title={t('Completion record')}/></div>
  {:else}
    <p class="box documentation secondary-text">{t('No completion documentation is recorded for this agreement.')}</p>
  {/if}
  {#if earlier}
    <details class="box documentation"><summary>{t('Earlier completion documentation')}</summary><EvidenceRecord {job} {workspaceId} {viewer} scope="earlier" title={t('Earlier completion documentation')}/></details>
  {/if}
</div>

<style>
  .completion-report { display: grid; gap: 16px; }
  .report { display: grid; gap: 18px; padding: 20px 22px; }
  .job-title { font-weight: 600; }
  .facts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px 24px; }
  .facts div { min-width: 0; }
  dt { color: var(--sk-muted); font-size: 13px; }
  dd { margin: 4px 0 0; overflow-wrap: anywhere; }
  .recorded-at { display: block; color: var(--sk-muted); font-size: 13px; margin-top: 4px; }
  .scope { display: grid; gap: 10px; border-top: 1px solid var(--sk-line); padding-top: 16px; }
  .documentation { padding: 20px 22px; }
  summary { cursor: pointer; font-weight: 600; min-height: 44px; align-content: center; }
  details[open] summary { margin-bottom: 16px; }
  @media (max-width: 639px) { .facts { grid-template-columns: 1fr; } .report, .documentation { padding: 18px 16px; } }
</style>
