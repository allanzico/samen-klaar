<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { AppState, AppActions, WorkspaceState } from '../../lib/types';
  import { requestStatus } from '../../lib/request-status';
  import { statusUi, canCancelRequest } from '../../lib/status-ui';
  import { dayAndTime, planUi } from '../../lib/plan-ui.svelte';
  import { t } from '../../lib/i18n.svelte';
  import { coverTitle, fitsTitle, tradeName, tradeOf } from '../../lib/trade';
  let { app, actions, workspace, money, top }: { app: AppState; actions: AppActions; workspace: WorkspaceState; money: (minor: number) => string; top?: Snippet } = $props();
  let job = $derived(workspace.job);
  let status = $derived(requestStatus(workspace, app.role));
  let other = $derived(app.role === 'customer' ? job.professional : job.customer);
  let ui = $derived(statusUi(status.code, status.label, app.role, other));
  let intake = $derived(job.id ? undefined : workspace.intake);
  let intakeTrade = $derived(tradeOf(intake?.category));
  let intakeArea = $derived(intake?.location?.label ?? '');
  let note = $state('');
  let issueNote = $state('');
  let canCancel = $derived(canCancelRequest(workspace));
  async function cancel(event: SubmitEvent) { event.preventDefault(); if (await actions.act('cancel-request', note)) { planUi.cancelling = false; note = ''; } }
  async function reportIssue(event: SubmitEvent) { event.preventDefault(); if (await actions.act('report-issue', issueNote)) { planUi.reportingIssue = false; issueNote = ''; } }

  type Row = { label: string; value: string; state: 'done' | 'pending' | 'todo' | 'lock'; note?: string };
  let agreed = $derived(job.quoteApproved || job.priceProposal?.status === 'accepted');
  let visit = $derived.by(() => {
    const confirming = job.appointmentConfirmation?.status === 'pending' && job.appointment;
    if (confirming) return { text: dayAndTime(job.appointment!.startUtc, job.appointment!.endUtc, job.appointment!.timezone), state: 'pending' as const, note: t('Awaiting professional confirmation') };
    if (job.dateProposal?.status === 'pending') return { text: dayAndTime(job.dateProposal.appointment.startUtc, job.dateProposal.appointment.endUtc, job.dateProposal.appointment.timezone), state: 'pending' as const, note: t('Not reserved') };
    if (job.appointment) return { text: dayAndTime(job.appointment.startUtc, job.appointment.endUtc, job.appointment.timezone), state: 'done' as const, note: '' };
    return { text: t('Not chosen yet'), state: 'todo' as const, note: '' };
  });
  let rows = $derived.by((): Row[] => {
    const list: Row[] = [
      { label: t('Service'), value: job.serviceDescription || job.title, state: agreed ? 'done' : 'pending' },
      { label: t('Price'), value: money(job.quote.totalMinor), state: agreed ? 'done' : 'pending', note: agreed ? undefined : t('Published') },
      { label: t('Visit'), value: visit.text, state: visit.state, note: visit.note || undefined },
      { label: t('Address'), value: app.role === 'professional' ? job.serviceAddress || t('Not added yet') : job.serviceAddress ? t('Only {name} will see it', { name: other || t('Professional') }) : job.location ? t('Not added yet') : t('Shared after you choose'), state: job.serviceAddress ? 'lock' : 'todo' }
    ];
    if (job.change?.status === 'pending') list.splice(2, 0, { label: t('Extra work'), value: `+ ${money(job.change.amountMinor)}`, state: 'pending', note: job.change.description });
    return list;
  });
  let barSummary = $derived(`${job.appointment || job.dateProposal?.status === 'pending' ? visit.text : t(status.label)} · ${money(job.quote.totalMinor)}`);
