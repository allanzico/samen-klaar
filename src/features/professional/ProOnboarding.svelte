<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from '../../ui/Icon.svelte';
  import LocationSearch from '../../ui/LocationSearch.svelte';
  import Pill from '../../ui/Pill.svelte';
  import Banner from '../../ui/Banner.svelte';
  import FreeTimes from './FreeTimes.svelte';
  import PayPalPayouts from './PayPalPayouts.svelte';
  import type { AppActions, Location, ProfessionalProfile, ProfessionalProfileInput } from '../../lib/types';
  import { api } from '../../lib/api';
  import { money } from '../../lib/format';
  import { initials } from '../../lib/trade';
  import { t } from '../../lib/i18n.svelte';

  // E01–E05. Every save is the same profile API call as before; the steps are presentation only.
  let { actions }: { actions: AppActions } = $props();

  type Form = { displayName: string; category: string; serviceName: string; serviceDescription: string; serviceArea: string; latitude: string; longitude: string; currency: string; price: string; callout: string; durationMinutes: number; materialsPolicy: string; exclusions: string };
  let profile = $state<ProfessionalProfile | null>(null);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let step = $state(0);
  let freeTimes = $state<FreeTimes>();
  let baseLocation = $state<Location | null>(null);
  let baseText = $state('');
  let travelRadius = $state(25);
  let areaError = $state('');
  let otherDuration = $state(false);
  let form = $state<Form>({ displayName: '', category: 'Plumbing', serviceName: '', serviceDescription: '', serviceArea: '', latitude: '', longitude: '', currency: 'EUR', price: '', callout: '0', durationMinutes: 60, materialsPolicy: '', exclusions: '' });

  const parts = ['Trade and areas', 'Your first service', 'When you’re free'];
  const trades = [
    { value: 'Plumbing', note: 'Bookable jobs' },
    { value: 'Carpentry', note: 'Bookable jobs' },
    { value: 'Electrical assessment', note: 'Assessment visits only for now' }
  ];
  const areas = $derived(form.serviceArea.split(',').map((a) => a.trim()).filter(Boolean));
  const previewMinor = $derived(Math.round((Number(form.price) || 0) * 100) + Math.round((Number(form.callout) || 0) * 100));
  const presets = [30, 60, 90];

  onMount(async () => {
    try {
      profile = (await api.professionalProfile()).profile;
      if (profile) {
        form = { displayName: profile.displayName, category: profile.category, serviceName: profile.serviceName, serviceDescription: profile.serviceDescription, serviceArea: profile.serviceArea, latitude: profile.latitude ? String(profile.latitude) : '', longitude: profile.longitude ? String(profile.longitude) : '', currency: profile.currency, price: (profile.priceMinor / 100).toFixed(2), callout: (profile.calloutMinor / 100).toFixed(2), durationMinutes: profile.durationMinutes, materialsPolicy: profile.materialsPolicy, exclusions: (profile.exclusions ?? []).join('\n') };
        baseLocation = profile.baseLocation ?? null; baseText = baseLocation?.address || baseLocation?.label || ''; travelRadius = profile.travelRadiusKm || 25;
        otherDuration = !presets.includes(profile.durationMinutes);
        step = 4;
      }
    } catch (e) { error = e instanceof Error ? e.message : t('Could not load the professional account.'); }
    finally { loading = false; }
  });

  function continueBasics(event: SubmitEvent) {
    event.preventDefault();
    areaError = baseLocation ? '' : t('Choose a base address or postcode from the suggestions.');
    if (!areaError) step = 2;
  }
  async function saveService(event: SubmitEvent) {
    event.preventDefault();
    if (saving) return;
    saving = true;
    error = '';
    try {
      const input: ProfessionalProfileInput = {
        displayName: form.displayName, category: form.category, serviceName: form.serviceName, serviceDescription: form.serviceDescription, serviceArea: baseLocation?.label || form.serviceArea, baseLocation, travelRadiusKm: travelRadius,
        latitude: baseLocation?.latitude ?? Number(form.latitude || 0), longitude: baseLocation?.longitude ?? Number(form.longitude || 0), currency: form.currency.toUpperCase(),
        priceMinor: Math.round(Number(form.price) * 100), calloutMinor: Math.round(Number(form.callout) * 100), durationMinutes: Number(form.durationMinutes),
        materialsPolicy: form.materialsPolicy, exclusions: form.exclusions.split('\n').map((x) => x.trim()).filter(Boolean)
      };
      profile = (await api.saveProfessionalProfile(input)).profile;
      step = 3;
    } catch (e) { error = e instanceof Error ? e.message : t('Could not save your profile.'); }
    finally { saving = false; }
  }
