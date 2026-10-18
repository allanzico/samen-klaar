<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import RequestCard from './RequestCard.svelte';
  import type { AppActions, AppState } from '../../lib/types';
  import { listFilter, otherFromLabel, statusUi, type ListFilter } from '../../lib/status-ui';
  import { t } from '../../lib/i18n.svelte';

  // B02 / B03: your requests with filter tabs. Counts cover the loaded pages.
  let { app, actions }: { app: AppState; actions: AppActions } = $props();
  let filter = $state<'all' | ListFilter>('all');
  const sorted = $derived([...app.workspaces].sort((a, b) => {
    const rank = statusUi(a.status, 'customer').rank - statusUi(b.status, 'customer').rank;
    return rank || (b.updatedAt ?? '').localeCompare(a.updatedAt ?? '');
  }));
  const count = (key: ListFilter) => app.workspaces.filter((item) => listFilter(item.status, 'customer') === key).length;
  const visible = $derived(filter === 'all' ? sorted : sorted.filter((item) => listFilter(item.status, 'customer') === filter));
  const tabs = $derived([
    { key: 'all' as const, label: t('All'), count: app.workspaces.length },
    { key: 'your-turn' as const, label: t('Your turn'), count: count('your-turn') },
    { key: 'progress' as const, label: t('In progress'), count: count('progress') },
    { key: 'done' as const, label: t('Done'), count: count('done') }
  ]);
  const needs = $derived(count('your-turn'));
</script>

<div class="requests">
  <header class="head">
    <div>
      <h1 class="page-title">{t('Your requests')}</h1>
      {#if app.workspaces.length}
        <p class="summary">{#if needs}<strong>{needs === 1 ? t('1 needs you.') : t('{count} need you.', { count: needs })}</strong>{' '}{/if}{t('{progress} in progress, {done} klaar.', { progress: count('progress'), done: count('done') })}</p>
      {/if}
    </div>
    <button type="button" class="btn btn-primary btn-strong" disabled={app.pending} onclick={actions.showHome}><Icon name="plus"/>{t('New request')}</button>
  </header>

  {#if app.workspaces.length}
    <div class="filter-tabs" role="tablist" aria-label={t('Filter requests')}>
      {#each tabs as tab (tab.key)}
        <button type="button" role="tab" aria-selected={filter === tab.key} aria-controls="request-cards" onclick={() => { filter = tab.key; }}>{tab.label}<span class="count" class:count--turn={tab.key === 'your-turn' && tab.count > 0}>{tab.count}</span></button>
      {/each}
    </div>
    <div id="request-cards" class="cards" role="tabpanel">
      {#each visible as item (item.id)}<RequestCard {item} onOpen={(id) => actions.selectRequest(id)}/>{/each}
      {#if !visible.length}<p class="secondary-text none">{t('Nothing here right now.')}</p>{/if}
    </div>
    {#if app.workspaceCursor}<button type="button" class="btn btn-light load" disabled={app.pending || app.loading} onclick={actions.loadMoreWorkspaces}>{t('Load older requests')}</button>{/if}
  {:else}
    <div class="empty">
      <span class="icon-tile icon-tile-48" aria-hidden="true"><Icon name="list"/></span>
      <h2>{t('No requests yet')}</h2>
      <p>{t('Describe what needs fixing and we’ll find a reviewed professional who covers your area.')}</p>
      <button type="button" class="btn btn-primary" onclick={actions.showHome}>{t('Describe a job')}</button>
    </div>
  {/if}
</div>

<style>
  .requests { display: grid; gap: 24px; max-width: 1040px; margin: 0 auto; padding: 64px 48px 72px; }
  .head { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 20px; }
  .summary { margin-top: 14px; font-size: 16px; color: var(--sk-muted); }
  .summary strong { color: var(--sk-ink); font-weight: 700; }
  .filter-tabs { justify-self: start; }
  .cards { display: grid; gap: 12px; }
  .none { padding: 20px 0; }
  .load { justify-self: center; }
  @media (max-width: 639px) {
    .requests { gap: 20px; padding: 28px 16px 40px; }
    .head .btn { width: 100%; }
  }
</style>
