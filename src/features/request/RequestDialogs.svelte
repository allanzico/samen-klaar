<script lang="ts">
import EvidenceRecord from './EvidenceRecord.svelte';
  import Modal from '../../ui/Modal.svelte';
  import DeleteRequestDialog from './DeleteRequestDialog.svelte';
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import { requestUi } from '../../lib/request-ui.svelte';
  import { firstName } from '../../lib/status-ui';
  import { money } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';

  // C15 / F10 cancel, D07 report a problem, F12 extra work, and the job history.
  let { app, actions, workspace, work, historyOpen = $bindable(false) }: {
    app: AppState; actions: AppActions; workspace: WorkspaceState; work: { changeDescription: string; changePrice: string }; historyOpen?: boolean;
  } = $props();
  const job = $derived(workspace.job);
  const pro = $derived(app.role === 'professional');
  const customer = $derived(firstName(job.customer) || t('the customer'));
  let note = $state('');
  let issue = $state('');
  const extraMinor = $derived(Math.max(0, Math.round(Number(work.changePrice) * 100) || 0));
  const extraReason = $derived(work.changeDescription.trim().length < 5 || extraMinor <= 0 ? t('Describe the extra work and add a cost to continue') : '');

  async function cancel() { if (await actions.act('cancel-request', note)) { requestUi.cancelling = false; note = ''; } }
  async function report() { if (await actions.act('report-issue', issue)) { requestUi.reporting = false; issue = ''; } }
  async function proposeExtra() {
    if (extraReason) return;
    if (await actions.act('propose-change', work.changeDescription, undefined, extraMinor)) { work.changeDescription = ''; work.changePrice = ''; requestUi.extraWork = false; }
  }
</script>

<Modal bind:open={requestUi.cancelling} title={pro ? t('Cancel this job?') : t('Cancel this request?')}>
  <p>{pro ? t('{name} can look for another professional. Any reserved time is released.', { name: customer }) : t('More than 24 hours before a visit, cancelling releases it. Within 24 hours, our team reviews it and the visit stays booked meanwhile. No fee is charged automatically.')}</p>
  <div class="field"><label for="cancel-note">{t('Note (optional)')}</label><textarea id="cancel-note" class="textarea" rows="2" maxlength="500" bind:value={note} disabled={app.pending}></textarea></div>
  {#snippet footer()}
    <button type="button" class="btn btn-danger" disabled={app.pending} onclick={cancel}>{pro ? t('Cancel job') : t('Cancel request')}</button>
    <button type="button" class="btn btn-outline" onclick={() => { requestUi.cancelling = false; }}>{pro ? t('Keep job') : t('Keep request')}</button>
  {/snippet}
</Modal>

{#if app.workspaceId}
  <DeleteRequestDialog {app} {actions} bind:open={requestUi.deleting} workspaceId={app.workspaceId}
    title={job.id ? job.title || job.description : workspace.intake?.description || 'New request'}
    counterpart={pro ? firstName(job.customer) : job.professional} cancelled={workspace.cancelled}/>
{/if}

<Modal bind:open={requestUi.reporting} title={t('What’s not right?')}>
  <p class="secondary-text">{t('Payment pauses while our team reviews it. People decide on rework or a refund.')}</p>
  <div class="field">
    <label for="issue-note">{t('Describe the problem')}</label>
    <textarea id="issue-note" class="textarea" rows="4" maxlength="1000" bind:value={issue} disabled={app.pending} aria-describedby="issue-help"></textarea>
    {#if issue.trim().length < 10}<p id="issue-help" class="field-help">{t('Write at least 10 characters to continue')}</p>{/if}
  </div>
  {#snippet footer()}
    <button type="button" class="btn btn-danger" disabled={app.pending || issue.trim().length < 10} onclick={report}>{t('Report problem')}</button>
    <button type="button" class="btn btn-outline" onclick={() => { requestUi.reporting = false; }}>{t('Cancel')}</button>
  {/snippet}
</Modal>

{#if pro}
  <Modal bind:open={requestUi.extraWork} title={t('Propose extra work')}>
    <div class="field"><label for="extra-description">{t('What else needs doing')}</label><textarea id="extra-description" class="textarea" rows="3" minlength="5" maxlength="300" bind:value={work.changeDescription} disabled={app.pending}></textarea></div>
    <div class="field"><label for="extra-price">{t('Extra cost')}</label><div class="money-input"><span aria-hidden="true">€</span><input id="extra-price" type="number" inputmode="decimal" min="0.01" step="0.01" bind:value={work.changePrice} disabled={app.pending}/></div></div>
    <dl class="ledger"><div class="ledger-total"><dt>{t('New total')}</dt><dd>{money(job.quote.totalMinor + extraMinor, job.quote.currency)}</dd></div></dl>
    <p class="helper">{t('Wait for {name}’s yes before doing this work. If they decline, finish the original job.', { name: customer })}</p>
    {#snippet footer()}
      <div class="stacked">
        <button type="button" class="btn btn-primary" disabled={app.pending || !!extraReason} onclick={proposeExtra}>{t('Send to {name}', { name: customer })}</button>
        {#if extraReason}<p class="reason">{extraReason}</p>{/if}
      </div>
    {/snippet}
  </Modal>
{/if}

<Modal bind:open={historyOpen} title={t('Job history and photos')}>
  {#if job.evidence.length}<EvidenceRecord {job} workspaceId={app.workspaceId} title={t('Completion record')} viewer={app.role==='professional'?'professional':'customer'}/>{/if}
  {#if job.timeline.length}
    <ol class="rows history">{#each [...job.timeline].reverse() as event, i (i)}<li><span class="meta">{event.when}</span><span>{event.text}</span></li>{/each}</ol>
  {:else}<p class="secondary-text">{t('Nothing recorded yet.')}</p>{/if}
  <p class="helper">{t('Plan version {version}', { version: job.scopeVersion })}</p>
</Modal>

<style>
  .history li { display: grid; grid-template-columns: 52px 1fr; gap: 10px; padding: 10px 0; font-size: 13px; }
  .stacked { display: grid; gap: 4px; }
</style>
