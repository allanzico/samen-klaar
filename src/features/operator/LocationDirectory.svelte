<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../../lib/api';
  import type { AdminLocationPage } from '../../lib/operator-admin';
  import { t } from '../../lib/i18n.svelte';
  import Banner from '../../ui/Banner.svelte';
  import Pill from '../../ui/Pill.svelte';

  let { onBrowse }: { onBrowse: (area: string) => void } = $props();
  let query = $state('');
  let status = $state('');
  let filter = $state({ query: '', status: '' });
  let page = $state<AdminLocationPage>({ locations: [], total: 0, offset: 0, hasMore: false });
  let loading = $state(true);
  let error = $state('');
  let requestID = 0;
  onMount(() => { void load(); });
  async function load(offset = 0) {
    const request = ++requestID;
    loading = true; error = '';
    try { const result = await api.operatorLocations(filter.query, filter.status, offset); if (request === requestID) page = result; }
    catch (e) { if (request === requestID) error = e instanceof Error ? e.message.trim() : t('Could not load locations. Try again.'); }
    finally { if (request === requestID) loading = false; }
  }
  function search(event: SubmitEvent) { event.preventDefault(); filter = { query: query.trim(), status }; void load(); }
</script>

<section class="locations" aria-labelledby="locations-title">
  <header class="heading"><div><h2 id="locations-title" class="detail-title">{t('Locations')}</h2><p class="secondary-text">{t('Published service areas and customer demand, with the professionals behind each location.')}</p></div><button class="btn btn-outline btn-sm" disabled={loading} onclick={() => load(page.offset)}>{t('Refresh locations')}</button></header>
  <form class="filters" onsubmit={search}><div class="field"><label for="location-search">{t('Search locations')}</label><input id="location-search" class="input" bind:value={query} maxlength="160" placeholder={t('City, neighbourhood or postcode')}/></div><div class="field"><label for="coverage-status">{t('Coverage')}</label><select id="coverage-status" class="input" bind:value={status}><option value="">{t('All locations')}</option><option value="available">{t('Active professionals')}</option><option value="uncovered">{t('No active professionals')}</option><option value="waitlisted">{t('Waitlisted demand')}</option></select></div><button class="btn btn-primary" disabled={loading}>{t('Search')}</button></form>
  <p class="meta">{t('Coverage is grouped by published area or base postcode. A full visit address confirms whether a professional can attend.')}</p>
  {#if error}<Banner kind="error" text={t(error)}/><button class="btn btn-outline" onclick={() => load(page.offset)}>{t('Try again')}</button>{/if}
  <p class="meta" role="status">{loading ? t('Loading locations…') : t('{count} matching locations', { count: page.total })}</p>
  {#if !loading && !page.locations.length}<div class="empty"><h3 class="small-title">{t('No locations match')}</h3><p class="secondary-text">{t('Locations appear when professionals publish coverage or customers search an area. Try another search or coverage filter.')}</p></div>{/if}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users need to scroll the wide table.) -->
  <div class="table-wrap" tabindex="0" role="region" aria-label={t('Location coverage directory')} aria-busy={loading}><table><caption class="visually-hidden">{t('Location coverage directory')}</caption><thead><tr><th scope="col">{t('Location')}</th><th scope="col">{t('Active pros')}</th><th scope="col">{t('Pending pros')}</th><th scope="col">{t('Customer jobs')}</th><th scope="col">{t('Waitlisted')}</th><th scope="col">{t('Manage')}</th></tr></thead><tbody>{#each page.locations as location (location.label)}<tr><th scope="row"><b>{location.label}</b><span class="meta categories">{location.categories.map((category) => t(category)).join(', ') || t('No published services')}</span><Pill variant={location.activeProfessionals ? 'done' : 'neutral'} label={location.activeProfessionals ? t('Active coverage') : t('No active professionals')} small/></th><td>{location.activeProfessionals}</td><td>{location.pendingProfessionals}</td><td>{location.requests}</td><td>{location.waitlistedRequests}</td><td><button class="btn btn-outline btn-sm" disabled={loading || !location.totalProfessionals} onclick={() => onBrowse(location.label)} aria-label={t('View professionals in {area}', { area: location.label })}>{t('View professionals')}</button></td></tr>{/each}</tbody></table></div>
  <nav class="pager" aria-label={t('Location pages')}><button class="btn btn-outline btn-sm" disabled={loading || page.offset === 0} onclick={() => load(Math.max(0, page.offset - 25))}>{t('Previous')}</button><span class="meta">{page.total ? t('{start}–{end} of {total}', { start: page.offset + 1, end: page.offset + page.locations.length, total: page.total }) : t('No results')}</span><button class="btn btn-outline btn-sm" disabled={loading || !page.hasMore} onclick={() => load(page.offset + 25)}>{t('Next')}</button></nav>
</section>

<style>
  .locations { display: grid; gap: 20px; min-width: 0; }
  .heading { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: start; gap: 16px; }
  .heading > div { display: grid; gap: 8px; max-width: 65ch; }
  .detail-title { font: 800 26px/1.1 var(--sk-fd); }
  .filters { display: grid; grid-template-columns: minmax(200px,1fr) minmax(180px,.6fr) auto; gap: 12px; align-items: end; }
  .table-wrap { overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; text-align: left; font-variant-numeric: tabular-nums; }
  th, td { padding: 16px 12px; border-bottom: 1px solid var(--sk-line); vertical-align: top; }
  thead th { font-size: 12px; color: var(--sk-muted); font-weight: 600; white-space: nowrap; }
  tbody th { font-weight: 400; min-width: 190px; }
  td { min-width: 76px; }
  td:last-child { min-width: 160px; }
  .categories { display: block; margin: 6px 0 10px; }
  .pager { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; }
  .empty { display: grid; gap: 8px; padding: 20px 0; }
  @media (max-width: 767px) { .filters { grid-template-columns: 1fr; } .filters > .btn { justify-self: start; } }
  @media (max-width: 639px) { .detail-title { font-size: 20px; } }
</style>
