<script lang="ts">
  import ProposalComposer from './ProposalComposer.svelte';
  import type { AppState, AppActions } from '../../lib/types';
  import { aiProviderLabel } from '../../lib/ai-provider';
  import { t } from '../../lib/i18n.svelte';
  let { app, actions, assigned = false, matching = false }: { app: AppState; actions: AppActions; assigned?: boolean; matching?: boolean } = $props();
  let minimum = $derived(assigned ? 2 : matching || !app.serverState?.intake ? 5 : 2);
  let recipient = $derived(assigned ? (app.role === 'professional' ? app.serverState?.job.customer || 'your customer' : app.serverState?.job.professional || 'your professional') : app.brandName);
  let planning = $derived(assigned && !app.serverState?.cancelled && !app.serverState?.job.workStarted && !app.serverState?.job.paid);
</script>
{#if planning}
  <div class="composer-modes" aria-label={t('Message or proposal')}>
    <button type="button" class:current={app.composeMode === 'message'} aria-pressed={app.composeMode === 'message'} disabled={app.pending} onclick={actions.closeProposal}>{t('Message')}</button>
    {#if !app.serverState?.job.quoteApproved}<button type="button" class:current={app.composeMode === 'price'} aria-pressed={app.composeMode === 'price'} disabled={app.pending} onclick={() => actions.openProposal('price')}>{t('Price offer')}</button>
    {:else}<button type="button" class:current={app.composeMode === 'date'} aria-pressed={app.composeMode === 'date'} disabled={app.pending} onclick={() => actions.openProposal('date')}>{t(app.serverState?.job.appointment ? 'Change visit' : 'Visit proposal')}</button>{/if}
  </div>
{/if}
{#if planning}<ProposalComposer {app} {actions}/>{/if}
{#if app.composeMode === 'message' || !planning}
<form class="composer" onsubmit={actions.sendMessage}>
  <label for="message">{matching ? t('Correct or add to your job description') : `${t('Message to')} ${recipient}`}</label>
  <textarea id="message" bind:value={app.draft} oninput={() => actions.persistDraft()} placeholder={t(assigned ? 'Ask about the work, price or visit…' : matching ? 'Tell us what we should change about the request…' : app.serverState?.intake ? 'Your answer, in your own words…' : 'Tell us what needs help…')} rows="2" maxlength="500" disabled={app.pending} required minlength={minimum} aria-describedby="message-help draft-status" onkeydown={(event) => { if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) { event.preventDefault(); if (app.draft.trim().length >= minimum) void actions.sendMessage(); } }}></textarea>
  <div class="composer-bottom">
    <span id="message-help">{t(assigned ? 'Enter sends · Shift + Enter adds a line.' : matching ? 'We’ll check the corrected request. Timing and budget stay saved.' : 'No need to choose a trade. Shift + Enter adds a new line.')}</span>
    <button class="send-button" type="submit" aria-label={`${t('Send message to')} ${recipient}`} disabled={app.pending || app.draft.trim().length < minimum}>
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M12 19V5m-6 6 6-6 6 6"/></svg>
    </button>
  </div>
  <p id="draft-status" class="draft-status" role="status">{t(app.draftRestored ? 'Unsent draft restored.' : app.draft && app.draftSaved ? 'Draft saved in this browser tab.' : app.draft && !app.draftSaved ? 'Draft storage is unavailable. Keep this tab open.' : '')}</p>
</form>
{/if}
{#if !assigned}<p class="privacy-note">{t('Your description is shared with')} {aiProviderLabel(app.providerConfig?.aiProvider)} {t('to help understand your request. Leave out sensitive details.')}</p>{/if}
