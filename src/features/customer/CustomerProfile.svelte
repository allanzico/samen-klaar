<script lang="ts">
  import { onMount } from 'svelte';
  import Banner from '../../ui/Banner.svelte';
  import LocationSearch from '../../ui/LocationSearch.svelte';
  import Pill from '../../ui/Pill.svelte';
  import { api } from '../../lib/api';
  import type { AppState, Location } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';
  import { showToast } from '../../ui/toast-state.svelte';

  let { app }: { app: AppState } = $props();
  let name = $state('');
  let email = $state('');
  let phone = $state('');
  let locationText = $state('');
  let location = $state<Location | null>(null);
  let loading = $state(true);
  let loaded = $state(false);
  let saving = $state(false);
  let error = $state('');
  onMount(() => { void load(); });
  async function load() {
    loading = true; error = '';
    try {
      const profile = await api.customerProfile();
      name = profile.displayName; email = profile.email; phone = profile.phone;
      location = profile.location ?? null; locationText = location?.address || location?.label || '';
      loaded = true;
    } catch (e) { error = e instanceof Error ? e.message : t('Could not load your profile.'); }
    finally { loading = false; }
  }
  async function save(event: SubmitEvent) {
    event.preventDefault(); if (saving) return;
    if (locationText.trim() && !location) { error = t('Choose your location from the suggestions.'); return; }
    saving = true; error = '';
    try {
      const profile = await api.saveCustomerProfile({ displayName: name.trim(), phone: phone.trim(), location });
      name = profile.displayName; phone = profile.phone;
      location = profile.location ?? null; locationText = location?.address || location?.label || '';
      if (app.session?.account) app.session.account.displayName = profile.displayName;
      showToast(t('Profile saved'));
    } catch (e) { error = e instanceof Error ? e.message : t('Could not save your profile.'); }
    finally { saving = false; }
  }
</script>

<div class="profile-page">
  <header><h1 class="page-title">{t('Your profile')}</h1><p class="lead">{t('Your contact details and usual location.')}</p></header>
  {#if error}<Banner kind="error" text={t(error)}/>{/if}
  {#if loading}<p role="status">{t('Loading your profile…')}</p>
  {:else if !loaded}<button type="button" class="btn btn-outline" onclick={() => void load()}>{t('Try again')}</button>
  {:else}
    <form class="box profile-form" onsubmit={save}>
      <h2 class="section-title">{t('Your details')}</h2>
      <div class="field"><label for="customer-name">{t('Display name')}</label><input id="customer-name" class="input" bind:value={name} autocomplete="name" minlength="2" maxlength="80" required disabled={saving}/></div>
      <div class="field"><label for="customer-email">{t('Email')}</label><input id="customer-email" class="input" value={email} type="email" readonly/><p class="field-help">{t('This is the email you use to sign in.')}</p></div>
      <div class="field"><label for="customer-phone">{t('Phone number (optional)')}</label><input id="customer-phone" class="input" type="tel" autocomplete="tel" maxlength="30" bind:value={phone} disabled={saving}/></div>
      <LocationSearch id="customer-location" label={t('Usual address, postcode or city (optional)')} bind:value={locationText} bind:selected={location} disabled={saving} help={t('Your location stays private. Existing requests keep the address you agreed for that job.')}/>
      {#if locationText}<button type="button" class="btn btn-quiet" disabled={saving} onclick={() => { location = null; locationText = ''; }}>{t('Clear saved location')}</button>{/if}
      <button class="btn btn-primary" disabled={saving}>{saving ? t('Saving…') : t('Save profile')}</button>
    </form>
    <section class="box payment-account" aria-labelledby="customer-payment-title">
      <div class="row"><h2 id="customer-payment-title" class="section-title">{t('PayPal account')}</h2><Pill variant="neutral" label={t('Not connected')}/></div>
      <p id="wallet-unavailable">{t('Saving a PayPal account is not available yet. You can still use PayPal at checkout.')}</p>
      <p class="secondary-text">{t('You approve each payment for the agreed job amount.')}</p>
      <button type="button" class="btn btn-outline" disabled aria-describedby="wallet-unavailable">{t('Connect PayPal')}</button>
    </section>
  {/if}
</div>

<style>
  .profile-page { display: grid; gap: 24px; max-width: 780px; margin: 0 auto; padding: 56px 32px 72px; }
  .lead { margin-top: 12px; color: var(--sk-muted); }
  .profile-form, .payment-account { display: grid; gap: 18px; padding: 24px; }
  .profile-form > .btn, .payment-account > .btn { justify-self: start; }
  .row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  @media(max-width:639px) { .profile-page { padding: 28px 16px 40px; } .profile-form, .payment-account { padding: 18px 16px; } }
</style>
