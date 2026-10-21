<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import Stepper from '../../ui/Stepper.svelte';
  import type { AppActions, AppState, Intake } from '../../lib/types';
  import { money, dayTime, time, shortDay } from '../../lib/format';
  import { coverTitle, initials, tradeName, tradeOf } from '../../lib/trade';
  import { firstName } from '../../lib/status-ui';
  import { t } from '../../lib/i18n.svelte';

  type Match = Intake['matches'][number];
  // B07 best match with a slim side rail, B08 compare. Smart tags come from published data only.
  let { app, actions, intake }: { app: AppState; actions: AppActions; intake: Intake } = $props();
  let comparing = $state(false);
  const matches = $derived(intake.matches.slice(0, 3));
  const best = $derived(matches[0]);
  const others = $derived(matches.slice(1));
  const trade = $derived(tradeOf(intake.category));
  const area = $derived(intake.location?.label ?? '');

  // A tag only when the data singles one professional out; ties get no tag.
  const unique = <T,>(values: T[], pick: (list: T[]) => T | undefined) => { const target = pick(values); return values.filter((v) => v === target).length === 1 ? target : undefined; };
  const fastestId = $derived.by(() => {
    const timed = matches.filter((m) => m.earliestAppointment);
    if (timed.length < 2) return undefined;
    const earliest = unique(timed.map((m) => m.earliestAppointment!.startUtc), (list) => [...list].sort()[0]);
    return earliest ? timed.find((m) => m.earliestAppointment!.startUtc === earliest)?.id : undefined;
  });
  const cheapestId = $derived.by(() => {
    if (matches.length < 2) return undefined;
    const lowest = unique(matches.map((m) => m.totalMinor), (list) => Math.min(...list));
    return lowest !== undefined ? matches.find((m) => m.totalMinor === lowest)?.id : undefined;
  });
  function tags(match: Match, index: number) {
    const list: { variant: 'best' | 'smart'; label: string; icon?: 'zap' | 'euro' | 'message' }[] = [];
    if (index === 0) list.push({ variant: 'best', label: t('Best overall') });
    if (match.id === fastestId) list.push({ variant: 'smart', label: t('Fastest'), icon: 'zap' });
    if (match.id === cheapestId) list.push({ variant: 'smart', label: t('Cheapest'), icon: 'euro' });
    if (!match.earliestAppointment) list.push({ variant: 'smart', label: t('Flexible time'), icon: 'message' });
    return list.slice(0, 3);
  }
  const earliest = (match: Match) => match.earliestAppointment ? { main: time(match.earliestAppointment.startUtc, match.earliestAppointment.timezone), sub: shortDay(match.earliestAppointment.startUtc, match.earliestAppointment.timezone) } : { main: t('In chat'), sub: t('No set times') };
  const select = (match: Match) => actions.act('select-match', match.id);
</script>

