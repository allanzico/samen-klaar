<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../../lib/api';
  import { currentLocale, t } from '../../lib/i18n.svelte';
  import type { UserNotification } from '../../lib/types';

  let { onSelect }: { onSelect: (workspaceID: string) => Promise<void> } = $props();
  let notifications = $state<UserNotification[]>([]);
  let open = $state(false);
  let loading = $state(true);
  let failed = $state(false);
  let busyID = $state<number | null>(null);
  let unreadCount = $derived(notifications.filter((item) => !item.readAt).length);

  const notificationCopy: Record<string, string> = {
    'send-message': 'New message about your request',
    'propose-price': 'A price offer needs your attention',
    'propose-date': 'A visit time needs your attention',
    'confirm-appointment': 'Your visit has been confirmed',
    'appointment-expired': 'The visit reservation expired',
    'appointment-reminder': 'Your visit is tomorrow',
    'submit-evidence': 'Work is ready for review',
    'payment-approval-reminder': 'Payment needs your approval',
    'payment-review': 'A payment update needs review',
    'refund-completed': 'Your refund is complete',
    'issue-reported': 'An issue was reported',
    'cancel-request': 'A cancellation update',
    'submit-review': 'A customer left a review',
    'respond-to-review': 'A response was added to your review'
  };

  async function refresh() {
    try {
      notifications = (await api.notifications()).notifications;
      failed = false;
    } catch {
      failed = true;
    } finally {
      loading = false;
    }
  }

  function formatDate(value: string) {
    return new Intl.DateTimeFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', {
      year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
    }).format(new Date(value));
  }

  async function openNotification(item: UserNotification) {
    if (busyID !== null) return;
    busyID = item.id;
    failed = false;
    if (!item.readAt) {
      try {
        await api.markNotificationRead(item.id);
        notifications = notifications.map((notification) => notification.id === item.id
          ? { ...notification, readAt: new Date().toISOString() }
          : notification);
      } catch {
        failed = true;
      }
    }
    open = false;
    busyID = null;
    await onSelect(item.workspaceId);
  }

  onMount(() => {
    void refresh();
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void refresh();
    }, 30000);
    return () => window.clearInterval(timer);
  });
</script>

<details class="notification-center" bind:open>
  <summary aria-label={unreadCount ? `${t('Notifications')}, ${unreadCount} ${t('unread')}` : t('Notifications')}>
    <svg class="icon" aria-hidden="true" viewBox="0 0 24 24" width="20" height="20"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9ZM13.7 21a2 2 0 0 1-3.4 0"/></svg>
    {#if unreadCount}<span class="notification-dot" aria-hidden="true"></span>{/if}
  </summary>
  <section class="notification-panel" aria-label={t('Notifications')}>
    <div class="notification-panel-heading"><h2>{t('Notifications')}</h2><button class="text-button" onclick={() => { open = false; }}>{t('Close')}</button></div>
    {#if failed}<p class="notification-message" role="status">{t('Notifications are temporarily unavailable.')}</p>{/if}
    {#if loading}<p class="notification-message" role="status">{t('Loading…')}</p>
    {:else if notifications.length === 0}<p class="notification-message">{t('No notifications yet.')}</p>
    {:else}
      <ul class="notification-list">
        {#each notifications as item (item.id)}
          <li><button class="notification-item" class:unread={!item.readAt} disabled={busyID === item.id} onclick={() => void openNotification(item)}>
            <span class="notification-item-title">{t(notificationCopy[item.eventType] ?? 'Service request update')}</span>
            <time datetime={item.createdAt}>{formatDate(item.createdAt)}</time>
          </button></li>
        {/each}
      </ul>
    {/if}
  </section>
</details>

<style>
  .notification-center { position: relative; z-index: 6; flex: 0 0 auto; }
  .notification-center > summary { position: relative; display: grid; place-items: center; width: 44px; height: 44px; min-height: 44px; min-width: 44px; padding: 0; list-style: none; border: 1px solid var(--sk-line); border-radius: var(--sk-r-control); background: var(--sk-bg); cursor: pointer; color: var(--sk-ink); }
  .notification-center > summary:hover { background: var(--sk-subtle); border-color: var(--sk-line-strong); }
  .notification-center > summary:focus-visible { outline: 2px solid var(--sk-blue-700); outline-offset: 3px; }
  .notification-center > summary::-webkit-details-marker { display: none; }
  .notification-center > summary::after { content: none; }
  .notification-dot { position: absolute; top: 9px; right: 10px; width: 10px; height: 10px; border-radius: 50%; background: var(--sk-blue-700); box-shadow: 0 0 0 2px var(--sk-bg); }
  .notification-panel { position: absolute; top: calc(100% + 8px); right: 0; width: min(360px, calc(100vw - 32px)); max-height: min(70dvh, 560px); overflow: auto; border: 1px solid var(--sk-line); border-radius: var(--sk-r-panel); background: var(--sk-bg); box-shadow: var(--sk-shadow-float); }
  .notification-panel-heading { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 14px 16px; border-bottom: 1px solid var(--sk-line); }
  .notification-panel-heading h2 { margin: 0; font-size: 17px; font-weight: 600; }
  .notification-panel-heading .text-button { padding: 7px; }
  .notification-message { margin: 0; padding: 20px 16px; color: var(--sk-muted); font-size: 13px; }
  .notification-list { display: grid; gap: 0; margin: 0; padding: 0; list-style: none; }
  .notification-list li + li { border-top: 1px solid var(--sk-line); }
  .notification-item { display: grid; width: 100%; gap: 7px; border: 0; border-radius: 0; padding: 14px 16px; background: var(--sk-bg); text-align: left; }
  .notification-item:hover:not(:disabled) { background: var(--sk-subtle); border-color: transparent; }
  .notification-item:disabled { background: var(--sk-bg); border-color: transparent; color: var(--sk-muted); }
  .notification-item.unread { border-left: 3px solid var(--sk-blue-700); padding-left: 13px; background: var(--sk-blue-50); }
  .notification-item.unread:hover:not(:disabled) { background: var(--sk-blue-50); border-left-color: var(--sk-blue-700); }
  .notification-item-title { font-size: 13px; font-weight: 500; }
  .notification-item time { color: var(--sk-muted); font-size: 12px; }
  /* On phones the panel spans the viewport so it can never clip, wherever the bell sits. */
  @media (max-width: 600px) { .notification-panel { position: fixed; top: 64px; left: 16px; right: 16px; width: auto; } }
</style>
