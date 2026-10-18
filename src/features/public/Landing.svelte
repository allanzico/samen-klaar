<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Logo from '../../ui/Logo.svelte';
  import Banner from '../../ui/Banner.svelte';
  import Pill from '../../ui/Pill.svelte';
  import PublicHeader from './PublicHeader.svelte';
  import type { AppState } from '../../lib/types';
  import { currentLocale, setLocale, t } from '../../lib/i18n.svelte';

  // A01–A03: the public landing page, with the sign-in banners on top of the hero.
  let { app }: { app: AppState } = $props();
  const authError = new URLSearchParams(window.location.search).get('auth_error');
  const authMessages: Record<string, string> = {
    cancelled: 'Sign-in was cancelled. You can start again when you’re ready.',
    state: 'We couldn’t finish signing you in. Start again from this page in the same browser.',
    expired: 'We couldn’t finish signing you in. That link expired. Start again.',
    failed: 'We couldn’t finish signing you in. Please try again.',
    unavailable: 'Sign-in is temporarily unavailable. Try again later.',
    access: 'This account can’t open that workspace. Try signing in as a customer.'
  };
  const authMessage = authError ? authMessages[authError] : undefined;
  const unavailable = $derived(app.session?.configured === false);
  const steps = [
    ['Describe', 'Tell us the problem. Photos help.'],
    ['Match', 'Get the best-fit pro, plus two alternatives.'],
    ['Agree', 'Fixed price, your time.'],
    ['Do the job', 'Extras need your OK first.'],
    ['Check', 'Review photos before you approve.'],
    ['Pay', 'Pay by PayPal when you’re happy.']
  ];
  const promises = [
    ['Chat never changes the deal.', 'Changes arrive as offers. Nothing changes until you accept.'],
    ['The price is the published price.', 'Published prices. No surprise quotes.'],
    ['You pay after you’ve checked.', 'Approve the work, then pay.']
  ];
</script>

