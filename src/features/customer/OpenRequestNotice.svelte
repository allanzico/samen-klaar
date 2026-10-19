<script lang="ts">
  import Banner from '../../ui/Banner.svelte';
  import type { AppState } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  // A customer can have a few open requests at once, since each keeps an assistant working.
  // One short of the cap this warns; at the cap it explains the block and what frees a place.
  let { app, onShowRequests, warn = true }: { app: AppState; onShowRequests?: () => void; warn?: boolean } = $props();
  const open = $derived(app.openRequests ?? 0);
  const limit = $derived(app.openRequestLimit);
</script>

{#if app.role === 'customer' && app.openRequests !== null}
  {#if open >= limit}
    <Banner kind="warn" title={t('You have {count} open requests, the most you can have at once', { count: open })}>
      <p>{t('Finish, cancel or delete one before you start another. A request stops counting once you approve its plan.')}</p>
      {#if onShowRequests}<button type="button" class="btn btn-quiet notice-link" onclick={onShowRequests}>{t('Go to your requests')}</button>{/if}
    </Banner>
  {:else if warn && open === limit - 1}
    <Banner kind="info" text={t('You have {count} open requests. You can have {limit} at once, so this will be your last until you finish, cancel or delete one.', { count: open, limit })}/>
  {/if}
{/if}

<style>
  .notice-link { margin-top: 6px; padding: 0; min-height: 0; text-decoration: underline; }
</style>
