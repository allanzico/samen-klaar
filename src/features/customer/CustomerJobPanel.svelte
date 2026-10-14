<script lang="ts">
  import type { AppActions, AppState, WorkspaceState, Intake, Job } from '../../lib/types';
  import PlanNegotiation from '../workspaces/PlanNegotiation.svelte';
  import JobReviewPanel from '../workspaces/JobReviewPanel.svelte';
  import { aiProviderLabel } from '../../lib/ai-provider';
  import { currentLocale, t } from '../../lib/i18n.svelte';
  import { dayAndTime, effectiveSlot, planUi } from '../../lib/plan-ui.svelte';
  import { requestStatus } from '../../lib/request-status';
  import { initials } from '../../lib/trade';

  let showAllSlots = $state(false);
  let serviceAddress = $state('');
  let addressWorkspace = $state<string | null>(null);
  let addressDirty = $state(false);
  let selectedEvidenceIDs = $state<string[]>([]);
  let editingAddress = $state(false);
  let visitGroups = $derived.by(() => {
    const groups = new Map<string, {day:string; timezone:string; slots:NonNullable<WorkspaceState>['availableSlots']}>();
    for (const slot of (workspace?.availableSlots ?? []).slice(0, showAllSlots ? 20 : 3)) {
      const day = new Intl.DateTimeFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB',{timeZone:slot.timezone,weekday:'long',month:'short',day:'numeric'}).format(new Date(slot.startUtc));
      const key = `${day}:${slot.timezone}`;
      const group = groups.get(key) ?? {day,timezone:slot.timezone,slots:[]};
      group.slots.push(slot);groups.set(key,group);
    }
    return [...groups.values()];
  });
  function timeLabel(value:string,timezone:string) { return new Intl.DateTimeFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB',{timeZone:timezone,hour:'2-digit',minute:'2-digit'}).format(new Date(value)); }
  let { app, actions, state: workspace, job, intake, activeJob, missingDocs, statusLabel, money }: { app: AppState; actions: AppActions; state: WorkspaceState | null; job: Job | undefined; intake: Intake | undefined; activeJob: boolean; missingDocs: boolean; statusLabel: string; money: (minor: number) => string } = $props();

  $effect(() => {
    const selected = workspace?.workspaceId ?? null;
    const saved = job?.serviceAddress ?? '';
    if (selected !== addressWorkspace) {
      addressWorkspace = selected;
      serviceAddress = saved;
      addressDirty = false;
      selectedEvidenceIDs = [];
    } else if (!addressDirty) serviceAddress = saved;
  });

  async function saveServiceAddress(event: SubmitEvent) {
    event.preventDefault();
    const saved = serviceAddress.trim();
    if (await actions.act('set-service-address', saved)) {
      addressDirty = false;
      editingAddress = false;
      serviceAddress = saved;
    }
  }

  function toggleReviewFile(id: string, checked: boolean) {
    selectedEvidenceIDs = checked ? [...selectedEvidenceIDs, id] : selectedEvidenceIDs.filter((item) => item !== id);
  }

  let providerLabel = $derived( aiProviderLabel(app.providerConfig?.aiProvider));
  let needsAddress = $derived(!!job?.location && !job?.serviceAddress);
  let slot = $derived(effectiveSlot(workspace?.availableSlots ?? [], !needsAddress));
  let status = $derived(requestStatus(workspace, 'customer'));
  let planPage = $derived(!!job && !job.quoteApproved && job.priceProposal?.status !== 'pending');
  let showAddressForm = $derived(!job?.serviceAddress || editingAddress);
  let slotsAvailable = $derived((workspace?.availableSlots.length ?? 0) > 0);
</script>

{#snippet addressForm()}
  <form class="stack address-form" onsubmit={(event) => { void saveServiceAddress(event); }}>
    <label for="service-address">{t('Street, house number, postcode and any access details')}</label>
    <textarea id="service-address" bind:value={serviceAddress} oninput={() => { addressDirty = true; }} required minlength="8" maxlength="250" rows="2" autocomplete="street-address" placeholder={t('Your full service address')}></textarea>
    <button class="secondary" disabled={app.pending || serviceAddress.trim().length < 8}>{t(job?.serviceAddress ? 'Update private address' : 'Save private address')}</button>
    {#if serviceAddress.trim().length < 8}<p class="helper">{t('Enter at least 8 characters to save your address')}</p>{/if}
  </form>
{/snippet}

{#snippet picker()}
  {#if job}
    <div class="slot-picker" role="group" aria-label={t('Visit times')}>
      {#each visitGroups as group}
        <section class="visit-day" aria-label={`${group.day} in ${group.timezone}`}><h3>{group.day}</h3>
          <div class="slot-row">{#each group.slots as option}<button class="slot" aria-pressed={slot?.slotId === option.slotId} disabled={needsAddress} onclick={() => { planUi.slotId = option.slotId; }}>{timeLabel(option.startUtc, option.timezone)}</button>{/each}</div>
        </section>
      {/each}
    </div>
    <p class="picker-foot">{#if slot}{t('Ends {time} · {timezone} time', { time: timeLabel(slot.endUtc, slot.timezone), timezone: slot.timezone })}{:else}{t('Times shown in {timezone}', { timezone: workspace?.availableSlots[0]?.timezone ?? '' })}{/if}</p>
    {#if (workspace?.availableSlots.length ?? 0) > 3}<button class="text-button" aria-expanded={showAllSlots} disabled={app.pending} onclick={() => { showAllSlots = !showAllSlots; }}>{t(showAllSlots ? 'Show fewer times' : 'More days')}</button>{/if}
  {/if}
{/snippet}

{#if activeJob && job}
  <section id="current-plan" class="task-panel" aria-label={t('Current decision')}>
    <PlanNegotiation {app} {job} {money}/>
    {#if job.change?.status === 'pending'}
      <section class="offer-card awaiting-you" aria-label={t('Review the additional work')}>
        <div class="offer-head"><h2>{t('Review the additional work')}</h2><span class="pill pill--your-turn">{t('Your turn')}</span></div>
        <p class="offer-value">+ {money(job.change.amountMinor)} <s class="offer-old">{money(job.quote.totalMinor)}</s></p>
        <blockquote class="offer-note">{job.change.description}</blockquote>
        <p class="answer-hint">{t('Answer on the right')} <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg></p>
      </section>
    {/if}
    {#if planPage}
      {#if job.needsClarification}
        <form class="stack" onsubmit={(event) => {event.preventDefault(); void actions.act('answer-clarification', app.clarificationAnswer);}}>
          <label for="legacy-answer">{job.clarificationQuestion}</label>
          <input id="legacy-answer" bind:value={app.clarificationAnswer} required minlength="3" maxlength="500"/>
          <button disabled={app.pending}>{t('Send answer')}</button>
        </form>
      {/if}
      <div class="plan-head"><span class="person-avatar" aria-hidden="true">{initials(job.professional)}</span><div><b>{job.professional}</b><span>{job.serviceDescription || job.category}</span></div></div>
      {#if job.intakeRoute === 'assessment'}<p class="quiet-note">{t('This is the professional’s published starting offer. Agree the scope and price together before approving the plan.')}</p>{/if}
      <div class="quote-lines">
        {#each job.quote.lines as line}<div><span>{line.label}</span><b>{money(line.amountMinor)}</b></div>{/each}
        {#if job.quote.exclusions?.length}<div class="muted-row"><span>{t('Not included')}</span><span>{job.quote.exclusions.join(' · ')}</span></div>{/if}
        <div class="total"><span>{t('Total')}</span><strong>{money(job.quote.totalMinor)}</strong></div>
      </div>
      <details class="included"><summary>{t('What’s included')}</summary>{#if job.serviceDescription}<p>{job.serviceDescription}</p>{/if}<p>{job.quote.materialsPolicy}</p></details>
      <section class="checklist" aria-label={t('To book, confirm three things')}>
        <h2>{t('To book, confirm three things')}</h2>
        <ol class="check-rows">
          <li class="check-row done">
            <span class="check-circle" aria-hidden="true"><svg class="icon" viewBox="0 0 24 24"><path d="m5 12 5 5 9-10"/></svg></span>
            <div class="check-main"><b>{t('Price')}</b><span>{money(job.quote.totalMinor)} · {t('published price')}</span></div>
            <button class="text-button" disabled={app.pending} onclick={() => actions.openProposal('price')}>{t('Make an offer')}</button>
          </li>
          <li class="check-row" class:done={!showAddressForm} class:active={showAddressForm}>
            <span class="check-circle" aria-hidden="true">{#if !showAddressForm}<svg class="icon" viewBox="0 0 24 24"><path d="m5 12 5 5 9-10"/></svg>{:else}2{/if}</span>
            <div class="check-main"><b>{t('Your address')}</b><span>{t('Only {name} will see it', { name: job.professional })}</span>
              {#if showAddressForm}{@render addressForm()}{:else}<span class="check-value">{job.serviceAddress}</span>{/if}
            </div>
            {#if !showAddressForm}<button class="text-button" disabled={app.pending} onclick={() => { editingAddress = true; }}>{t('Change')}</button>{/if}
          </li>
          <li class="check-row" class:done={!!slot && !needsAddress && slotsAvailable} class:active={!needsAddress && !(slot && slotsAvailable)}>
            <span class="check-circle" aria-hidden="true">{#if slot && !needsAddress}<svg class="icon" viewBox="0 0 24 24"><path d="m5 12 5 5 9-10"/></svg>{:else}3{/if}</span>
            <div class="check-main"><b>{t('Visit time')}</b>
              {#if !slotsAvailable}<span>{t('No published times. You’ll agree a visit time together.')}</span>
              {:else if needsAddress}<span>{t('Earliest')}: {dayAndTime((workspace?.availableSlots[0])!.startUtc, workspace!.availableSlots[0].endUtc, workspace!.availableSlots[0].timezone)}</span>
              {:else}{@render picker()}{/if}
            </div>
          </li>
        </ol>
      </section>
    {:else if job.quoteApproved && !job.appointment && job.dateProposal?.status !== 'pending'}
      <h2>{t('Choose a visit')}</h2>
      {#if job.appointmentConfirmation?.status === 'expired' || job.appointmentConfirmation?.status === 'declined'}
        <p class="quiet-note">{t('The previous time was released. Choose another published visit time or propose a different time.')}</p>
      {:else}<p class="quiet-note">{t('Choose a published time, or propose your own below.')}</p>{/if}
      {#if slotsAvailable}{@render picker()}{:else}<p>{t('No published times are available. Suggest a visit time below.')}</p>{/if}
    {/if}
    {#if (job.location || job.serviceAddress) && !planPage}
      <section class="service-address-panel" aria-label={t('Private visit address')}>
        <h3>{t('Exact service address')}</h3>
        {#if job.appointment}
          <p>{job.serviceAddress || t('No full address was saved.')}</p>
        {:else}
          <p>{t('Only you and the professional you selected can see this address. It will be shared after selection so they can prepare for the visit.')}</p>
          {@render addressForm()}
        {/if}
      </section>
    {/if}
    {#if job.evidenceSubmitted && !job.completionApproved && !job.paid && !(job.evidenceSubmitted && !job.detailAdded)}
      <h3>{t('Check the completion record')}</h3>
      <p>{t('Review the completion record below before approving the work.')}</p>
      <div class="evidence-grid">
      {#each job.evidence as evidence}
        <div class="evidence-item"><b>{evidence.label}</b><p>{evidence.description}</p>{#if evidence.assetId}<a href={`/api/evidence/${evidence.assetId}?workspaceId=${encodeURIComponent(app.workspaceId ?? '')}`} target="_blank" rel="noreferrer">{t('Open uploaded evidence')} <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7h10v10M7 17 17 7"/></svg></a>{/if}</div>
      {/each}
      </div>
      {#if app.providerConfig?.aiProvider}
        <details>
          <summary>{t('Help reviewing the evidence')}</summary>
          <p>{t('The assistant can point out gaps. It can’t confirm workmanship or safety.')}</p>
          <p>{t('Send uploaded evidence to')} {providerLabel} {t('to compare it with the agreed scope. This cannot verify workmanship or safety.')}</p>
          <p>{t('Select up to five files for this review, with a combined size of 12 MB. Leave all unchecked to review the newest files that fit.')}</p>
          {#each job.evidence.filter((item) => !!item.assetId) as evidence, index}
            <label><input type="checkbox" checked={selectedEvidenceIDs.includes(evidence.assetId ?? '')} disabled={app.pending || selectedEvidenceIDs.length >= 5 && !selectedEvidenceIDs.includes(evidence.assetId ?? '')} onchange={(event) => toggleReviewFile(evidence.assetId ?? '', event.currentTarget.checked)}/>{evidence.label} {index + 1}</label>
          {/each}
          <button disabled={app.pending || !job.evidence.some((item) => item.assetId)} onclick={() => actions.reviewEvidence(selectedEvidenceIDs)}>{t('Review evidence with AI')}</button>
          {#if job.evidenceReview}<p>{job.evidenceReview.summary}</p><small>{job.evidenceReview.limitations}</small>{/if}
        </details>
      {/if}
    {:else if job.evidenceSubmitted && !job.detailAdded && !job.completionApproved && !job.paid}
      <p>{t(job.detailsRequested ? 'Your professional needs to add the requested work note before you can review completion.' : 'One work note is still missing. Ask your professional to add it before approving completion.')}</p>
    {/if}
    {#if status.code === 'payment'}
      <h3>{t('What you agreed')}</h3>
      <div class="quote-lines">
        {#each job.quote.lines as line}<div><span>{line.label}</span><b>{money(line.amountMinor)}</b></div>{/each}
        <div class="total"><span>{t('Total')}</span><strong>{money(job.quote.totalMinor)}</strong></div>
      </div>
    {/if}
    <JobReviewPanel {app} {actions} {job}/>

    <details class="job-details">
      <summary>{t('Plan & activity')}</summary>
      <p>{job.description}</p>
      {#if job.serviceDescription}<p>{t('Agreed service:')} {job.serviceDescription}</p>{/if}
      {#if job.quote.exclusions?.length}<p>{t('Excluded:')} {job.quote.exclusions.join(' · ')}.</p>{/if}<p>{t('Estimated duration:')} {job.quote.durationMinutes} {t('minutes')}.</p>
      <p>{t('Plan version')} {job.scopeVersion} · {money(job.quote.totalMinor)} · {job.quote.materialsPolicy}</p>
      {#each [...job.timeline].reverse() as event}<div class="activity"><span>{event.when}</span>{event.text}</div>{/each}
    </details>
  </section>
{/if}
