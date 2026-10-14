<script lang="ts">
  import VisitTimeFields from '../workspaces/VisitTimeFields.svelte';
  import { formatVisit, validateVisit } from '../../lib/visit-time';
  import { onMount } from 'svelte';
  import type { AppActions, AvailabilitySlot, ProfessionalProfile, ProfessionalProfileInput } from '../../lib/types';
  import PayPalPayoutSetup from '../professional/PayPalPayoutSetup.svelte';
  import { api } from '../../lib/api';
  import { currentLocale, t } from '../../lib/i18n.svelte';
  import { initials } from '../../lib/trade';

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

  // 0 intro, 1 trade & areas, 2 first service, 3 free times, 4 in review. Pure UI: every save still goes through the same API calls.
  let step = $state(0);
  let areaInput = $state('');
  let areaError = $state('');
  let otherDuration = $state(false);
  const progressSteps = $derived([t('Trade & areas'), t('Your first service'), t('When you’re free')]);
  const areas = $derived(form.serviceArea.split(',').map((item) => item.trim()).filter(Boolean));
  const tradeOptions = [
    { value: 'Plumbing', label: 'Plumbing', note: '' },
    { value: 'Carpentry', label: 'Carpentry', note: '' },
    { value: 'Electrical assessment', label: 'Electrical assessment', note: 'Assessment visits only for now' }
  ];
  const durationPresets = [30, 60, 90];
  const previewMinor = $derived(Math.round((Number(form.price) || 0) * 100) + Math.round((Number(form.callout) || 0) * 100));
  const previewExclusions = $derived(form.exclusions.split('\n').map((item) => item.trim()).filter(Boolean));

  function commitArea() {
    const value = areaInput.replace(/,/g, ' ').trim();
    if (value && !areas.includes(value)) form.serviceArea = [...areas, value].join(', ');
    areaInput = '';
  }
  function removeArea(area: string) { form.serviceArea = areas.filter((item) => item !== area).join(', '); }
  function areaKey(event: KeyboardEvent) {
    if (event.key === 'Enter' || event.key === ',') { event.preventDefault(); commitArea(); }
    else if (event.key === 'Backspace' && !areaInput && areas.length) removeArea(areas[areas.length - 1]);
  }
  function continueFromBasics(event: SubmitEvent) {
    event.preventDefault();
    commitArea();
    areaError = form.serviceArea.trim().length < 2 ? t('Add at least one area to continue') : '';
    if (!areaError) step = 2;
  }
  async function submitService(event: SubmitEvent) {
    await saveProfile(event);
    if (!error && profile) step = 3;
  }

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
      if (profile && step === 0) step = 4;
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
    const currency = /^[A-Za-z]{3}$/.test(profile?.currency ?? form.currency) ? profile?.currency ?? form.currency : 'EUR';
    return new Intl.NumberFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', { style: 'currency', currency }).format(minor / 100);
  }
</script>

