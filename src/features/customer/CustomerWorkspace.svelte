<script lang="ts">
  import AgreementSummary from '../workspaces/AgreementSummary.svelte';
  import IntakeConversation from './IntakeConversation.svelte';
  import CustomerJobPanel from './CustomerJobPanel.svelte';
  import type { AppActions, AppState, WorkspaceState, Intake, Job } from '../../lib/types';
  import MessageComposer from '../workspaces/MessageComposer.svelte';
  import NextStepPanel from '../workspaces/NextStepPanel.svelte';
  import { resetPlanUi } from '../../lib/plan-ui.svelte';
  import { t } from '../../lib/i18n.svelte';

  let { app, actions, state, job, intake, activeJob, composing, missingDocs, statusLabel, money }: { app: AppState; actions: AppActions; state: WorkspaceState | null; job: Job | undefined; intake: Intake | undefined; activeJob: boolean; composing: boolean; missingDocs: boolean; statusLabel: string; money: (minor: number) => string } = $props();

  resetPlanUi();

  const welcome = $derived(!intake && !activeJob && !state?.cancelled);
  // Details · Area · Match, only while the request is still being described and matched.
  const phase = $derived(!intake || activeJob ? -1 : ['detail', 'urgency', 'budget'].includes(intake.step) ? 0 : intake.step === 'location' ? 1 : ['matches', 'waitlist'].includes(intake.step) ? 2 : -1);
  const steps = $derived([t('Details'), t('Area'), t('Match')]);
  const showJobSoFar = $derived(!!intake && !activeJob && phase >= 0 && phase < 2);
  const jobRows = $derived.by(() => {
    if (!intake) return [];
    const step = intake.step;
    const rows = [
      { label: t('Trade'), value: intake.category ? t(intake.category) : '', asking: false },
      { label: t('Problem'), value: [intake.description, intake.detail].filter(Boolean).join(' · '), asking: step === 'detail' && !intake.detail },
      { label: t('When'), value: intake.urgency, asking: step === 'urgency' },
      { label: t('Budget'), value: intake.budget, asking: step === 'budget' },
      { label: t('Area'), value: intake.location?.label ?? '', asking: step === 'location' }
    ];
    return rows.map((row) => ({ ...row, state: row.value ? 'done' : row.asking ? 'asking' : 'next' }));
  });
  const examples = [
    { trade: 'Plumbing', text: 'My kitchen sink is leaking' },
    { trade: 'Carpentry', text: 'My cupboard door is stuck' },
    { trade: 'Electrical', text: 'An outlet feels warm', note: 'Assessment visit' }
  ];
</script>

{#snippet wideNextStep()}{#if state}<NextStepPanel {app} {actions} workspace={state} {money} placement="panel"/>{/if}{/snippet}

{#snippet jobSoFar()}
  <h2 class="side-title">{t('Your job so far')}</h2>
  <ul class="job-rows">
    {#each jobRows as row (row.label)}
      <li class="job-row {row.state}">
        <span class="job-row-mark" aria-hidden="true">{#if row.state === 'done'}<svg class="icon" viewBox="0 0 24 24"><path d="m5 12 5 5 9-10"/></svg>{/if}</span>
        <span class="job-row-label">{row.label}</span>
        <span class="job-row-value">{row.state === 'done' ? row.value : row.state === 'asking' ? t('Asking now…') : t('Next')}</span>
      </li>
    {/each}
  </ul>
{/snippet}

<section class="request-workspace" class:matching-layout={intake?.step === 'matches' && !activeJob} aria-label={t('Your service request')}>
  {#if state && (intake || activeJob || state.cancelled)}<AgreementSummary {app} {actions} workspace={state} {money} top={activeJob ? wideNextStep : undefined}/>{/if}
  {#if showJobSoFar}
    <aside class="side-panel job-panel" aria-label={t('Your job so far')}>{@render jobSoFar()}</aside>
    <details class="job-bar"><summary>{t('Your job so far')}</summary>{@render jobSoFar()}</details>
  {/if}
  {#if welcome}
    <aside class="side-panel happens-next">
      <h2 class="side-title">{t('What happens next')}</h2>
      <ol class="next-steps">
        <li>{t('We ask a few short questions.')}</li>
        <li>{t('You see the professional who fits best.')}</li>
        <li>{t('You approve the price and a visit time.')}</li>
        <li>{t('You pay only after you approve the finished work.')}</li>
      </ol>
    </aside>
  {/if}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (This scroll region must be reachable for keyboard scrolling.) -->
  <div id="request-thread" class="thread-scroll" role="region" tabindex="0" aria-label={t('Request conversation and decisions')}>
    {#if phase >= 0}
      <ol class="intake-progress" aria-label={t('Request progress')}>
        {#each steps as step, index (index)}<li class:done={index < phase} class:current={index === phase} aria-current={index === phase ? 'step' : undefined}><span class="progress-bar" aria-hidden="true"></span>{step}</li>{/each}
      </ol>
    {/if}
    {#if welcome}
      <div class="welcome"><h1>{t('What needs fixing?')}</h1><p>{t('Describe it in your own words. We’ll ask a few short questions, then find a professional nearby.')}</p></div>
      <div class="welcome-composer"><MessageComposer {app} {actions}/></div>
      <div class="starters" aria-label={t('Example requests')}>
        {#each examples as example}<button disabled={app.pending} onclick={() => { app.draft = t(example.text); actions.persistDraft(); actions.focusComposer(); }}><b>{t(example.trade)}</b><span>{t(example.text)}</span>{#if example.note}<small class="starter-note">{t(example.note)}</small>{/if}</button>{/each}
      </div>
    {:else}
      {#if state?.cancelled}<section class="cancelled-request"><h2>{t('This request is closed.')}</h2><p>{t('Your history remains available. Start a new request for another service.')}</p><button disabled={app.pending} onclick={actions.createRequest}>{t('Start a new request')}</button></section>{/if}
      {#if !state?.cancelled}<CustomerJobPanel {app} {actions} {state} {job} {intake} {activeJob} {missingDocs} {statusLabel} {money}/>{/if}
      <IntakeConversation {app} {actions} {job} {intake} {activeJob} composing={composing && !state?.cancelled}/>
    {/if}
    {#if app.pending && intake?.step !== 'location'}<p class="thinking" role="status">{app.brandName} {t('is working on that…')}</p>{/if}
    <div id="conversation-end"></div>
  </div>
  {#if !welcome && intake?.step !== 'location' && (!state?.cancelled || activeJob)}<div class="composer-dock">{#if state && activeJob}<NextStepPanel {app} {actions} workspace={state} {money} placement="dock"/>{/if}<MessageComposer {app} {actions} assigned={activeJob} matching={intake?.step === 'matches' || intake?.step === 'waitlist'}/></div>{/if}
</section>
