<script lang="ts">
  import type { Snippet } from 'svelte';
  import Pill from '../../ui/Pill.svelte';
  import Banner from '../../ui/Banner.svelte';
  import type { StepButton, StepView } from '../../lib/step-view';
  import { t } from '../../lib/i18n.svelte';

  // Renders one next step. "aside" is the web column; "dock" sits above the composer
  // below 1280 px and on phones, where it becomes the turn card.
  let { view, pending, placement, extra, demoted = false, display = 'all' }: { view: StepView; pending: boolean; placement: 'aside' | 'dock'; extra?: Snippet<[string]>; demoted?: boolean; display?: 'all' | 'actions' | 'details' } = $props();
  const headingId = $derived(`next-step-${placement}-${display}`);
</script>

{#snippet button(btn: StepButton, style: 'main' | 'alt' | 'quiet')}
  {#if btn.href}
    <a class="btn {style === 'main' ? 'btn-turn' : style === 'alt' ? 'btn-outline' : 'btn-quiet'}" class:btn-block={style !== 'quiet'} href={btn.href} target="_blank" rel="noopener noreferrer">{btn.label}</a>
  {:else}
    <button type="button" class="btn {style === 'main' ? (demoted ? 'btn-outline' : view.kind === 'your-turn' ? 'btn-turn' : 'btn-primary') : style === 'alt' ? 'btn-outline' : 'btn-quiet'}" class:btn-block={style !== 'quiet'}
      disabled={pending || btn.disabled} aria-busy={style === 'main' && pending && !btn.disabled ? 'true' : undefined}
      aria-describedby={btn.reason ? `${headingId}-${style}-reason` : undefined} onclick={() => { void btn.run?.(); }}>{btn.label}</button>
  {/if}
  {#if btn.reason}<p class="reason" id="{headingId}-{style}-reason">{btn.reason}</p>{/if}
{/snippet}

<section class="next-step {view.kind} {placement}" class:turn-panel={view.kind === 'your-turn'} class:actions-only={display === 'actions'} aria-labelledby={headingId}>
  {#if display !== 'actions'}
  {#if view.banner}<Banner kind={view.banner.kind} title={view.banner.title} text={view.banner.text}/>{/if}
  {#if demoted}<p class="helper demoted">{t('Your assistant is asking you this too. Either way works.')}</p>{/if}
  <div class="head">
    {#if display !== 'details'}<Pill variant={view.tag.variant} label={view.tag.label}/>{/if}
    <h2 id={headingId}>{view.title}</h2>
    {#if view.amount}<p class="amount step-amount" class:when={!/^[+€$£]|^\d/.test(view.amount.trim())}>{view.amount}</p>{/if}
    {#if view.text}<p class="step-text">{view.text}</p>{/if}
  </div>
  {#if view.ledger}
    <dl class="step-ledger">
      {#each view.ledger as row (row.label)}<div class:total={row.total}><dt>{row.label}</dt><dd>{row.value}</dd></div>{/each}
    </dl>
  {/if}
  {#if view.list}
    <div class="proof">
      <p class="eyebrow">{view.list.title}</p>
      <ul>{#each view.list.items as item}<li>{item}</li>{/each}</ul>
    </div>
  {/if}
  {@render extra?.(placement)}
  {/if}
  {#if display === 'details' && view.helper}<p class="helper">{view.helper}</p>{/if}
  {#if display === 'actions'}<div class="action-label"><Pill variant={view.tag.variant} label={view.tag.label}/><h2 id={headingId} class="sr-only">{view.title}</h2></div>{/if}
  {#if display !== 'details' && (view.primary || view.secondary || view.quiet?.length || view.helper || view.danger)}
    <div class="actions">
      {#if view.primary}<div>{@render button(view.primary, 'main')}</div>{/if}
      {#if view.secondary}<div>{@render button(view.secondary, 'alt')}</div>{/if}
      {#if view.helper && display !== 'actions'}<p class="helper">{view.helper}</p>{/if}
      {#if view.quiet?.length}<div class="quiet">{#each view.quiet as btn (btn.label)}<div>{@render button(btn, 'quiet')}</div>{/each}</div>{/if}
      {#if view.danger}<div><button type="button" class="btn btn-danger-link" disabled={pending || view.danger.disabled} onclick={() => { void view.danger?.run?.(); }}>{view.danger.label}</button></div>{/if}
    </div>
  {/if}
  {#if display !== 'actions' && view.after?.length && placement === 'dock'}
    <p class="dock-after">{t('Next')}: {view.after[0]}</p>
  {/if}
</section>

<style>
  .next-step { display: grid; gap: 14px; padding: 22px; border-radius: var(--sk-r-box); }
  .next-step:not(.turn-panel) { background: var(--sk-surface); border: 1px solid var(--sk-line); }
  .actions-only { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; background: var(--sk-surface); border: 1px solid var(--sk-turn-line); box-shadow: var(--sk-shadow-pop); padding: 10px 14px; }
  .actions-only .actions { display: flex; align-items: center; flex-wrap: wrap; flex: 1; gap: 8px 14px; }
  .actions-only .quiet { display: flex; flex-direction: row; flex-wrap: wrap; gap: 8px; margin: 0; }
  .actions-only .reason { margin-top: 4px; font-size: 12px; max-width: 280px; }
  .actions-only .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  .action-label { flex: 0 0 auto; }
  .head { display: grid; justify-items: start; gap: 8px; }
  h2 { font: 700 21px/1.2 var(--sk-fd); letter-spacing: -0.015em; }
  .step-amount { font-size: 24px; white-space: normal; }
  .step-amount.when { font-size: 19px; line-height: 1.2; }
  .step-text { font-size: 14px; }
  .step-ledger { display: grid; }
  .step-ledger div { display: flex; justify-content: space-between; gap: 12px; padding: 5px 0; font-size: 14px; }
  .step-ledger dd { font-variant-numeric: tabular-nums; white-space: nowrap; }
  .step-ledger .total { align-items: baseline; margin-top: 6px; padding-top: 10px; border-top: 1.5px solid var(--sk-on-signal); font-weight: 600; }
  .step-ledger .total dd { font: 800 24px/1.05 var(--sk-fd); letter-spacing: -0.02em; }
  .proof ul { display: grid; gap: 4px; margin-top: 6px; }
  .proof li { padding-left: 16px; position: relative; font-size: 14px; }
  .proof li::before { content: ""; position: absolute; left: 2px; top: 9px; width: 6px; height: 6px; border-radius: 50%; background: var(--sk-ink); }
  .actions { display: grid; gap: 10px; }
  .quiet { display: flex; flex-direction: column; align-items: flex-start; gap: 0; margin-top: -2px; }
  .helper { font-size: 12px; }
  .turn-panel .helper { color: var(--sk-on-signal); }

  /* Dock: compact card above the composer (below 1280 px) and the phone turn card. */
  .dock { gap: 10px; padding: 14px 16px; }
  .dock h2 { font-size: 17px; }
  .dock .step-amount { font-size: 19px; }
  .dock .step-text { font-size: 13px; }
  .dock:not(.actions-only) .actions { grid-template-columns: repeat(auto-fit, minmax(min(200px, 100%), 1fr)); align-items: start; }
  .dock .helper, .dock .quiet { grid-column: 1 / -1; }
  .dock:not(.actions-only) .quiet { flex-direction: row; flex-wrap: wrap; gap: 4px 12px; }
  .dock-after { font-size: 12px; color: var(--sk-muted); }
</style>
