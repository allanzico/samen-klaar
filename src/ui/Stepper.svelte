<script lang="ts">
  import Icon from './Icon.svelte';
  import { JOURNEY, type JourneyStep } from '../lib/status-ui';
  import { t } from '../lib/i18n.svelte';

  // One six-step journey everywhere: full under request headers, strip on phones, compact in cards.
  let { step, variant = 'full', aside = '', onNavy = false }: { step: JourneyStep; variant?: 'full' | 'strip' | 'compact'; aside?: string; onNavy?: boolean } = $props();

  const current = $derived(step === 'done' ? 7 : step ?? 0);
  const stepName = $derived(step && step !== 'done' ? t(JOURNEY[step - 1]) : '');
  const stripTitles = ['Describe the job', 'Choose a professional', 'Agree on the plan', 'Do the job', 'Check the work', 'Pay'];
  const label = $derived(step === 'done' ? t('Klaar · all 6 steps done') : step ? t('Progress: step {n} of 6, {step}', { n: step, step: stepName }) : '');
  const state = (index: number) => (index + 1 < current ? 'done' : index + 1 === current ? 'current' : 'todo');
</script>

{#if step}
  {#if variant === 'compact'}
    <span class="compact" class:on-navy={onNavy} role="img" aria-label={label}>
      <span class="compact-tag" aria-hidden="true">{step === 'done' ? t('Klaar · all 6 steps done') : t('Step {n} of 6 · {step}', { n: step, step: stepName })}</span>
      <span class="segments mini" aria-hidden="true">{#each JOURNEY as _, index}<span class="seg {state(index)}"></span>{/each}</span>
    </span>
  {:else if variant === 'strip'}
    <div class="strip" role="img" aria-label={label}>
      <span class="segments" aria-hidden="true">{#each JOURNEY as _, index}<span class="seg {state(index)}"></span>{/each}</span>
      <span class="strip-row" aria-hidden="true">
        <b>{step === 'done' ? t('Klaar') : t(stripTitles[step - 1])}</b>
        <span>{aside || (step === 'done' ? t('All 6 steps done') : t('Step {n} of 6', { n: step }))}</span>
      </span>
    </div>
  {:else}
    <div class="full" role="img" aria-label={label}>
      <span class="segments" aria-hidden="true">{#each JOURNEY as _, index}<span class="seg {state(index)}"></span>{/each}</span>
      <span class="labels" aria-hidden="true">
        {#each JOURNEY as name, index}
          <span class="label {state(index)}">{#if state(index) === 'done'}<Icon name="check" size={12}/>{/if}{t(name)}</span>
        {/each}
      </span>
    </div>
  {/if}
{/if}

<style>
  .segments { display: flex; gap: 4px; }
  .seg { flex: 1; height: 6px; border-radius: var(--sk-r-seg); background: var(--sk-line); }
  .seg.done { background: var(--sk-brand); }
  .seg.current { background: var(--sk-signal); }

  .full { display: grid; gap: 8px; }
  .labels { display: grid; grid-template-columns: repeat(6, 1fr); gap: 4px; }
  .label { display: inline-flex; align-items: center; gap: 4px; min-width: 0; font-size: 12px; font-weight: 500; color: var(--sk-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .label.done { color: var(--sk-ink); font-weight: 600; }
  .label.current { color: var(--sk-ink); font: 800 13px/1.2 var(--sk-fd); }
  @media (max-width: 759px) { .label:not(.current) { visibility: hidden; } .label.current { overflow: visible; } }

  .strip { display: grid; gap: 8px; }
  .strip .seg { height: 5px; }
  .strip-row { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; }
  .strip-row b { font: 700 14px/1.2 var(--sk-fd); }
  .strip-row span { font-size: 12px; color: var(--sk-muted); white-space: nowrap; }

  .compact { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 10px; }
  .compact-tag { padding: 4px 10px; border-radius: var(--sk-r-pill); background: var(--sk-ground); font-size: 12px; font-weight: 700; white-space: nowrap; }
  .mini { gap: 3px; }
  .mini .seg { flex: 0 0 22px; height: 5px; }
  .compact.on-navy .compact-tag { background: var(--sk-brand-2); color: var(--sk-on-brand); }
  .compact.on-navy .seg { background: var(--sk-brand-2); }
  .compact.on-navy .seg.done { background: var(--sk-on-brand-2); }
  .compact.on-navy .seg.current { background: var(--sk-signal); }
</style>
