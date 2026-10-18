<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import type { WorkspaceRole, WorkspaceSummary } from '../../lib/types';
  import { firstName, listFilter, otherFromLabel, statusUi } from '../../lib/status-ui';
  import { dayTime, money } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';

  // The navy rail on web request and job pages. Below 1024 px the list page replaces it.
  let { items, currentID, role, busy, canLoadMore, onSelect, onCreate, onLoadMore }: {
    items: WorkspaceSummary[]; currentID: string | null; role: WorkspaceRole; busy: boolean; canLoadMore: boolean;
    onSelect: (id: string) => void; onCreate?: () => void; onLoadMore: () => void;
  } = $props();

  let filter = $state<'all' | 'new' | 'active' | 'finished'>('all');
  const ui = (item: WorkspaceSummary) => statusUi(item.status, role, firstName(item.counterpart) || otherFromLabel(item.statusLabel));
  // The rail line: a booked visit shows its time, everything else the next step in words.
  const line = (item: WorkspaceSummary) => {
    const status = ui(item);
    if ((status.pill === 'booked' || item.status === 'confirm_visit') && item.visitStartUtc) return dayTime(item.visitStartUtc, item.visitTimezone);
    return status.line;
  };
  const sorted = $derived([...items].sort((a, b) => ui(a).rank - ui(b).rank || (b.updatedAt ?? '').localeCompare(a.updatedAt ?? '')));
  const groups = $derived.by(() => {
    if (role !== 'professional') return [{ key: 'all', title: '', items: sorted }];
    return (['new', 'active', 'finished'] as const)
      .filter((key) => filter === 'all' || filter === key)
      .map((key) => ({ key, title: t(key === 'new' ? 'New' : key === 'active' ? 'Active' : 'Finished'), items: sorted.filter((item) => listFilter(item.status, role) === key) }))
      .filter((group) => group.items.length);
  });
  const title = (item: WorkspaceSummary) => (item.title === 'New request' ? t('New request') : item.title);
</script>

<nav class="rail on-navy" aria-label={t(role === 'professional' ? 'Your jobs' : 'Your requests')}>
  {#if onCreate}<button type="button" class="btn btn-on-navy btn-block" disabled={busy} onclick={onCreate}><Icon name="plus"/>{t('New request')}</button>{/if}
  {#if role === 'professional'}
    <div class="rail-filter" role="tablist" aria-label={t('Filter jobs')}>
      {#each [['all', 'All'], ['new', 'New'], ['active', 'Active'], ['finished', 'Finished']] as [key, label] (key)}
        <button type="button" role="tab" aria-selected={filter === key} onclick={() => { filter = key as typeof filter; }}>{t(label)}</button>
      {/each}
    </div>
  {/if}
  {#each groups as group (group.key)}
    {#if group.title}<p class="group-title">{group.title}</p>{/if}
    <ul class="items">
      {#each group.items as item (item.id)}
        {@const status = ui(item)}
        <li>
          <button type="button" class="item" aria-current={item.id === currentID ? 'page' : undefined} disabled={busy && item.id !== currentID} onclick={() => { if (item.id !== currentID) onSelect(item.id); }}>
            <span class="item-top"><span class="item-title">{title(item)}</span>{#if role === 'professional' && item.totalMinor}<span class="item-price">{money(item.totalMinor, item.currency)}</span>{/if}</span>
            <span class="item-line {status.pill}">
              {#if status.pill === 'your-turn'}<span class="glyph turn" aria-hidden="true"></span>
              {:else if status.pill === 'booked'}<Icon name="calendar" size={14}/>
              {:else if status.pill === 'done' || status.icon === 'check'}<Icon name="check" size={14}/>
              {:else}<span class="glyph hollow" aria-hidden="true"></span>{/if}
              <span>{line(item)}</span>
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/each}
  {#if canLoadMore}<button type="button" class="btn btn-sm btn-outline-navy" disabled={busy} onclick={onLoadMore}>{t('Show older requests')}</button>{/if}
</nav>

<style>
  .rail { display: flex; flex-direction: column; gap: 14px; width: 248px; flex: 0 0 248px; padding: 20px 14px; background: var(--sk-brand); color: var(--sk-on-brand); overflow-y: auto; }
  .rail-filter { display: flex; padding: 3px; border-radius: var(--sk-r-pill); background: var(--sk-brand-3); }
  .rail-filter button { flex: 1; min-height: 32px; padding: 0 6px; border: 0; border-radius: var(--sk-r-pill); background: transparent; color: var(--sk-on-brand-2); font: 600 12px/1 var(--sk-ft); }
  .rail-filter button[aria-selected="true"] { background: var(--sk-on-brand); color: var(--sk-brand); font-weight: 700; }
  .group-title { margin: 4px 0 -6px; padding: 0 14px; font-size: 12px; font-weight: 600; color: var(--sk-on-brand-2); }
  .items { display: grid; gap: 4px; }
  .item { display: grid; gap: 4px; width: 100%; padding: 12px 14px; border: 0; border-radius: var(--sk-r-card); background: transparent; color: var(--sk-on-brand); text-align: left; }
  .item:hover:not(:disabled) { background: color-mix(in srgb, var(--sk-brand-2) 60%, transparent); }
  .item[aria-current="page"] { background: var(--sk-brand-2); }
  .item:disabled { cursor: default; }
  .item-top { display: flex; justify-content: space-between; gap: 8px; }
  .item-price { font-size: 13px; font-weight: 600; color: var(--sk-on-brand-2); white-space: nowrap; font-variant-numeric: tabular-nums; }
  .item-title { font-size: 14px; font-weight: 500; line-height: 1.3; overflow-wrap: anywhere; }
  .item[aria-current="page"] .item-title { font-weight: 600; }
  .item-line { display: flex; align-items: center; gap: 7px; font-size: 13px; color: var(--sk-on-brand-2); line-height: 1.3; }
  .item-line.your-turn { color: var(--sk-turn-on-brand); font-weight: 600; }
  .item-line.booked { color: var(--sk-signal); }
  .glyph { width: 8px; height: 8px; flex: 0 0 auto; border-radius: 50%; }
  .glyph.turn { background: var(--sk-turn-on-brand); }
  .glyph.hollow { box-shadow: inset 0 0 0 1.5px var(--sk-on-brand-2); }
</style>
