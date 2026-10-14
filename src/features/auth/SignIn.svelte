<script lang="ts">
  import type { AppState } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  let { app }: { app: AppState } = $props();
  const authError = new URLSearchParams(window.location.search).get('auth_error');
  const authMessages: Record<string, string> = {
    cancelled: 'Sign-in was cancelled. You can start again when you’re ready.',
    state: 'We couldn’t finish that sign-in. Please start again from this page in the same browser.',
    expired: 'That sign-in link has expired or was already used. Please start again.',
    failed: 'We couldn’t complete sign-in. Please try again.',
    unavailable: 'Sign-in is temporarily unavailable. Please try again shortly.',
    access: 'This account cannot access that workspace. Try signing in as a customer.'
  };
  const authMessage = authError ? authMessages[authError] : undefined;
  const steps = [
    { title: 'Start with your own words.', text: 'Describe what is happening in your own words. AI asks useful follow-up questions and helps identify the right kind of professional.' },
    { title: 'Make a plan together.', text: 'Review the service, discuss the price, and agree on a visit time. Your decisions stay in the same conversation.' },
    { title: 'Stay on the same page.', text: 'Follow the work, review photos and notes, and approve changes and completion. Every detail stays with your request.' }
  ];
  const promises = ['Prices come from the professional’s published service.', 'Your street address stays private until you choose a professional.', 'People, not AI, decide disputes and refunds.'];
</script>

{#snippet check()}<svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10"/></svg>{/snippet}

<section class="landing-hero" aria-labelledby="hero-heading">
  <div class="hero-grid">
    <div class="hero-copy">
      <p class="eyebrow">{t('Home repairs in Amsterdam')}</p>
      <h1 id="hero-heading">{t('Describe the problem. We handle the back‑and‑forth.')}</h1>
      <p class="hero-description">{t('AI clarifies the job and helps professionals respond sooner. Quotes, plans, and decisions stay together until the work is resolved.')}</p>
      {#if authMessage}<p class="error" role="alert">{t(authMessage)}</p>{/if}
      {#if app.error}<p class="error" role="status">{app.error}</p>{/if}
      {#if app.session?.configured === false}
        <p class="hero-notice" role="status">{t('Sign-in is temporarily unavailable. Please try again later.')}</p>
      {:else}
        <div class="hero-actions">
          <a class="button-link primary" href="/api/auth/login">{t('Describe your job')}</a>
          <a class="button-link" href="/api/auth/login">{t('I’m a professional')}</a>
        </div>
      {/if}
      <ul class="promises">
        <li>{@render check()}{t('You approve the price and a visit time.')}</li>
        <li>{@render check()}{t('You pay only after you approve the finished work.')}</li>
      </ul>
    </div>
    <div class="hero-preview" aria-hidden="true">
      <p class="eyebrow">{t('Example')}</p>
      <div class="preview-request"><b>{t('My kitchen sink is leaking')}</b><span class="pill pill--your-turn">{t('Your turn')}</span></div>
      <div class="message"><span class="message-avatar assistant"><svg viewBox="0 0 48 48"><g fill="none" stroke-width="4" stroke-linecap="round"><path d="M24 11v26M11 24h26M15 15l18 18M15 33l18-18"/></g></svg></span><span class="message-label">{t('{brand} assistant', { brand: app.brandName })}</span><p>{t('Where is the leak?')}</p></div>
      <div class="preview-chips"><span>{t('Under the tap')}</span><span>{t('Under the sink')}</span></div>
      <div class="offer-card awaiting-you"><div class="offer-head"><b>{t('Visit proposed')}</b><span class="pill pill--your-turn">{t('Your turn')}</span></div><p class="offer-value">{t('Thu 09:00–10:30')}</p><span class="preview-button">{t('Accept and book this visit')}</span></div>
    </div>
  </div>
</section>

<section id="how-it-works" class="how-it-works" aria-labelledby="how-heading">
  <div class="section-heading">
    <span class="eyebrow">{t('LESS BACK-AND-FORTH')}</span>
    <h2 id="how-heading">{t('From the first message')}<br/>{t('to the final detail.')}</h2>
    <p>{t('Good help should feel simple. Keep the conversation,')}<br class="desktop-break"/> {t('the decisions, and the work together.')}</p>
  </div>
  <div class="service-steps">
    {#each steps as step, index}
      <article class="service-step">
        <span class="step-number">0{index + 1}</span>
        <h3>{t(step.title)}</h3>
        <p>{t(step.text)}</p>
      </article>
    {/each}
  </div>
</section>

<section class="promise-band" aria-label={t('Our promises')}>
  <ul class="promise-grid">{#each promises as promise}<li>{@render check()}{t(promise)}</li>{/each}</ul>
</section>

<section class="amsterdam" aria-labelledby="amsterdam-heading">
  <p class="eyebrow">{t('Starting in Amsterdam')}</p>
  <h2 id="amsterdam-heading">{t('A focused start, matched to real coverage')}</h2>
  <p>{t('We only match you with approved professionals who cover your area.')}</p>
  <ul class="trade-chips"><li>{t('Plumbing')}</li><li>{t('Carpentry')}</li><li>{t('Electrical')} · {t('Assessment visit')}</li></ul>
</section>

<section class="pro-invite" aria-labelledby="pro-invite-heading">
  <div><h2 id="pro-invite-heading">{t('Get jobs that are already clear')}</h2><p>{t('Takes about 5 minutes. Our team reviews every profile before customers can book you.')}</p></div>
  <a class="button-link primary" href="/api/auth/login">{t('Become a professional')}</a>
</section>

<footer class="landing-footer">
  <a class="brand" href="/" aria-label="{app.brandName} home"><svg class="brand-mark" viewBox="0 0 48 48" aria-hidden="true"><rect width="48" height="48" rx="12"/><g fill="none" stroke-width="3" stroke-linecap="round"><path d="M24 11v26M11 24h26M15 15l18 18M15 33l18-18"/></g></svg>{app.brandName}</a>
  <span>{t('Home services, handled faster.')}</span>
  <a href="https://github.com/allanzico/samen-klaar" target="_blank" rel="noreferrer">{t('Source code')} <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7h10v10M7 17 17 7"/></svg></a>
</footer>
