<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../../lib/api';
  import { money } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';
  import type { ProfessionalDashboard, WorkspaceSummary } from '../../lib/types';

  let { workspaces, currency }: { workspaces: WorkspaceSummary[]; currency: string } = $props();
  let overview = $state<ProfessionalDashboard | null>(null);
  let error = $state(false);
  let loading = $state(false);
  let generation = 0;
  let disposed = false;
  const balances = $derived(overview?.balances.length ? overview.balances : [{ currency, receivedMinor: 0, awaitingPaymentMinor: 0 }]);

  async function load(signal?: AbortSignal) {
    const current = ++generation;
    loading = true;
    try {
      const result = await api.professionalDashboard(signal);
      if (disposed || current !== generation) return;
      overview = result;
      error = false;
    } catch {
      if (!disposed && current === generation && !signal?.aborted) error = true;
    } finally {
      if (!disposed && current === generation) loading = false;
    }
  }
  $effect(() => {
    workspaces;
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  });
  onMount(() => {
    const timer = window.setInterval(() => { if (!document.hidden) void load(); }, 30000);
    return () => { disposed = true; generation++; window.clearInterval(timer); };
  });
</script>

<section class="overview" aria-label={t('Your work at a glance')} aria-busy={loading}>
  {#if error}
    <div class="overview-error" role="status"><p>{t('Your stats could not be refreshed. Your jobs are still available.')}</p><button type="button" class="btn btn-quiet btn-sm" disabled={loading} onclick={() => load()}>{t('Retry')}</button></div>
  {/if}
  {#if overview}
    <dl class="figures">
      <div class="money-stat"><dt>{t('Earnings')}<small>{t('All time, before PayPal fees')}</small></dt><dd>{#each balances as balance (balance.currency)}<span>{money(balance.receivedMinor, balance.currency)}</span>{/each}</dd></div>
      <div class="money-stat"><dt>{t('Awaiting payment')}<small>{t('Approved work, not yet paid')}</small></dt><dd>{#each balances as balance (balance.currency)}<span>{money(balance.awaitingPaymentMinor, balance.currency)}</span>{/each}</dd></div>
      <div><dt>{t('Upcoming visits')}</dt><dd>{overview.upcomingVisits}</dd></div>
      <div><dt>{t('Completed jobs')}</dt><dd>{overview.completedJobs}</dd></div>
      <div><dt>{t('To confirm')}</dt><dd>{overview.awaitingConfirmation}</dd></div>
    </dl>
    <p class="helper">{t('Refunded payments and payments under refund or reversal review are excluded from earnings.')}</p>
  {:else if !error}
    <p class="secondary-text" role="status">{t('Loading your stats…')}</p>
  {/if}
</section>

<style>
  .overview { display: grid; gap: 12px; padding: 20px 0; border-block: 1px solid var(--sk-line); }
  .figures { display: grid; grid-template-columns: 1.5fr 1.5fr repeat(3, 1fr); gap: 20px; margin: 0; }
  .figures > div { min-width: 0; }
  dt { font-size: 13px; font-weight: 600; color: var(--sk-muted); }
  dd { display: grid; gap: 4px; margin: 8px 0 0; font-size: 22px; font-weight: 700; line-height: 1.25; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
  .money-stat small { display: block; margin-top: 6px; font-size: 12px; font-weight: 400; }
  .overview-error { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .overview-error p { margin: 0; font-size: 13px; }
  @media (max-width: 760px) {
    .figures { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px 12px; }
    .money-stat { grid-column: span 3; display: grid; grid-template-columns: 1fr 1fr; column-gap: 12px; align-items: baseline; }
    .money-stat dd { text-align: right; margin: 0; }
    dd { font-size: 20px; }
  }
</style>
