<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import type { AppActions, AppState, WorkspaceSummary } from '../../lib/types';
  import { api } from '../../lib/api';
  import { firstName, listFilter, otherFromLabel, statusUi, type ListFilter } from '../../lib/status-ui';
  import { dayMonth, dayNumber, money, shortDay, time, untilShort, weekdayShort } from '../../lib/format';
  import { agentUi } from '../../lib/agent.svelte';
  import { t } from '../../lib/i18n.svelte';

  // F01 / F02 / J02: jobs with All · New · Active · Finished. A job awaiting confirmation can be
  // confirmed or declined from its row; it is the same action the job page sends.
  let { app, actions, onSetTimes, onProfile }: { app: AppState; actions: AppActions; onSetTimes: () => void; onProfile: () => void } = $props();
  let filter = $state<'all' | ListFilter>('all');
  let busy = $state('');
  let error = $state('');
  const ui = (item: WorkspaceSummary) => statusUi(item.status, 'professional', firstName(item.counterpart) || otherFromLabel(item.statusLabel));
  const sorted = $derived([...app.workspaces].sort((a, b) => ui(a).rank - ui(b).rank || (a.visitStartUtc ?? '9').localeCompare(b.visitStartUtc ?? '9') || (b.updatedAt ?? '').localeCompare(a.updatedAt ?? '')));
  const count = (key: ListFilter) => app.workspaces.filter((item) => listFilter(item.status, 'professional') === key).length;
  const visible = $derived(filter === 'all' ? sorted : sorted.filter((item) => listFilter(item.status, 'professional') === filter));
  const tabs = $derived([
    { key: 'all' as const, label: t('All'), count: app.workspaces.length },
    { key: 'new' as const, label: t('New'), count: count('new') },
    { key: 'active' as const, label: t('Active'), count: count('active') },
    { key: 'finished' as const, label: t('Finished'), count: count('finished') }
  ]);

  // The assistant's open question for this job, if any: the name of the bot that is asking.
  const asking = (item: WorkspaceSummary) => { const q = agentUi.escalations.find((e) => e.workspaceId === item.id); return q ? q.params?.agent || t('Your assistant') : ''; };
  function tag(item: WorkspaceSummary) {
    const status = ui(item);
    if (item.status === 'confirm_visit' && item.confirmDueAt) return { variant: 'your-turn' as const, label: t('Confirm within {time}', { time: untilShort(item.confirmDueAt) }), light: false };
    return { variant: status.pill, label: status.pillLabel, light: status.pill === 'booked', icon: status.icon };
  }
  function when(item: WorkspaceSummary) {
    if (!item.visitStartUtc) return ui(item).line;
    const range = t('{start} to {end}', { start: time(item.visitStartUtc, item.visitTimezone), end: time(item.visitEndUtc ?? item.visitStartUtc, item.visitTimezone) });
    return listFilter(item.status, 'professional') === 'finished' ? t('Done {day}', { day: shortDay(item.visitStartUtc, item.visitTimezone) }) : `${shortDay(item.visitStartUtc, item.visitTimezone)} · ${range}`;
  }
  async function decide(item: WorkspaceSummary, action: 'confirm-appointment' | 'decline-appointment') {
    if (busy) return;
    busy = item.id;
    error = '';
    try { await api.action(item.id, action); await actions.refresh(); }
    catch (e) { error = e instanceof Error ? e.message.trim() : t('That action could not be completed.'); }
    finally { busy = ''; }
  }
</script>

