<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Logo from '../../ui/Logo.svelte';
  import RequestCard from './RequestCard.svelte';
  import type { AppActions, AppState } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  // B01: "What needs fixing?" with the hero composer, example chips and recent requests.
  let { app, actions }: { app: AppState; actions: AppActions } = $props();
  let field = $state<HTMLTextAreaElement>();
  const examples = ['My kitchen sink is leaking', 'My cupboard door is stuck', 'An outlet feels warm'];
  const ready = $derived(app.draft.trim().length >= 5);
  const recent = $derived([...app.workspaces].sort((a, b) => (b.updatedAt ?? '').localeCompare(a.updatedAt ?? '')).slice(0, 3));

  function start(event: SubmitEvent) { event.preventDefault(); void actions.startRequest(app.draft); }
  function useExample(example: string) { app.draft = t(example); actions.persistDraft(); field?.focus(); }
  function onKey(event: KeyboardEvent) {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing && ready) { event.preventDefault(); void actions.startRequest(app.draft); }
  }
</script>

<div class="home">
  <section class="hero" aria-labelledby="home-title">
    <h1 id="home-title" class="page-title">{t('What needs fixing?')}</h1>
    <p class="lead">{t('Say it in your own words. We’ll ask what the pro needs to know.')}</p>
    <form class="box composer" onsubmit={start}>
      <div class="identity">
        <span class="assistant" aria-hidden="true"><Logo onNavy wordmark={false} size={22}/></span>
        <span><b>{t('{brand} assistant', { brand: app.brandName })}</b><small>{t('Your own words, English or Dutch')}</small></span>
      </div>
      <label class="visually-hidden" for="home-description">{t('Describe your job')}</label>
      <textarea id="home-description" bind:this={field} bind:value={app.draft} oninput={actions.persistDraft} onkeydown={onKey} rows="3" minlength="5" maxlength="500" disabled={app.pending} placeholder={t('What’s wrong? For example, my kitchen tap keeps dripping…')} aria-describedby="home-reason"></textarea>
      <div class="foot">
        <button class="btn btn-primary btn-strong" disabled={app.pending || !ready} aria-busy={app.pending ? 'true' : undefined}>{t('Start my request')}<Icon name="arrow-right"/></button>
      </div>
      {#if !ready && app.draft}<p id="home-reason" class="reason">{t('Write a few more words to start')}</p>{/if}
    </form>
    <div class="examples">
      <span class="secondary-text">{t('Or try:')}</span>
      {#each examples as example}<button type="button" class="chip chip-light" disabled={app.pending} onclick={() => useExample(example)}>{t(example)}</button>{/each}
    </div>
    <p class="privacy"><Icon name="lock" size={14}/>{t('Leave out sensitive personal details.')}</p>
  </section>

  {#if recent.length}
    <section class="recent" aria-labelledby="recent-title">
      <div class="recent-head"><h2 id="recent-title" class="small-title">{t('Recent requests')}</h2><button type="button" class="btn btn-quiet" onclick={actions.showRequests}>{t('View all')}</button></div>
      <div class="recent-grid">{#each recent as item (item.id)}<RequestCard {item} compact onOpen={(id) => actions.selectRequest(id)}/>{/each}</div>
    </section>
  {/if}
</div>

<style>
  .home { display: grid; gap: 56px; max-width: 1040px; margin: 0 auto; padding: 72px 48px 72px; }
  .hero { display: grid; justify-items: center; gap: 18px; text-align: center; }
  .lead { font-size: 16px; color: var(--sk-muted); }
  .composer { display: grid; gap: 14px; width: 100%; max-width: 720px; margin-top: 18px; padding: 20px 22px 18px; border: 1px solid var(--sk-field); text-align: left; }
  .composer:focus-within { border-color: var(--sk-ink); }
  .identity { display: flex; align-items: center; gap: 10px; }
  .assistant { display: grid; place-items: center; width: 36px; height: 36px; border-radius: var(--sk-r-tile); background: var(--sk-brand); }
  .identity b { display: block; font-size: 13px; font-weight: 700; }
  .identity small { display: block; font-size: 12px; color: var(--sk-muted); }
  textarea { width: 100%; min-height: 120px; padding: 0; border: 0; background: transparent; resize: none; color: var(--sk-ink); font: 500 20px/1.4 var(--sk-fd); }
  textarea:focus-visible { outline: none; }
  textarea::placeholder { color: var(--sk-muted); }
  .foot { display: flex; justify-content: flex-end; padding-top: 14px; border-top: 0.5px solid var(--sk-line); }
  .examples { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 8px; margin-top: 8px; }
  .privacy { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--sk-muted); }
  .recent { display: grid; gap: 14px; }
  .recent-head { display: flex; justify-content: space-between; align-items: center; }
  .recent-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(260px, 100%), 1fr)); gap: 12px; }
  @media (max-width: 639px) {
    .home { gap: 36px; padding: 32px 16px 40px; }
    .composer { padding: 14px; }
    textarea { font-size: 17px; min-height: 96px; }
    .foot .btn { width: 100%; }
  }
</style>
