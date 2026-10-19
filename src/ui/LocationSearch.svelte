<script lang="ts">
  import { api } from '../lib/api';
  import type { Location } from '../lib/types';
  import { currentLocale, t } from '../lib/i18n.svelte';

  let { id, label, value = $bindable(''), selected = $bindable<Location | null>(null), purpose = 'search', disabled = false, help = '' }: {
    id: string; label: string; value?: string; selected?: Location | null; purpose?: 'search' | 'base' | 'address'; disabled?: boolean; help?: string;
  } = $props();
  let results = $state<Location[]>([]);
  let focused = $state(false);
  let loading = $state(false);
  let error = $state('');
  let searched = $state(false);
  let active = $state(-1);
  const open = $derived(focused && !selected && (loading || searched || !!error));

  $effect(() => {
    const query = value.trim();
    const language = currentLocale();
    if (!focused || selected || disabled || query.length < 2) { results = []; loading = false; searched = false; error = ''; return; }
    const controller = new AbortController();
    results = []; active = -1; error = ''; searched = false; loading = true;
    const timer = window.setTimeout(async () => {
      try {
        const response = await api.locations(query, language, purpose, controller.signal);
        if (!controller.signal.aborted) { results = response.locations; searched = true; }
      } catch {
        if (!controller.signal.aborted) error = t('Location search is unavailable. Please try again.');
      } finally { if (!controller.signal.aborted) loading = false; }
    }, 350);
    return () => { window.clearTimeout(timer); controller.abort(); };
  });

  function choose(location: Location) { selected = location; value = location.address || location.label; results = []; active = -1; }
  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') { focused = false; return; }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      focused = true;
      if (!results.length) return;
      event.preventDefault();
      active = (active + (event.key === 'ArrowDown' ? 1 : -1) + results.length) % results.length;
      document.getElementById(`${id}-option-${active}`)?.scrollIntoView({ block: 'nearest' });
    }
    if (event.key === 'Enter' && !selected) { event.preventDefault(); if (active >= 0 && results[active]) choose(results[active]); }
  }
</script>

<div class="field location-search">
  <label for={id}>{label}</label>
  <div class="input-wrap">
  <input {id} class="input" {disabled} bind:value oninput={() => { selected = null; focused = true; }} onfocus={() => { focused = true; }} onblur={() => { focused = false; }} onkeydown={keydown}
    role="combobox" aria-autocomplete="list" aria-expanded={open} aria-controls={`${id}-results`} aria-activedescendant={active >= 0 ? `${id}-option-${active}` : undefined}
    aria-describedby={`${id}-help ${id}-status`} autocomplete="off" maxlength="256" placeholder={t('Enter an address, postcode or city')}/>
  {#if open}
    <div class="suggestions">
      <ul id={`${id}-results`} role="listbox" aria-label={t('Location suggestions')}>
        {#each results as location, i (location.id)}
          <li id={`${id}-option-${i}`} role="option" aria-selected={active === i}>
            <button type="button" class:active={active === i} tabindex="-1" onpointerdown={(event) => event.preventDefault()} onclick={() => choose(location)}>{location.address || location.label}</button>
          </li>
        {/each}
      </ul>
      {#if loading}<p class="meta" role="status">{t('Searching locations…')}</p>
      {:else if error}<p class="field-error" role="alert">{error}</p><button type="button" class="btn btn-quiet btn-sm" onpointerdown={(event) => event.preventDefault()} onclick={() => { focused = false; queueMicrotask(() => { focused = true; }); }}>{t('Try again')}</button>
      {:else if searched && !results.length}<p class="meta" role="status">{t('No locations found. Try a postcode or add a city name.')}</p>{/if}
      <p class="attribution">© <a href="https://www.mapbox.com/about/maps/" target="_blank" rel="noreferrer">Mapbox</a></p>
    </div>
  {/if}
  </div>
  <p id={`${id}-help`} class="field-help">{help || t('Choose a location from the suggestions.')}</p>
  <span id={`${id}-status`} class="visually-hidden" role="status">{selected ? t('Location selected') : loading ? t('Searching locations…') : ''}</span>
</div>

<style>
  .location-search { min-width: 0; }
  .input-wrap { position: relative; }
  .suggestions { position: absolute; top: calc(100% + 4px); left: 0; right: 0; z-index: 20; max-height: 320px; overflow-y: auto; border: 1px solid var(--sk-line); border-radius: var(--sk-r-card); background: var(--sk-surface); }
  ul { list-style: none; margin: 0; padding: 4px; }
  li + li { border-top: 1px solid var(--sk-line-2); }
  li button { display: block; width: 100%; min-height: 44px; padding: 10px 12px; border: 0; border-radius: 8px; text-align: left; font: inherit; color: var(--sk-ink); background: var(--sk-surface); overflow-wrap: anywhere; }
  li button:hover, li button.active { background: var(--sk-turn-soft); }
  .suggestions p { padding: 8px 12px; }
  .attribution { font-size: 12px; color: var(--sk-muted); text-align: right; }
  .attribution a { color: inherit; }
</style>
