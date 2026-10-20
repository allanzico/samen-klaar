<script lang="ts">
  import Modal from '../../ui/Modal.svelte';
  import Banner from '../../ui/Banner.svelte';
  import { api } from '../../lib/api';
  import { agentUi } from '../../lib/agent.svelte';
  import type { AgentMandate, AppState } from '../../lib/types';
  import { showToast } from '../../ui/toast-state.svelte';
  import { t } from '../../lib/i18n.svelte';

  // Your agent acts for you only inside what you set here. Anything outside it comes back as a
  // question. Approving finished work and paying are always yours.
  let { app }: { app: AppState } = $props();
  const pro = $derived(app.role === 'professional');
  const days = [[1, 'Mon'], [2, 'Tue'], [3, 'Wed'], [4, 'Thu'], [5, 'Fri'], [6, 'Sat'], [7, 'Sun']] as const;

  let loaded = $state(false);
  let saving = $state(false);
  let error = $state('');
  let paused = $state(false);
  let autoWork = $state(false);
  let bookAlone = $state(false);
  let weekdays = $state<number[]>([1, 2, 3, 4, 5]);
  let startHour = $state(8);
  let endHour = $state(18);
  let preferredHour = $state(8);
  let limit = $state('');
  let extra = $state('0');
  let horizon = $state(14);
  let address = $state('');
  let floor = $state(90);
  let autoConfirm = $state(true);
  let autoDates = $state(false);
  let outsideHours = $state(false);

  const euros = (minor?: number) => (minor ? String(minor / 100) : '');
  const toMinor = (value: string) => Math.round(Number(value.replace(',', '.')) * 100) || 0;

  $effect(() => {
    if (!agentUi.settingsOpen) { loaded = false; return; }
    error = '';
    api.agentMandate().then((m) => {
      paused = !m.enabled; autoWork = !!m.autoApproveWork; bookAlone = !!m.bookWithoutAsking; weekdays = m.weekdays?.length ? m.weekdays : [1, 2, 3, 4, 5];
      startHour = m.startHour ?? 8; endHour = m.endHour ?? 18; preferredHour = m.preferredHour ?? m.startHour ?? 8;
      limit = euros(m.maxTotalMinor); extra = euros(m.autoApproveExtraMinor) || '0'; horizon = m.maxDaysAhead || 14; address = m.serviceAddress ?? '';
      floor = m.floorPercent || 90; autoConfirm = m.version ? !!m.autoConfirmVisits : true; autoDates = m.version ? !!m.autoAcceptDates : true;
      outsideHours = !!m.acceptOutsideHours;
      loaded = true;
    }).catch((e) => { error = e instanceof Error ? e.message.trim() : t('Could not load your agent settings'); });
  });

  function toggleDay(day: number, on: boolean) { weekdays = on ? [...weekdays, day].sort() : weekdays.filter((d) => d !== day); }

  async function save() {
    saving = true;
    error = '';
    const mandate: AgentMandate = { enabled: !paused, weekdays, startHour: Number(startHour), endHour: Number(endHour) };
    if (!pro) mandate.preferredHour = Number(preferredHour);
    if (pro) Object.assign(mandate, { floorPercent: Number(floor), autoConfirmVisits: autoConfirm, autoAcceptDates: autoDates, acceptOutsideHours: outsideHours });
    else Object.assign(mandate, { autoApproveWork: autoWork, bookWithoutAsking: bookAlone, maxTotalMinor: toMinor(limit), autoApproveExtraMinor: toMinor(extra), maxDaysAhead: Number(horizon), serviceAddress: address.trim() });
    try {
      await api.saveAgentMandate(mandate);
      showToast(paused ? t('Your agent is paused') : t('Preferences saved'));
      agentUi.settingsOpen = false;
    } catch (e) { error = e instanceof Error ? e.message.trim() : t('Could not save your agent settings'); }
    finally { saving = false; }
  }
</script>