</script>
{#snippet ledger()}
  <ul class="agree-rows">
    {#each rows as row (row.label)}
      <li class="agree-row {row.state}">
        <span class="agree-mark" aria-hidden="true">
          {#if row.state === 'done'}<svg class="icon" viewBox="0 0 24 24"><path d="m5 12 5 5 9-10"/></svg>
          {:else if row.state === 'pending'}<svg class="icon" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>
          {:else if row.state === 'lock'}<svg class="icon" viewBox="0 0 24 24"><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></svg>{/if}
        </span>
        <span class="agree-label">{row.label}</span>
        <span class="agree-value">{row.value}{#if row.note}<small>{row.note}</small>{/if}</span>
      </li>
    {/each}
    <li class="agree-row total-row"><span class="agree-mark" aria-hidden="true"></span><span class="agree-label">{t('Total')}</span><span class="agree-value">{money(job.quote.totalMinor + (job.change?.status === 'pending' ? job.change.amountMinor : 0))}<small>{job.quote.durationMinutes} {t('minutes')}</small></span></li>
  </ul>
{/snippet}
<header class="agreement-summary">
  <div class="agreement-head">
  <div class="agreement-title-row">
    <h1>{#if job.id}{job.title}{:else if workspace.cancelled}{t('Request cancelled')}{:else if status.code === 'matching' && intake?.matches.length}<span class="title-compact">{fitsTitle(intakeTrade)}</span><span class="title-wide">{coverTitle(intakeTrade, Math.min(intake.matches.length, 3), intakeArea)}</span>{:else if intake?.step === 'waitlist'}{t('No {trade} covers {area} yet', { trade: tradeName(intakeTrade), area: intakeArea })}{:else if status.code === 'matching'}{t('Find the right help')}{:else}{t('Your request')}{/if}</h1>
    <details class="request-menu">
      <summary>{t('Request options')}</summary>
      <div>
        {#if canCancel}<button disabled={app.pending} onclick={() => { planUi.cancelling = !planUi.cancelling; }}>{t('Cancel')} {t(job.appointment ? 'visit and request' : 'request')}</button>
        {:else if !workspace.cancelled && job.id}<p>{t('Work or payment has begun. Message')} {other} {t('to discuss stopping or changing it.')}</p><button onclick={actions.focusComposer}>{t('Message about this request')}</button>{/if}
        <button disabled={app.pending} onclick={actions.refresh}>{t('Check for updates')}</button>
      </div>
    </details>
  </div>
  <p class="agreement-status" role="status"><span class="pill pill--{ui.pill}">{ui.pillLabel}</span> <span class="agreement-status-text">{t(status.label)}</span></p>
  </div>
  <div class="agreement-body">
  {#if top}<div class="next-step-wide">{@render top()}</div>{/if}
  {#if job.id}
    <p class="eyebrow agreement-eyebrow">{t('Agreement')}</p>
    {#if workspace.cancelled}<p class="agreement-visit">{t('Any reserved visit has been released. Your request history is saved.')}</p>
      {#if workspace.cancelledBy === 'professional' && app.role === 'customer'}<p class="quiet-note">{t('The professional could not take this request. No payment was taken. Your description is saved so you can find another professional.')}</p><button class="primary" disabled={app.pending} onclick={() => actions.act('return-to-matches')}>{t('Find another professional')}</button>{/if}
    {:else}
      <div class="agreement-ledger-wide">{@render ledger()}</div>
      <details class="agreement-bar"><summary><span>{barSummary}</span></summary>{@render ledger()}</details>
    {/if}
    {#if workspace.cancellationReview?.status === 'pending'}<p class="quiet-note">{t('Your cancellation request is under review. The visit remains booked until an operator updates the request.')}</p>{/if}
    {#if app.role === 'customer' && job.workStarted && !job.paid && job.dispute?.status !== 'open'}
      <section class="issue-report">
        {#if planUi.reportingIssue}
          <form onsubmit={reportIssue}>
            <label>{t('What is incomplete or damaged?')}<textarea bind:value={issueNote} rows="3" maxlength="1000" required disabled={app.pending}></textarea></label>
            <p class="quiet-note">{t('Reporting an issue pauses payment while an operator reviews it.')}</p>
            <div class="actions"><button class="danger-button" disabled={app.pending || issueNote.trim().length < 10}>{t('Send for review')}</button><button type="button" disabled={app.pending} onclick={() => { planUi.reportingIssue = false; }}>{t('Keep working')}</button></div>
            {#if issueNote.trim().length < 10}<p class="helper">{t('Describe the problem in at least 10 characters to continue')}</p>{/if}
          </form>
        {:else}<button class="secondary" disabled={app.pending} onclick={() => { planUi.reportingIssue = true; }}>{t('Report an issue')}</button>{/if}
      </section>
    {:else if job.dispute?.status === 'open'}<p class="quiet-note">{t(job.paid ? 'A payment dispute is under operator review.' : 'An issue is under operator review. Payment is paused.')}</p>
    {:else if job.dispute?.status === 'refund_pending'}<p class="quiet-note">{t('A full refund was approved and is being processed through PayPal.')}</p>
    {:else if job.dispute?.status === 'refunded'}<p class="quiet-note">{t('The full PayPal payment has been refunded.')}</p>{/if}
  {/if}
  {#if planUi.cancelling}
    <form class="cancel-confirmation" onsubmit={cancel}>
      <p><strong>{t('Cancel this request?')}</strong> {t('More than 24 hours before a visit, cancellation releases it. A request within 24 hours goes to operator review, and the visit stays booked while it is reviewed. No cancellation fee is charged automatically.')}</p>
      <label>{t('Cancellation note (optional)')}<input bind:value={note} maxlength="500" disabled={app.pending}/></label>
      <div class="actions"><button class="danger-button" disabled={app.pending}>{t('Cancel request')}</button><button type="button" disabled={app.pending} onclick={() => { planUi.cancelling = false; }}>{t('Keep request')}</button></div>
    </form>
  {/if}
  </div>
</header>
