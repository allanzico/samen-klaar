<script lang="ts">
  import { t } from '../../lib/i18n.svelte';
  // Handoff 6.10: two or three rows, the current one ringed.
  let { items, title = 'After this' }: { items: string[]; title?: string } = $props();
</script>

{#if items.length}
  <section class="after" aria-label={t(title)}>
    <p class="eyebrow">{t(title)}</p>
    <ol>
      {#each items as item, index}
        <li class:current={index === 0}><span class="mark" aria-hidden="true">{#if index > 0}{index + 1}{/if}</span><span>{item}</span></li>
      {/each}
    </ol>
  </section>
{/if}

<style>
  .after { display: grid; gap: 10px; }
  ol { display: grid; gap: 12px; }
  li { display: grid; grid-template-columns: 26px 1fr; gap: 12px; align-items: center; font-size: 14px; }
  li.current { font-weight: 600; }
  .mark { display: grid; place-items: center; width: 26px; height: 26px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--sk-field); font-size: 12px; font-weight: 600; }
  .current .mark { box-shadow: inset 0 0 0 2px var(--sk-ink); }
  .current .mark::after { content: ""; width: 10px; height: 10px; border-radius: 50%; background: var(--sk-ink); }
</style>
