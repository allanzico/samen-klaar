<script lang="ts">
  import { currentLocale, setLocale, t, type Locale } from '../../lib/i18n.svelte';

  // EN / NL as a radiogroup. "navy" sits in app bars; "light" on the landing page and in sheets.
  let { tone = 'navy' }: { tone?: 'navy' | 'light' } = $props();
  const options: { value: Locale; label: string; name: string }[] = [
    { value: 'en', label: 'EN', name: 'English' },
    { value: 'nl', label: 'NL', name: 'Nederlands' }
  ];
  function key(event: KeyboardEvent) {
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return;
    event.preventDefault();
    const next = currentLocale() === 'en' ? 'nl' : 'en';
    setLocale(next);
    (event.currentTarget as HTMLElement).querySelector<HTMLElement>(`[data-locale="${next}"]`)?.focus();
  }
</script>

<div class="toggle {tone}" role="radiogroup" aria-label={t('Language')} tabindex="-1" onkeydown={key}>
  {#each options as option (option.value)}
    <button type="button" role="radio" data-locale={option.value} aria-checked={currentLocale() === option.value} aria-label={option.name} tabindex={currentLocale() === option.value ? 0 : -1} onclick={() => setLocale(option.value)}>{option.label}</button>
  {/each}
</div>

<style>
  .toggle { display: inline-flex; gap: 2px; padding: 4px; border-radius: var(--sk-r-pill); }
  .toggle:focus { outline: none; }
  .toggle button { min-width: 44px; min-height: 36px; padding: 0 14px; border: 0; border-radius: var(--sk-r-pill); background: transparent; font: 500 13px/1 var(--sk-ft); }
  .navy { background: var(--sk-brand-2); }
  .navy button { color: var(--sk-on-brand-2); }
  .navy button:focus-visible { outline-color: var(--sk-on-brand); }
  .navy button[aria-checked="true"] { background: var(--sk-ground); color: var(--sk-ink); font-weight: 600; }
  .light { background: var(--sk-surface-2); }
  .light button { color: var(--sk-muted); }
  .light button[aria-checked="true"] { background: var(--sk-surface); color: var(--sk-ink); font-weight: 600; box-shadow: var(--sk-shadow-raised); }
</style>
