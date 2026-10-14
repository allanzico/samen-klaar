<script lang="ts">
  import AgreementSummary from '../workspaces/AgreementSummary.svelte';
  import ExtraWorkSheet from './ExtraWorkSheet.svelte';
  import ProNextStep from '../workspaces/ProNextStep.svelte';
  import { shortName } from '../../lib/trade';
  import { draftKey, readFormDraft, saveDraft } from '../../lib/drafts';
  import MessageComposer from '../workspaces/MessageComposer.svelte';
  import IntakeConversation from '../customer/IntakeConversation.svelte';
  import PlanNegotiation from '../workspaces/PlanNegotiation.svelte';
  import JobReviewPanel from '../workspaces/JobReviewPanel.svelte';
  import type { AppActions, AppState, Intake, Job } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  let { app, actions, job, intake, activeJob, statusLabel, money }: { app: AppState; actions: AppActions; job: Job | undefined; intake: Intake | undefined; activeJob: boolean; statusLabel: string; money: (minor: number) => string } = $props();

  const key = (() => draftKey(app.session?.account?.id, app.role, app.workspaceId, 'work-notes'))();
  const saved = readFormDraft(key);
  let completionNote = $state(saved.completionNote || '');
  let changeDescription = $state(saved.changeDescription || '');
  let changePrice = $state(saved.changePrice || '');
  let draftSaved = $state(true);
  let extraOpen = $state(false);
  const customer = $derived(shortName(job?.customer || t('Customer')));
  const noteReady = $derived(completionNote.trim().length >= 12);
  $effect(() => { draftSaved = saveDraft(key, [completionNote,changeDescription,changePrice].some(Boolean) ? JSON.stringify({completionNote,changeDescription,changePrice:String(changePrice ?? '')}) : ''); });

  async function saveNote() {
    if (await actions.act('add-evidence', completionNote)) completionNote = '';
  }
  async function addWorkNote(event: SubmitEvent) {
    event.preventDefault();
    await saveNote();
  }

  async function proposeChange(event: SubmitEvent) {
    event.preventDefault();
    const amount = Math.round(Number(changePrice) * 100);
    if (amount > 0 && await actions.act('propose-change', changeDescription, undefined, amount)) {
      changeDescription = '';
      changePrice = '';
      extraOpen = false;
    }
  }
</script>

