<script lang="ts">
  import VisitTimeFields from '../workspaces/VisitTimeFields.svelte';
  import { formatVisit, validateVisit } from '../../lib/visit-time';
  import { onMount } from 'svelte';
  import type { AppActions, AvailabilitySlot, ProfessionalProfile, ProfessionalProfileInput } from '../../lib/types';
  import PayPalPayoutSetup from '../professional/PayPalPayoutSetup.svelte';
  import { api } from '../../lib/api';
  import { currentLocale, t } from '../../lib/i18n.svelte';

  let { actions }: { actions: AppActions } = $props();

  type ProfileForm = {
    displayName: string; category: string; serviceName: string; serviceDescription: string;
    serviceArea: string; latitude: string; longitude: string; currency: string;
    price: string; callout: string; durationMinutes: number; materialsPolicy: string; exclusions: string;
  };

  let profile = $state<ProfessionalProfile | null>(null);
  let slots = $state<AvailabilitySlot[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let addingSlot = $state(false);
  let error = $state('');
  let slotError = $state('');
  let timezone = $state('Europe/Amsterdam');
  let startLocal = $state('');
  let endLocal = $state('');
  let form = $state<ProfileForm>({
    displayName: '', category: 'Plumbing', serviceName: '', serviceDescription: '', serviceArea: '',
    latitude: '', longitude: '', currency: 'EUR', price: '', callout: '0', durationMinutes: 60,
    materialsPolicy: '', exclusions: ''
  });

  onMount(() => { void load(); });

  async function load() {
    loading = true;
    error = '';
    try {
      const result = await api.professionalProfile();
      profile = result.profile;
      if (profile) {
        form = {
          displayName: profile.displayName, category: profile.category, serviceName: profile.serviceName,
          serviceDescription: profile.serviceDescription, serviceArea: profile.serviceArea,
          latitude: profile.latitude ? String(profile.latitude) : '', longitude: profile.longitude ? String(profile.longitude) : '',
          currency: profile.currency, price: (profile.priceMinor / 100).toFixed(2), callout: (profile.calloutMinor / 100).toFixed(2),
          durationMinutes: profile.durationMinutes, materialsPolicy: profile.materialsPolicy, exclusions: (profile.exclusions ?? []).join('\n')
        };
      }
      const availability = await api.professionalAvailability();
      slots = availability.slots;
      timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || timezone;
    } catch (e) {
      error = e instanceof Error ? e.message : t('Could not load the professional account.');
    } finally {
      loading = false;
    }
  }

  async function saveProfile(event: SubmitEvent) {
    event.preventDefault();
    if (saving) return;
    saving = true;
    error = '';
    try {
      const input: ProfessionalProfileInput = {
        displayName: form.displayName,
        category: form.category,
        serviceName: form.serviceName,
        serviceDescription: form.serviceDescription,
        serviceArea: form.serviceArea,
        latitude: Number(form.latitude || 0),
        longitude: Number(form.longitude || 0),
        currency: form.currency.toUpperCase(),
        priceMinor: Math.round(Number(form.price) * 100),
        calloutMinor: Math.round(Number(form.callout) * 100),
        durationMinutes: Number(form.durationMinutes),
        materialsPolicy: form.materialsPolicy,
        exclusions: form.exclusions.split('\n').map((item) => item.trim()).filter(Boolean)
      };
      const result = await api.saveProfessionalProfile(input);
      profile = result.profile;
    } catch (e) {
      error = e instanceof Error ? e.message : t('Could not save your profile.');
    } finally {
      saving = false;
    }
  }

  async function addAvailability(event: SubmitEvent) {
    event.preventDefault();
    if (addingSlot) return;
    slotError = validateVisit(startLocal, endLocal, profile?.durationMinutes || 15);
    if (slotError) return;
    addingSlot = true;
    slotError = '';
    try {
      await api.addProfessionalAvailability({
        startUtc: new Date(startLocal).toISOString(),
        endUtc: new Date(endLocal).toISOString(),
        timezone
      });
      slots = (await api.professionalAvailability()).slots;
      startLocal = '';
      endLocal = '';
    } catch (e) {
      slotError = e instanceof Error ? e.message : t('Could not add availability.');
    } finally {
      addingSlot = false;
    }
  }

  function amountLabel(minor: number) {
    return new Intl.NumberFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', { style: 'currency', currency: profile?.currency ?? form.currency }).format(minor / 100);
  }
</script>

<section class="professional-onboarding">
  <div class="onboarding-heading">
    <span class="eyebrow">{t('PROFESSIONAL ACCOUNT')}</span>
    <h1>{t('Set up your')}<br/>{t('service profile.')}</h1>
    <p class="lead">{t('Customers see your published service, price and available appointment times before they choose you.')}</p>
  </div>
  {#if error}<div class="error" role="alert">{error}</div>{/if}
  {#if loading}
    <div class="card loading" role="status">{t('Loading your professional profile…')}</div>
  {:else}
    <form class="card onboarding-card" onsubmit={saveProfile}>
      <div class="onboarding-section"><span class="eyebrow">{t('PUBLIC PROFILE')}</span><h2>{t('How customers find you')}</h2>
        <label>{t('Display name')}<input bind:value={form.displayName} required minlength="2" maxlength="80" autocomplete="name"/></label>
        <label>{t('Service category')}<select bind:value={form.category}><option value="Plumbing">{t('Plumbing')}</option><option value="Carpentry">{t('Carpentry')}</option><option value="Electrical assessment">{t('Electrical assessment')}</option></select></label>
        <label>{t('Amsterdam service areas')}<input bind:value={form.serviceArea} required minlength="2" maxlength="120" placeholder={t('Amsterdam Centrum, De Pijp, 1012')}/><small>{t('Separate neighbourhoods or four-digit postcode areas with commas. Customers share only their area until they select you.')}</small></label>
      </div>
      <div class="onboarding-section"><span class="eyebrow">{t('YOUR CATALOG')}</span><h2>{t('One service to start')}</h2>
        <label>{t('Service name')}<input bind:value={form.serviceName} required minlength="3" maxlength="100" placeholder={t('e.g. Sink leak repair')}/></label>
        <label>{t('What this service includes')}<textarea bind:value={form.serviceDescription} maxlength="600" rows="3" placeholder={t('Describe the work this price covers.')}></textarea></label>
        <div class="form-grid"><label>{t('Currency')}<input bind:value={form.currency} required minlength="3" maxlength="3" pattern={'[A-Za-z]{3}'}/></label><label>{t('Duration (minutes)')}<input type="number" bind:value={form.durationMinutes} required min="15" max="360" step="15"/></label></div>
        <div class="form-grid"><label>{t('Service price')}<input type="number" bind:value={form.price} required min="0.01" step="0.01"/></label><label>{t('Call-out fee')}<input type="number" bind:value={form.callout} required min="0" step="0.01"/></label></div>
        <label>{t('Materials and exclusions')}<textarea bind:value={form.materialsPolicy} required minlength="3" maxlength="300" rows="2" placeholder={t('Materials and replacement parts are discussed before they are added.')}></textarea></label>
        <label>{t('Not included (one per line)')}<textarea bind:value={form.exclusions} rows="3" placeholder={t('Replacement parts') + '\n' + t('Wall repairs')}></textarea></label>
      </div>
      <button class="primary" type="submit" disabled={saving}>{saving ? t('Saving…') : profile ? t('Save service profile') : t('Submit service profile')}</button>
      {#if profile}<p class="quiet-note">{t('Profile status:')} {t(profile.status === 'active' ? 'approved' : 'waiting for operator review')}. {t('Your published prices stay under your control.')}</p>{/if}
    </form>

    {#if profile}
      <PayPalPayoutSetup/>
    {:else}
      <section class="card payout-setup-card"><span class="eyebrow">{t('PROFESSIONAL PAYMENTS')}</span><h2>{t('Connect PayPal payouts')}</h2><p>{t('Save your professional profile first. Then connect your PayPal account so customers can pay you through the marketplace.')}</p></section>
    {/if}

    <section class="card availability-card">
      <span class="eyebrow">{t('APPOINTMENTS')}</span><h2>{t('Offer available times')}</h2>
      <p>{t('Times use')} {timezone}. {t('A slot appears to customers only after your professional account is approved.')}</p>
      <form class="availability-form" onsubmit={addAvailability}>
<VisitTimeFields bind:start={startLocal} bind:end={endLocal} duration={profile?.durationMinutes || 15} leadMinutes={15} prefix="onboarding-availability" bind:error={slotError} busy={addingSlot}/>
        <button disabled={addingSlot || !profile}>{addingSlot ? t('Adding…') : t('Add time')}</button>
      </form>
      {#if slots.length}
        <ul class="availability-list">{#each slots as slot (slot.slotId)}<li><span>{formatVisit(slot.startUtc, slot.endUtc, slot.timezone)}</span><small>{t(slot.status === 'available' ? 'Available to book' : slot.status === 'booked' ? 'Booked' : 'Unavailable')}</small></li>{/each}</ul>
      {:else}<p class="quiet-note">{t('No future times added yet.')}</p>{/if}
      {#if profile?.status === 'pending'}<p class="quiet-note">{t('Your profile is ready for operator review. Published availability is optional; you can also agree visit times with customers.')}</p>{/if}
    </section>
  {/if}
  <div class="account-actions"><button class="secondary" onclick={actions.logout}>{t('Sign out')}</button><button class="text-button" onclick={() => window.location.reload()}>{t('Refresh account status')}</button></div>
</section>
