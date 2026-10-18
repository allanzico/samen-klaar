<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import { agreementSummary, agreementTerms } from '../../lib/agreement';
  import type { Job, WorkspaceRole } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  // Handoff 6.9: rows in the web aside, a four-column strip on plan pages, a collapsible bar on phones.
  let { job, role, variant }: { job: Job; role: WorkspaceRole | null; variant: 'rows' | 'strip' | 'bar' } = $props();
  const terms = $derived(agreementTerms(job, role));
  let open = $state(false);
  const allAgreed = $derived(terms.filter((term) => term.key === 'price' || term.key === 'visit').every((term) => term.agreed));
</script>

{#snippet rows()}
  <ul class="rows-list">
    {#each terms as term (term.key)}
      <li><Icon name={term.icon}/><span><small>{term.label}</small><b>{term.value}</b></span></li>
    {/each}
  </ul>
{/snippet}

{#if variant === 'rows'}
  <section class="agreement-rows" aria-labelledby="agreement-eyebrow">
    <p class="eyebrow" id="agreement-eyebrow">{t('Agreement')}</p>
    {@render rows()}
  </section>
{:else if variant === 'strip'}
  <dl class="strip">
    {#each terms.filter((term) => term.key !== 'extra') as term (term.key)}
      <div><dt><Icon name={term.icon} size={13}/>{term.label}</dt><dd>{term.value}</dd></div>
    {/each}
  </dl>
{:else}
  <div class="bar">
    <button type="button" class="bar-toggle" aria-expanded={open} aria-controls="agreement-bar-rows" onclick={() => { open = !open; }}>
      <Icon name={allAgreed ? 'check' : 'clock'}/>
      <span class="bar-summary">{agreementSummary(job, role)}</span>
      <span class="bar-state">{allAgreed ? t('Agreed') : t('In progress')}</span>
      <Icon name={open ? 'chevron-up' : 'chevron-down'}/>
    </button>
    <div id="agreement-bar-rows" class="bar-rows" hidden={!open}>{@render rows()}</div>
  </div>
{/if}

<style>
  .agreement-rows { display: grid; gap: 4px; }
  .rows-list li { display: grid; grid-template-columns: 20px 1fr; gap: 10px; padding: 10px 0; }
  .rows-list li + li { border-top: 0.5px solid var(--sk-line); }
  .rows-list :global(.icon) { margin-top: 2px; }
  .rows-list small { display: block; font-size: 12px; color: var(--sk-muted); }
  .rows-list b { display: block; font-size: 14px; font-weight: 600; overflow-wrap: anywhere; }

  .strip { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); padding: 12px 0; border-radius: var(--sk-r-card); background: var(--sk-ground); }
  .strip > div { display: grid; gap: 2px; padding: 0 16px; min-width: 0; }
  .strip > div + div { border-left: 1px solid var(--sk-line); }
  .strip dt { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--sk-muted); }
  .strip dd { font-size: 14px; font-weight: 600; overflow-wrap: anywhere; }
  @media (max-width: 899px) {
    .strip { grid-template-columns: repeat(2, minmax(0, 1fr)); row-gap: 12px; }
    .strip > div:nth-child(3) { border-left: 0; }
  }

  .bar { background: var(--sk-surface); border-bottom: 0.5px solid var(--sk-line); }
  .bar-toggle { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 40px; padding: 0 16px; border: 0; background: transparent; color: var(--sk-ink); text-align: left; }
  .bar-summary { flex: 1; min-width: 0; font-size: 13px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .bar-state { font-size: 12px; color: var(--sk-muted); }
  .bar-rows { padding: 0 16px 8px; }
</style>
