<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import { t } from '../lib/i18n.svelte';

  // Info, attention, problem and success banners. Problems are alerts; the rest are status.
  let { kind = 'info', title, text, onDismiss, children }: { kind?: 'info' | 'warn' | 'error' | 'success'; title?: string; text?: string; onDismiss?: () => void; children?: Snippet } = $props();
</script>

<div class="banner banner--{kind}" role={kind === 'error' ? 'alert' : 'status'}>
  <Icon name={kind === 'success' ? 'check-circle' : kind === 'info' ? 'info' : 'alert'}/>
  <div>
    {#if title}<b>{title}</b>{/if}
    {#if text}<p>{text}</p>{/if}
    {@render children?.()}
  </div>
  {#if onDismiss}<button type="button" class="icon-btn" aria-label={t('Dismiss')} onclick={onDismiss}><Icon name="close"/></button>{/if}
</div>
