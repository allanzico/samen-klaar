<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import { canCancelRequest } from '../../lib/status-ui';
  import { requestUi } from '../../lib/request-ui.svelte';
  import { t } from '../../lib/i18n.svelte';

  // "Request options": the request-level actions available right now.
  let { app, actions, workspace, onHistory, onNavy = false }: { app: AppState; actions: AppActions; workspace: WorkspaceState; onHistory: () => void; onNavy?: boolean } = $props();
  let open = $state(false);
  let root = $state<HTMLElement>();
  let trigger = $state<HTMLButtonElement>();
  const job = $derived(workspace.job);
  const canCancel = $derived(!!job.id && canCancelRequest(workspace));
  const canWithdraw = $derived(app.role === 'customer' && !!job.id && !workspace.cancelled && workspace.cancellationReview?.status !== 'pending');
  const canDelete = $derived(app.workspaces.find((item) => item.id === app.workspaceId)?.canDelete ?? (app.role === 'customer' && !job.id));
  const canReport = $derived(app.role === 'customer' && job.workStarted && !job.paid && !job.dispute);

  function run(fn: () => unknown) { open = false; void fn(); }
  function onWindow(event: Event) {
    if (!open) return;
    if (event instanceof KeyboardEvent) { if (event.key === 'Escape') { open = false; trigger?.focus(); } }
    else if (!root?.contains(event.target as Node)) open = false;
  }
</script>

<svelte:window onkeydown={onWindow} onclick={onWindow}/>

<div class="options" bind:this={root}>
  {#if onNavy}
    <button type="button" class="icon-btn on-navy" bind:this={trigger} aria-haspopup="true" aria-expanded={open} aria-label={t('Request options')} onclick={() => { open = !open; }}><Icon name="more"/></button>
  {:else}
    <button type="button" class="btn btn-quiet trigger" bind:this={trigger} aria-haspopup="true" aria-expanded={open} onclick={() => { open = !open; }}><Icon name="more"/>{t('Request options')}</button>
  {/if}
  {#if open}
    <div class="menu">
      {#if canReport}<button type="button" class="menu-row" onclick={() => run(() => { requestUi.reporting = true; })}><Icon name="alert"/>{t('Report a problem')}</button>{/if}
      {#if canWithdraw}<button type="button" class="menu-row" onclick={() => run(() => { requestUi.withdrawing = true; })}><Icon name="sign-out"/>{t('Withdraw from the service')}</button>{/if}
      {#if job.id}<button type="button" class="menu-row" onclick={() => run(onHistory)}><Icon name="list"/>{t('Job history')}</button>{/if}
      <button type="button" class="menu-row" disabled={app.pending} onclick={() => run(actions.refresh)}><Icon name="refresh"/>{t('Check for updates')}</button>
      {#if canDelete}<button type="button" class="menu-row danger" onclick={() => run(() => { app.error = ''; requestUi.deleting = true; })}><Icon name="trash"/>{app.role === 'professional' ? t('Remove from my jobs') : t('Delete request')}</button>{/if}
      {#if canCancel}<button type="button" class="menu-row danger" onclick={() => run(() => { requestUi.cancelling = true; })}><Icon name="close"/>{app.role === 'professional' ? t('Cancel job') : t('Cancel request')}</button>
      {:else if job.id && !workspace.cancelled && !job.paid}<p class="menu-note">{t('Work or payment has started. Message the other person to change or stop it.')}</p>{/if}
    </div>
  {/if}
</div>

<style>
  .options { position: relative; }
  .trigger { text-decoration: none; }
  .trigger:hover { text-decoration: underline; }
  .menu { position: absolute; top: calc(100% + 6px); right: 0; z-index: 35; display: grid; width: 280px; padding: 8px; border: 1px solid var(--sk-line); border-radius: var(--sk-r-box); background: var(--sk-surface); color: var(--sk-ink); box-shadow: var(--sk-shadow-pop); }
  .menu-row { display: flex; align-items: center; gap: 12px; min-height: 40px; padding: 0 12px; border: 0; border-radius: var(--sk-r-card); background: transparent; color: var(--sk-ink); font-size: 14px; text-align: left; }
  .menu-row:hover:not(:disabled) { background: var(--sk-ground); }
  .menu-row.danger { color: var(--sk-danger); }
  .menu-note { padding: 10px 12px; font-size: 12px; color: var(--sk-muted); border-top: 1px solid var(--sk-line-2); }
</style>
