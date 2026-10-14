<script lang="ts">
  import type { AppState } from '../../lib/types';
  import type { StepButton, StepView } from '../../lib/step-view';
  import { t } from '../../lib/i18n.svelte';

  let { app, view, placement }: { app: AppState; view: StepView | null; placement: 'panel' | 'dock' } = $props();
  const eyebrow = $derived(view?.kind === 'your-turn' ? t('Your turn') : view?.kind === 'done' ? t('Klaar') : t('Next step'));
</script>

{#snippet button(btn: StepButton, kind: 'primary' | 'secondary' | 'quiet' | 'danger')}
  {#if btn.href}
    <a class="next-step-button button-link" class:secondary={kind === 'secondary'} class:text-button={kind === 'quiet'} href={btn.href} target="_blank" rel="noopener noreferrer">{btn.label}</a>
  {:else}
    <button class:primary={kind === 'primary'} class:secondary={kind === 'secondary'} class:text-button={kind === 'quiet'} class:danger={kind === 'danger'} class="next-step-button" disabled={app.pending || btn.disabled} aria-busy={kind === 'primary' && app.pending && !btn.disabled ? 'true' : undefined} onclick={() => { void btn.run?.(); }}>{btn.label}</button>
  {/if}
  {#if btn.reason}<p class="reason">{btn.reason}</p>{/if}
{/snippet}

{#if view}
  <section class="next-step {view.kind} next-step-{placement}" aria-label={t('Next step')}>
    {#if view.banner}<div class="banner {view.banner.kind}" role={view.banner.kind === 'error' ? 'alert' : undefined}>{#if view.banner.title}<b>{view.banner.title}</b>{/if}<span>{view.banner.text}</span></div>{/if}
    <p class="eyebrow next-step-eyebrow">{eyebrow}</p>
    <h2 class="next-step-title">{view.title}</h2>
    {#if view.amount}<p class="next-step-amount">{view.amount}</p>{/if}
    {#if view.text}<p class="next-step-text">{view.text}</p>{/if}
    {#if view.ledger}<dl class="ledger">{#each view.ledger as [label, value], index}<div class:total-row={index === view.ledger.length - 1}><dt>{label}</dt><dd>{value}</dd></div>{/each}</dl>{/if}
    {#if view.list}
      <p class="eyebrow next-step-after">{view.list.title}</p>
      <ul class="proof-list">{#each view.list.items as item}<li><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10"/></svg>{item}</li>{/each}</ul>
    {/if}
    {#if view.steps}
      <p class="eyebrow next-step-after">{t('After that')}</p>
      <ol class="after-steps">{#each view.steps as step, index}<li class={step.state}><span class="step-dot" aria-hidden="true">{#if step.state === 'done'}<svg class="icon" viewBox="0 0 24 24"><path d="m5 12 5 5 9-10"/></svg>{:else}{index + 1}{/if}</span>{step.text}</li>{/each}</ol>
    {/if}
    <div class="next-step-actions">
      {#if view.primary}{@render button(view.primary, 'primary')}{/if}
      {#if view.secondary}{@render button(view.secondary, 'secondary')}{/if}
      {#if view.helper}<p class="helper">{view.helper}</p>{/if}
      {#if view.quiet?.length}<div class="next-step-quiet">{#each view.quiet as btn}{@render button(btn, 'quiet')}{/each}</div>{/if}
      {#if view.danger}{@render button(view.danger, 'danger')}{/if}
    </div>
  </section>
{/if}
