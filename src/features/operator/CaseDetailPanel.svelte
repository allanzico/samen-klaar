<script lang="ts">
  import { onMount } from 'svelte';
  import Pill from '../../ui/Pill.svelte';
  import { workflowApi, type CaseDetail } from '../../lib/workflow-api';
  import type { OperatorCase } from '../../lib/types';
  import { dayTime, money } from '../../lib/format';
  import { showToast } from '../../ui/toast-state.svelte';
  import { t } from '../../lib/i18n.svelte';

  // G05: the facts an operator needs to decide, in one place: the agreement and its revisions, the
  // conversation, the proof, payment state, and an audited way to contact people or record an outcome.
  // Reading a case is logged; nothing here approves work or moves money.
  let { item }: { item: OperatorCase } = $props();
  let detail = $state<CaseDetail | null>(null);
  let error = $state('');
  let note = $state('');
  let recipient = $state('customer');
  let workItem = $state('');
  let busy = $state(false);

  const job = $derived(detail?.state.job);
  const noteReady = $derived(note.trim().length >= 10);
  const assigned = $derived(!!detail?.assignedTo);
  const closed = $derived(detail?.status === 'closed');
  const evidenceURL = (id: string) => `/api/operator/cases/${encodeURIComponent(item.workspaceId)}/evidence/${encodeURIComponent(id)}?kind=${encodeURIComponent(item.kind)}`;
  const statusLabel = (status: string) => t(status === 'closed' ? 'Closed' : status === 'waiting' ? 'Waiting on someone' : 'Open');
  const paymentLabel = (status: string) => t(({ not_started: 'Not started', authorized: 'Held by PayPal', captured: 'Paid', refunded: 'Refunded', voided: 'Released', expired: 'Hold expired' } as Record<string, string>)[status] ?? status.replace(/_/g, ' '));
  const actionLabel = (action: string) => t(({ claim: 'Case claimed', contact: 'Participant contacted', wait: 'Waiting state recorded', close: 'Case closed', 'resolve-evidence': 'Documentation decision recorded' } as Record<string, string>)[action] ?? action);

  async function load() {
    busy = true;
    error = '';
    try { detail = await workflowApi.caseDetail(item.workspaceId, item.kind); }
    catch (e) { error = e instanceof Error ? e.message.trim() : 'Could not load case'; }
    finally { busy = false; }
  }
  async function act(action: 'claim' | 'contact' | 'wait' | 'close' | 'resolve-evidence', toast = '') {
    busy = true;
    error = '';
    try {
      detail = await workflowApi.caseAction(item.workspaceId, item.kind, action, action === 'claim' ? '' : note, action === 'contact' ? recipient : '', action === 'resolve-evidence' ? workItem : '');
      if (action !== 'claim') note = '';
      if (toast) showToast(toast);
    } catch (e) { error = e instanceof Error ? e.message.trim() : 'Could not update case'; }
    finally { busy = false; }
  }
  onMount(() => { void load(); });
</script>