<Modal bind:open={agentUi.settingsOpen} title={t('Assistant preferences')} wide>
  <form class="agent-form" onsubmit={(event) => { event.preventDefault(); void save(); }}>
    <p class="secondary-text">{pro
      ? t('Your agent is already working for you. It answers price offers, accepts visit times and confirms visits inside your working hours. It never does the work or decides a dispute, and payments always stay with you.')
      : t('Your agent is already working for you from your first message: it picks a professional, negotiates price and visit time once, and books a visit. Payments always stay with you, and so does approving finished work unless you hand that over below.')}</p>
    {#if error}<Banner kind="error" text={error}/>{/if}
    {#if loaded}
      <label class="switch"><input type="checkbox" role="switch" bind:checked={paused}/><span class="track" aria-hidden="true"></span><span>{t('Pause my agent')}</span></label>
      <p class="field-help">{t('While paused, your agent does nothing and every step is yours.')}</p>
      <fieldset disabled={paused || saving}>
        {#if pro}
          <div class="field"><label for="agent-floor">{t('Lowest price I accept, as a percentage of my listed price')}</label>
            <input id="agent-floor" class="input narrow" type="number" min="50" max="100" step="1" bind:value={floor}/>
            <p class="field-help">{t('Offers at or above this are accepted. Below it, your agent counters once at this price, then asks you.')}</p></div>
          <label class="check"><input type="checkbox" bind:checked={autoConfirm}/>{t('Confirm reserved visits that fall inside my days and hours')}</label>
          <label class="check"><input type="checkbox" bind:checked={autoDates}/>{t('Accept a visit time the customer proposes inside my days and hours')}</label>
          <label class="check"><input type="checkbox" bind:checked={outsideHours}/>{t('Also accept proposed visits outside my working hours')}</label>
          <p class="field-help">{t('Only proposed appointments can use this permission. Your published working hours stay the same. Days off, conflicting visits and late changes still need review.')}</p>
        {:else}
          <div class="field"><label for="agent-limit">{t('Never agree to more than (optional)')}</label>
            <div class="money-input narrow"><span aria-hidden="true">€</span><input id="agent-limit" inputmode="decimal" bind:value={limit} placeholder="150"/></div>
            <p class="field-help">{t('Your agent also stays within the budget you give in the chat, and asks you to approve the price when you give none. Slightly above this, it offers this amount once. Further above, it asks you.')}</p></div>
          <label class="check"><input type="checkbox" bind:checked={bookAlone}/>{t('Book a visit time without asking me first, using the days and hours below')}</label>
          <label class="check"><input type="checkbox" bind:checked={autoWork}/>{t('Approve finished work for me when the professional marks it complete. Payment still needs me.')}</label>
          <div class="field"><label for="agent-extra">{t('Extra work I allow without asking, up to')}</label>
            <div class="money-input narrow"><span aria-hidden="true">€</span><input id="agent-extra" inputmode="decimal" bind:value={extra}/></div></div>
          <div class="field"><label for="agent-address">{t('Service address to share with the professional')}</label>
            <input id="agent-address" class="input" maxlength="250" bind:value={address} placeholder={t('Street, number, postcode, city')}/></div>
          <div class="field"><label for="agent-horizon">{t('Book visits up to this many days ahead')}</label>
            <input id="agent-horizon" class="input narrow" type="number" min="1" max="90" bind:value={horizon}/></div>
        {/if}
        {#if !pro}
        <div class="field"><span class="field-label" id="agent-days">{t('Visit days')}</span>
          <div class="days" role="group" aria-labelledby="agent-days">
            {#each days as [day, name]}<label class="chip-check"><input type="checkbox" checked={weekdays.includes(day)} onchange={(event) => toggleDay(day, event.currentTarget.checked)}/><span>{t(name)}</span></label>{/each}
          </div></div>
        <div class="hours">
          <div class="field"><label for="agent-from">{t('From')}</label><select id="agent-from" class="select" bind:value={startHour}>{#each Array.from({ length: 24 }, (_, i) => i) as hour}<option value={hour}>{String(hour).padStart(2, '0')}:00</option>{/each}</select></div>
          <div class="field"><label for="agent-to">{t('Until')}</label><select id="agent-to" class="select" bind:value={endHour}>{#each Array.from({ length: 24 }, (_, i) => i + 1) as hour}<option value={hour}>{String(hour).padStart(2, '0')}:00</option>{/each}</select></div>
        </div>
          <div class="field"><label for="agent-preferred">{t('When none of the listed times fit, propose a visit at')}</label>
            <select id="agent-preferred" class="select narrow" bind:value={preferredHour}>{#each Array.from({ length: endHour - startHour }, (_, i) => startHour + i) as hour}<option value={hour}>{String(hour).padStart(2, '0')}:00</option>{/each}</select>
            <p class="field-help">{t('Your agent proposes this hour on the first allowed day. The professional’s agent can accept it or suggest the closest time that suits them, once.')}</p></div>
        {/if}
      </fieldset>
    {:else if !error}<p role="status">{t('Loading…')}</p>{/if}
  </form>
  {#snippet footer()}
    <button type="button" class="btn btn-primary" disabled={!loaded || saving} onclick={() => void save()}>{saving ? t('Saving…') : t('Save')}</button>
    <button type="button" class="btn btn-outline" onclick={() => { agentUi.settingsOpen = false; }}>{t('Cancel')}</button>
  {/snippet}
</Modal>

<style>
  .agent-form, fieldset { display: grid; gap: 16px; }
  fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
  fieldset:disabled { opacity: .55; }
  .narrow { max-width: 180px; }
  .switch { display: inline-flex; align-items: center; gap: 12px; cursor: pointer; font-weight: 600; min-height: 44px; position: relative; }
  .switch input { position: absolute; opacity: 0; width: 44px; height: 26px; }
  .track { width: 44px; height: 26px; border-radius: 20px; background: var(--sk-surface-2); box-shadow: inset 0 0 0 1.5px var(--sk-field); position: relative; flex: none; }
  .track::after { content: ''; position: absolute; width: 20px; height: 20px; top: 3px; left: 3px; background: var(--sk-surface); border-radius: 50%; box-shadow: 0 1px 2px var(--sk-shadow); }
  input:checked + .track { background: var(--sk-primary, var(--sk-brand)); box-shadow: none; }
  input:checked + .track::after { left: 21px; }
  input:focus-visible + .track { outline: 2px solid var(--sk-ink); outline-offset: 4px; }
  .check { display: flex; align-items: flex-start; gap: 10px; font-size: 14px; }
  .check input { width: 18px; height: 18px; margin-top: 2px; accent-color: var(--sk-ink); }
  .days { display: flex; flex-wrap: wrap; gap: 8px; }
  .chip-check { position: relative; }
  .chip-check input { position: absolute; opacity: 0; inset: 0; }
  .chip-check span { display: inline-flex; align-items: center; min-height: 36px; padding: 0 14px; border-radius: var(--sk-r-pill); box-shadow: inset 0 0 0 1px var(--sk-field); font-size: 14px; font-weight: 500; cursor: pointer; }
  .chip-check input:checked + span { background: var(--sk-ink); color: var(--sk-surface); box-shadow: none; }
  .chip-check input:focus-visible + span { outline: 2px solid var(--sk-ink); outline-offset: 2px; }
  .hours { display: grid; grid-template-columns: repeat(2, minmax(0, 160px)); gap: 12px; }
</style>
