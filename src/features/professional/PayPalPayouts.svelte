<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import Banner from '../../ui/Banner.svelte';
  import { api } from '../../lib/api';
  import type { PayPalPayoutSetupStatus } from '../../lib/types';
  import type { PillVariant } from '../../lib/status-ui';
  import { t } from '../../lib/i18n.svelte';

  // I01–I08: the PayPal payouts card, rendered from the seller status the API reports.
  let { layout = 'card', onStatus }: { layout?: 'card' | 'page'; onStatus?: (status: PayPalPayoutSetupStatus | null) => void } = $props();
  let status = $state<PayPalPayoutSetupStatus | null>(null);
  let loading = $state(true);
  let starting = $state(false);
  let returned = $state(false);
  let returnedMerchantID = $state<string | undefined>(undefined);
  let error = $state('');

  onMount(() => {
    const url = new URL(window.location.href);
    returned = url.searchParams.get('paypal_onboarding') === '1';
    returnedMerchantID = returned ? url.searchParams.get('merchantIdInPayPal') ?? undefined : undefined;
    void refresh();
  });

  async function refresh() {
    loading = true;
    error = '';
    try {
      status = await api.paypalSellerStatus(returnedMerchantID);
      onStatus?.(status);
      // Keep the return reference until the server has checked and saved it.
      // A failed request must remain retryable, including after a page reload.
      if (returned) {
        const url = new URL(window.location.href);
        for (const key of ['paypal_onboarding', 'merchantId', 'merchantIdInPayPal', 'permissionsGranted', 'accountStatus', 'consentStatus', 'productIntentId', 'productIntentID', 'isEmailConfirmed', 'returnMessage', 'riskStatus']) url.searchParams.delete(key);
        window.history.replaceState({}, '', url.pathname + url.search + url.hash);
        returned = false;
        returnedMerchantID = undefined;
      }
    }
    catch (e) { error = e instanceof Error ? e.message : t('We couldn’t check your PayPal status.'); }
    finally { loading = false; }
  }
  async function connect() {
    if (starting) return;
    starting = true;
    error = '';
    try { window.location.assign((await api.beginPayPalSellerOnboarding()).actionUrl); }
    catch (e) {
      error = e instanceof Error ? e.message : t('We couldn’t open PayPal. Nothing changed. Try again in a moment.');
      starting = false;
    }
  }

  const awaitingEmailConfirmation = $derived(status?.status === 'pending' && status.paymentsReceivable && !status.primaryEmailConfirmed);
  type State = { tag: PillVariant; label: string; title: string; text: string; action?: string };
  const card = $derived.by((): State => {
    const s = status?.status ?? 'not_started';
    if (status?.connected && status.platformFeeReady === false) return { tag: 'warn', label: t('Update permissions'), title: t('Enable the platform fee in PayPal'), text: t('Your account is connected. Update PayPal permissions to receive payments for new jobs with the 5% platform fee.'), action: t('Update PayPal permissions') };
    if (status?.connected) return { tag: 'done', label: t('Connected and eligible'), title: t('PayPal is connected'), text: t('When a customer approves the work and pays, the money goes to this PayPal account.') };
    if (awaitingEmailConfirmation) return { tag: 'warn', label: t('Confirm email'), title: t('Confirm your PayPal email'), text: t('PayPal still needs you to confirm your email address. Confirm it in PayPal, then choose Check status.') };
    if (s === 'revoked') return { tag: 'error', label: t('Access removed'), title: t('Reconnect PayPal payouts'), text: t('PayPal access was removed. Reconnect to receive payments for your jobs.'), action: t('Reconnect PayPal') };
    if (s === 'ineligible') return { tag: 'error', label: t('Can’t receive payments'), title: t('PayPal can’t take payments for you yet'), text: t('PayPal needs more from you before it can receive payments.'), action: t('Continue PayPal setup') };
    if (s === 'pending' || s === 'creating') return { tag: 'warn', label: t('Not finished'), title: t('Finish your PayPal setup'), text: t('A few steps in PayPal are still open. Finish them, then check the status here.'), action: t('Continue PayPal setup') };
    return { tag: 'neutral', label: t('Not connected'), title: t('Get paid through PayPal'), text: t('Customers pay through PayPal after they approve your work. Connect before your first paid job.'), action: t('Connect PayPal') };
  });
  const checks = $derived([
    { done: !!status?.paymentsReceivable, text: t('Your account can receive payments') },
    { done: !!status?.primaryEmailConfirmed, text: t('Your email is confirmed with PayPal') },
    { done: !!status?.connected, text: t('SamenKlaar may take payments and refunds for your jobs') },
    { done: !!status?.platformFeeReady, text: t('PayPal may deduct SamenKlaar’s 5% platform fee') }
  ]);
