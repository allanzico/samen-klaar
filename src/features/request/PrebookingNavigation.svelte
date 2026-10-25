<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import { canReviseUnbookedRequest, previousIntakeStep } from '../../lib/prebooking';
  import { t } from '../../lib/i18n.svelte';
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  let {app,actions,workspace}: {app:AppState;actions:AppActions;workspace:WorkspaceState} = $props();
  const previous = $derived(previousIntakeStep(workspace.intake?.step ?? ''));
</script>
{#if app.role === 'customer' && canReviseUnbookedRequest(workspace)}
  {#if workspace.job.id && workspace.intake?.step !== 'professional_review'}
    <nav class="previous" aria-label={t('Request steps')}>
      <button type="button" class="btn btn-quiet btn-sm" disabled={app.pending} onclick={()=>actions.act('review-professional',workspace.job.id)}><Icon name="chevron-left"/>{t('Back to professional')}</button>
      <button type="button" class="btn btn-quiet btn-sm" disabled={app.pending} onclick={()=>actions.act('return-to-matches')}>{t('Choose another professional')}</button>
    </nav>
  {:else if !workspace.job.id && previous}
    <nav class="previous" aria-label={t('Request steps')}><button type="button" class="btn btn-quiet btn-sm" disabled={app.pending} onclick={()=>actions.act('edit-intake',previous)}><Icon name="chevron-left"/>{t('Back to previous step')}</button></nav>
  {/if}
{/if}
<style>.previous {display:flex;flex-wrap:wrap;gap:8px;padding-top:12px;padding-bottom:4px}</style>
