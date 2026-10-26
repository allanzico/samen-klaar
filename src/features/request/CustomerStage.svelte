<script lang="ts">
  import Banner from '../../ui/Banner.svelte';
  import Pill from '../../ui/Pill.svelte';
  import PlanStage from './PlanStage.svelte';
  import EvidenceRecord from './EvidenceRecord.svelte';
  import AgreedLedger from './AgreedLedger.svelte';
  import JobReview from './JobReview.svelte';
  import CompletedJobReport from './CompletedJobReport.svelte';
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import type { RequestCode } from '../../lib/request-status';
  import { firstName } from '../../lib/status-ui';
  import { money } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';

  // What the main column shows above the conversation for the customer (plan, check, pay, after).
  let { app, actions, workspace, code }: { app: AppState; actions: AppActions; workspace: WorkspaceState; code: RequestCode } = $props();
  const job = $derived(workspace.job);
  const pro = $derived(firstName(job.professional));
  let selected = $state<string[]>([]);
  const files = $derived(job.evidence.filter((item) => !!item.assetId));
  const toggle = (id: string, on: boolean) => { selected = on ? [...selected, id] : selected.filter((item) => item !== id); };
  const coverageVariant = (coverage: string) => (coverage === 'documented' || coverage === 'shown' ? 'done' : coverage === 'missing' ? 'warn' : 'warn');
  const coverageLabel = (coverage: string) => t(coverage === 'documented' || coverage === 'shown' ? 'Shown' : coverage === 'missing' ? 'Missing' : 'Unclear');
</script>

{#if workspace.intake?.step === 'professional_review' || code === 'quote' || ((code === 'booking' || code === 'choose_visit') && !app.serverState?.job.appointment)}
  <PlanStage {app} {actions} {workspace} {code} reviewOnly={workspace.intake?.step === 'professional_review'}/>
{:else if code === 'in_progress'}
  <Banner kind="info" title={t('Nothing extra without your yes')} text={t('If {name} finds more to fix, it comes here as an offer first.', { name: pro })}/>
{:else if code === 'review'}
  <div class="stack">
    <EvidenceRecord {job} workspaceId={app.workspaceId} title={t('{name}’s completion record', { name: pro })} viewer="customer"/>
    <AgreedLedger {job}/>
    {#if job.evidenceReview || app.providerConfig?.guidedReviewAvailable && files.length}
      <section class="box eyes" aria-labelledby="eyes-title">
        <h2 id="eyes-title" class="small-title">{job.evidenceReview ? t('What the assistant found') : t('Want a second pair of eyes?')}</h2>
        <p class="secondary-text">{t('The assistant compares the photos with what you agreed and points out gaps. It can’t confirm workmanship or safety.')}</p>
        {#if files.length > 1}
          <fieldset class="pick"><legend class="field-label">{t('Files to compare (up to five)')}</legend>
            {#each files as file, i (file.assetId)}
              <label class="pick-row"><input type="checkbox" checked={selected.includes(file.assetId ?? '')} disabled={app.pending || (selected.length >= 5 && !selected.includes(file.assetId ?? ''))} onchange={(event) => toggle(file.assetId ?? '', event.currentTarget.checked)}/>{file.kind === 'photo' ? t('Photo {n}', { n: i + 1 }) : t('Document {n}', { n: i + 1 })}</label>
            {/each}
            <p class="helper">{t('Leave all unchecked to compare the newest files.')}</p>
          </fieldset>
        {/if}
        {#if app.providerConfig?.guidedReviewAvailable && files.length}<button type="button" class="btn btn-outline btn-sm" disabled={app.pending} aria-busy={app.reviewingEvidence} onclick={() => actions.reviewEvidence(selected)}>{app.reviewingEvidence ? t('Comparing photos…') : job.evidenceReview ? t('Check again') : t('Compare photos')}</button>{/if}
        {#if job.evidenceReview}
          <div class="found">
            <p>{job.evidenceReview.summary}</p>
            <ul class="rows">
              {#each job.evidenceReview.coverage as item (item.scopeItem)}
                <li class="found-row"><span>{item.scopeItem}<small>{item.observation}</small></span><Pill variant={coverageVariant(item.coverage)} label={coverageLabel(item.coverage)}/></li>
              {/each}
            </ul>
            <p class="helper">{job.evidenceReview.limitations}</p>
          </div>
        {/if}
      </section>
    {/if}
  </div>
{:else if code === 'payment'}
  <div class="stack">
    <AgreedLedger {job}/>
    {#if job.evidence.length}<details class="disclosure box history"><summary>{t('Job history and photos')}</summary><EvidenceRecord {job} workspaceId={app.workspaceId} title={t('Completion record')} viewer="customer"/></details>{/if}
  </div>
{:else if code === 'paid' || code === 'refund_pending' || code === 'refunded'}
  <div class="stack">
    {#if code === 'refunded'}<Banner kind="success" text={t('Your refund is complete. {amount} goes back to your PayPal.', { amount: money(job.quote.totalMinor, job.quote.currency) })}/>{/if}
    <CompletedJobReport {job} workspaceId={app.workspaceId} viewer="customer" status={code}/>
    <JobReview {app} {actions} {job}/>
  </div>
{:else if code === 'dispute_review' && job.dispute}
  <div class="stack">
    {#if job.evidence.length}<EvidenceRecord {job} workspaceId={app.workspaceId} title={t('Completion record')} viewer="customer"/>{/if}
    <section class="box report" aria-labelledby="report-title">
      <h2 id="report-title" class="small-title">{t('Your report')}</h2>
      <p class="quote">{job.dispute.reason}</p>
    </section>
    <section class="box report" aria-labelledby="now-title">
      <h2 id="now-title" class="small-title">{t('What happens now')}</h2>
      <p class="secondary-text">{t('Payment is paused. Our team looks at the report, the agreement and the proof, then decides on rework or a refund. Nothing moves without a person deciding.')}</p>
    </section>
  </div>
{:else if code === 'cancellation_review' && workspace.cancellationReview}
  <section class="box report" aria-labelledby="note-title">
    <h2 id="note-title" class="small-title">{t('Your note')}</h2>
    <p class="quote">{workspace.cancellationReview.reason || t('No note added.')}</p>
    <p class="secondary-text">{workspace.cancellationReview.kind === 'withdrawal' ? t('Our team reviews every withdrawal request. No refund or fee is decided automatically.') : t('Within 24 hours of the visit, a cancellation goes to our team. No fee is charged automatically.')}</p>
  </section>
{/if}

<style>
  .stack { display: grid; gap: 20px; }
  .eyes { display: grid; gap: 12px; padding: 20px 22px; justify-items: start; }
  .pick { display: grid; gap: 4px; }
  .pick-row { display: flex; align-items: center; gap: 10px; min-height: 44px; }
  .pick-row input { width: 18px; height: 18px; accent-color: var(--sk-ink); }
  .found { display: grid; gap: 8px; width: 100%; }
  .found-row { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; padding: 10px 0; }
  .found-row small { display: block; color: var(--sk-muted); font-size: 13px; }
  .history { padding: 8px 22px 18px; }
  .report { display: grid; gap: 10px; padding: 20px 22px; }
</style>
