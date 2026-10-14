<script lang="ts">
  import AgreementSummary from '../workspaces/AgreementSummary.svelte';
  import ProfessionalAvailability from './ProfessionalAvailability.svelte';
  import PayPalPayoutSetup from './PayPalPayoutSetup.svelte';
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
  $effect(() => { draftSaved = saveDraft(key, [completionNote,changeDescription,changePrice].some(Boolean) ? JSON.stringify({completionNote,changeDescription,changePrice:String(changePrice ?? '')}) : ''); });

  async function addWorkNote(event: SubmitEvent) {
    event.preventDefault();
    if (await actions.act('add-evidence', completionNote)) completionNote = '';
  }

  async function proposeChange(event: SubmitEvent) {
    event.preventDefault();
    const amount = Math.round(Number(changePrice) * 100);
    if (amount > 0 && await actions.act('propose-change', changeDescription, undefined, amount)) {
      changeDescription = '';
      changePrice = '';
    }
  }
</script>

<section class="request-workspace" aria-label={t('Your assigned request')}>
  {#if app.serverState}<AgreementSummary {app} {actions} workspace={app.serverState} {money}/>{/if}
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
      <details class="job-description"><summary>{t('Job details and preferences')}</summary><p>{job.description}</p>{#if intake}<p>{intake.detail}</p><p>{t('Timing:')} {intake.urgency} · {t('Budget:')} {intake.budget}</p>{/if}</details>
      {#if !job.appointment && !job.dateProposal && !job.priceProposal}
        <p>{t('The customer must approve the plan before a visit is booked. Use Price offer to propose a different total.')}</p>
      {:else if job.appointment}
        {#if job.appointmentConfirmation?.status === 'pending'}
          <p>{t('This visit is reserved for you. Confirm it within two business hours or decline so the customer can choose another time.')}</p>
          <div class="actions"><button class="primary" disabled={app.pending} onclick={() => actions.act('confirm-appointment')}>{t('Confirm visit')}</button><button disabled={app.pending} onclick={() => actions.act('decline-appointment')}>{t('Decline and release time')}</button></div>
        {:else if !job.workStarted}
          <button class="primary" disabled={app.pending || job.dateProposal?.status === 'pending' || job.change?.status === 'pending'} onclick={() => actions.act('start-job')}>{t('Start the job')}</button>
        {:else if !job.evidenceSubmitted}
          {#if app.providerConfig?.evidenceProvider === 'r2'}
            <div class="stack">
              <label for="evidence-file">{t('Add photos or a document')}</label>
              <input id="evidence-file" type="file" accept="image/jpeg,image/png,image/webp,application/pdf" onchange={actions.selectEvidenceFile}/>
              <button disabled={app.pending || !app.evidenceFile} onclick={actions.uploadEvidence}>{t('Upload evidence')}</button>
              <small>JPEG, PNG, WebP or PDF · {t('up to 10 MB')}</small>
            </div>
          {/if}
          {#if !job.detailAdded}
            <form class="stack" onsubmit={addWorkNote}>
              <label>{t('Completion work note')}<textarea bind:value={completionNote} required minlength="12" maxlength="1200" rows="3" placeholder={t('Describe the work completed and any checks performed.')}></textarea></label>
              <button disabled={app.pending}>{t('Save completion note')}</button>
            </form>
          {:else}<p>{t('Completion note saved.')}</p>{/if}
          <button class="primary" disabled={app.pending || !job.evidence.some((item) => item.assetId) || !job.detailAdded || job.change?.status === 'pending'} onclick={() => actions.act('submit-evidence')}>{t('Submit completion')}</button>
        {:else if job.detailsRequested && !job.detailAdded}
          <p>{t('The customer requested the missing work note.')}</p>
          <form class="stack" onsubmit={addWorkNote}>
            <label>{t('Describe the work completed')}<textarea bind:value={completionNote} required minlength="12" maxlength="1200" rows="3" placeholder={t('Record what you did and any checks you performed. Avoid claiming that a photo proves safety.')}></textarea></label>
            <button class="primary" disabled={app.pending || completionNote.trim().length < 12}>{t('Add work note')}</button>
          </form>
        {:else}
          <p>{t(job.paid ? 'Payment complete.' : 'Waiting for the customer’s next step.')}</p>
        {/if}
        {#if job.quoteApproved && !job.paid}
          <details>
            <summary>{t('Need to change the scope?')}</summary>
            {#if job.change?.status === 'pending'}
              <p>{t('Your proposal is waiting for customer approval.')}</p>
            {:else}
              <form class="stack" onsubmit={proposeChange}>
                <label>{t('Additional work')}<textarea bind:value={changeDescription} required minlength="5" maxlength="300" rows="2" placeholder={t('Describe the additional work. The customer must approve it before it is added.')}></textarea></label>
                <label>{t('Price in')} {job.quote.currency}<input type="number" bind:value={changePrice} required min="0.01" step="0.01"/></label>
                <button disabled={app.pending}>{t('Send price change for approval')}</button>
              </form>
            {/if}
          </details>
        {/if}
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
    <div class="card empty-state"><span>✓</span><h2>Nothing to act on yet.</h2><p>A job will appear here once the customer chooses a professional.</p></div>
  {/if}
  <details class="service-settings"><summary>{t('Manage service and availability')}</summary><ProfessionalAvailability/></details>
  <PayPalPayoutSetup/>
  {#if app.pending}<p class="thinking" role="status">{t('Saving your update…')}</p>{/if}
  <div id="conversation-end"></div>
  </div>
  {#if activeJob}<div class="composer-dock"><MessageComposer {app} {actions} assigned={true}/></div>{/if}
</section>
