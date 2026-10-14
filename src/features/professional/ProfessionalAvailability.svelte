<script lang="ts">
  import VisitTimeFields from '../workspaces/VisitTimeFields.svelte';
  import { formatVisit, validateVisit } from '../../lib/visit-time';
  import { onMount } from 'svelte';
  import type { AvailabilitySlot, ProfessionalProfile } from '../../lib/types';
  import { api } from '../../lib/api';
  import { currentLocale, t } from '../../lib/i18n.svelte';

  export let show: 'all' | 'profile' | 'times' = 'all';
  let profile: ProfessionalProfile | null = null;
  let slots: AvailabilitySlot[] = [];
  let startLocal = '';
  let endLocal = '';
  let timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  let loading = true;
  let saving = false;
  let error = '';
  let slotError = '';

  onMount(() => { void load(); });

  async function load() {
    loading = true;
    error = '';
    try {
      [profile, slots] = await Promise.all([
        api.professionalProfile().then((result) => result.profile),
        api.professionalAvailability().then((result) => result.slots)
      ]);
    } catch (e) {
      error = e instanceof Error ? e.message : t('Could not load availability.');
    } finally {
      loading = false;
    }
  }

  async function addSlot(event: SubmitEvent) {
    event.preventDefault();
    if (saving) return;
    slotError = validateVisit(startLocal, endLocal, profile?.durationMinutes || 15);
    if (slotError) return;
    saving = true;
    slotError = '';
    try {
      await api.addProfessionalAvailability({ startUtc: new Date(startLocal).toISOString(), endUtc: new Date(endLocal).toISOString(), timezone });
      slots = (await api.professionalAvailability()).slots;
      startLocal = '';
      endLocal = '';
    } catch (e) {
      slotError = e instanceof Error ? e.message : t('Could not add this appointment time.');
    } finally {
      saving = false;
    }
  }
</script>

<section class="availability-card">

  {#if loading}<p role="status">{t('Loading profile and availability…')}</p>
  {:else if profile}
    {#if show !== 'times'}
    <h2>{profile.displayName}</h2>
    <p>{profile.serviceName} · {profile.category}</p>
    <p>{new Intl.NumberFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', { style: 'currency', currency: profile.currency }).format((profile.priceMinor + profile.calloutMinor) / 100)} {t('including call-out')}</p>
    {/if}
    {#if show !== 'profile'}
    <form class="availability-form" onsubmit={addSlot}>
<VisitTimeFields bind:start={startLocal} bind:end={endLocal} duration={profile.durationMinutes} leadMinutes={15} prefix="availability" bind:error={slotError} busy={saving}/>
      <button class="primary" disabled={saving}>{saving ? t('Adding…') : t('Add available time')}</button>
    </form>
    {#if slots.length}
      <ul class="availability-list">{#each slots.slice(0, 10) as slot (slot.slotId)}<li><span>{formatVisit(slot.startUtc, slot.endUtc, slot.timezone)}</span><small>{t(slot.status === 'available' ? 'Available to book' : slot.status === 'booked' ? 'Booked' : 'Unavailable')}</small></li>{/each}</ul>{#if slots.length > 10}<details><summary>{t('{count} more published times', { count: slots.length - 10 })}</summary><ul class="availability-list">{#each slots.slice(10) as slot (slot.slotId)}<li><span>{formatVisit(slot.startUtc, slot.endUtc, slot.timezone)}</span><small>{t(slot.status === 'available' ? 'Available to book' : slot.status === 'booked' ? 'Booked' : 'Unavailable')}</small></li>{/each}</ul></details>{/if}
    {:else}<p class="quiet-note">{t('Add future appointment times so customers can book your service.')}</p>{/if}
    {/if}
  {:else}<p>{t('Your service profile could not be loaded. Retry to check your account.')}</p><button onclick={load}>{t('Retry loading profile')}</button>{/if}
  {#if error}<div class="error" role="alert"><p>{t(error)}</p><button disabled={loading} onclick={load}>{t('Retry')}</button></div>{/if}
</section>
