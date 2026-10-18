<script lang="ts">
  import Logo from '../../ui/Logo.svelte';
  import LanguageToggle from '../shell/LanguageToggle.svelte';
  import { t } from '../../lib/i18n.svelte';

  // Light public header: landing, sign-in and operator gates.
  let { brand, links = true, signIn = true }: { brand: string; links?: boolean; signIn?: boolean } = $props();
</script>

<header class="public-head">
  <a class="home" href="/" aria-label={t('{brand} home', { brand })}><Logo size={34} large name={brand}/></a>
  {#if links}
    <nav class="links" aria-label={t('Main navigation')}><a href="#how-it-works">{t('How it works')}</a><a href="#for-professionals">{t('For professionals')}</a></nav>
  {/if}
  <div class="end">
    <LanguageToggle tone="light"/>
    {#if signIn}<a class="btn btn-quiet sign-in" href="/api/auth/login">{t('Sign in')}</a>{/if}
  </div>
</header>

<style>
  .public-head { display: flex; align-items: center; gap: 28px; max-width: 1376px; margin: 0 auto; padding: 18px 32px; }
  .home { display: inline-flex; min-height: 44px; align-items: center; text-decoration: none; border-radius: 10px; }
  .links { display: flex; gap: 22px; }
  .links a { font-size: 14px; font-weight: 500; text-decoration: none; }
  .links a:hover { text-decoration: underline; }
  .end { display: flex; align-items: center; gap: 12px; margin-left: auto; }
  .sign-in { font-weight: 600; text-decoration: none; }
  .sign-in:hover { text-decoration: underline; }
  @media (max-width: 767px) { .links { display: none; } .public-head { padding: 12px 16px; gap: 12px; } }
  @media (max-width: 379px) { .public-head { gap: 6px; padding: 10px 12px; } .end { gap: 4px; } .home :global(.wordmark) { font-size: 17px; } .home :global(svg) { width: 28px; height: 28px; } .sign-in { padding: 0 4px; } }
</style>