</script>

<section class="payouts" class:page={layout === 'page'} aria-labelledby="payouts-title">
  {#if layout === 'page'}
    <span class="state-icon" class:connected={status?.connected} aria-hidden="true"><Icon name={status?.connected ? 'check' : 'euro'} size={28}/></span>
    <h1 id="payouts-title" class="page-title">{returned && loading ? t('Welcome back') : card.title}</h1>
    <p class="lead">{card.text}</p>
  {/if}
  <div class="box card">
    <div class="card-head">
      {#if layout === 'page'}<h2 class="small-title">{t('PayPal payouts')}</h2>{:else}<h2 id="payouts-title" class="small-title">{t('PayPal payouts')}</h2>{/if}
      {#if loading}<Pill variant="neutral" label={t('Checking')}/>{:else}<Pill variant={card.tag} label={card.label}/>{/if}
    </div>
    {#if layout === 'card' && !loading}<p class="secondary-text">{card.text}</p>{/if}
    {#if !loading && !status?.connected}
      <p class="secondary-text">{t('This connects an account to receive payment for your work. Customers use their own PayPal account at checkout and do not need this setup.')}</p>
      <details class="disclosure">
        <summary>{t('Which PayPal account should I use?')}</summary>
        <p>{t('Already have PayPal? Choose Log in on the PayPal page to connect your existing account.')}</p>
        <p>{t('Eligible individual sellers in the Netherlands can use a personal PayPal account. Businesses can use a business account. If PayPal asks you to create a company account or upgrade, contact our team before continuing.')}</p>
      </details>
    {/if}
    {#if starting}<Banner kind="info" text={t('Opening PayPal. You’ll finish on PayPal, then come back here.')}/>{/if}
    {#if error}<Banner kind="error" text={t(error)}/>{/if}
    {#if loading}
      <p class="status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Checking PayPal account status…')}</p>
    {:else}
      <ul class="checks">
        {#each checks as check (check.text)}<li class:done={check.done}><span class="mark" aria-hidden="true">{#if check.done}<Icon name="check" size={13}/>{/if}</span>{check.text}<span class="visually-hidden">{check.done ? t('Done') : t('Not yet')}</span></li>{/each}
      </ul>
    {/if}
    <p class="helper">{t('SamenKlaar deducts 5% of the agreed job total, including call-out, from your payment. PayPal processing fees are separate.')}</p>
    <div class="actions">
      {#if card.action}<button type="button" class="btn btn-primary" disabled={loading || starting} aria-busy={starting ? 'true' : undefined} onclick={connect}>{starting ? t('Opening PayPal…') : card.action}</button>{/if}
      <button type="button" class="btn btn-outline" disabled={loading || starting} onclick={() => refresh()}>{t('Check status')}</button>
      {#if status?.connected || awaitingEmailConfirmation}<a class="btn btn-quiet" href="https://www.sandbox.paypal.com/" target="_blank" rel="noopener noreferrer">{t('Open PayPal')}<Icon name="external" size={14}/></a>{/if}
    </div>
    {#if status?.connected}
      <details class="disclosure">
        <summary>{t('Use a different PayPal account')}</summary>
        <p>{t('First remove SamenKlaar’s access in PayPal, under Third-party API permissions. Then check the status here and reconnect with the other account.')}</p>
      </details>
    {/if}
  </div>
</section>

<style>
  .payouts { display: grid; gap: 16px; }
  .page { justify-items: start; gap: 14px; }
  .state-icon { display: grid; place-items: center; width: 64px; height: 64px; border-radius: 50%; background: var(--sk-soft); color: var(--sk-brand); }
  .state-icon.connected { background: var(--sk-brand); color: var(--sk-on-brand); }
  .page .page-title { font-size: clamp(28px, 3vw, 36px); }
  .lead { font-size: 15px; color: var(--sk-muted); max-width: 56ch; }
  .card { display: grid; gap: 14px; width: 100%; padding: 20px 22px; }
  .card-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
  .status { display: flex; align-items: center; gap: 10px; }
  .checks { display: grid; }
  .checks li { display: flex; align-items: center; gap: 12px; padding: 10px 0; font-size: 14px; color: var(--sk-muted); }
  .checks li + li { border-top: 0.5px solid var(--sk-line); }
  .checks li.done { color: var(--sk-ink); }
  .mark { display: grid; place-items: center; flex: 0 0 auto; width: 24px; height: 24px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--sk-field); }
  .done .mark { background: var(--sk-soft); box-shadow: none; }
  .actions { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; }
  @media (max-width: 639px) { .actions .btn:not(.btn-quiet) { width: 100%; } .card { padding: 18px 16px; } }
</style>
