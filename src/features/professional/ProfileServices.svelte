<script lang="ts">
import ServiceEvidenceEditor from './ServiceEvidenceEditor.svelte';
  import { onMount } from 'svelte';
  import Pill from '../../ui/Pill.svelte';
  import Banner from '../../ui/Banner.svelte';
  import PayPalPayouts from './PayPalPayouts.svelte';
  import ProfessionalProfileEditor from './ProfessionalProfileEditor.svelte';
  import { api } from '../../lib/api';
  import type { PayPalPayoutSetupStatus, ProfessionalProfile } from '../../lib/types';
  import { money } from '../../lib/format';
  import { initials } from '../../lib/trade';
  import { t } from '../../lib/i18n.svelte';

  let profile = $state<ProfessionalProfile | null>(null);
  let loading = $state(true);
  let error = $state('');
  let paypal = $state<PayPalPayoutSetupStatus | null>(null);
  const areas = $derived((profile?.serviceArea ?? '').split(',').map((a) => a.trim()).filter(Boolean));

  onMount(async () => {
    try { profile = (await api.professionalProfile()).profile; }
    catch (e) { error = e instanceof Error ? e.message : t('Could not load your profile.'); }
    finally { loading = false; }
  });
</script>

<div class="profile">
  <h1 class="page-title profile-title">{t('Profile and services')}</h1>
  {#if paypal?.status === 'revoked'}<Banner kind="error" title={t('PayPal access was removed')} text={t('Customers can’t pay you until you reconnect PayPal payouts.')}/>{/if}
  {#if error}<Banner kind="error" text={t(error)}/>{/if}
  {#if loading}<p class="status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Loading your profile…')}</p>
  {:else if profile}
    <div class="grid">
      <div class="col">
        <section class="box card" aria-labelledby="profile-card-title">
          <div class="who"><span class="avatar avatar-48" aria-hidden="true">{initials(profile.displayName)}</span><div><h2 id="profile-card-title" class="small-title">{profile.displayName}</h2><p class="secondary-text">{t(profile.category)}</p></div></div>
          {#if areas.length}<div class="areas">{#each areas as area}<span class="pill pill--fact">{area}</span>{/each}</div>{/if}
        </section>
        <section class="box card" aria-labelledby="service-card-title">
          <div class="card-head"><h2 id="service-card-title" class="small-title">{profile.serviceName}</h2><Pill variant={profile.status === 'active' ? 'progress' : 'pending'} label={profile.status === 'active' ? t('Live') : t('In review')}/></div>
          {#if profile.serviceDescription}<p class="secondary-text">{profile.serviceDescription}</p>{/if}
          <dl class="ledger">
            <div><dt>{t('Service price')}</dt><dd>{money(profile.priceMinor, profile.currency)}</dd></div>
            <div><dt>{t('Call-out fee')}</dt><dd>{money(profile.calloutMinor, profile.currency)}</dd></div>
            <div><dt>{t('Visit length')}</dt><dd>{t('{minutes} minutes', { minutes: profile.durationMinutes })}</dd></div>
            <div class="ledger-total"><dt>{t('Customers see')}</dt><dd>{money(profile.priceMinor + profile.calloutMinor, profile.currency)}</dd></div>
          </dl>
          <details class="disclosure"><summary>{t('Materials and what isn’t included')}</summary><p>{profile.materialsPolicy}</p>{#if profile.exclusions?.length}<p>{t('Not included: {list}', { list: profile.exclusions.join(', ') })}</p>{/if}</details>
          <ProfessionalProfileEditor {profile} onSaved={(saved) => { profile = saved; }}/>
          <ServiceEvidenceEditor/>
        </section>
      </div>
      <div class="col"><PayPalPayouts onStatus={(s) => { paypal = s; }}/></div>
    </div>
  {:else}
    <div class="empty"><h2>{t('No service profile yet')}</h2><p>{t('Your service profile could not be found. Refresh the page or contact our team.')}</p></div>
  {/if}
</div>

<style>
  .profile { display: grid; gap: 24px; max-width: 1120px; margin: 0 auto; padding: 56px 48px 72px; }
  .profile-title { font-size: 36px; }
  .status { display: flex; gap: 10px; align-items: center; }
  .grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; align-items: start; }
  .col { display: grid; gap: 20px; }
  .card { display: grid; gap: 14px; padding: 20px 22px; }
  .who { display: flex; align-items: center; gap: 12px; }
  .areas { display: flex; flex-wrap: wrap; gap: 6px; }
  .card-head { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 10px; }
  @media (max-width: 900px) { .grid { grid-template-columns: 1fr; } }
  @media (max-width: 639px) { .profile { padding: 28px 16px 40px; } .profile-title { font-size: 30px; } .card { padding: 18px 16px; } }
</style>
