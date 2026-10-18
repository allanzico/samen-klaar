<script lang="ts">
  import Pill from '../../ui/Pill.svelte';
  import Banner from '../../ui/Banner.svelte';
  import PublicHeader from './PublicHeader.svelte';
  import type { AppActions, AppState } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  // A04 operator sign-in, A05 operator access required, plus the access check while it runs.
  let { app, actions, mode }: { app: AppState; actions: AppActions; mode: 'checking' | 'sign-in' | 'no-access' } = $props();
</script>

<div class="gate-page">
  <PublicHeader brand={app.brandName} links={false} signIn={false}/>
  <main id="main-content" class="gate">
    {#if mode === 'checking'}
      <p class="status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Checking access…')}</p>
    {:else}
      <section class="box card" aria-labelledby="gate-title">
        {#if mode === 'sign-in'}
          <Pill variant="fact" label={t('Staff only')}/>
          <h1 id="gate-title" class="title">{t('Operator sign-in')}</h1>
          <p class="secondary-text">{t('Sign in with your SamenKlaar staff account to open the operator dashboard.')}</p>
          {#if app.session?.configured === false}<Banner kind="info" text={t('Sign-in is temporarily unavailable. Try again later.')}/>
          {:else}<a class="btn btn-primary" href="/api/auth/login?workspace=operator">{t('Sign in as operator')}</a>{/if}
        {:else}
          <Pill variant="error" label={t('No access')}/>
          <h1 id="gate-title" class="title">{t('Operator access required')}</h1>
          <p class="secondary-text">{t('This account doesn’t have access to the operator dashboard.')}</p>
          {#if app.error}<Banner kind="error" text={t(app.error)}/>{/if}
          <div class="row"><a class="btn btn-primary" href="/">{t('Return to SamenKlaar')}</a><button type="button" class="btn btn-outline" onclick={actions.logout}>{t('Sign out')}</button></div>
        {/if}
      </section>
    {/if}
  </main>
</div>

<style>
  .gate-page { min-height: 100svh; }
  .gate { display: grid; place-items: start center; padding: 10vh 16px 48px; }
  .card { display: grid; justify-items: start; gap: 16px; width: min(520px, 100%); padding: 28px; }
  .title { font: 800 28px/1.1 var(--sk-fd); }
  .row { display: flex; flex-wrap: wrap; gap: 10px; }
  .status { display: flex; gap: 10px; align-items: center; }
</style>