<div class="landing">
  <PublicHeader brand={app.brandName}/>
  <main id="main-content">
    <section class="hero" aria-labelledby="hero-title">
      <div class="hero-copy">
        {#if authMessage}<Banner kind="error" text={t(authMessage)}/>{/if}
        {#if unavailable}<Banner kind="info" text={t('Sign-in is temporarily unavailable. Try again later.')}/>{/if}
        {#if app.error}<Banner kind="info" text={t(app.error)}/>{/if}
        <h1 id="hero-title" class="hero-title">{t('From ‘it’s dripping’ to klaar.')}</h1>
        <p class="hero-text">{t('Tell us what’s wrong. We find a reviewed pro and keep price, visit and payment in one place.')}</p>
        {#if !unavailable}
          <div class="hero-actions">
            <a class="btn btn-primary btn-strong" href="/api/auth/login">{t('Describe your job')}</a>
            <a class="btn btn-quiet" href="/api/auth/login">{t('I’m a professional')}</a>
          </div>
        {/if}
        <ul class="trades">
          <li class="chip chip-light"><Icon name="droplet"/>{t('Plumbing')}</li>
          <li class="chip chip-light"><Icon name="hammer"/>{t('Carpentry')}</li>
          <li class="chip chip-light"><Icon name="zap"/>{t('Electrical assessment visits')}</li>
        </ul>
      </div>
      <div class="hero-preview on-navy" aria-hidden="true">
        <p class="preview-title">{t('Dripping kitchen tap')}</p>
        <p class="preview-sub">{t('Example request')}</p>
        <div class="preview-steps"><span class="tag">{t('Step {n} of 6 · {step}', { n: 4, step: t('Do the job') })}</span><span class="segs">{#each [1, 2, 3, 4, 5, 6] as n}<span class:done={n < 4} class:current={n === 4}></span>{/each}</span></div>
        <div class="preview-message"><span class="avatar">DM</span><p>{t('Found it: a worn cartridge. The valve under the sink is seized too. I can replace it while I’m here.')}</p></div>
        <div class="preview-offer">
          <div class="offer-row"><Pill variant="your-turn" label={t('Your turn')}/><b class="amount">+ € 35,00</b></div>
          <p class="offer-title">{t('Extra work to approve')}</p>
          <div class="offer-buttons"><span class="btn btn-sm btn-primary">{t('Approve')}</span><span class="btn btn-sm btn-outline">{t('Keep plan')}</span></div>
        </div>
      </div>
    </section>

    <section id="how-it-works" class="how" aria-labelledby="how-title">
      <h2 id="how-title" class="page-title section-big">{t('How a job goes')}</h2>
      <ol class="timeline">
        {#each steps as [title, text], i}
          <li><span class="dot">{i + 1}</span><b>{t(title)}</b><p>{t(text)}</p></li>
        {/each}
      </ol>
    </section>

    <section class="promises" aria-label={t('Our promises')}>
      <ul>{#each promises as [title, text]}<li><h3>{t(title)}</h3><p>{t(text)}</p></li>{/each}</ul>
    </section>

    <section id="for-professionals" class="pros on-navy" aria-labelledby="pros-title">
      <div class="pros-copy">
        <h2 id="pros-title" class="page-title section-big">{t('Get jobs that are already clear.')}</h2>
        <p>{t('Requests arrive described, with your published price agreed.')}</p>
        <ul class="ticks">
          <li><Icon name="check"/>{t('Set your own service, price, areas and free times.')}</li>
          <li><Icon name="check"/>{t('Extra work goes out as an offer.')}</li>
          <li><Icon name="check"/>{t('Get paid through PayPal after the customer approves.')}</li>
        </ul>
        <div class="pros-actions"><a class="btn btn-on-navy btn-strong" href="/api/auth/login">{t('Join as a professional')}</a><span>{t('Our team reviews every profile before customers can book you.')}</span></div>
      </div>
      <div class="pros-preview" aria-hidden="true">
        <p class="needs">{t('Needs you')}</p>
        <div class="pro-card"><Pill variant="your-turn" label={t('Confirm visit')}/><b>{t('Dripping kitchen tap')}</b><p>{t('Example job')}</p><div class="offer-buttons"><span class="btn btn-sm btn-primary">{t('Confirm visit')}</span><span class="btn btn-sm btn-outline">{t('Read the job')}</span></div></div>
      </div>
    </section>
  </main>
  <footer class="foot on-navy">
    <div class="foot-brand"><Logo onNavy name={app.brandName}/><span>{t('Home repairs in Amsterdam')}</span></div>
    <nav class="foot-links" aria-label={t('Footer')}>
      <a href="https://github.com/allanzico/samen-klaar" target="_blank" rel="noreferrer">{t('Source code')}</a>
      <button type="button" class="link" onclick={() => setLocale(currentLocale() === 'nl' ? 'en' : 'nl')}>{currentLocale() === 'nl' ? 'English' : 'Nederlands'}</button>
    </nav>
  </footer>
</div>

<style>
  .landing { background: var(--sk-ground); }
  .hero { display: grid; grid-template-columns: minmax(0, 1.25fr) minmax(320px, .75fr); gap: 56px; align-items: center; max-width: 1376px; margin: 0 auto; padding: 40px 32px 72px; }
  .hero-copy { display: grid; justify-items: start; gap: 24px; }
  .hero-title { font: 800 clamp(40px, 5.4vw, 76px)/.92 var(--sk-fd); font-stretch: 84%; letter-spacing: -0.035em; }
  .hero-text { max-width: 46ch; font-size: 17px; }
  .hero-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 16px; }
  .trades { display: flex; flex-wrap: wrap; gap: 8px; }
  .trades .chip { min-height: 36px; font-size: 13px; }
  .hero-preview { display: grid; gap: 12px; padding: 20px; border-radius: var(--sk-r-box); background: var(--sk-brand); color: var(--sk-on-brand); box-shadow: var(--sk-shadow-dialog); }
  .preview-title { font: 700 17px/1.2 var(--sk-fd); }
  .preview-sub { font-size: 12px; color: var(--sk-on-brand-2); margin-top: -8px; }
  .preview-steps { display: grid; gap: 8px; }
  .preview-steps .tag { justify-self: start; font-size: 12px; font-weight: 700; }
  .segs { display: flex; gap: 4px; }
  .segs span { flex: 1; height: 5px; border-radius: 3px; background: var(--sk-brand-2); }
  .segs .done { background: var(--sk-on-brand); }
  .segs .current { background: var(--sk-signal); }
  .preview-message { display: flex; gap: 10px; padding: 12px; border-radius: var(--sk-r-card); background: var(--sk-surface); color: var(--sk-ink); font-size: 13px; }
  .preview-offer { display: grid; gap: 8px; padding: 14px; border-radius: var(--sk-r-card); background: var(--sk-turn-soft); color: var(--sk-ink); border: 0; }
  .offer-row { display: flex; justify-content: space-between; align-items: center; }
  .offer-row .amount { font-size: 17px; }
  .offer-title { font-weight: 700; font-family: var(--sk-fd); font-size: 15px; }
  .offer-buttons { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
  .offer-buttons .btn { pointer-events: none; }
  .how { max-width: 1376px; margin: 0 auto; padding: 40px 32px 72px; }
  .section-big { font-size: clamp(28px, 3.2vw, 42px); }
  .timeline { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 20px; margin-top: 36px; }
  .timeline li { position: relative; display: grid; gap: 8px; }
  .timeline li:not(:last-child)::after { content: ""; position: absolute; top: 15px; left: 40px; right: -12px; height: 2px; background: var(--sk-signal); }
  .dot { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 50%; background: var(--sk-signal); color: var(--sk-on-signal); font: 700 13px/1 var(--sk-ft); }
  .timeline b { margin-top: 8px; font: 700 17px/1.2 var(--sk-fd); }
  .timeline p { font-size: 13px; color: var(--sk-muted); }
  .promises { background: var(--sk-surface); }
  .promises ul { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 40px; max-width: 1376px; margin: 0 auto; padding: 72px 32px; }
  .promises li { display: grid; gap: 10px; padding-top: 20px; border-top: 2px solid var(--sk-ink); }
  .promises h3 { font: 700 22px/1.1 var(--sk-fd); }
  .promises p { font-size: 14px; }
  .pros { display: grid; grid-template-columns: minmax(0, 1.3fr) minmax(280px, .7fr); gap: 48px; align-items: center; padding: 72px max(32px, calc((100vw - 1312px) / 2)); background: var(--sk-brand); color: var(--sk-on-brand); }
  .pros-copy { display: grid; justify-items: start; gap: 18px; }
  .pros .page-title { color: var(--sk-on-brand); }
  .ticks { display: grid; gap: 10px; }
  .ticks li { display: flex; align-items: center; gap: 10px; }
  .ticks :global(.icon) { color: var(--sk-signal); }
  .pros-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 16px; }
  .pros-actions span { font-size: 13px; color: var(--sk-on-brand-2); }
  .pros-preview { display: grid; gap: 10px; }
  .needs { font-size: 13px; font-weight: 600; }
  .pro-card { display: grid; justify-items: start; gap: 8px; padding: 16px; border-radius: var(--sk-r-card); background: var(--sk-turn-soft); color: var(--sk-ink); }
  .pro-card b { font: 700 17px/1.2 var(--sk-fd); }
  .pro-card .offer-buttons { width: 100%; }
  .foot { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 20px; padding: 32px max(32px, calc((100vw - 1312px) / 2)); background: var(--sk-brand-3); color: var(--sk-on-brand); }
  .foot-brand { display: grid; gap: 4px; font-size: 12px; color: var(--sk-on-brand-2); }
  .foot-links { display: flex; gap: 20px; align-items: center; }
  .foot-links a, .link { min-height: 44px; display: inline-flex; align-items: center; padding: 0; border: 0; background: none; color: var(--sk-on-brand); font-size: 13px; text-decoration: underline; text-underline-offset: 4px; }
  @media (max-width: 1023px) {
    .hero, .pros { grid-template-columns: 1fr; }
    .timeline { grid-template-columns: repeat(3, minmax(0, 1fr)); row-gap: 32px; }
    .timeline li:nth-child(3)::after { display: none; }
  }
  @media (max-width: 639px) {
    .hero { padding: 16px 16px 48px; gap: 32px; }
    .how { padding: 24px 16px 48px; }
    .timeline { grid-template-columns: 1fr; gap: 20px; }
    .timeline li { grid-template-columns: 32px 1fr; column-gap: 14px; }
    .timeline li::after { display: none; }
    .timeline b { margin-top: 4px; }
    .timeline p { grid-column: 2; }
    .promises ul { grid-template-columns: 1fr; padding: 48px 16px; gap: 28px; }
    .pros { padding: 48px 16px; }
    .foot { padding: 28px 16px; }
  }
</style>
