<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import { api } from '../../lib/api';
  import { t } from '../../lib/i18n.svelte';
  import { stamp } from '../../lib/format';
  import type { UserNotification, WorkspaceSummary } from '../../lib/types';

  // A07 / J03: a panel under the bell on web, a full screen on phones. Rows link to their request.
  let { onSelect, workspaces }: { onSelect: (workspaceID: string) => Promise<void>; workspaces: WorkspaceSummary[] } = $props();
  let notifications = $state<UserNotification[]>([]);
  let open = $state(false);
  let loading = $state(true);
  let failed = $state(false);
  let busyID = $state<number | null>(null);
  let root = $state<HTMLElement>();
  let trigger = $state<HTMLButtonElement>();
  const unread = $derived(notifications.filter((item) => !item.readAt).length);

  const copy: Record<string, string> = {
    'send-message': 'New message about your request',
    'propose-price': 'A price offer needs your attention',
    'propose-date': 'A visit time needs your attention',
    'confirm-appointment': 'Your visit has been confirmed',
    'appointment-expired': 'The visit reservation expired',
    'appointment-reminder': 'Your visit is tomorrow',
    'submit-evidence': 'Work is ready for review',
    'request-completion-correction': 'Add clearer completion photos or work details',
    'payment-approval-reminder': 'Payment needs your approval',
    'payment-review': 'A payment update needs review',
    'refund-completed': 'Your refund is complete',
    'issue-reported': 'An issue was reported',
    'cancel-request': 'A cancellation update',
    'agent-question': 'Your agent needs your decision',
    'submit-review': 'A customer left a review',
    'respond-to-review': 'A response was added to your review'
  };
  const requestTitle = (id: string) => workspaces.find((item) => item.id === id)?.title ?? '';

  async function refresh() {
    try { notifications = (await api.notifications()).notifications; failed = false; }
    catch { failed = true; }
    finally { loading = false; }
  }
  async function openNotification(item: UserNotification) {
    if (busyID !== null) return;
    busyID = item.id;
    if (!item.readAt) {
      try {
        await api.markNotificationRead(item.id);
        notifications = notifications.map((n) => (n.id === item.id ? { ...n, readAt: new Date().toISOString() } : n));
      } catch { failed = true; }
    }
    open = false;
    busyID = null;
    await onSelect(item.workspaceId);
  }
  async function toggle() {
    open = !open;
    if (open) { void refresh(); await tick(); root?.querySelector<HTMLElement>('.panel h2')?.focus(); }
  }
  function close() { open = false; trigger?.focus(); }
  function onWindow(event: Event) {
    if (!open) return;
    if (event instanceof KeyboardEvent) { if (event.key === 'Escape') close(); }
    else if (!root?.contains(event.target as Node)) open = false;
  }

  onMount(() => {
    void refresh();
    const timer = window.setInterval(() => { if (document.visibilityState === 'visible') void refresh(); }, 30000);
    return () => window.clearInterval(timer);
  });
</script>

<svelte:window onkeydown={onWindow} onclick={onWindow}/>

<div class="bell-wrap" bind:this={root}>
  <button type="button" class="icon-btn on-navy bell" bind:this={trigger} aria-expanded={open} aria-label={unread ? t('Notifications, {count} unread', { count: unread }) : t('Notifications')} onclick={toggle}>
    <Icon name="bell"/>{#if unread}<span class="dot" aria-hidden="true"></span>{/if}
  </button>
  {#if open}
    <section class="panel" aria-labelledby="notifications-title">
      <header class="panel-head">
        <h2 id="notifications-title" tabindex="-1">{t('Notifications')}</h2>
        {#if unread}<Pill variant="your-turn" label={t('{count} new', { count: unread })}/>{/if}
        <button type="button" class="icon-btn close" aria-label={t('Close notifications')} onclick={close}><Icon name="close"/></button>
      </header>
      {#if failed}<p class="message" role="status">{t('Notifications are temporarily unavailable.')}</p>{/if}
      {#if loading}<p class="message" role="status">{t('Loading…')}</p>
      {:else if !notifications.length}<p class="message">{t('No notifications yet.')}</p>
      {:else}
        <ul class="list">
          {#each notifications as item (item.id)}
            <li>
              <button type="button" class="row" class:unread={!item.readAt} disabled={busyID === item.id} onclick={() => void openNotification(item)}>
                {#if !item.readAt}<span class="unread-dot" aria-hidden="true"></span><span class="visually-hidden">{t('Unread')}:</span>{/if}
                <span class="row-text">
                  <b>{t(copy[item.eventType] ?? 'Service request update')}</b>
                  {#if requestTitle(item.workspaceId)}<span class="row-request">{requestTitle(item.workspaceId)}</span>{/if}
                  <time datetime={item.createdAt}>{stamp(item.createdAt)}</time>
                </span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}
</div>

<style>
  .bell-wrap { position: relative; }
  .bell { position: relative; }
  .dot { position: absolute; top: 10px; right: 11px; width: 9px; height: 9px; border-radius: 50%; background: var(--sk-signal); box-shadow: 0 0 0 2px var(--sk-brand); }
  .panel { position: absolute; top: calc(100% + 10px); right: -52px; z-index: 40; width: min(400px, calc(100vw - 32px)); max-height: min(72svh, 600px); overflow: auto; border: 1px solid var(--sk-line); border-radius: var(--sk-r-box); background: var(--sk-surface); color: var(--sk-ink); box-shadow: var(--sk-shadow-pop); }
  .panel-head { display: flex; align-items: center; gap: 10px; padding: 14px 10px 10px 18px; }
  .panel-head h2 { font: 700 17px/1.2 var(--sk-fd); }
  .panel-head h2:focus { outline: none; }
  .close { margin-left: auto; }
  .message { padding: 12px 18px 20px; color: var(--sk-muted); font-size: 13px; }
  .list > li + li { border-top: 0.5px solid var(--sk-line); }
  .row { display: flex; gap: 12px; width: 100%; padding: 14px 18px; border: 0; background: var(--sk-surface); color: var(--sk-ink); text-align: left; }
  .row:hover:not(:disabled) { background: var(--sk-ground); }
  .row.unread { background: var(--sk-turn-soft); }
  .unread-dot { flex: 0 0 auto; width: 10px; height: 10px; margin-top: 6px; border-radius: 50%; background: var(--sk-brand); box-shadow: 0 0 0 1.5px var(--sk-turn-line); }
  .row-text { display: grid; gap: 2px; }
  .row-text b { font-size: 14px; font-weight: 500; }
  .row.unread .row-text b { font-weight: 700; }
  .row-request { font-size: 13px; color: var(--sk-muted); }
  time { font-size: 12px; color: var(--sk-muted); }
  @media (max-width: 639px) {
    .panel { position: fixed; inset: 0; width: auto; max-height: none; border: 0; border-radius: 0; }
    .panel-head { min-height: 56px; padding: 6px 8px 6px 16px; background: var(--sk-brand); color: var(--sk-on-brand); }
    .panel-head h2 { color: var(--sk-on-brand); }
    .close { color: var(--sk-on-brand); }
    .close:focus-visible { outline-color: var(--sk-on-brand); }
  }
</style>