{#snippet facts(match: Match)}
  {#if match.distanceKnown && intake.location?.kind !== 'place'}<p class="meta">{t('About {distance} km away (straight-line)', { distance: new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(match.distanceKm) })}</p>{/if}
  <dl class="facts">
    <div><dt>{t('Price')}</dt><dd>{money(match.totalMinor, match.currency)}</dd><span>{t('incl. call-out')}</span></div>
    <div><dt>{t('Takes')}</dt><dd>{t('{minutes} min', { minutes: match.durationMinutes })}</dd><span>{t('on site')}</span></div>
    <div><dt>{t('Earliest')}</dt><dd>{earliest(match).main}</dd><span>{earliest(match).sub}</span></div>
  </dl>
{/snippet}

{#snippet included(match: Match)}
  <details class="disclosure">
    <summary>{t('What’s included and what isn’t')}</summary>
    {#if match.serviceDescription}<p>{match.serviceDescription}</p>{/if}
    <p>{match.materialsPolicy}</p>
    {#if match.exclusions?.length}<p>{t('Not included: {list}', { list: match.exclusions.join(', ') })}</p>{/if}
  </details>
{/snippet}

<div class="match-layout">
  <div class="match-main">
    {#if intake.location?.source === 'mapbox' && intake.location?.kind !== 'address'}<p class="helper">{t('Results are approximate until we confirm your full visit address.')}</p>{/if}
    {#if comparing}
      <button type="button" class="btn btn-quiet back" onclick={() => { comparing = false; }}><Icon name="chevron-left"/>{t('Back to best match')}</button>
      <h1 class="match-title">{t('Compare {trade}', { trade: tradeName(trade, true) })}</h1>
      <div class="compare">
        {#each matches as match, index (match.id)}
          <article class="box compare-card" aria-labelledby="compare-{match.id}">
            <div class="tags">{#each tags(match, index) as tag (tag.label)}<Pill variant={tag.variant} label={tag.label} icon={tag.icon} small/>{/each}</div>
            <div class="person"><span class="avatar avatar-44" aria-hidden="true">{initials(match.name)}</span><div><h2 id="compare-{match.id}">{match.name}</h2><p class="secondary-text">{match.serviceName || t(match.category)}</p></div></div>
            {@render facts(match)}
            {@render included(match)}
            <button type="button" class="btn {index === 0 ? 'btn-primary' : 'btn-outline'} btn-block push" disabled={app.pending} onclick={() => select(match)}>{t('Choose {name} and continue', { name: firstName(match.name) })}</button>
          </article>
        {/each}
      </div>
    {:else if best}
      <h1 class="match-title">{coverTitle(trade, matches.length, area)}</h1>
      <p class="secondary-text">{t('Prices include call-out.')}</p>
      <Stepper step={2}/>
      <article class="box pick" aria-labelledby="pick-name">
        <div class="pick-top"><p class="eyebrow small">{t('Our pick for you')}</p><div class="tags">{#each tags(best, 0) as tag (tag.label)}<Pill variant={tag.variant} label={tag.label} icon={tag.icon} small/>{/each}</div></div>
        <div class="person"><span class="avatar avatar-48" aria-hidden="true">{initials(best.name)}</span><div><h2 id="pick-name">{best.name}</h2><p class="secondary-text">{t('{trade} in {area}', { trade: t(best.category), area: best.area || area })}</p></div></div>
        <p class="service"><Icon name="check-circle"/><span><b>{best.serviceName}</b>{#if best.fitReason}<small>{best.fitReason}</small>{/if}</span></p>
        {@render facts(best)}
        {@render included(best)}
        <button type="button" class="btn btn-primary btn-block btn-strong" disabled={app.pending} onclick={() => select(best)}>{t('Choose {name} and continue', { name: firstName(best.name) })}</button>
        <p class="helper center">{t('You choose the professional. Your assistant then helps with the plan and visit.')}</p>
      </article>
    {/if}
  </div>

  <aside class="side" aria-label={t('Your job and alternatives')}>
    <section class="box side-job">
      <div class="side-head"><h2 class="small-title">{t('Your job')}</h2><button type="button" class="btn btn-quiet" disabled={app.pending} onclick={actions.focusComposer}>{t('Edit')}</button></div>
      <p>{intake.description}</p>
      <div class="facts-tags">{#if intake.category}<span class="pill pill--fact">{t(intake.category)}</span>{/if}{#if area}<span class="pill pill--fact">{area}</span>{/if}{#if intake.urgency}<span class="pill pill--fact">{t(intake.urgency)}</span>{/if}</div>
    </section>
    <button type="button" class="btn btn-quiet" disabled={app.pending} onclick={actions.focusComposer}>{t('Not your problem? Describe it again')}</button>
    {#if others.length && !comparing}
      <div class="side-head"><h2 class="small-title">{t('Also available')}</h2>{#if matches.length > 1}<button type="button" class="btn btn-quiet" onclick={() => { comparing = true; }}>{matches.length === 3 ? t('Compare all three') : t('Compare both')}</button>{/if}</div>
      <ul class="box rows alts">
        {#each others as match (match.id)}
          <li>
            <span class="avatar" aria-hidden="true">{initials(match.name)}</span>
            <span class="alt-text"><b>{match.name}</b><small>{money(match.totalMinor, match.currency)} · {t('{minutes} min', { minutes: match.durationMinutes })} · {match.earliestAppointment ? dayTime(match.earliestAppointment.startUtc, match.earliestAppointment.timezone) : t('time in chat')}</small></span>
            <button type="button" class="btn btn-outline btn-sm" disabled={app.pending} onclick={() => select(match)}>{t('Choose {name}', { name: firstName(match.name) })}</button>
          </li>
        {/each}
      </ul>
    {/if}
    <button type="button" class="btn btn-quiet" disabled={app.pending} onclick={() => actions.act('refresh-matches')}>{t('Refresh availability')}</button>
  </aside>
</div>

<style>
  .match-layout { display: grid; grid-template-columns: minmax(0, 1fr) minmax(300px, 380px); min-height: 100%; }
  .match-main { display: grid; align-content: start; gap: 14px; width: 100%; max-width: 760px; margin: 0 auto; padding: 32px 40px 48px; }
  .match-title { font: 800 26px/1.1 var(--sk-fd); font-stretch: 90%; letter-spacing: -0.015em; }
  .back { justify-self: start; margin-left: -8px; }
  .pick { display: grid; gap: 16px; margin-top: 14px; padding: 22px; box-shadow: 0 16px 36px -24px var(--sk-shadow); }
  .pick-top { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 10px; }
  .eyebrow.small { font-size: 12px; }
  .tags { display: flex; flex-wrap: wrap; gap: 6px; }
  .person { display: flex; align-items: center; gap: 12px; }
  .person h2 { font: 700 17px/1.2 var(--sk-fd); }
  .service { display: flex; gap: 10px; padding: 12px 14px; border-radius: var(--sk-r-card); background: var(--sk-soft); }
  .service :global(.icon) { margin-top: 2px; }
  .service b { display: block; font-weight: 600; }
  .service small { display: block; font-size: 13px; color: var(--sk-ink); }
  .facts { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
  .facts > div { display: grid; gap: 2px; padding: 12px; border-radius: var(--sk-r-tile); background: var(--sk-ground); }
  .facts dt, .facts span { font-size: 12px; color: var(--sk-muted); }
  .facts dd { font: 800 17px/1.1 var(--sk-fd); font-variant-numeric: tabular-nums; }
  .center { text-align: center; }
  .side { display: grid; align-content: start; justify-items: start; gap: 14px; padding: 24px; border-left: 1px solid var(--sk-line); background: var(--sk-ground); }
  .side-job { display: grid; gap: 10px; width: 100%; padding: 18px; }
  .side-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; width: 100%; }
  .facts-tags { display: flex; flex-wrap: wrap; gap: 6px; }
  .alts { width: 100%; padding: 0 14px; }
  .alts li { display: grid; grid-template-columns: 36px 1fr auto; align-items: center; gap: 10px; padding: 12px 0; }
  .alt-text b { display: block; font-size: 14px; }
  .alt-text small { display: block; font-size: 12px; color: var(--sk-muted); }
  .compare { display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap: 12px; }
  .compare-card { display: flex; flex-direction: column; gap: 14px; padding: 18px; }
  .compare-card h2 { font-size: 15px; }
  .compare-card .facts { grid-template-columns: 1fr; }
  .push { margin-top: auto; }
  @media (max-width: 1023px) {
    .match-layout { grid-template-columns: 1fr; }
    .side { border-left: 0; border-top: 1px solid var(--sk-line); }
  }
  @media (max-width: 639px) {
    .match-main { padding: 20px 16px 28px; }
    .match-title { font-size: 20px; }
    .facts { grid-template-columns: 1fr 1fr 1fr; gap: 6px; }
    .facts dd { font-size: 14px; }
    .pick { padding: 18px 16px; }
    .side { padding: 20px 16px; }
  }
</style>
