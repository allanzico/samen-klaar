<script lang="ts">
  import Banner from '../../ui/Banner.svelte';
  import { api } from '../../lib/api';
  import type { ProfessionalProfile, ProfessionalProfileInput } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';
  import { showToast } from '../../ui/toast-state.svelte';

  let { profile, onSaved }: { profile: ProfessionalProfile; onSaved: (profile: ProfessionalProfile) => void } = $props();
  let editing = $state(false);
  let saving = $state(false);
  let error = $state('');
  let draft = $state<ProfessionalProfileInput | null>(null);
  let price = $state(0);
  let callout = $state(0);
  let exclusions = $state('');
  function begin() { const { id: _id, status: _status, ...input } = profile; draft = { ...input }; price = profile.priceMinor / 100; callout = profile.calloutMinor / 100; exclusions = (profile.exclusions ?? []).join('\n'); error = ''; editing = true; }
  async function save(event: SubmitEvent) {
    event.preventDefault(); if (!draft || saving) return; saving = true; error = '';
    try { const result = await api.saveProfessionalProfile({ ...draft, priceMinor: Math.round(Number(price) * 100), calloutMinor: Math.round(Number(callout) * 100), durationMinutes: Number(draft.durationMinutes), exclusions: exclusions.split('\n').map((value) => value.trim()).filter(Boolean) }); onSaved(result.profile); editing = false; showToast(t(result.profile.status === 'active' ? 'Service updated' : 'Changes sent for review')); }
    catch (e) { error = e instanceof Error ? e.message : t('Could not save your profile.'); }
    finally { saving = false; }
  }
</script>

{#if editing && draft}
  <form class="edit-service" onsubmit={save}>
    <h3 class="small-title">{t('Edit your service')}</h3>
    <Banner kind="info" text={t('Changes to your name, service, area, visit length or terms need review. Price-only changes go live immediately. Existing bookings keep their agreed price and scope.')}/>
    <div class="field"><label for="edit-display">{t('Display name')}</label><input id="edit-display" class="input" bind:value={draft.displayName} required minlength="2" maxlength="80" disabled={saving}/></div>
    <div class="field"><label for="edit-category">{t('Service category')}</label><select id="edit-category" class="input" bind:value={draft.category} disabled={saving}>{#each ['Plumbing', 'Carpentry', 'Electrical assessment'] as category}<option value={category}>{t(category)}</option>{/each}</select></div>
    <div class="field"><label for="edit-name">{t('Service name')}</label><input id="edit-name" class="input" bind:value={draft.serviceName} required minlength="3" maxlength="100" disabled={saving}/></div>
    <div class="field"><label for="edit-description">{t('What the price covers')}</label><textarea id="edit-description" class="textarea" bind:value={draft.serviceDescription} maxlength="600" rows="3" disabled={saving}></textarea></div>
    <div class="field"><label for="edit-area">{t('Neighbourhoods or postcode areas')}</label><input id="edit-area" class="input" bind:value={draft.serviceArea} required minlength="2" maxlength="120" disabled={saving}/></div>
    <div class="pair"><div class="field"><label for="edit-price">{t('Service price')}</label><input id="edit-price" class="input" type="number" min="0.01" step="0.01" bind:value={price} required disabled={saving}/></div><div class="field"><label for="edit-callout">{t('Call-out fee')}</label><input id="edit-callout" class="input" type="number" min="0" step="0.01" bind:value={callout} required disabled={saving}/></div></div>
    <div class="pair"><div class="field"><label for="edit-currency">{t('Currency')}</label><input id="edit-currency" class="input" bind:value={draft.currency} pattern="[A-Z]{3}" required minlength="3" maxlength="3" disabled={saving}/></div><div class="field"><label for="edit-duration">{t('Duration (minutes)')}</label><input id="edit-duration" class="input" type="number" min="15" max="360" bind:value={draft.durationMinutes} required disabled={saving}/></div></div>
    <div class="field"><label for="edit-materials">{t('Materials')}</label><textarea id="edit-materials" class="textarea" bind:value={draft.materialsPolicy} required minlength="3" maxlength="300" rows="2" disabled={saving}></textarea></div>
    <div class="field"><label for="edit-exclusions">{t('Not included (one per line)')}</label><textarea id="edit-exclusions" class="textarea" bind:value={exclusions} rows="3" disabled={saving}></textarea></div>
    {#if error}<Banner kind="error" text={t(error)}/>{/if}
    <div class="actions"><button class="btn btn-primary" disabled={saving}>{saving ? t('Saving…') : t('Save changes')}</button><button class="btn btn-outline" type="button" disabled={saving} onclick={() => { editing = false; }}>{t('Cancel')}</button></div>
  </form>
{:else}<button class="btn btn-outline" type="button" onclick={begin}>{t('Edit your service')}</button>{/if}

<style>
  .edit-service { display: grid; gap: 18px; padding-top: 8px; }
  .pair { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 12px; }
  .actions { display: flex; flex-wrap: wrap; gap: 12px; }
  @media(max-width: 639px) { .pair { grid-template-columns: 1fr; } }
</style>