<div class="jobs">
  <h1 class="page-title jobs-title">{t('Your jobs')}</h1>
  {#if error}<div class="banner banner--error" role="alert"><Icon name="alert"/><p>{t(error)}</p></div>{/if}
  {#if app.workspaces.length}
    <div class="filter-tabs compact" role="tablist" aria-label={t('Filter jobs')}>
      {#each tabs as tab (tab.key)}
        <button type="button" role="tab" aria-selected={filter === tab.key} aria-controls="job-rows" onclick={() => { filter = tab.key; }}>{tab.label}<span class="count" class:count--turn={tab.key === 'new' && tab.count > 0}>{tab.count}</span></button>
      {/each}
    </div>
    <div id="job-rows" role="tabpanel"><ul class="rows-list">
      {#each visible as item (item.id)}
        {@const label = tag(item)}
        {@const confirming = item.status === 'confirm_visit'}
        <li class="row" class:turn={ui(item).pill === 'your-turn'}>
          <a class="row-link" href="/?workspaceId={encodeURIComponent(item.id)}" onclick={(event) => { event.preventDefault(); void actions.selectRequest(item.id); }}>
            <span class="date-tile" aria-hidden="true">
              {#if item.visitStartUtc}<small>{weekdayShort(item.visitStartUtc, item.visitTimezone)}</small><b>{dayNumber(item.visitStartUtc, item.visitTimezone)}</b>
              {:else}<Icon name="calendar"/>{/if}
            </span>
            <span class="row-main"><span class="row-title">{item.title}</span><span class="row-when">{when(item)}{#if !item.visitStartUtc && item.updatedAt}{' · '}{dayMonth(item.updatedAt)}{/if}</span></span>
            <span class="row-tag"><Pill variant={label.variant} label={label.label} light={label.light} icon={label.icon} row/></span>
            <span class="row-price">{item.totalMinor ? money(item.totalMinor, item.currency) : ''}</span>
            {#if !confirming}<Icon name="chevron-right" class="row-chev"/>{/if}
          </a>
          {#if asking(item)}<p class="asking"><Icon name="bot" size={14}/>{t('{name} is asking you in the chat', { name: asking(item) })}</p>{/if}
          {#if confirming}
            <div class="row-actions">
              <button type="button" class="btn btn-outline btn-sm" disabled={!!busy || app.pending} onclick={() => decide(item, 'decline-appointment')}>{t('Decline')}</button>
              <button type="button" class="btn btn-primary btn-sm" disabled={!!busy || app.pending} aria-busy={busy === item.id ? 'true' : undefined} onclick={() => decide(item, 'confirm-appointment')}><Icon name="check"/>{t('Confirm')}</button>
            </div>
          {/if}
        </li>
      {/each}
      {#if !visible.length}<li class="secondary-text none">{t('Nothing here right now.')}</li>{/if}
    </ul></div>
    {#if app.workspaceCursor}<button type="button" class="btn btn-light load" disabled={app.pending} onclick={actions.loadMoreWorkspaces}>{t('Load older jobs')}</button>{/if}
  {:else}
    <div class="empty">
      <span class="icon-tile icon-tile-48" aria-hidden="true"><Icon name="briefcase"/></span>
      <h2>{t('No jobs yet')}</h2>
      <p>{t('When a customer chooses you, the job appears here with everything they told us.')}</p>
      <div class="empty-actions"><button type="button" class="btn btn-primary" onclick={onSetTimes}>{t('Add free times')}</button><button type="button" class="btn btn-outline" onclick={onProfile}>{t('Review your service')}</button></div>
    </div>
    <section class="wait" aria-labelledby="wait-title">
      <p id="wait-title" class="eyebrow">{t('While you wait')}</p>
      <p class="secondary-text">{t('Publish free times so customers can book you directly. Without them, you agree a time in chat.')}</p>
    </section>
  {/if}
</div>

<style>
  .jobs { display: grid; gap: 22px; max-width: 920px; margin: 0 auto; padding: 56px 48px 72px; }
  .jobs-title { font-size: 36px; }
  .filter-tabs { justify-self: start; }
  .rows-list { display: grid; gap: 12px; }
  .row { display: flex; align-items: center; gap: 12px; padding-right: 20px; border-radius: var(--sk-r-box); background: var(--sk-surface); border: 1px solid var(--sk-line); }
  .row.turn { background: var(--sk-turn-soft); border-color: transparent; }
  .row:not(.turn):hover { border-color: var(--sk-field); }
  .row-link { flex: 1; display: grid; grid-template-columns: 56px 1fr auto auto 20px; gap: 20px; align-items: center; min-width: 0; padding: 16px 0 16px 20px; color: var(--sk-ink); text-decoration: none; }
  .row:has(.row-actions) .row-link { grid-template-columns: 56px 1fr auto auto; }
  .date-tile { display: grid; place-items: center; align-content: center; width: 56px; height: 56px; border-radius: var(--sk-r-tile); background: var(--sk-ground); }
  .date-tile small { font-size: 12px; font-weight: 600; color: var(--sk-muted); line-height: 1; }
  .date-tile b { font: 800 19px/1.1 var(--sk-fd); }
  .row-main { display: grid; gap: 2px; min-width: 0; }
  .row-title { font: 700 17px/1.2 var(--sk-fd); overflow-wrap: anywhere; }
  .row-when { font-size: 13px; color: var(--sk-muted); }
  .row-price { min-width: 92px; text-align: right; font: 700 16px/1 var(--sk-fd); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .row-actions { display: flex; gap: 8px; }
  .none { padding: 16px 0; }
  .load { justify-self: center; }
  .empty-actions { display: flex; flex-wrap: wrap; gap: 10px; }
  .wait { display: grid; gap: 6px; }
  @media (max-width: 760px) {
    .jobs { padding: 28px 16px 40px; gap: 18px; }
    .jobs-title { font-size: 32px; }
    .row { flex-direction: column; align-items: stretch; gap: 0; padding: 0; }
    .row-link, .row:has(.row-actions) .row-link { grid-template-columns: 48px 1fr auto; gap: 8px 14px; padding: 16px; }
    .date-tile { width: 48px; height: 48px; }
    .row-tag { grid-column: 2; }
    .row-price { grid-column: 3; grid-row: 2; min-width: 0; }
    .row-link :global(.row-chev) { grid-column: 3; grid-row: 1; justify-self: end; }
    .row-actions { padding: 0 16px 16px; }
    .row-actions .btn { flex: 1; }
  }
  .asking { display: flex; align-items: center; gap: 6px; margin: 0; padding: 0 16px 12px; font-size: 13px; font-weight: 600; }
</style>
