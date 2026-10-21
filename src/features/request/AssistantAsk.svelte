<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import LocationSearch from '../../ui/LocationSearch.svelte';
  import Composer from './Composer.svelte';
  import PaymentTimingChoice from './PaymentTimingChoice.svelte';
  import type { AppActions, AppState, WorkspaceState, Location } from '../../lib/types';
  import { noteText } from '../../lib/agent-copy';
  import { requestInteraction } from '../../lib/request-interaction';
  import { isCurrentEvidence } from '../../lib/evidence';
  import { t } from '../../lib/i18n.svelte';

  // Question and answer share one place, using the saved dialog as the source of truth.
  let { app, actions, workspace }: { app: AppState; actions: AppActions; workspace: WorkspaceState } = $props();
  const role = $derived(app.role);
  const interaction = $derived(requestInteraction(workspace, role ?? 'customer'));
  const dialog = $derived(interaction?.dialog);
  const question = $derived(interaction?.question);
  const choices = $derived(dialog?.choices ?? []);
  const timingChoice = $derived(role === 'customer' && !!app.providerConfig?.paymentsAvailable && !workspace.job.appointment && (dialog?.stage === 'offer' || ['approve-quote', 'accept-date'].includes(dialog?.pending ?? '')));
  let minimized = $state(false);
  const questionSequence = $derived(question?.seq ?? dialog?.asked);
  $effect(() => { if (questionSequence) minimized = false; });
  let address = $state('');
  let selected = $state<Location | null>(null);
  const mapbox = $derived(workspace.job.location?.source === 'mapbox' || !!app.providerConfig?.locationSearchAvailable);
  async function saveAddress(event: SubmitEvent) {
    event.preventDefault();
    if (app.pending || (mapbox && !selected)) return;
    await actions.act('set-service-address', address.trim(), selected ?? undefined);
  }
  // A question about finished work shows the proof next to it, so the person decides with it in view.
  const proof = $derived(dialog?.pending === 'approve-work' ? workspace.job.evidence.filter((e) => isCurrentEvidence(workspace.job, e) && e.kind === 'photo' && e.assetId).slice(0, 3) : []);
  const photoSrc = (assetId?: string) => `/api/evidence/${encodeURIComponent(assetId ?? '')}?workspaceId=${encodeURIComponent(workspace.workspaceId)}`;
</script>

{#if dialog?.awaiting && question}
  <section class="ask" aria-label={question.agent || t('Your assistant')} aria-live="polite">
    <div class="head">
      <span class="avatar" aria-hidden="true"><Icon name="bot" size={18}/></span>
      <b>{question.agent || t('Your assistant')}</b>
      <Pill variant="your-turn" label={t('Needs you')} small/>
      <button type="button" class="icon-btn" aria-label={t(minimized ? 'Expand current question' : 'Minimize current question')} aria-expanded={!minimized} onclick={() => { minimized = !minimized; }}><Icon name={minimized ? 'chevron-down' : 'chevron-up'} size={16}/></button>
    </div>
    <div class="answer" hidden={minimized}>
    <p class="text">{noteText(question)}</p>
    {#if proof.length}
      <ul class="proof" aria-label={t('Photos from the professional')}>
        {#each proof as photo, i (photo.assetId)}<li><a href={photoSrc(photo.assetId)} target="_blank" rel="noreferrer" aria-label={t('Open photo {n}', { n: i + 1 })}><img src={photoSrc(photo.assetId)} alt="" loading="lazy"/></a></li>{/each}
      </ul>
    {/if}
    {#if interaction?.address}
      <form class="address" onsubmit={saveAddress}>
        {#if mapbox}
          <LocationSearch id="assistant-address" label={t('Full visit address')} purpose="address" bind:value={address} bind:selected disabled={app.pending} help={t('Choose your street and house number from the suggestions. Your address stays private.')}/>
        {:else}
          <div class="field"><label for="assistant-address">{t('Full visit address')}</label><input id="assistant-address" class="input" bind:value={address} autocomplete="street-address" required minlength="8" maxlength="250" disabled={app.pending}/></div>
        {/if}
        <button class="btn btn-primary" disabled={app.pending || address.trim().length < 8 || (mapbox && !selected)}>{t('Confirm address')}</button>
      </form>
    {:else}
    {#if timingChoice}<PaymentTimingChoice {app} {actions} {workspace} id="assistant"/>{/if}
    {#if choices.length}
      <div class="choices" role="group" aria-label={t('Quick replies')}>
        {#each choices as choice (choice.id + choice.label)}
          <button type="button" class="chip" disabled={app.pending} onclick={() => actions.act('agent-reply', choice.label, undefined, undefined, { choiceId: choice.id })}>{choice.label}</button>
        {/each}
      </div>
    {/if}
    <Composer {app} {actions} assigned={!!workspace.job.id} replyOnly compact/>
    {/if}
    </div>
  </section>
{/if}

<style>
  .ask { display: grid; gap: 12px; max-width: 820px; margin: 0 auto; padding: 16px 18px; border-radius: var(--sk-r-box); background: var(--sk-turn-soft); }
  .head { display: flex; align-items: center; gap: 8px; }
  .head b { margin-right: auto; font-size: 14px; }
  .avatar { display: grid; place-items: center; width: 32px; height: 32px; border-radius: var(--sk-r-tile); background: var(--sk-brand); color: var(--sk-on-brand); }
  .text { font-size: 15px; line-height: 1.55; white-space: pre-wrap; overflow-wrap: anywhere; }
  .proof { display: flex; gap: 8px; }
  .proof img { display: block; width: 96px; height: 72px; object-fit: cover; border-radius: var(--sk-r-tile); background: var(--sk-surface-2); }
  .choices { display: flex; flex-wrap: wrap; gap: 8px; }
  .address { display: grid; gap: 14px; }
  .address > button { justify-self: start; }
  .answer { display: grid; gap: 12px; }
  .answer[hidden] { display: none; }
</style>
