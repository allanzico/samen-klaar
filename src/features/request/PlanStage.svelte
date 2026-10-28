<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import LocationSearch from '../../ui/LocationSearch.svelte';
  import Pill from '../../ui/Pill.svelte';
  import Banner from '../../ui/Banner.svelte';
  import TimePicker from './TimePicker.svelte';
  import type { AppActions, AppState, WorkspaceState, Location } from '../../lib/types';
  import type { RequestCode } from '../../lib/request-status';
  import { firstName } from '../../lib/status-ui';
  import { money, dayTime } from '../../lib/format';
  import { initials } from '../../lib/trade';
  import { effectiveSlot } from '../../lib/request-ui.svelte';
  import { t } from '../../lib/i18n.svelte';
  import { reviewingPlan } from '../../lib/prebooking';

  // C01–C03 and C14: the plan card, then "To book, confirm three things" (price, address, time).
  let { app, actions, workspace, code, reviewOnly = false }: { app: AppState; actions: AppActions; workspace: WorkspaceState; code: RequestCode; reviewOnly?: boolean } = $props();
  const job = $derived(workspace.job);
  const pro = $derived(firstName(job.professional));
  const slots = $derived(workspace.availableSlots);
  const needsAddress = $derived(!!job.location && !job.serviceAddress);
  const slot = $derived(effectiveSlot(slots, !needsAddress));
  const planPage = $derived(code === 'quote' || reviewOnly || reviewingPlan(workspace));
  const assessment = $derived(job.intakeRoute === 'assessment');

  let address = $state('');
  let addressLocation = $state<Location | null>(null);
  let addressFor = $state<string | null>(null);
  let dirty = $state(false);
  let editing = $state(false);
  let clarification = $state('');
  $effect(() => {
    if (workspace.workspaceId !== addressFor) { addressFor = workspace.workspaceId; address = job.serviceAddress ?? ''; dirty = false; editing = false; }
    else if (!dirty && job.serviceAddress && !editing) address = job.serviceAddress ?? '';
  });
  const showAddressForm = $derived(!job.serviceAddress || editing);
  async function saveAddress(event: SubmitEvent) {
    event.preventDefault();
    const value = address.trim();
    if (await actions.act('set-service-address', value, addressLocation ?? undefined)) { dirty = false; editing = false; address = value; }
  }
  const lineTotal = $derived(job.quote.lines.reduce((sum, line) => sum + line.amountMinor, 0));
</script>

