<script lang="ts">
  import type { WorkspaceSummary } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';
  import { statusUi } from '../../lib/status-ui';

  let { items = [], currentID = null, canCreate = false, canLoadMore = false, busy = false, onSelect, onCreate, onLoadMore }: {
    items?: WorkspaceSummary[];
    currentID?: string | null;
    canCreate?: boolean;
    canLoadMore?: boolean;
    busy?: boolean;
    onSelect: (id: string) => void;
    onCreate: () => void;
    onLoadMore?: () => void;
  } = $props();

  // Your turn first, then scheduled, waiting and finished. Newest first within a group.
  const sorted = $derived([...items].sort((a, b) => ui(a).rank - ui(b).rank || (b.updatedAt ?? '').localeCompare(a.updatedAt ?? '')));
  const ui = (item: WorkspaceSummary) => statusUi(item.status, item.statusLabel ?? '', item.role);
  // Professionals get their jobs in sections: what needs them, what is coming up, what waits on the customer.
  const grouped = $derived(items.some((item) => item.role === 'professional'));
  const groups = $derived.by(() => {
    if (!grouped) return [{ title: '', items: sorted }];
    const titles: Record<string, string> = { 'your-turn': t('Needs you'), scheduled: t('Upcoming'), waiting: t('Waiting on customer') };
    const result: { title: string; items: WorkspaceSummary[] }[] = [];
    for (const item of sorted) {
      const title = titles[ui(item).pill] ?? t('Other');
      const last = result[result.length - 1];
      if (last && last.title === title) last.items.push(item); else result.push({ title, items: [item] });
    }
    return result;
  });
  function itemLabel(item: WorkspaceSummary) {
    return item.title === 'New request' ? t('Start with a message') : ui(item).line;
  }
</script>

{#if items.length || canCreate}
  <nav class="workspace-navigation" aria-label={t('Your service requests')}>
    {#if canCreate}<button class="request-create" disabled={busy} onclick={onCreate}><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg> {t('New request')}</button>{/if}
    {#each groups as group (group.title)}
    {#if group.title}<p class="eyebrow list-group">{group.title}</p>{/if}
    <div class="workspace-navigation-list">
      {#each group.items as item (item.id)}
        <button class:current={item.id === currentID} class="request-tab" disabled={busy} aria-current={item.id === currentID ? 'page' : undefined} onclick={() => { if (item.id !== currentID) onSelect(item.id); }}>
          <span class="request-tab-top"><span class="request-tab-title">{item.title === 'New request' ? t('Draft request') : item.title}</span><span class="pill pill--{ui(item).pill}">{ui(item).pillLabel}</span></span>
          <small>{itemLabel(item)}</small>
        </button>
      {/each}
    </div>
    {/each}
    {#if canLoadMore}<button class="text-button" disabled={busy} onclick={onLoadMore}>{t('Show older requests')}</button>{/if}
  </nav>
{/if}
