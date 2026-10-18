<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import Stepper from '../../ui/Stepper.svelte';
  import type { WorkspaceSummary } from '../../lib/types';
  import { firstName, otherFromLabel, statusUi } from '../../lib/status-ui';
  import { dayMonth, dayTime, money, shortDay } from '../../lib/format';
  import { agentUi } from '../../lib/agent.svelte';
  import { tradeIcon, tradeOf } from '../../lib/trade';
  import { t } from '../../lib/i18n.svelte';

  // Request card: the whole card opens the request.
  let { item, onOpen, compact = false }: { item: WorkspaceSummary; onOpen: (id: string) => void; compact?: boolean } = $props();
  const status = $derived(statusUi(item.status, 'customer', firstName(item.counterpart) || otherFromLabel(item.statusLabel)));
  const title = $derived(item.title === 'New request' ? t('New request') : item.title);
  const line = $derived(status.pill === 'booked' && item.visitStartUtc ? dayTime(item.visitStartUtc, item.visitTimezone) : status.line);
  const asking = $derived(agentUi.escalations.find((e) => e.workspaceId === item.id));
  // Price and visit at a glance, once there is a plan.
  const facts = $derived([item.totalMinor ? money(item.totalMinor, item.currency) : '', item.visitStartUtc ? shortDay(item.visitStartUtc, item.visitTimezone) : ''].filter(Boolean).join(' · '));
  const who = $derived(item.counterpart || (item.jobId ? '' : t('No professional yet')));
</script>

<a class="card-link" class:turn={status.pill === 'your-turn'} class:compact href="/requests/{encodeURIComponent(item.id)}" onclick={(event) => { event.preventDefault(); onOpen(item.id); }}>
  <span class="icon-tile" aria-hidden="true"><Icon name={tradeIcon(tradeOf(item.category))}/></span>
  <span class="body">
    <span class="title">{title}</span>
    {#if !compact}<span class="sub">{[who, line].filter(Boolean).join(' · ')}</span>{/if}
    {#if !compact && status.step}<Stepper step={status.step} variant="compact"/>{/if}
    {#if asking}<span class="asking"><Icon name="bot" size={14}/>{t('{name} is asking you in the chat', { name: asking.params?.agent || t('Your assistant') })}</span>{/if}
  </span>
  <span class="end">
    <Pill variant={status.pill} label={status.pillLabel} icon={status.icon} light={status.pill === 'booked' && compact} row/>
    <span class="date">{facts || dayMonth(item.updatedAt)}</span>
  </span>
  {#if !compact}<Icon name="chevron-right" class="chev"/>{/if}
</a>

<style>
  .card-link { display: grid; grid-template-columns: 56px 1fr auto 20px; gap: 20px; align-items: center; padding: 20px 22px; border-radius: var(--sk-r-box); background: var(--sk-surface); border: 1px solid var(--sk-line); color: var(--sk-ink); text-decoration: none; }
  .card-link:hover { border-color: var(--sk-field); }
  .card-link.turn { background: var(--sk-turn-soft); border-color: transparent; }
  .body { display: grid; gap: 6px; min-width: 0; }
  .title { font: 700 18px/1.2 var(--sk-fd); overflow-wrap: anywhere; }
  .sub { font-size: 14px; color: var(--sk-muted); }
  .asking { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 600; }
  .end { display: grid; justify-items: end; gap: 6px; }
  .date { font-size: 12px; color: var(--sk-muted); }
  .compact { grid-template-columns: 1fr; gap: 12px; padding: 16px; align-content: start; }
  .compact .icon-tile { display: none; }
  .compact .title { font-size: 15px; }
  .compact .end { grid-auto-flow: column; justify-content: space-between; align-items: center; }
  @media (max-width: 639px) {
    .card-link:not(.compact) { grid-template-columns: 48px 1fr 20px; gap: 12px 14px; padding: 16px; }
    .card-link:not(.compact) .icon-tile { width: 48px; height: 48px; }
    .card-link:not(.compact) .body { grid-column: 2; }
    .card-link:not(.compact) .end { grid-column: 1 / -1; grid-row: 2; grid-auto-flow: column; justify-content: space-between; align-items: center; padding-top: 12px; border-top: 0.5px solid var(--sk-line); }
    .card-link :global(.chev) { grid-column: 3; grid-row: 1; }
    .title { font-size: 16px; }
  }
</style>
