<script lang="ts">
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import { automaticReleaseOn, autonomyChoiceKey } from '../../lib/booking-autonomy';
  import { money } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';
  let { app, actions, workspace, id }: { app: AppState; actions: AppActions; workspace: WorkspaceState; id: string } = $props();
  const job = $derived(workspace.job);
  const chosen = $derived(automaticReleaseOn(job) ? 'automatic' : job.bookingAutonomy?.mode === 'manual' ? 'manual' : '');
  let value = $state('');
  $effect(() => { if (!app.pending) value = chosen; });
  async function choose(mode: 'automatic' | 'manual') {
    await actions.act('set-booking-autonomy', mode === 'automatic' ? `automatic|${autonomyChoiceKey(job)}` : 'manual');
  }
</script>

<fieldset class="autonomy" disabled={app.pending}>
  <legend>{t('Who checks the work and releases payment?')}</legend>
  <label class="radio-card"><input type="radio" name="autonomy-{id}" value="automatic" bind:group={value} disabled={!app.providerConfig?.automaticReleaseAvailable} onchange={() => choose('automatic')}/><span class="radio-dot" aria-hidden="true"></span><span><b>{t('Let my assistant review and release')}</b><small>{t('Check the professional’s photos and note, approve documented work and release {amount} from my PayPal hold.', { amount: money(job.quote.totalMinor, job.quote.currency) })}</small></span></label>
  <label class="radio-card"><input type="radio" name="autonomy-{id}" value="manual" bind:group={value} onchange={() => choose('manual')}/><span class="radio-dot" aria-hidden="true"></span><span><b>{t('I’ll review and pay myself')}</b><small>{t('Ask me before approving completion or releasing payment.')}</small></span></label>
  <p class="helper">{t('Unclear evidence comes back to you. Photos cannot confirm workmanship or safety. Turn autonomy off before payment release starts.')}</p>
  {#if !app.providerConfig?.automaticReleaseAvailable}<p class="helper">{t('Automatic review and release are unavailable right now. You can review and pay yourself.')}</p>{/if}
</fieldset>

<style>
  .autonomy { display: grid; gap: 10px; }
  legend { margin-bottom: 8px; font-size: 14px; font-weight: 600; }
</style>
