<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Banner from '../../ui/Banner.svelte';
  import type { AppActions, AppState, Intake } from '../../lib/types';
  import { tradeName, tradeOf } from '../../lib/trade';
  import { t } from '../../lib/i18n.svelte';

  // B04–B06, B09, B10: answer chips, the area card, finding professionals and the waitlist.
  let { app, actions, intake }: { app: AppState; actions: AppActions; intake: Intake } = $props();
  let area = $state('');
  let searching = $state(false);
  $effect(() => { if (intake.location?.label && !area) area = intake.location.label; });
  const trade = $derived(tradeOf(intake.category));
  async function setArea(event: SubmitEvent) {
    event.preventDefault();
    searching = true;
    try { await actions.act('set-location', area.trim()); } finally { searching = false; }
  }
</script>

{#if ['detail', 'urgency', 'budget'].includes(intake.step) && intake.options?.length}
  <div class="chips" role="group" aria-label={t('Suggested answers')}>
    {#each intake.options as option, i}<button type="button" class="chip" disabled={app.pending} onclick={() => actions.chooseAnswer(intake.optionIds?.[i] || option)}>{t(option)}</button>{/each}
  </div>
{/if}

{#if searching}
  <section class="finding" aria-live="polite">
    <div class="box job-card">
      <p class="eyebrow">{t('Your job')}</p>
      <p class="strong">{intake.description}</p>
      <div class="facts">{#if intake.category}<span class="pill pill--fact">{t(intake.category)}</span>{/if}<span class="pill pill--fact">{area.trim()}</span>{#if intake.urgency}<span class="pill pill--fact">{t(intake.urgency)}</span>{/if}</div>
    </div>
    <p class="status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Checking {trade} who serve {area}…', { trade: tradeName(trade, true), area: area.trim() })}</p>
    <div class="box skeleton-card" aria-hidden="true">
      <span class="skeleton bar short"></span>
      <div class="sk-person"><span class="skeleton dot"></span><span class="sk-lines"><span class="skeleton bar"></span><span class="skeleton bar short"></span></span></div>
      <div class="sk-facts"><span class="skeleton tile"></span><span class="skeleton tile"></span><span class="skeleton tile"></span></div>
      <span class="skeleton bar button"></span>
    </div>
  </section>
{:else if intake.step === 'location'}
  <form class="box area" onsubmit={setArea}>
    <div class="field">
      <label for="service-area">{t('Neighbourhood or postcode area')}</label>
      <input id="service-area" class="input" bind:value={area} required minlength="2" maxlength="80" autocomplete="off" placeholder={t('For example De Pijp or 1072')} aria-describedby="area-help"/>
      <p id="area-help" class="field-help lock"><Icon name="lock" size={14}/>{t('Address stays private until you choose.')}</p>
    </div>
    <button class="btn btn-primary" disabled={app.pending || area.trim().length < 2}>{t('Find professionals')}</button>
  </form>
{:else if intake.step === 'waitlist'}
  <section class="box waitlist" aria-labelledby="waitlist-title">
    <span class="icon-tile icon-tile-48" aria-hidden="true"><Icon name="pin"/></span>
    <h2 id="waitlist-title" class="section-title">{t('No {trade} covers {area} yet', { trade: tradeName(trade), area: intake.location?.label ?? area })}</h2>
    <p class="secondary-text">{t('We haven’t found a reviewed professional who serves this area. You can save the request for our team. This isn’t a booking, and we can’t promise a match.')}</p>
    {#if intake.waitlisted}
      <Banner kind="success" text={t('Saved. Your request stays in your account.')}/>
    {:else}
      <button type="button" class="btn btn-primary" disabled={app.pending} onclick={() => actions.act('join-waitlist')}>{t('Save to the waitlist')}</button>
    {/if}
    <form class="retry" onsubmit={setArea}>
      <div class="field"><label for="change-area">{t('Try a different area')}</label><input id="change-area" class="input" bind:value={area} required minlength="2" maxlength="80" autocomplete="off"/></div>
      <button class="btn btn-outline" disabled={app.pending || area.trim().length < 2}>{t('Search this area')}</button>
    </form>
  </section>
{/if}

<style>
  .chips { display: flex; flex-wrap: wrap; gap: 8px; max-width: 820px; width: 100%; margin: -6px auto 0; padding-left: 48px; }
  .area { display: grid; gap: 14px; max-width: 560px; width: 100%; margin: 0 auto 0 48px; padding: 20px; }
  .lock { display: flex; align-items: center; gap: 6px; }
  .finding { display: grid; gap: 14px; max-width: 720px; width: 100%; margin: 0 auto; }
  .job-card { display: grid; gap: 8px; padding: 18px 20px; }
  .facts { display: flex; flex-wrap: wrap; gap: 8px; }
  .status { display: flex; align-items: center; gap: 10px; font-weight: 600; }
  .skeleton-card { display: grid; gap: 14px; padding: 22px; }
  .bar { display: block; height: 14px; }
  .bar.short { width: 40%; }
  .bar.button { height: 48px; border-radius: var(--sk-r-pill); }
  .sk-person { display: flex; gap: 12px; align-items: center; }
  .dot { width: 48px; height: 48px; border-radius: 50%; }
  .sk-lines { display: grid; gap: 8px; flex: 1; }
  .sk-facts { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }
  .tile { height: 64px; border-radius: var(--sk-r-tile); }
  .waitlist { display: grid; justify-items: start; gap: 14px; max-width: 720px; width: 100%; margin: 0 auto; padding: 24px; }
  .retry { display: grid; grid-template-columns: 1fr auto; align-items: end; gap: 12px; width: 100%; padding-top: 14px; border-top: 0.5px solid var(--sk-line); }
  @media (max-width: 639px) {
    .chips { padding-left: 0; }
    .area { margin: 0; }
    .retry { grid-template-columns: 1fr; }
  }
</style>
