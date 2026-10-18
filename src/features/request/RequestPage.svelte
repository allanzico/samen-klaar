<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Stepper from '../../ui/Stepper.svelte';
  import Banner from '../../ui/Banner.svelte';
  import RequestRail from '../shell/RequestRail.svelte';
  import NextStep from './NextStep.svelte';
  import Agreement from './Agreement.svelte';
  import AfterThis from './AfterThis.svelte';
  import Thread from './Thread.svelte';
import EvidenceRequirements from './EvidenceRequirements.svelte';
import CoordinationPanel from './CoordinationPanel.svelte';
import AgentPanel from '../agent/AgentPanel.svelte';
import AssistantAsk from './AssistantAsk.svelte';
import SupportNotes from './SupportNotes.svelte';
import RequestPhotos from './RequestPhotos.svelte';
import PolicyDialogs from './PolicyDialogs.svelte';
import RelatedJobs from './RelatedJobs.svelte';
  import Composer from './Composer.svelte';
  import CustomerStage from './CustomerStage.svelte';
  import ProStage from './ProStage.svelte';
  import RequestOptions from './RequestOptions.svelte';
  import RequestDialogs from './RequestDialogs.svelte';
  import IntakeStage from '../customer/IntakeStage.svelte';
  import MatchView from '../customer/MatchView.svelte';
  import JobSoFar from '../customer/JobSoFar.svelte';
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import { requestStatus } from '../../lib/request-status';
  import { canCancelRequest, firstName, statusUi } from '../../lib/status-ui';
  import { customerStep, proStep, showTimingChoice } from '../../lib/next-steps';
  import { effectiveSlot, requestUi, resetRequestUi } from '../../lib/request-ui.svelte';
  import { draftKey, readFormDraft, saveDraft } from '../../lib/drafts';
  import { shortName, tradeName, tradeOf, initials } from '../../lib/trade';
  import { t } from '../../lib/i18n.svelte';

  // A request lives in its chat: rail · main · "Your next step".
  let { app, actions, workspace, onBack }: { app: AppState; actions: AppActions; workspace: WorkspaceState; onBack: () => void } = $props();

  resetRequestUi();
  const role = $derived(app.role ?? 'customer');
  const pro = $derived(role === 'professional');
  const job = $derived(workspace.job);
  const intake = $derived(app.newConversation ? undefined : workspace.intake);
  const code = $derived(requestStatus(workspace, role));
  const other = $derived(firstName(pro ? job.customer : job.professional));
  const ui = $derived(statusUi(code, role, other));
  // While the assistant is asking something it is handling this step; the buttons below stay available.
  const assistantAsking = $derived(!!(app.role === 'professional' ? workspace.proDialog : workspace.agentDialog)?.awaiting);
  const hasJob = $derived(!!job.id);
  const matching = $derived(!hasJob && intake?.step === 'matches' && !!intake.matches.length);
  const welcome = $derived(!hasJob && !intake && !workspace.cancelled);

  // Professional work notes and extra work, saved per account, role and request in this tab.
  const workKey = (() => draftKey(app.session?.account?.id, app.role, app.workspaceId, 'work-notes'))();
  const savedWork = readFormDraft(workKey);
  const work = $state({ note: savedWork.completionNote || '', changeDescription: savedWork.changeDescription || '', changePrice: savedWork.changePrice || '' });
  let workSaved = $state(true);
  $effect(() => { if (pro) workSaved = saveDraft(workKey, [work.note, work.changeDescription, work.changePrice].some(Boolean) ? JSON.stringify({ completionNote: work.note, changeDescription: work.changeDescription, changePrice: String(work.changePrice ?? '') }) : ''); });
  async function saveNote() { if (await actions.act('add-evidence', work.note)) work.note = ''; }

  // Payment timing follows the saved choice per job; paying after the work is the default.
  $effect(() => {
    if (requestUi.timingJob !== job.id) { requestUi.timingJob = job.id; requestUi.timing = job.paymentAuthorizationTiming ?? 'after_completion'; }
    else if (job.paymentAuthorizationTiming) requestUi.timing = job.paymentAuthorizationTiming;
  });

  const slot = $derived(effectiveSlot(workspace.availableSlots, !(job.location && !job.serviceAddress)));
  const view = $derived(pro
    ? proStep({ app, actions, workspace, code, noteReady: work.note.trim().length >= 12, saveNote, openExtra: () => { requestUi.extraWork = true; } })
    : customerStep({ app, actions, workspace, code, timing: requestUi.timing, slot, openReport: () => { requestUi.reporting = true; }, openPolicy: () => { requestUi.policy = true; } }));
  const timingChoice = $derived(!pro && showTimingChoice(app, workspace, code));

  const customerPages = ['quote', 'booking', 'choose_visit', 'review', 'payment', 'paid', 'refunded', 'refund_pending', 'dispute_review', 'cancellation_review'];
  const proPages = ['in_progress', 'note_requested', 'review', 'payment', 'paid', 'dispute_review'];
  const stagePage = $derived(hasJob && (pro ? proPages : customerPages).includes(code) && !(code === 'booking' && !!job.appointment));
  const showComposer = $derived(pro ? hasJob : intake?.step !== 'location' && (!workspace.cancelled || hasJob));
  const canCancel = $derived(hasJob && canCancelRequest(workspace));
  let historyOpen = $state(false);

  const title = $derived(hasJob ? job.title : intake?.description || t('New request'));
  const trade = $derived(tradeOf(job.category || intake?.category));
  const subline = $derived(hasJob ? (pro ? t('{name}, customer', { name: shortName(job.customer) }) : t('{name}, {trade}', { name: job.professional, trade: tradeName(trade) })) : '');
  const area = $derived(job.location?.label || intake?.location?.label || '');
  const listLabel = $derived(t(pro ? 'Your jobs' : 'Your requests'));

  // The next step sits in the side column on wide screens and in the dock on narrow ones; focus whichever is showing.
  function focusNextStep() { [...document.querySelectorAll<HTMLElement>('.next-step .btn')].find((button) => button.offsetParent !== null)?.focus(); }
