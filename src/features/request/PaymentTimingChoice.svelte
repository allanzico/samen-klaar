<script lang="ts">
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import { automaticReleaseOn } from '../../lib/booking-autonomy';
  import { t } from '../../lib/i18n.svelte';
  let { app, actions, workspace, id }: { app: AppState; actions: AppActions; workspace: WorkspaceState; id: string } = $props();
  const chosen = $derived(workspace.job.paymentAuthorizationTiming || 'after_completion');
  let value = $state('after_completion');
  $effect(() => { if (!app.pending) value = chosen; });
  async function choose(value: 'after_completion' | 'at_booking') {
    if (value !== chosen) await actions.act('set-payment-timing', '', undefined, undefined, undefined, undefined, value);
  }
</script>

{#if automaticReleaseOn(workspace.job)}
<p class="helper">{t('Authorize the agreed amount in PayPal at booking. Your assistant releases it only after checking the completion record.')}</p>
{:else}
<fieldset class="timing" disabled={app.pending}>
  <legend>{t('When should PayPal authorize the amount?')}</legend>
  <label class="radio-card"><input type="radio" name="timing-{id}" value="after_completion" bind:group={value} onchange={() => choose('after_completion')}/><span class="radio-dot" aria-hidden="true"></span><span><b>{t('After the work is approved')}</b><small>{t('Pay after you check the work.')}</small></span></label>
  <label class="radio-card"><input type="radio" name="timing-{id}" value="at_booking" bind:group={value} onchange={() => choose('at_booking')}/><span class="radio-dot" aria-hidden="true"></span><span><b>{t('At booking')}</b><small>{t('PayPal holds it. Nothing is taken until you approve.')}</small></span></label>
</fieldset>
{/if}

<style>
  .timing { display: grid; gap: 10px; }
  legend { margin-bottom: 8px; font-size: 14px; font-weight: 600; }
</style>
