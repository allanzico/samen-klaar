<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../../lib/api';
  import type { PayPalPayoutSetupStatus } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  let status = $state<PayPalPayoutSetupStatus | null>(null);
  let loading = $state(true);
  let starting = $state(false);
  let error = $state('');

  onMount(() => {
    const url = new URL(window.location.href);
    const returned = url.searchParams.get('paypal_onboarding') === '1';
    const merchantID = returned ? url.searchParams.get('merchantIdInPayPal') ?? undefined : undefined;
    void refresh(merchantID, returned);
  });

  async function refresh(merchantID?: string, returned = false) {
    loading = true;
    error = '';
    try {
      status = await api.paypalSellerStatus(merchantID);
    } catch (e) {
      error = e instanceof Error ? e.message : t('Could not check your PayPal payout setup.');
    } finally {
      loading = false;
      if (returned) clearReturnParameters();
    }
  }

  function clearReturnParameters() {
    const url = new URL(window.location.href);
    for (const key of ['paypal_onboarding', 'merchantId', 'merchantIdInPayPal', 'permissionsGranted', 'accountStatus', 'consentStatus', 'productIntentId', 'productIntentID', 'isEmailConfirmed', 'returnMessage', 'riskStatus']) {
      url.searchParams.delete(key);
    }
    window.history.replaceState({}, '', url.pathname + url.search + url.hash);
  }

  async function connect() {
    if (starting) return;
    starting = true;
    error = '';
    try {
      const result = await api.beginPayPalSellerOnboarding();
      window.location.assign(result.actionUrl);
    } catch (e) {
      error = e instanceof Error ? e.message : t('Could not open PayPal seller setup.');
      starting = false;
    }
  }

  function statusMessage() {
    if (!status || status.status === 'not_started') return t('Connect a PayPal account so customers can pay you through the marketplace.');
    if (status.connected) return t('Your PayPal account is connected and eligible to receive payments.');
    if (status.status === 'revoked') return t('PayPal access was removed. Reconnect your account to receive marketplace payments.');
    if (status.status === 'ineligible') return t('PayPal needs you to finish account setup before it can receive payments.');
    if (status.status === 'pending' || status.status === 'creating') return t('PayPal setup is not complete yet. Finish the steps in PayPal, then check the status again.');
    return t('Check your PayPal setup before accepting marketplace payments.');
  }
</script>

<section class="card payout-setup-card" aria-labelledby="paypal-payout-heading">
  <span class="eyebrow">{t('PROFESSIONAL PAYMENTS')}</span>
  <h2 id="paypal-payout-heading">{t('Connect PayPal payouts')}</h2>
  <p>{t('When the customer approves completed work, the payment is sent to your PayPal account. PayPal handles payment settlement; the marketplace does not hold money in escrow.')}</p>
  {#if loading}
    <p role="status">{t('Checking PayPal account status…')}</p>
  {:else}
    <p class="quiet-note" role="status">{statusMessage()}</p>
  {/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}
  <div class="actions">
    <button class="primary" disabled={loading || starting} onclick={connect}>
      {starting ? t('Opening PayPal…') : status?.connected ? t('Reconnect PayPal') : status?.status === 'pending' || status?.status === 'creating' ? t('Continue PayPal setup') : t('Connect PayPal')}
    </button>
    <button class="secondary" disabled={loading || starting} onclick={() => refresh()}>{t('Check status')}</button>
  </div>
</section>