{#snippet addressForm()}
  <form class="address-form" onsubmit={saveAddress}>
    {#if job.location?.source === 'mapbox'}
      <LocationSearch id="service-address" label={t('Full visit address')} purpose="address" bind:value={address} bind:selected={addressLocation} disabled={app.pending} help={t('We check that your address is inside this professional’s service area.')}/>
    {:else}
      <div class="field"><label for="service-address">{t('Street, house number and postcode')}</label><textarea id="service-address" class="textarea" rows="2" required minlength="8" maxlength="250" autocomplete="street-address" bind:value={address} oninput={() => { dirty = true; }}></textarea></div>
    {/if}
    <div class="row-actions">
      <button class="btn btn-outline btn-sm" disabled={app.pending || address.trim().length < 8 || (job.location?.source === 'mapbox' && !addressLocation)}>{t('Save private address')}</button>
      {#if editing}<button type="button" class="btn btn-quiet" onclick={() => { editing = false; dirty = false; }}>{t('Cancel')}</button>{/if}
    </div>
    {#if address.trim().length < 8}<p class="helper">{t('Enter at least 8 characters to save your address')}</p>{/if}
  </form>
{/snippet}

<div class="plan-stage">
  {#if job.needsClarification}
    <form class="box clarify" onsubmit={(event) => { event.preventDefault(); void actions.act('answer-clarification', clarification); }}>
      <div class="field"><label for="clarification">{job.clarificationQuestion}</label><input id="clarification" class="input" bind:value={clarification} required minlength="3" maxlength="500"/></div>
      <button class="btn btn-primary btn-sm" disabled={app.pending || clarification.trim().length < 3}>{t('Send answer')}</button>
    </form>
  {/if}

  {#if planPage}
    <section class="box plan-card" aria-labelledby="plan-title">
      <div class="plan-head">
        <span class="avatar avatar-48" aria-hidden="true">{initials(job.professional)}</span>
        <div class="plan-who"><h2 id="plan-title">{job.professional}</h2><p class="secondary-text">{t('{service}, {minutes} minutes', { service: job.serviceDescription || job.title, minutes: job.quote.durationMinutes })}</p></div>
        <Pill variant={assessment ? 'neutral' : 'progress'} label={assessment ? t('Starting price') : t('Published price')}/>
      </div>
      {#if assessment}<Banner kind="info" title={t('A starting price')} text={t('{name}’s starting offer. Agree scope and price first.', { name: pro })}/>{/if}
      <dl class="ledger">
        {#each job.quote.lines as line}<div><dt>{t(line.label)}</dt><dd>{money(line.amountMinor, job.quote.currency)}</dd></div>{/each}
        {#if job.quote.exclusions?.length}<div class="ledger-muted"><dt>{t('Not included')}</dt><dd>{job.quote.exclusions.join(', ')}</dd></div>{/if}
        <div class="ledger-total"><dt>{t('Total')}</dt><dd>{money(lineTotal || job.quote.totalMinor, job.quote.currency)}</dd></div>
      </dl>
      <details class="disclosure">
        <summary>{t('What’s included and what isn’t')}</summary>
        {#if job.serviceDescription}<p>{job.serviceDescription}</p>{/if}
        <p>{job.quote.materialsPolicy}</p>
      </details>
    </section>

    {#if !reviewOnly && !reviewingPlan(workspace)}<section class="box checklist" aria-labelledby="checklist-title">
      <h2 id="checklist-title" class="section-title">{t('To book, confirm three things')}</h2>
      <ol class="rows">
        <li class="check done">
          <span class="mark" aria-hidden="true"><Icon name="check"/></span>
          <div class="check-main"><b>{t('Price')}</b><span class="secondary-text">{money(job.quote.totalMinor, job.quote.currency)}</span></div>
          <button type="button" class="btn btn-quiet" disabled={app.pending} onclick={() => actions.openProposal('price')}>{t('Make an offer')}</button>
        </li>
        <li class="check" class:done={!showAddressForm} class:active={showAddressForm}>
          <span class="mark" aria-hidden="true">{#if showAddressForm}2{:else}<Icon name="check"/>{/if}</span>
          <div class="check-main"><b>{t('Your address')}</b>
            {#if showAddressForm}<span class="secondary-text">{t('Only {name} will see it.', { name: pro })}</span>{@render addressForm()}
            {:else}<span class="secondary-text">{t('{address}. Only {name} will see it.', { address: job.serviceAddress ?? '', name: pro })}</span>{/if}
          </div>
          {#if !showAddressForm}<button type="button" class="btn btn-quiet" disabled={app.pending} onclick={() => { editing = true; }}>{t('Change')}</button>{/if}
        </li>
        <li class="check" class:done={!!slot && !needsAddress} class:active={!needsAddress && !slot}>
          <span class="mark" aria-hidden="true">{#if slot && !needsAddress}<Icon name="check"/>{:else}3{/if}</span>
          <div class="check-main"><b>{t('Visit time')}</b>
            {#if !slots.length}<span class="secondary-text">{t('{name} hasn’t published free times. You agree a time in chat after approving.', { name: pro })}</span>
            {:else if needsAddress}<span class="secondary-text">{t('Earliest: {time}', { time: dayTime(slots[0].startUtc, slots[0].timezone) })}</span>
            {:else}<span class="secondary-text">{t('Pick one of {name}’s published times', { name: pro })}</span><TimePicker {slots} disabled={app.pending}/>{/if}
          </div>
        </li>
      </ol>
    </section>{/if}
  {:else}
    <section class="box checklist" aria-labelledby="pick-title">
      <h2 id="pick-title" class="section-title">{code === 'choose_visit' ? t('Choose another time') : t('Choose a visit time')}</h2>
      {#if needsAddress}
        <div class="address-only"><b>{t('Your address')}</b><span class="secondary-text">{t('Only {name} will see it.', { name: pro })}</span>{@render addressForm()}</div>
      {/if}
      {#if slots.length}
        <TimePicker {slots} disabled={app.pending || needsAddress} unavailable={code === 'choose_visit' ? job.appointmentConfirmation?.appointment : undefined}/>
      {:else}
        <p class="secondary-text">{t('{name} hasn’t published free times. Suggest a time and agree it in chat.', { name: pro })}</p>
        <button type="button" class="btn btn-outline btn-sm suggest" disabled={app.pending} onclick={() => actions.openProposal('date')}>{t('Suggest a visit time')}</button>
      {/if}
    </section>
  {/if}
</div>

<style>
  .plan-stage { display: grid; gap: 20px; }
  .clarify { display: grid; gap: 12px; padding: 18px 22px; }
  .plan-card { display: grid; gap: 14px; padding: 22px; }
  .plan-head { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
  .plan-who { flex: 1; min-width: 180px; }
  .plan-who h2 { font: 700 17px/1.2 var(--sk-fd); }
  .checklist { display: grid; gap: 8px; padding: 22px; }
  .check { display: grid; grid-template-columns: 28px 1fr auto; gap: 14px; align-items: start; padding: 16px 0; }
  .check:first-child { padding-top: 8px; }
  .mark { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--sk-field); font-size: 12px; font-weight: 700; }
  .mark :global(.icon) { width: 15px; height: 15px; }
  .check.done .mark { background: var(--sk-soft); box-shadow: none; }
  .check.active .mark { background: var(--sk-brand); color: var(--sk-on-brand); box-shadow: none; }
  .check-main { display: grid; gap: 4px; min-width: 0; }
  .check-main b { font-size: 14px; font-weight: 600; }
  .check .btn-quiet { margin-top: -10px; }
  .address-form { display: grid; gap: 10px; margin-top: 10px; }
  .row-actions { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
  .address-only { display: grid; gap: 4px; padding-bottom: 12px; border-bottom: 0.5px solid var(--sk-line); }
  .suggest { justify-self: start; }
  @media (max-width: 639px) {
    .plan-card, .checklist { padding: 18px 16px; }
    .check { grid-template-columns: 28px 1fr; }
    .check .btn-quiet { grid-column: 2; justify-self: start; margin: -6px 0 0 -8px; }
  }
</style>