<section class="case-facts" aria-label={t('Case file')}>
  <div class="head">
    <h3 class="small-title">{t('Case file')}</h3>
    {#if detail}<Pill variant={closed ? 'neutral' : 'pending'} label={statusLabel(detail.status)} small/>{/if}
  </div>
  {#if error}<p class="field-error" role="alert">{t(error)} <button class="btn btn-quiet" type="button" onclick={load}>{t('Try again')}</button></p>{/if}

  {#if detail && job}
    <dl class="facts">
      <div><dt>{t('Agreed total')}</dt><dd>{money(job.quote.totalMinor, job.quote.currency || 'EUR')}</dd></div>
      <div><dt>{t('Plan version {version}', { version: job.scopeVersion })}</dt><dd>{job.description}</dd></div>
      <div><dt>{t('Payment')}</dt><dd>{paymentLabel(detail.paymentStatus)}</dd></div>
      <div><dt>{t('Case owner')}</dt><dd>{assigned ? t('Assigned to an operator') : t('Unassigned')}</dd></div>
      {#if detail.outcome}<div><dt>{t('Outcome')}</dt><dd>{detail.outcome}</dd></div>{/if}
    </dl>
    {#if !assigned && !closed}<button type="button" class="btn btn-outline btn-sm claim" disabled={busy} onclick={() => act('claim', t('Case assigned to you'))}>{t('Assign this case to me')}</button>{/if}

    <details class="disclosure">
      <summary>{t('Agreement and revisions')}</summary>
      <ul class="rows">
        {#each job.scopeHistory as scope (scope.version)}
          <li><b>{t('Scope {version}', { version: scope.version })} · {money(scope.totalMinor, job.quote.currency)}</b>
            <ul>{#each scope.lines as line}<li>{line.label}</li>{/each}</ul></li>
        {/each}
      </ul>
    </details>
    <details class="disclosure">
      <summary>{t('Conversation and activity')}</summary>
      {#each detail.state.intake?.messages ?? [] as message}<p><b>{message.senderName || t(message.role === 'professional' ? 'Professional' : message.role === 'customer' ? 'Customer' : 'Assistant')}</b> {message.text}</p>{/each}
      <ol class="rows">{#each detail.decisions as event, i (i)}<li><b>{actionLabel(event.action)}</b> · {dayTime(event.at)}</li>{/each}</ol>
    </details>
    <details class="disclosure">
      <summary>{t('Completion documentation')}</summary>
      <ul class="rows">
        {#each job.evidence as file, i (i)}
          <li>{#if file.assetId}<a href={evidenceURL(file.assetId)} target="_blank" rel="noreferrer">{file.label}</a>{:else}<p>{file.description}</p>{/if}
            <span class="meta">{t('Scope {version}', { version: file.scopeVersion ?? 1 })}{file.completionRound && file.completionRound > 1 ? ` · ${t('Completion attempt {round}', { round: file.completionRound })}` : ''}</span></li>
        {:else}<li class="meta">{t('Nothing was uploaded.')}</li>{/each}
      </ul>
    </details>

    {#if !closed}
      <form class="case-action" onsubmit={(e) => { e.preventDefault(); void act('contact', t('Message sent')); }}>
        <div class="field"><label for="case-note">{t('Case note or message')}</label><textarea id="case-note" class="textarea" bind:value={note} rows="3" minlength="10" maxlength="1000" disabled={busy}></textarea>
          {#if !noteReady}<p class="field-help">{t('Write at least 10 characters to continue')}</p>{/if}</div>
        <div class="field"><label for="case-recipient">{t('Contact participant')}</label>
          <select id="case-recipient" class="select" bind:value={recipient}><option value="customer">{t('Customer')}</option><option value="professional">{t('Professional')}</option></select></div>
        <div class="row">
          <button class="btn btn-outline" disabled={busy || !noteReady}>{t('Send case message')}</button>
          <button type="button" class="btn btn-quiet" disabled={busy || !noteReady} onclick={() => act('wait', t('Waiting state recorded'))}>{t('Record waiting state')}</button>
          <button type="button" class="btn btn-quiet" disabled={busy || !noteReady} onclick={() => act('close', t('Case closed'))}>{t('Close case with this outcome')}</button>
        </div>
        {#if job.workItems?.length}
          <div class="field"><label for="case-item">{t('Documentation requirement')}</label>
            <select id="case-item" class="select" bind:value={workItem}><option value="">{t('Choose a requirement')}</option>{#each job.workItems as requirement (requirement.id)}<option value={requirement.id}>{requirement.description}</option>{/each}</select></div>
          <button type="button" class="btn btn-outline btn-sm decide" disabled={busy || !noteReady || !workItem} onclick={() => act('resolve-evidence', t('Documentation decision recorded'))}>{t('Record human documentation decision')}</button>
          <p class="helper">{t('Record the facts and approved policy. This does not certify workmanship or approve payment.')}</p>
        {/if}
      </form>
    {/if}

    {#if detail.notes.length}
      <h4 class="small-title">{t('Support and decision notes')}</h4>
      <ul class="rows">
        {#each detail.notes as entry (entry.id)}
          <li><p>{entry.note}</p><p class="meta">{dayTime(entry.createdAt)} · {entry.recipientRole ? t(entry.recipientRole === 'professional' ? 'Professional' : 'Customer') : t('Operator note')}</p></li>
        {/each}
      </ul>
    {/if}
  {:else if busy}<p role="status">{t('Loading case file…')}</p>{/if}
</section>

<style>
  .case-facts { display: grid; gap: 14px; padding-block: 16px; border-top: 0.5px solid var(--sk-line); overflow-wrap: anywhere; }
  .head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .facts { display: grid; }
  .facts > div { display: grid; grid-template-columns: minmax(110px, 1fr) 2fr; gap: 12px; padding: 8px 0; }
  .facts > div + div { border-top: 0.5px solid var(--sk-line); }
  .facts dt { color: var(--sk-muted); }
  .case-action { display: grid; gap: 12px; }
  .row { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
  .claim, .decide { justify-self: start; }
  .rows ul { margin: 4px 0 0 16px; list-style: disc; font-size: 13px; color: var(--sk-muted); }
  .rows li { padding: 6px 0; }
  .disclosure :global(p) { margin: 4px 0; }
</style>
