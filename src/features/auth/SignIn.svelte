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

</script>

<section class="landing-hero" aria-labelledby="hero-heading">
  <div class="hero-content">
    <span class="hero-badge"><span class="status-dot" aria-hidden="true"></span>{t('Home services, coordinated')}</span>
    <h1 id="hero-heading">{t('Home jobs.')}<br/>{t('Handled faster.')}</h1>
    <p class="hero-description">{t('AI clarifies the job and helps professionals respond sooner. Quotes, plans, and decisions stay together until the work is resolved.')}</p>
    {#if authMessage}<p class="error" role="alert">{t(authMessage)}</p>{/if}
    {#if app.error}<p class="error" role="status">{app.error}</p>{/if}
    {#if app.session?.configured === false}
      <p class="hero-notice" role="status">{t('Sign-in is temporarily unavailable. Please try again later.')}</p>
    {:else}
      <div class="hero-actions">
        <a class="hero-primary" href="/api/auth/login">{t('Get started')} <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7h10v10M7 17 17 7"/></svg></a>
      </div>
    {/if}
    <p class="hero-footnote">{t('One conversation for the job, from first question to approval.')}</p>
  </div>
</section>

<section id="how-it-works" class="how-it-works" aria-labelledby="how-heading">
  <div class="section-heading">
    <span class="eyebrow">{t('LESS BACK-AND-FORTH')}</span>
    <h2 id="how-heading">{t('From the first message')}<br/>{t('to the final detail.')}</h2>
    <p>{t('Good help should feel simple. Keep the conversation,')}<br class="desktop-break"/> {t('the decisions, and the work together.')}</p>
  </div>
  <div class="service-steps">
    <article class="service-step">
      <span class="step-number">01 <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7h10v10M7 17 17 7"/></svg></span>
      <h3>{t('Start with your own words.')}</h3>
      <p>{t('Describe what is happening in your own words. AI asks useful follow-up questions and helps identify the right kind of professional.')}</p>
    </article>
    <article class="service-step">
      <span class="step-number">02 <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7h10v10M7 17 17 7"/></svg></span>
      <h3>{t('Make a plan together.')}</h3>
      <p>{t('Review the service, discuss the price, and agree on a visit time. Your decisions stay in the same conversation.')}</p>
    </article>
    <article class="service-step">
      <span class="step-number">03 <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7h10v10M7 17 17 7"/></svg></span>
      <h3>{t('Stay on the same page.')}</h3>
      <p>{t('Follow the work, review photos and notes, and approve changes and completion. Every detail stays with your request.')}</p>
    </article>
  </div>
</section>

<footer class="landing-footer">
  <a class="brand" href="/" aria-label="{app.brandName} home"><svg class="brand-mark" viewBox="0 0 48 48" aria-hidden="true"><rect width="48" height="48" rx="12"/><g fill="none" stroke-width="3" stroke-linecap="round"><path d="M24 11v26M11 24h26M15 15l18 18M15 33l18-18"/></g></svg>{app.brandName}</a>
  <span>{t('Home services, handled faster.')}</span>
  <a href="https://github.com/allanzico/samen-klaar" target="_blank" rel="noreferrer">{t('Open source')} <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7h10v10M7 17 17 7"/></svg></a>
</footer>
