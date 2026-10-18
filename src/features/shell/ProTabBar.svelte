<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import type { IconName } from '../../ui/icons';
  import { t } from '../../lib/i18n.svelte';

  // Phone tab bar for professionals.
  let { items }: { items: { label: string; icon: IconName; current: boolean; badge?: number; onclick: () => void }[] } = $props();
</script>

<nav class="tabbar" aria-label={t('Professional navigation')}>
  {#each items as item (item.label)}
    <button type="button" aria-current={item.current ? 'page' : undefined} onclick={item.onclick}>
      <span class="pillbox"><Icon name={item.icon}/>{#if item.badge}<span class="badge">{item.badge}<span class="visually-hidden"> {t('need you')}</span></span>{/if}</span>
      <span class="label">{item.label}</span>
    </button>
  {/each}
</nav>

<style>
  .tabbar { display: none; }
  @media (max-width: 879px) {
    .tabbar { position: sticky; bottom: 0; z-index: 25; display: grid; grid-template-columns: repeat(3, 1fr); padding: 6px 8px calc(10px + env(safe-area-inset-bottom)); background: var(--sk-surface); border-top: 0.5px solid var(--sk-line); }
    button { display: grid; justify-items: center; gap: 4px; min-height: 52px; padding: 2px 0; border: 0; background: transparent; color: var(--sk-muted); }
    .pillbox { position: relative; display: grid; place-items: center; width: 56px; height: 30px; border-radius: var(--sk-r-pill); }
    [aria-current="page"] .pillbox { background: var(--sk-soft); color: var(--sk-ink); }
    .label { font-size: 12px; font-weight: 500; }
    [aria-current="page"] .label { color: var(--sk-ink); font-weight: 600; }
    .badge { position: absolute; top: -4px; right: 2px; min-width: 18px; height: 18px; padding: 0 5px; border-radius: var(--sk-r-pill); background: var(--sk-turn-soft); color: var(--sk-on-turn-soft); box-shadow: 0 0 0 2px var(--sk-surface), inset 0 0 0 1px var(--sk-turn-line); font-size: 11px; font-weight: 700; line-height: 18px; }
  }
</style>