{#snippet wideNextStep()}{#if app.serverState}<ProNextStep {app} {actions} workspace={app.serverState} {money} placement="panel" {noteReady} {saveNote} openExtra={() => { extraOpen = true; }}/>{/if}{/snippet}

<section class="request-workspace" aria-label={t('Your assigned request')}>
  {#if app.serverState}<AgreementSummary {app} {actions} workspace={app.serverState} {money} top={activeJob ? wideNextStep : undefined}/>{/if}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (This scroll region must be reachable for keyboard scrolling.) -->
  <div id="request-thread" class="thread-scroll" role="region" tabindex="0" aria-label={t('Request conversation and decisions')}>
  {#if activeJob && job}
    {#if !app.serverState?.cancelled}
    <section id="current-plan" class="task-panel" aria-label={t('Current decision')}>
      <PlanNegotiation {app} {actions} {job} {money}/>
      {#if job.location}
        <section class="service-address-panel" aria-label={t('Customer visit area and address')}>
          <h3>{t('Visit location')}</h3>
          <p>{t('Area:')} {job.location.label}</p>
          {#if job.serviceAddress}<p>{t('Full service address:')} {job.serviceAddress}</p>{:else}<p>{t('The customer must add the full address before approving the plan.')}</p>{/if}
        </section>
      {/if}
      <details class="job-description"><summary>{t('Read the intake conversation')}</summary><p>{job.description}</p>{#if intake}<p>{intake.detail}</p><p>{t('Timing:')} {intake.urgency} · {t('Budget:')} {intake.budget}</p>{/if}</details>
      {#if job.appointment && job.workStarted && !job.evidenceSubmitted}
        <section class="proof-section" aria-label={t('Finish and send proof')}>
          <h3>{t('Finish and send proof')}</h3>
          {#if app.providerConfig?.evidenceProvider === 'r2'}
            <div class="stack upload-tile">
              <label for="evidence-file">{t('Add photos or a document')}</label>
              <input id="evidence-file" type="file" accept="image/jpeg,image/png,image/webp,application/pdf" onchange={actions.selectEvidenceFile}/>
              <button class="secondary" disabled={app.pending || !app.evidenceFile} onclick={actions.uploadEvidence}>{t('Upload evidence')}</button>
              {#if !app.evidenceFile}<p class="helper">{t('Choose a file to upload')}</p>{/if}
              <small>JPEG, PNG, WebP or PDF · {t('up to 10 MB')}</small>
            </div>
          {/if}
          {#if job.evidence.length}<div class="evidence-grid">{#each job.evidence as evidence}<div class="evidence-item"><b>{evidence.label}</b><p>{evidence.description}</p></div>{/each}</div>{/if}
          {#if !job.detailAdded}
            <form class="stack" onsubmit={addWorkNote}>
              <label>{t('Completion work note')}<textarea bind:value={completionNote} required minlength="12" maxlength="1200" rows="3" placeholder={t('Describe the work completed and any checks performed.')}></textarea></label>
              <button class="secondary" disabled={app.pending || !noteReady}>{t('Save completion note')}</button>
              {#if !noteReady}<p class="helper">{t('Write at least 12 characters to continue')}</p>{/if}
            </form>
          {:else}<p class="success-line"><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10"/></svg>{t('Completion note saved.')}</p>{/if}
        </section>
      {:else if job.appointment && job.evidenceSubmitted && job.detailsRequested && !job.detailAdded}
        <section class="proof-section">
          <p>{t('The customer requested the missing work note.')}</p>
          <form class="stack" onsubmit={addWorkNote}>
            <label>{t('Describe the work completed')}<textarea bind:value={completionNote} required minlength="12" maxlength="1200" rows="3" placeholder={t('Record what you did and any checks you performed. Avoid claiming that a photo proves safety.')}></textarea></label>
          </form>
        </section>
      {/if}
      {#if [completionNote,changeDescription,changePrice].some(Boolean)}<p class="quiet-note" role="status">{t(draftSaved ? 'Unsent work notes saved in this browser tab.' : 'Draft storage is unavailable. Keep this tab open.')}</p>{/if}
      <details>
        <summary>{t('Agreed plan and activity')}</summary>
        {#if job.serviceDescription}<p>{job.serviceDescription}</p>{/if}
        {#each job.quote.lines as line}<p>{line.label} · {money(line.amountMinor)}</p>{/each}
        <p>{t('Plan version')} {job.scopeVersion} · {money(job.quote.totalMinor)} · {job.quote.materialsPolicy}</p>
        {#if job.quote.exclusions?.length}<p>{t('Excluded:')} {job.quote.exclusions.join(' · ')}</p>{/if}
        {#each [...job.timeline].reverse() as event}<div class="activity"><span>{event.when}</span>{event.text}</div>{/each}
      </details>
    </section>
    {:else}<section class="cancelled-request"><h2>{t('This request is closed.')}</h2><p>{t('Any reserved visit has been released. Your conversation and history remain available.')}</p></section>{/if}
    {#if job}<JobReviewPanel {app} {actions} {job}/>{/if}
    <IntakeConversation {app} {actions} {job} {intake} {activeJob} composing={false}/>
  {:else}
    <div class="card empty-state"><span>✓</span><h2>{t('Nothing to act on yet.')}</h2><p>{t('A job will appear here once the customer chooses a professional.')}</p></div>
  {/if}
  {#if app.pending}<p class="thinking" role="status">{t('Saving your update…')}</p>{/if}
  <div id="conversation-end"></div>
  </div>
  {#if activeJob}<div class="composer-dock">{#if app.serverState}<ProNextStep {app} {actions} workspace={app.serverState} {money} placement="dock" {noteReady} {saveNote} openExtra={() => { extraOpen = true; }}/>{/if}<MessageComposer {app} {actions} assigned={true}/></div>{/if}
</section>
{#if job}<ExtraWorkSheet bind:open={extraOpen} {customer} currentTotalMinor={job.quote.totalMinor} {money} pending={app.pending} bind:description={changeDescription} bind:price={changePrice} onsubmit={proposeChange}/>{/if}
