<script lang="ts">
  import type { AppState, Job } from '../../lib/types';
  import { dayAndTime } from '../../lib/plan-ui.svelte';
  import { t } from '../../lib/i18n.svelte';
  let { app, job, money }: { app: AppState; job: Job; money: (minor: number) => string } = $props();
  let other = $derived(app.role === 'customer' ? job.professional : job.customer);
</script>

{#snippet answerHint()}
  <p class="answer-hint">{t('Answer on the right')} <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg></p>
{/snippet}

<!-- Offer cards only describe the offer; the buttons live in the next-step panel. -->
{#if !job.workStarted && !job.paid && !app.serverState?.cancelled}
  {#if job.priceProposal?.status === 'pending'}
    {@const mine = job.priceProposal.actor === app.role}
    <section class="offer-card" class:awaiting-you={!mine} class:awaiting-them={mine} aria-label="Price offer">
      <div class="offer-head"><h2>{mine ? t('Your price offer') : t('{person} proposed a price', { person: other })}</h2><span class="pill {mine ? 'pill--waiting' : 'pill--your-turn'}">{mine ? t('Waiting on {name}', { name: other }) : t('Your turn')}</span></div>
      <p class="offer-value">{money(job.priceProposal.amountMinor)} {t('total')}{#if mine} <s class="offer-old">{money(job.quote.totalMinor)}</s>{/if}</p>
      {#if job.priceProposal.note}<blockquote class="offer-note">{job.priceProposal.note}</blockquote>{/if}
      {#if !mine}{@render answerHint()}{/if}
    </section>
  {/if}
  {#if job.dateProposal?.status === 'pending'}
    {@const mine = job.dateProposal.actor === app.role}
    <section class="offer-card" class:awaiting-you={!mine} class:awaiting-them={mine} aria-label="Visit proposal">
      <div class="offer-head"><h2>{mine ? t('Your proposed visit') : t('{person} proposed a visit', { person: other })}</h2><span class="pill {mine ? 'pill--waiting' : 'pill--your-turn'}">{mine ? t('Waiting on {name}', { name: other }) : t('Your turn')}</span></div>
      <p class="offer-value">{dayAndTime(job.dateProposal.appointment.startUtc, job.dateProposal.appointment.endUtc, job.dateProposal.appointment.timezone)}</p>
      {#if job.dateProposal.note}<blockquote class="offer-note">{job.dateProposal.note}</blockquote>{/if}
      {#if !mine}{@render answerHint()}{/if}
    </section>
  {/if}
{/if}
