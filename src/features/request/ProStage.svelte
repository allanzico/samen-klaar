<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Banner from '../../ui/Banner.svelte';
  import EvidenceRecord from './EvidenceRecord.svelte';
  import AgreedLedger from './AgreedLedger.svelte';
  import JobReview from './JobReview.svelte';
  import type { AppActions, AppState, Intake, WorkspaceState } from '../../lib/types';
  import type { RequestCode } from '../../lib/request-status';
  import { firstName } from '../../lib/status-ui';
  import { t } from '../../lib/i18n.svelte';
  import { isCurrentEvidence } from '../../lib/evidence';

  // The professional's main column above the conversation: the job from intake, proof, what was sent.
  let { app, actions, workspace, intake, code, work }: {
    app: AppState; actions: AppActions; workspace: WorkspaceState; intake?: Intake; code: RequestCode;
    work: { note: string };
  } = $props();
  const job = $derived(workspace.job);
  const customer = $derived(firstName(job.customer) || t('the customer'));
  // With agreed documentation items, each file and note is added against its item (EvidenceRequirements).
  const itemised = $derived((job.workItems?.length ?? 0) > 0);
  const itemisedNotes = $derived(job.workItems?.some(item=>item.kind==='note'));
  const photos = $derived(job.evidence.filter((item) => !!item.assetId));
  const savedNote = $derived([...job.evidence].reverse().find(item=>item.kind==='note'&&isCurrentEvidence(job,item))?.description);
  const src = (assetId?: string) => `/api/evidence/${encodeURIComponent(assetId ?? '')}?workspaceId=${encodeURIComponent(app.workspaceId ?? '')}`;
  let dragging = $state(false);
  const planning = $derived(['quote', 'booking', 'review_price', 'waiting_price', 'review_date', 'waiting_date', 'choose_visit', 'authorize_booking', 'confirm_visit', 'booked'].includes(code));

  async function upload(file: File | undefined | null) {
    if (!file) return;
    app.evidenceFile = file;
    await actions.uploadEvidence();
  }
  function pick(event: Event) { void upload((event.currentTarget as HTMLInputElement).files?.[0]); }
  function drop(event: DragEvent) { event.preventDefault(); dragging = false; void upload(event.dataTransfer?.files?.[0]); }
</script>