</script>

{#snippet timing(placement: string)}
  {#if timingChoice}
    <fieldset class="timing" disabled={app.pending}>
      <legend>{t('When should PayPal authorize the amount?')}</legend>
      <label class="radio-card"><input type="radio" name="timing-{placement}" value="after_completion" bind:group={requestUi.timing}/><span class="radio-dot" aria-hidden="true"></span><span><b>{t('After the work is approved')}</b><small>{t('Pay after you check the work.')}</small></span></label>
      <label class="radio-card"><input type="radio" name="timing-{placement}" value="at_booking" bind:group={requestUi.timing}/><span class="radio-dot" aria-hidden="true"></span><span><b>{t('At booking')}</b><small>{t('PayPal holds it. Nothing is taken until you approve.')}</small></span></label>
    </fieldset>
  {/if}
{/snippet}

{#snippet asideContent()}
  {#if view}<NextStep {view} pending={app.pending} placement="aside" extra={timing}/>{/if}
  {#if !hasJob && !workspace.cancelled}
    <JobSoFar {intake} busy={app.pending} onEdit={actions.focusComposer}/>
  {:else if hasJob && !stagePage && !workspace.cancelled}
    <Agreement {job} {role} variant="rows"/>
  {/if}
  {#if view?.after?.length}<AfterThis items={view.after} title={view.kind === 'waiting' ? 'After that' : 'After this'}/>{/if}
  {#if canCancel}<button type="button" class="btn btn-danger-link cancel" onclick={() => { requestUi.cancelling = true; }}>{pro ? t('Cancel job') : job.appointment ? t('Cancel visit') : t('Cancel request')}</button>{/if}
{/snippet}

<div class="request-page" class:with-aside={!matching}>
  <div class="rail-col">
    <RequestRail items={app.workspaces} currentID={app.workspaceId} {role} busy={app.pending || app.loading} canLoadMore={!!app.workspaceCursor}
      onSelect={(id) => actions.selectRequest(id)} onCreate={pro ? undefined : actions.createRequest} onLoadMore={actions.loadMoreWorkspaces}/>
  </div>

  <div class="main-col" class:page={stagePage}>
    <header class="phone-head on-navy">
      <button type="button" class="icon-btn on-navy" aria-label={t('Back to {list}', { list: listLabel })} onclick={onBack}><Icon name="chevron-left"/></button>
      <div class="phone-title"><p class="phone-h">{title}</p>{#if subline || area}<p class="phone-sub">{[subline, area].filter(Boolean).join(' · ')}</p>{/if}</div>
      <RequestOptions {app} {actions} {workspace} onHistory={() => { historyOpen = true; }} onNavy/>
    </header>
    {#if ui.step}<div class="phone-strip"><Stepper step={ui.step} variant="strip"/></div>{/if}
    {#if hasJob && !workspace.cancelled}<div class="bar-wrap"><Agreement {job} {role} variant="bar"/></div>{/if}

    {#if !matching}
      <header class="web-head">
        <button type="button" class="btn btn-quiet back-link" onclick={onBack}><Icon name="chevron-left"/>{listLabel}</button>
        <div class="title-row">
          <div class="title-block">
            <h1 class="request-title">{title}</h1>
            {#if subline || area}
              <p class="subline">
                {#if subline}<span class="avatar avatar-24" aria-hidden="true">{initials(pro ? job.customer : job.professional)}</span><span>{subline}</span>{/if}
                {#if area}<span class="area"><Icon name="pin" size={15}/>{area}</span>{/if}
              </p>
            {/if}
          </div>
          <RequestOptions {app} {actions} {workspace} onHistory={() => { historyOpen = true; }}/>
        </div>
        {#if ui.step}<Stepper step={ui.step} variant={stagePage || !hasJob ? 'full' : 'compact'}/>{/if}
        {#if stagePage && !workspace.cancelled}<Agreement {job} {role} variant="strip"/>{/if}
      </header>
    {/if}

    <div id="request-thread" class="scroll" data-chat={!stagePage && !matching ? 'true' : undefined}>
      {#if app.error}<div class="content-pad"><Banner kind="error" text={t(app.error)} onDismiss={actions.dismissError}/></div>{/if}
      <div class="dock-step content-pad"><AssistantAsk {app} {actions} {workspace}/></div>
      {#if view && !matching && !assistantAsking}<div class="dock-step content-pad"><NextStep {view} pending={app.pending} placement="dock" extra={timing}/></div>{/if}
      {#if matching && intake}
        <div class="content-pad"><AgentPanel {app} {workspace}/></div>
        <MatchView {app} {actions} {intake}/>
      {:else}
        <div class="content-pad content">
          {#if hasJob}
            {#if pro}<ProStage {app} {actions} {workspace} {intake} {code} {work}/>{:else}<CustomerStage {app} {actions} {workspace} {code}/>{/if}
          {/if}
          {#if hasJob}<EvidenceRequirements {app} {actions} {job}/><AgentPanel {app} {workspace}/><CoordinationPanel {app} {workspace}/><SupportNotes {workspace}/>{/if}
          {#if !pro}<RelatedJobs {app} {actions}/>{/if}
          {#if welcome}
            <div class="welcome"><h2 class="section-title">{t('What needs fixing?')}</h2><p class="secondary-text">{t('Say it in your own words. We’ll ask what the pro needs to know.')}</p></div>
          {:else}
            <RequestPhotos {app} {actions} {workspace}/>
            {#if stagePage && hasJob}<h2 class="conversation-title small-title">{t('Conversation')}</h2>{/if}
            <Thread {app} job={hasJob ? job : undefined} {intake} onAnswer={focusNextStep}/>
            {#if intake && !hasJob}<IntakeStage {app} {actions} {intake}/>{/if}
          {/if}
          {#if app.pending && !hasJob}<p class="thinking" role="status"><span class="spinner" aria-hidden="true"></span>{t('{brand} is working on that…', { brand: app.brandName })}</p>{/if}
          {#if pro && [work.note, work.changeDescription, work.changePrice].some(Boolean)}<p class="helper" role="status">{t(workSaved ? 'Unsent work notes saved in this browser tab.' : 'Draft storage is unavailable. Keep this tab open.')}</p>{/if}
        </div>
      {/if}
    </div>

    {#if showComposer}
      <div class="composer-row"><Composer {app} {actions} compact={stagePage && app.composeMode === 'message'} assigned={hasJob} matching={!hasJob && (intake?.step === 'matches' || intake?.step === 'waitlist')}/></div>
    {/if}
  </div>

  {#if !matching}<aside class="aside-col" aria-label={t('Your next step')}>{@render asideContent()}</aside>{/if}
</div>

<RequestDialogs {app} {actions} {workspace} {work} bind:historyOpen/>
{#if !pro}<PolicyDialogs {app} {actions} {workspace}/>{/if}

<style>
  .request-page { display: flex; height: calc(100svh - var(--sk-topbar-h)); min-height: 0; }
  .rail-col { display: flex; flex: 0 0 248px; min-height: 0; }
  .main-col { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; background: var(--sk-ground); }
  .main-col.page .web-head, .main-col.page .scroll { background: var(--sk-surface); }
  .aside-col { display: flex; flex-direction: column; gap: 22px; flex: 0 1 380px; min-width: 320px; padding: 24px; overflow-y: auto; background: var(--sk-surface); border-left: 1px solid var(--sk-line); }
  .main-col.page ~ .aside-col { background: var(--sk-ground); border-left: 0; }

  .phone-head, .phone-strip, .bar-wrap { display: none; }
  .web-head { display: grid; gap: 14px; padding: 22px 40px 18px; background: var(--sk-surface); border-bottom: 0.5px solid var(--sk-line); }
  .back-link { display: none; justify-self: start; margin: -8px 0 -6px -8px; text-decoration: none; }
  .title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
  .title-block { display: grid; gap: 6px; min-width: 0; }
  .request-title { font: 800 26px/1.12 var(--sk-fd); font-stretch: 92%; letter-spacing: -0.015em; overflow-wrap: anywhere; }
  .subline { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; font-size: 13px; color: var(--sk-muted); }
  .area { display: inline-flex; align-items: center; gap: 4px; margin-left: 6px; }

  .scroll { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; }
  .content-pad { padding: 0 40px; }
  .content { display: grid; gap: 24px; align-content: start; padding-top: 24px; padding-bottom: 28px; }
  .main-col.page .content > :global(*) { max-width: 820px; }
  .dock-step { display: none; padding-top: 16px; }
  .conversation-title { margin-top: 8px; }
  .welcome { display: grid; gap: 6px; max-width: 820px; margin: 12vh auto 0; text-align: center; }
  .thinking { display: flex; align-items: center; gap: 10px; color: var(--sk-muted); font-size: 13px; max-width: 820px; width: 100%; margin: 0 auto; }
  .main-col.page .composer-row { padding-bottom: 16px; }
  .composer-row { padding: 12px 40px 28px; background: var(--sk-ground); }
  .main-col.page .composer-row { border-top: 0.5px solid var(--sk-line); background: var(--sk-surface); }

  .timing { display: grid; gap: 10px; }
  .timing legend { margin-bottom: 8px; font-size: 14px; font-weight: 600; }
  .cancel { align-self: flex-start; }

  /* On a short window the fixed header and composer would leave the conversation a sliver: tighten the
     header, and drop the agreement strip (the same facts sit in the side panel or one tap away). */
  @media (min-width: 640px) and (max-height: 820px) {
    .web-head { gap: 8px; padding: 12px 40px 10px; }
    .request-title { font-size: 20px; }
    .web-head :global(dl.strip) { display: none; }
    .composer-row, .main-col.page .composer-row { padding-top: 8px; padding-bottom: 12px; }
  }
  @media (max-width: 1279px) {
    .aside-col { display: none; }
    .dock-step { display: block; }
  }
  @media (max-width: 1023px) {
    .rail-col { display: none; }
    .back-link { display: inline-flex; }
  }
  @media (max-width: 639px) {
    .request-page { height: 100svh; }
    .web-head { display: none; }
    .phone-head { position: relative; z-index: 5; display: flex; align-items: center; gap: 4px; min-height: 60px; padding: 6px 4px 6px 8px; background: var(--sk-brand); color: var(--sk-on-brand); }
    .phone-title { flex: 1; min-width: 0; }
    .phone-h { font: 700 16px/1.2 var(--sk-fd); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .phone-sub { font-size: 12px; color: var(--sk-on-brand-2); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .phone-strip { display: block; padding: 12px 16px; background: var(--sk-surface); border-bottom: 0.5px solid var(--sk-line); }
    .bar-wrap { display: block; }
    .content-pad { padding: 0 16px; }
    .content { gap: 18px; padding-top: 16px; }
    .composer-row { padding: 10px 14px calc(14px + env(safe-area-inset-bottom)); border-top: 0.5px solid var(--sk-line); }
    .main-col.page .web-head, .main-col.page .scroll { background: var(--sk-ground); }
  }
</style>