</script>

{#snippet preview()}
  <aside class="box preview" aria-label={t('Customers will see')}>
    <p class="eyebrow small">{t('Customers will see')}</p>
    <div class="who"><span class="avatar avatar-44" aria-hidden="true">{initials(form.displayName || '?')}</span><div><b>{form.displayName || t('Your name')}</b><small>{t(form.category)}</small></div></div>
    <h3 class="small-title">{form.serviceName || t('Your first service')}</h3>
    {#if form.serviceDescription}<p class="secondary-text">{form.serviceDescription}</p>{/if}
    <p class="amount price">{money(previewMinor, form.currency)}</p>
    <p class="meta">{t('including call-out, {minutes} minutes', { minutes: form.durationMinutes })}</p>
    {#if baseLocation}<p class="meta">{t('Within {radius} km of {area}', { radius: travelRadius, area: baseLocation.label })}</p>{:else if areas.length}<div class="area-tags">{#each areas as area}<span class="pill pill--fact">{area}</span>{/each}</div>{/if}
  </aside>
{/snippet}

<div class="onboarding">
  {#if error}<Banner kind="error" text={t(error)}/>{/if}
  {#if loading}
    <p class="status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Loading your professional profile…')}</p>
  {:else if step === 0}
    <section class="intro" aria-labelledby="intro-title">
      <div class="intro-copy">
        <Pill variant="fact" label={t('For professionals')}/>
        <h1 id="intro-title" class="page-title">{t('Get jobs that are already clear')}</h1>
        <p class="lead">{t('Requests arrive described, with your published price. You agree the time, do the work, and get paid through PayPal.')}</p>
        <ol class="intro-steps">{#each parts as part, i}<li><span class="num" aria-hidden="true">{i + 1}</span>{t(part)}</li>{/each}</ol>
        <p class="secondary-text">{t('Our team reviews every profile before customers can book you.')}</p>
        <div class="row"><button type="button" class="btn btn-primary btn-strong" onclick={() => { step = 1; }}>{t('Start setup')}</button><button type="button" class="btn btn-quiet" onclick={() => actions.switchWorkspace('customer')}>{t('Back to my requests')}</button></div>
      </div>
    </section>
  {:else if step === 4}
    <section class="review" aria-labelledby="review-title">
      <Pill variant="pending" label={t('In review')}/>
      <h1 id="review-title" class="page-title">{t('Your profile is in review')}</h1>
      <p class="lead">{t('We check every profile before customers can book you. PayPal payouts need to be connected before approval.')}</p>
      <div class="review-grid">
        <div class="col">
          <section class="box card" aria-labelledby="checklist-title">
            <h2 id="checklist-title" class="small-title">{t('Your review')}</h2>
            <ol class="checks">
              <li class="done"><span class="mark" aria-hidden="true"><Icon name="check" size={13}/></span><span><b>{t('Profile sent')}</b><small>{profile ? `${profile.serviceName} · ${t(profile.category)}` : ''}</small></span></li>
              <li><span class="mark" aria-hidden="true"><Icon name="clock" size={13}/></span><span><b>{t('Identity and business check')}</b><small>{t('Our team does this. No action needed.')}</small></span></li>
              <li><span class="mark" aria-hidden="true">3</span><span><b>{t('Payout account')}</b><small>{t('Connect PayPal below.')}</small></span></li>
            </ol>
            <div class="row">
              <button type="button" class="btn btn-outline btn-sm" onclick={() => { step = 1; }}>{t('Edit your service')}</button>
              <button type="button" class="btn btn-outline btn-sm" onclick={() => { step = 3; }}>{t('Add free times')}</button>
            </div>
          </section>
          <PayPalPayouts/>
        </div>
        {@render preview()}
      </div>
      <div class="row foot"><button type="button" class="btn btn-quiet" onclick={() => window.location.reload()}>{t('Refresh account status')}</button><button type="button" class="btn btn-quiet" onclick={() => actions.switchWorkspace('customer')}>{t('Switch to my requests')}</button></div>
    </section>
  {:else}
    <h1 class="page-title setup-title">{t('Set up your service profile')}</h1>
    <ol class="progress" aria-label={t('Setup progress')}>
      {#each parts as part, i}
        <li class:done={i + 1 < step} class:current={i + 1 === step} aria-current={i + 1 === step ? 'step' : undefined}><span class="bar" aria-hidden="true"></span><span class="label">{#if i + 1 < step}<Icon name="check" size={14}/>{/if}{t(part)}</span></li>
      {/each}
    </ol>
    <div class="setup-grid">
      <div class="col">
        {#if step === 1}
          <form class="box card form" onsubmit={continueBasics}>
            <h2 class="section-title">{t('How customers find you')}</h2>
            <div class="field"><label for="display-name">{t('Display name')}</label><input id="display-name" class="input" bind:value={form.displayName} required minlength="2" maxlength="80" autocomplete="name"/></div>
            <fieldset class="field"><legend class="field-label">{t('Service category')}</legend>
              <div class="trades">{#each trades as trade (trade.value)}<label class="radio-card"><input type="radio" name="trade" value={trade.value} bind:group={form.category}/><span class="radio-dot" aria-hidden="true"></span><span><b>{t(trade.value)}</b><small>{t(trade.note)}</small></span></label>{/each}</div>
            </fieldset>
            <LocationSearch id="professional-base" label={t('Your base address or postcode')} purpose="base" bind:value={baseText} bind:selected={baseLocation} help={t('Your base stays private. We use it to find jobs within your travel radius.')}/>
            <div class="field"><label for="travel-radius">{t('How far will you travel?')}</label><select id="travel-radius" class="input" bind:value={travelRadius}>{#if ![5, 10, 15, 25, 50, 75, 100].includes(travelRadius)}<option value={travelRadius}>{travelRadius} km</option>{/if}{#each [5, 10, 15, 25, 50, 75, 100] as radius}<option value={radius}>{radius} km</option>{/each}</select></div>
            {#if areaError}<p class="field-error" role="alert">{areaError}</p>{/if}
            <div class="row"><button class="btn btn-primary">{t('Continue')}</button></div>
          </form>
        {:else if step === 2}
          <form class="box card form" onsubmit={saveService}>
            <h2 class="section-title">{t('One service to start')}</h2>
            <div class="field"><label for="service-name">{t('Service name')}</label><input id="service-name" class="input" bind:value={form.serviceName} required minlength="3" maxlength="100" placeholder={t('For example Door and window repair')}/></div>
            <div class="field"><label for="service-covers">{t('What the price covers')}</label><textarea id="service-covers" class="textarea" rows="3" maxlength="600" bind:value={form.serviceDescription}></textarea><p class="field-help">{t('Our assistant uses this to match the right jobs to you.')}</p></div>
            <div class="pair">
              <div class="field"><label for="service-price">{t('Service price')}</label><div class="money-input"><span aria-hidden="true">€</span><input id="service-price" type="number" inputmode="decimal" min="0.01" step="0.01" required bind:value={form.price}/></div></div>
              <div class="field"><label for="service-callout">{t('Call-out fee')}</label><div class="money-input"><span aria-hidden="true">€</span><input id="service-callout" type="number" inputmode="decimal" min="0" step="0.01" required bind:value={form.callout}/></div></div>
            </div>
            <div class="field"><span class="field-label" id="duration-label">{t('Duration (minutes)')}</span>
              <div class="segmented" role="radiogroup" aria-labelledby="duration-label">
                {#each presets as minutes}<button type="button" role="radio" aria-checked={!otherDuration && form.durationMinutes === minutes} onclick={() => { otherDuration = false; form.durationMinutes = minutes; }}>{minutes}</button>{/each}
                <button type="button" role="radio" aria-checked={otherDuration} onclick={() => { otherDuration = true; }}>{t('Other')}</button>
              </div>
              {#if otherDuration}<input class="input short" type="number" min="15" max="360" step="15" bind:value={form.durationMinutes} aria-label={t('Duration (minutes)')}/>{/if}
            </div>
            <div class="field"><label for="materials">{t('Materials')}</label><textarea id="materials" class="textarea" rows="2" required minlength="3" maxlength="300" bind:value={form.materialsPolicy} placeholder={t('Materials and replacement parts are discussed before they are added.')}></textarea></div>
            <div class="field"><label for="exclusions">{t('Not included (one per line)')}</label><textarea id="exclusions" class="textarea" rows="3" bind:value={form.exclusions}></textarea></div>
            <div class="row"><button type="button" class="btn btn-outline" onclick={() => { step = 1; }}>{t('Back')}</button><button class="btn btn-primary" disabled={saving} aria-busy={saving ? 'true' : undefined}>{saving ? t('Saving…') : t('Save service profile')}</button></div>
          </form>
        {:else}
          <section class="box card" aria-labelledby="free-title">
            <h2 id="free-title" class="section-title">{t('When you’re free')}</h2>
            <p class="secondary-text">{t('Add times customers can book. They appear once your profile is approved. You can also agree times in chat.')}</p>
            <FreeTimes compact bind:this={freeTimes}/>
            <div class="row"><button type="button" class="btn btn-outline" onclick={() => { step = 2; }}>{t('Back')}</button><button type="button" class="btn btn-primary" disabled={saving} onclick={async () => { saving = true; if (await freeTimes?.saveWorkingHours()) step = 4; saving = false; }}>{saving ? t('Saving…') : t('Send for review')}</button></div>
            <button type="button" class="btn btn-quiet skip" onclick={() => { step = 4; }}>{t('Skip, I’ll agree times in chat')}</button>
          </section>
        {/if}
      </div>
      {@render preview()}
    </div>
  {/if}
</div>

<style>
  .onboarding { display: grid; gap: 24px; max-width: 1120px; margin: 0 auto; padding: 56px 48px 72px; }
  .status { display: flex; gap: 10px; align-items: center; }
  .lead { font-size: 15px; color: var(--sk-muted); max-width: 60ch; }
  .intro-copy { display: grid; justify-items: start; gap: 18px; max-width: 720px; }
  .intro-steps { display: grid; gap: 10px; }
  .intro-steps li { display: flex; align-items: center; gap: 12px; font-weight: 600; }
  .num { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 50%; background: var(--sk-signal); color: var(--sk-on-signal); font-size: 12px; font-weight: 700; }
  .row { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; }
  .setup-title { font-size: 32px; }
  .progress { display: grid; grid-template-columns: repeat(3, 1fr); gap: 6px; }
  .progress li { display: grid; gap: 8px; }
  .bar { height: 8px; border-radius: var(--sk-r-seg); background: var(--sk-surface-2); }
  .done .bar { background: var(--sk-brand); }
  .current .bar { background: var(--sk-signal); }
  .label { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--sk-muted); }
  .done .label, .current .label { color: var(--sk-ink); font-weight: 600; }
  .setup-grid, .review-grid { display: grid; grid-template-columns: minmax(0, 1fr) 320px; gap: 24px; align-items: start; }
  .col { display: grid; gap: 20px; }
  .card { display: grid; gap: 18px; padding: 22px; }
  .form .row { margin-top: 4px; }
  .trades { display: grid; gap: 10px; }
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .short { max-width: 140px; margin-top: 8px; }
  .skip { justify-self: start; margin-left: -8px; }
  .preview { position: sticky; top: calc(var(--sk-topbar-h) + 20px); display: grid; gap: 10px; padding: 20px; }
  .eyebrow.small { font-size: 12px; }
  .who { display: flex; gap: 10px; align-items: center; }
  .who b { display: block; font-weight: 600; }
  .who small { display: block; font-size: 12px; color: var(--sk-muted); }
  .price { font-size: 22px; }
  .area-tags { display: flex; flex-wrap: wrap; gap: 6px; }
  .review { display: grid; justify-items: start; gap: 16px; }
  .review-grid { width: 100%; }
  .checks { display: grid; }
  .checks li { display: grid; grid-template-columns: 24px 1fr; gap: 12px; padding: 12px 0; }
  .checks li + li { border-top: 0.5px solid var(--sk-line); }
  .checks b { display: block; font-weight: 600; }
  .checks small { display: block; font-size: 13px; color: var(--sk-muted); }
  .checks .mark { display: grid; place-items: center; width: 24px; height: 24px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--sk-field); font-size: 12px; font-weight: 700; }
  .checks .done .mark { background: var(--sk-soft); box-shadow: none; }
  .foot { margin-top: 8px; }
  @media (max-width: 900px) { .setup-grid, .review-grid { grid-template-columns: 1fr; } .preview { position: static; } }
  @media (max-width: 639px) {
    .onboarding { padding: 28px 16px 40px; }
    .setup-title { font-size: 32px; }
    .card { padding: 18px 16px; }
    .pair { grid-template-columns: 1fr; }
    .label { font-size: 12px; }
  }
</style>