{#snippet preview()}
  <aside class="onboarding-preview" aria-label={t('Customers will see')}>
    <p class="eyebrow">{t('Customers will see')}</p>
    <div class="plan-head"><span class="person-avatar" aria-hidden="true">{initials(form.displayName || '?')}</span><div><b>{form.displayName || t('Your name')}</b><span>{t(form.category)}</span></div></div>
    <h3>{form.serviceName || t('Your first service')}</h3>
    {#if form.serviceDescription}<p class="preview-text">{form.serviceDescription}</p>{/if}
    <p class="preview-price"><strong>{amountLabel(previewMinor)}</strong> <span>{t('including call-out')} · {form.durationMinutes} {t('minutes')}</span></p>
    {#if areas.length}<p class="preview-areas">{areas.join(' · ')}</p>{/if}
    {#if previewExclusions.length}<p class="helper">{t('Excluded:')} {previewExclusions.join(' · ')}</p>{/if}
  </aside>
{/snippet}

<section class="professional-onboarding">
  {#if error}<div class="error" role="alert">{error}</div>{/if}
  {#if loading}
    <div class="card loading" role="status">{t('Loading your professional profile…')}</div>
  {:else if step === 0}
    <div class="onboarding-heading">
      <span class="eyebrow">{t('PROFESSIONAL ACCOUNT')}</span>
      <h1>{t('Get jobs that are already clear')}</h1>
      <p class="lead">{t('Customers see your published service, price and available appointment times before they choose you.')}</p>
    </div>
    <ol class="intro-steps">
      {#each progressSteps as label, index}<li><span class="step-dot" aria-hidden="true">{index + 1}</span>{label}</li>{/each}
    </ol>
    <p class="helper intro-note">{t('Takes about 5 minutes. Our team reviews every profile before customers can book you.')}</p>
    <button class="primary" onclick={() => { step = 1; }}>{t('Start setup')}</button>
  {:else if step === 4}
    <div class="onboarding-heading">
      <span class="pill pill--review">{t('In review')}</span>
      <h1>{t('Your profile is in review')}</h1>
      <p class="lead">{t('Published availability is optional; you can also agree visit times with customers.')}</p>
    </div>
    <ol class="check-rows review-checklist-steps">
      <li class="check-row done"><span class="check-circle" aria-hidden="true"><svg class="icon" viewBox="0 0 24 24"><path d="m5 12 5 5 9-10"/></svg></span><div class="check-main"><b>{t('Profile sent')}</b><span>{profile ? `${profile.serviceName} · ${t(profile.category)}` : ''}</span></div></li>
      <li class="check-row active"><span class="check-circle" aria-hidden="true">2</span><div class="check-main"><b>{t('Identity and business check')}</b><span>{t('Our team reviews every profile before customers can book you.')}</span></div></li>
      <li class="check-row"><span class="check-circle" aria-hidden="true">3</span><div class="check-main"><b>{t('Payout account')}</b><span>{t('Before paid jobs')}</span></div></li>
    </ol>
    {#if profile}<p class="quiet-note">{t('Profile status:')} {t(profile.status === 'active' ? 'approved' : 'waiting for operator review')}. {t('Your published prices stay under your control.')}</p>{/if}
    <div class="actions">
      <button class="secondary" onclick={() => { step = 2; }}>{t('Edit your service')}</button>
      <button class="secondary" onclick={() => { step = 3; }}>{t('Add free times')}</button>
      <button class="secondary" onclick={() => actions.switchWorkspace('customer')}>{t('Switch to customer view')}</button>
    </div>
    {#if profile}<PayPalPayoutSetup/>{/if}
  {:else}
    <div class="onboarding-heading">
      <span class="eyebrow">{t('PROFESSIONAL ACCOUNT')}</span>
      <h1>{t('Set up your service profile')}</h1>
    </div>
    <ol class="intake-progress" aria-label={t('Request progress')}>
      {#each progressSteps as label, index (index)}<li class:done={index + 1 < step} class:current={index + 1 === step} aria-current={index + 1 === step ? 'step' : undefined}><span class="progress-bar" aria-hidden="true"></span>{label}</li>{/each}
    </ol>
    <div class="onboarding-grid">
      <div class="onboarding-main">
        {#if step === 1}
          <form class="card onboarding-card" onsubmit={continueFromBasics}>
            <div class="onboarding-section"><h2>{t('How customers find you')}</h2>
              <label>{t('Display name')}<input bind:value={form.displayName} required minlength="2" maxlength="80" autocomplete="name"/></label>
              <fieldset class="trade-cards"><legend>{t('Service category')}</legend>
                {#each tradeOptions as option (option.value)}
                  <label class="trade-card" class:selected={form.category === option.value}><input type="radio" name="trade" value={option.value} bind:group={form.category}/><span><b>{t(option.label)}</b>{#if option.note}<small>{t(option.note)}</small>{/if}</span></label>
                {/each}
              </fieldset>
              <div class="field"><label for="area-input">{t('Amsterdam service areas')}</label>
                <div class="chips-input">
                  {#each areas as area (area)}<span class="chip">{area}<button type="button" class="chip-remove" aria-label={t('Remove {area}', { area })} onclick={() => removeArea(area)}><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg></button></span>{/each}
                  <input id="area-input" bind:value={areaInput} onkeydown={areaKey} onblur={commitArea} maxlength="60" placeholder={areas.length ? '' : t('Amsterdam Centrum, De Pijp, 1012')} aria-describedby="area-help area-error" aria-invalid={areaError ? 'true' : undefined}/>
                </div>
                <small id="area-help">{t('Separate neighbourhoods or four-digit postcode areas with commas. Customers share only their area until they select you.')}</small>
                <p id="area-error" class="field-error" role="alert">{areaError}</p>
              </div>
            </div>
            <button class="primary" type="submit">{t('Continue')}</button>
          </form>
        {:else if step === 2}
          <form class="card onboarding-card" onsubmit={submitService}>
            <div class="onboarding-section"><h2>{t('One service to start')}</h2>
              <label>{t('Service name')}<input bind:value={form.serviceName} required minlength="3" maxlength="100" placeholder={t('e.g. Sink leak repair')}/></label>
              <label>{t('What the price covers')}<textarea bind:value={form.serviceDescription} maxlength="600" rows="3" placeholder={t('Describe the work this price covers.')}></textarea><small>{t('Our assistant uses this to match the right jobs to you.')}</small></label>
              <div class="form-grid"><label>{t('Service price')}<input type="number" bind:value={form.price} required min="0.01" step="0.01" inputmode="decimal"/></label><label>{t('Call-out fee')}<input type="number" bind:value={form.callout} required min="0" step="0.01" inputmode="decimal"/></label></div>
              <div class="field"><span class="field-label" id="duration-label">{t('Duration (minutes)')}</span>
                <div class="segmented" role="group" aria-labelledby="duration-label">
                  {#each durationPresets as minutes}<button type="button" class:current={!otherDuration && form.durationMinutes === minutes} aria-pressed={!otherDuration && form.durationMinutes === minutes} onclick={() => { otherDuration = false; form.durationMinutes = minutes; }}>{minutes}</button>{/each}
                  <button type="button" class:current={otherDuration || !durationPresets.includes(form.durationMinutes)} aria-pressed={otherDuration || !durationPresets.includes(form.durationMinutes)} onclick={() => { otherDuration = true; }}>{t('Other')}</button>
                </div>
                {#if otherDuration || !durationPresets.includes(form.durationMinutes)}<input type="number" bind:value={form.durationMinutes} required min="15" max="360" step="15" aria-label={t('Duration (minutes)')}/>{/if}
              </div>
              <label>{t('Currency')}<input bind:value={form.currency} required minlength="3" maxlength="3" pattern={'[A-Za-z]{3}'}/></label>
              <label>{t('Materials and exclusions')}<textarea bind:value={form.materialsPolicy} required minlength="3" maxlength="300" rows="2" placeholder={t('Materials and replacement parts are discussed before they are added.')}></textarea></label>
              <label>{t('Not included (one per line)')}<textarea bind:value={form.exclusions} rows="3" placeholder={t('Replacement parts') + '\n' + t('Wall repairs')}></textarea></label>
            </div>
            <div class="actions"><button class="secondary" type="button" onclick={() => { step = 1; }}>{t('Back')}</button><button class="primary" type="submit" disabled={saving}>{saving ? t('Saving…') : t('Save service profile')}</button></div>
          </form>
        {:else}
          <section class="card availability-card">
            <h2>{t('Offer available times')}</h2>
            <p>{t('Times use')} {timezone}. {t('A slot appears to customers only after your professional account is approved.')}</p>
            <form class="availability-form" onsubmit={addAvailability}>
              <VisitTimeFields bind:start={startLocal} bind:end={endLocal} duration={profile?.durationMinutes || 15} leadMinutes={15} prefix="onboarding-availability" bind:error={slotError} busy={addingSlot}/>
              <button class="secondary" disabled={addingSlot || !profile}>{addingSlot ? t('Adding…') : t('Add time')}</button>
            </form>
            {#if slots.length}
              <ul class="availability-list">{#each slots as slot (slot.slotId)}<li><span>{formatVisit(slot.startUtc, slot.endUtc, slot.timezone)}</span><small>{t(slot.status === 'available' ? 'Available to book' : slot.status === 'booked' ? 'Booked' : 'Unavailable')}</small></li>{/each}</ul>
            {:else}<p class="quiet-note">{t('No future times added yet.')}</p>{/if}
            <div class="actions"><button class="secondary" type="button" onclick={() => { step = 2; }}>{t('Back')}</button><button class="primary" type="button" onclick={() => { step = 4; }}>{t('Send for review')}</button></div>
            <button class="text-button" type="button" onclick={() => { step = 4; }}>{t('Skip — I’ll agree times in chat')}</button>
          </section>
        {/if}
      </div>
      <div class="onboarding-side">{@render preview()}</div>
    </div>
  {/if}
  <div class="account-actions"><button class="secondary" onclick={actions.logout}>{t('Sign out')}</button><button class="text-button" onclick={() => window.location.reload()}>{t('Refresh account status')}</button></div>
</section>