{#snippet intakeCard()}
  <section class="box intake" aria-labelledby="intake-title">
    <h2 id="intake-title" class="small-title">{t('The job from intake')}</h2>
    <p class="quote">{job.description}</p>
    <div class="facts">
      {#if job.location?.label}<span class="pill pill--fact"><Icon name="pin"/>{job.location.label}</span>{/if}
      {#if intake?.urgency}<span class="pill pill--fact"><Icon name="clock"/>{t(intake.urgency)}</span>{/if}
      {#if intake?.budget}<span class="pill pill--fact"><Icon name="euro"/>{intake.budget}</span>{/if}
    </div>
    {#if intake?.detail}<details class="disclosure"><summary>{t('Read the whole intake conversation')}</summary><p>{intake.detail}</p>
      {#each intake.messages.filter((m) => !m.threadProfileId) as message}<p><b>{message.role === 'customer' ? customer : t('Assistant')}:</b> {message.text}</p>{/each}
    </details>{/if}
  </section>
{/snippet}

{#if planning}
  {@render intakeCard()}
{:else if code === 'in_progress' || code === 'note_requested'}
  <section class="box proof" aria-labelledby="proof-title">
    <h2 id="proof-title" class="section-title">{code === 'note_requested' ? t('Describe the work completed') : t('Finish and send proof')}</h2>
    <p class="secondary-text">{t('Show the completed repair clearly. Explain what you repaired or replaced, and what you checked afterward. “The work is complete” is not enough.')}</p>
    {#if app.checkingCompletion}<p class="checking" role="status"><span class="spinner" aria-hidden="true"></span>{t('Checking photos and work note…')}</p>{/if}
    {#if job.completionCheck && !job.evidenceSubmitted}
      <section class="check-feedback" aria-labelledby="completion-check-title" role="status">
        <h3 id="completion-check-title" class="small-title">{t('Improve this before sending')}</h3>
        <p>{job.completionCheck.summary}</p>
        <ul>{#each job.completionCheck.coverage.filter(item=>item.coverage!=='documented') as finding(finding.scopeItem)}<li>{finding.observation}</li>{/each}</ul>
        <p class="helper">{t('Your draft has not been sent to the customer. Add clearer photos or a more specific work note, then check again.')}</p>
      </section>
    {/if}
    {#if code === 'in_progress'}
      <div class="tiles">
        {#each photos as photo, i (photo.assetId)}
          {#if photo.kind === 'photo'}<img class="tile" src={src(photo.assetId)} alt={t('Photo {n}', { n: i + 1 })}/>
          {:else}<a class="tile doc" href={src(photo.assetId)} target="_blank" rel="noreferrer"><Icon name="file"/>{t('Document {n}', { n: i + 1 })}</a>{/if}
        {/each}
        {#if app.providerConfig?.evidenceUploadsAvailable && !itemised}
          <label class="tile upload" class:dragging ondragover={(event) => { event.preventDefault(); dragging = true; }} ondragleave={() => { dragging = false; }} ondrop={drop}>
            <input id="evidence-file" type="file" accept="image/jpeg,image/png,image/webp,application/pdf" disabled={app.pending} onchange={pick}/>
            <Icon name="upload"/><b class="upload-web">{t('Add or drop photos')}</b><b class="upload-phone">{t('Add photos')}</b>
            <small>{t('JPEG, PNG, WebP or PDF, up to 10 MB')}</small>
          </label>
        {/if}
      </div>
      {#if itemised}<p class="secondary-text">{t('Add each agreed item in “Agreed completion documentation” below.')}</p>{/if}
      {#if !app.providerConfig?.evidenceUploadsAvailable}<Banner kind="warn" text={t('Photo uploads aren’t available right now.')}/>{/if}
    {/if}
    {#if itemisedNotes}
      {#if job.detailAdded}<p class="saved"><Icon name="check-circle"/>{t('Work note saved.')}</p>{/if}
    {:else}
      {#if savedNote}<div class="saved-note"><b>{t('Saved work note')}</b><p>{savedNote}</p></div>{/if}
      <div class="field">
        <label for="work-note">{job.detailAdded?t('Add detail to your work note'):t('Completion work note')}</label>
        <textarea id="work-note" class="textarea" rows="4" minlength="12" maxlength="1200" bind:value={work.note} placeholder={t('What you did and what you checked.')} aria-describedby="work-note-help"></textarea>
        <p id="work-note-help" class="field-help">{job.detailAdded&&!work.note.trim() ? t('You can check the saved note, or add more detail above.') : work.note.trim().length < 12 ? t('Write at least 12 characters to continue') : t('Saved with your proof. Don’t claim a photo proves safety.')}</p>
      </div>
      <p class="helper">{t('The check reads your saved note and any details you add here. Nothing is sent until the completion record passes.')}</p>
    {/if}
  </section>
  <AgreedLedger {job}/>
{:else if code === 'review' || code === 'payment' || code === 'dispute_review'}
  <EvidenceRecord {job} workspaceId={app.workspaceId} title={t('What you sent')} viewer="professional"/>
  {#if code === 'dispute_review' && job.dispute}
    <section class="box intake" aria-labelledby="report-title"><h2 id="report-title" class="small-title">{t('{name}’s report', { name: customer })}</h2><p class="quote">{job.dispute.reason}</p></section>
  {/if}
{:else if code === 'paid'}
  <JobReview {app} {actions} {job}/>
{:else if code === 'cancelled' && workspace.cancellationReview?.reason}
  <section class="box intake"><h2 class="small-title">{t('{name}’s note', { name: customer })}</h2><p class="quote">{workspace.cancellationReview.reason}</p></section>
{/if}

<style>
  .intake, .proof { display: grid; gap: 12px; padding: 20px 22px; }
  .facts { display: flex; flex-wrap: wrap; gap: 8px; }
  .tiles { display: flex; flex-wrap: wrap; gap: 12px; }
  .tile { width: 96px; height: 96px; border-radius: var(--sk-r-card); object-fit: cover; background: var(--sk-surface-2); }
  .tile.doc { display: grid; place-items: center; align-content: center; gap: 4px; font-size: 12px; text-align: center; text-decoration: none; color: var(--sk-ink); }
  .upload { position: relative; display: grid; place-items: center; align-content: center; gap: 4px; width: auto; min-width: 96px; padding: 10px 14px; background: var(--sk-surface); border: 1.5px dashed var(--sk-field); text-align: center; cursor: pointer; }
  .upload.dragging { background: var(--sk-turn-soft); }
  .upload input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
  .upload:has(input:focus-visible) { outline: 2px solid var(--sk-ink); outline-offset: 3px; }
  .upload b { font-size: 13px; }
  .upload small { max-width: 18ch; font-size: 12px; color: var(--sk-muted); }
  .upload-phone { display: none; }
  .saved { display: flex; align-items: center; gap: 8px; font-weight: 600; }
  .saved-note { display:grid;gap:6px;overflow-wrap:anywhere; }
  .check-feedback { display:grid;gap:10px; padding-block:16px; border-block:1px solid var(--sk-line); overflow-wrap:anywhere; }
  .check-feedback ul { display:grid;gap:8px;list-style:disc;padding-inline-start:20px; }
  .check-feedback:has(ul:empty) ul { display:none; }
  .checking { display:flex;align-items:center;gap:10px; }
  @media (max-width: 639px) { .upload-web { display: none; } .upload-phone { display: inline; } .intake, .proof { padding: 18px 16px; } }
</style>
