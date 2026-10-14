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
  function itemLabel(item: WorkspaceSummary) {
    return item.title === 'New request' ? t('Start with a message') : ui(item).line;
  }
</script>

{#if items.length || canCreate}
  <nav class="workspace-navigation" aria-label={t('Your service requests')}>
    {#if canCreate}<button class="request-create" disabled={busy} onclick={onCreate}><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg> {t('New request')}</button>{/if}
    <div class="workspace-navigation-list">
      {#each sorted as item (item.id)}
        <button class:current={item.id === currentID} class="request-tab" disabled={busy} aria-current={item.id === currentID ? 'page' : undefined} onclick={() => { if (item.id !== currentID) onSelect(item.id); }}>
          <span class="request-tab-top"><span class="request-tab-title">{item.title === 'New request' ? t('Draft request') : item.title}</span><span class="pill pill--{ui(item).pill}">{ui(item).pillLabel}</span></span>
          <small>{itemLabel(item)}</small>
        </button>
      {/each}
    </div>
    {#if canLoadMore}<button class="text-button" disabled={busy} onclick={onLoadMore}>{t('Show older requests')}</button>{/if}
  </nav>
{/if}
