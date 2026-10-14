<script lang="ts">
  import type { AppActions, AppState, Job } from '../../lib/types';
  import { dayAndTime } from '../../lib/plan-ui.svelte';
  import { t } from '../../lib/i18n.svelte';
  let { app, actions, job, money }: { app: AppState; actions: AppActions; job: Job; money: (minor: number) => string } = $props();
  let other = $derived(app.role === 'customer' ? job.professional : job.customer);
  // Customers answer in the next-step panel; professionals keep their buttons on the card until their own phase.
  let inline = $derived(app.role !== 'customer');
</script>

{#snippet answerHint()}
  <p class="answer-hint">{t('Answer on the right')} <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg></p>
{/snippet}

{#if !job.workStarted && !job.paid && !app.serverState?.cancelled}
  {#if job.priceProposal?.status === 'pending'}
    {@const mine = job.priceProposal.actor === app.role}
    <section class="offer-card" class:awaiting-you={!mine} class:awaiting-them={mine} aria-label="Price offer">
      <div class="offer-head"><h2>{mine ? t('Your price offer') : t('{person} proposed a price', { person: other })}</h2><span class="pill {mine ? 'pill--waiting' : 'pill--your-turn'}">{mine ? t('Waiting on {name}', { name: other }) : t('Your turn')}</span></div>
      <p class="offer-value">{money(job.priceProposal.amountMinor)} {t('total')}{#if mine} <s class="offer-old">{money(job.quote.totalMinor)}</s>{/if}</p>
      {#if job.priceProposal.note}<blockquote class="offer-note">{job.priceProposal.note}</blockquote>{/if}
      {#if !inline}{#if !mine}{@render answerHint()}{/if}
      {:else if job.priceProposal.actor !== app.role}<div class="actions"><button class="primary" disabled={app.pending} onclick={() => actions.act('accept-price', '', undefined, undefined, {proposalId:job.priceProposal?.id})}>{t('Accept this price')}</button><button disabled={app.pending} onclick={() => actions.act('decline-price', '', undefined, undefined, {proposalId:job.priceProposal?.id})}>{t('Decline offer')}</button><button disabled={app.pending} onclick={() => actions.openProposal('price')}>{t('Counter with a price')}</button></div>
      {:else}<p class="quiet-note">{t('Waiting for {person}. The current price stays unchanged until accepted.', { person: other })}</p><div class="actions"><button disabled={app.pending} onclick={() => actions.openProposal('price')}>{t('Change my offer')}</button><button disabled={app.pending} onclick={() => actions.act('withdraw-price', '', undefined, undefined, {proposalId:job.priceProposal?.id})}>{t('Withdraw offer')}</button></div>{/if}
    </section>
  {/if}
  {#if job.dateProposal?.status === 'pending'}
    {@const mine = job.dateProposal.actor === app.role}
    <section class="offer-card" class:awaiting-you={!mine} class:awaiting-them={mine} aria-label="Visit proposal">
      <div class="offer-head"><h2>{mine ? t('Your proposed visit') : t('{person} proposed a visit', { person: other })}</h2><span class="pill {mine ? 'pill--waiting' : 'pill--your-turn'}">{mine ? t('Waiting on {name}', { name: other }) : t('Your turn')}</span></div>
      <p class="offer-value">{dayAndTime(job.dateProposal.appointment.startUtc, job.dateProposal.appointment.endUtc, job.dateProposal.appointment.timezone)}</p>
      {#if job.dateProposal.note}<blockquote class="offer-note">{job.dateProposal.note}</blockquote>{/if}
      {#if !inline}{#if !mine}{@render answerHint()}{/if}
      {:else if job.dateProposal.actor !== app.role}<p class="quiet-note">{t('Accepting reserves this visit at the agreed price.')}</p><div class="actions"><button class="primary" disabled={app.pending} onclick={() => actions.act('accept-date', '', undefined, undefined, {proposalId:job.dateProposal?.id})}>{t('Accept and book this visit')}</button><button disabled={app.pending} onclick={() => actions.act('decline-date', '', undefined, undefined, {proposalId:job.dateProposal?.id})}>{t('Decline visit')}</button><button disabled={app.pending} onclick={() => actions.openProposal('date')}>{t('Suggest another time')}</button></div>
      {:else}<p class="quiet-note">{t('Waiting for {person}. This time is not reserved.', { person: other })}</p><div class="actions"><button disabled={app.pending} onclick={() => actions.openProposal('date')}>{t('Change my proposed visit')}</button><button disabled={app.pending} onclick={() => actions.act('withdraw-date', '', undefined, undefined, {proposalId:job.dateProposal?.id})}>{t('Withdraw proposal')}</button></div>{/if}
    </section>
  {/if}
{/if}
