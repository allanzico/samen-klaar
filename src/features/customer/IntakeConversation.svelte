<script lang="ts">
  import type { AppActions, AppState, Intake, Job } from '../../lib/types';
  import { currentLocale, t } from '../../lib/i18n.svelte';
  import { formatVisit } from '../../lib/visit-time';
  import { initials } from '../../lib/trade';

  let alternatives = $state(false);
  let locationArea = $state('');
  let searching = $state(false);
  let { app, actions, job, intake, activeJob, composing }: { app: AppState; actions: AppActions; job: Job | undefined; intake: Intake | undefined; activeJob: boolean; composing: boolean } = $props();
  let compactHistory = $derived(activeJob || intake?.step === 'matches');
  let allMessages = $derived(intake?.messages ?? []);
  let discussion = $derived(activeJob ? allMessages.filter((m) => m.threadProfileId === job?.professionalProfileId && !!m.threadProfileId) : allMessages);
  let recent = $derived(compactHistory ? discussion.slice(-4) : discussion);
  let earlier = $derived(compactHistory ? allMessages.filter((m) => !recent.includes(m)) : []);
  function sender(message: { role: string; senderName?: string }) {
    return message.role === app.role ? t('You') : message.senderName || (message.role === 'professional' ? t('Professional') : message.role === 'customer' ? job?.customer || t('Customer') : t('{brand} assistant', { brand: app.brandName }));
  }
  $effect(() => {
    if (intake?.location?.label && !locationArea) locationArea = intake.location.label;
  });
  async function setLocation(event: SubmitEvent) {
    event.preventDefault();
    searching = true;
    try { await actions.act('set-location', locationArea.trim()); } finally { searching = false; }
  }
  function money(minor: number, currency: string) {
    return new Intl.NumberFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', { style: 'currency', currency: currency || 'EUR' }).format(minor / 100);
  }
  const firstName = (name: string) => name.trim().split(/\s+/)[0] ?? name;
</script>

{#snippet messageRow(message: { role: string; senderName?: string; text: string })}
  <div class="message" class:from-you={message.role === app.role}>
    {#if message.role !== app.role}
      <span class="message-avatar" class:assistant={message.role !== 'professional' && message.role !== 'customer'} aria-hidden="true">{#if message.role !== 'professional' && message.role !== 'customer'}<svg viewBox="0 0 48 48"><g fill="none" stroke-width="4" stroke-linecap="round"><path d="M24 11v26M11 24h26M15 15l18 18M15 33l18-18"/></g></svg>{:else}{initials(sender(message))}{/if}</span>
    {/if}
    <span class="message-label">{sender(message)}</span><p>{message.text}</p>
  </div>
{/snippet}

{#snippet jobCard(editable: boolean)}
  {#if intake}
    <section class="job-card" aria-label={t('Your job')}>
      <div class="job-card-head"><p class="eyebrow">{t('Your job')}</p>{#if editable}<button class="text-button" disabled={app.pending} onclick={actions.focusComposer}>{t('Edit')}</button>{/if}</div>
      <p class="job-card-title">{intake.description}</p>
      <p class="job-card-meta">{[intake.category ? t(intake.category) : '', intake.location?.label ?? locationArea.trim()].filter(Boolean).join(' · ')}</p>
      <p class="helper">{t('This summary is what the professional sees first.')}</p>
    </section>
  {/if}
{/snippet}

{#if intake || activeJob}
  {#if earlier.length}
    <details class="conversation-history"><summary>{t('Earlier conversation')} · {earlier.length} {t('messages')}</summary>
      <div class="messages">{#each earlier as message}{@render messageRow(message)}{/each}</div>
    </details>
  {/if}
  <div class="messages" role="log" aria-label={t('Conversation history')} aria-live="polite" aria-relevant="additions">
    {#if intake}
      {#each recent as message, index (allMessages.indexOf(message))}
        {@render messageRow(message)}
      {/each}
    {:else if job}
        <div class="message from-you"><span class="message-label">{t('You')}</span><p>{job.description}</p></div>
    {/if}
  </div>
  {#if composing && intake && ['detail', 'urgency', 'budget'].includes(intake.step)}
    <div class="answer-options" aria-label={t('Suggested answers')}>
      {#each intake.options ?? [] as option}<button disabled={app.pending} onclick={() => actions.chooseAnswer(option)}>{option}</button>{/each}
    </div>

  {/if}
  {#if intake && ['matches', 'review'].includes(intake.step)}
    <button class="text-button" disabled={app.pending} onclick={() => actions.act('refresh-matches')}>{t('Refresh availability')}</button>
  {/if}
  {#if searching && intake && ['location', 'waitlist'].includes(intake.step)}
    <section class="finding" aria-live="polite">
      {@render jobCard(false)}
      <p class="finding-status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Checking professionals who serve {area}…', { area: locationArea.trim() })}</p>
      <div class="match-card skeleton" aria-hidden="true">
        <span class="skeleton-bar short"></span>
        <div class="match-person"><span class="skeleton-dot"></span><div class="skeleton-lines"><span class="skeleton-bar"></span><span class="skeleton-bar short"></span></div></div>
        <span class="skeleton-bar"></span>
        <div class="skeleton-facts"><span class="skeleton-bar"></span><span class="skeleton-bar"></span><span class="skeleton-bar"></span></div>
        <span class="skeleton-bar button"></span>
      </div>
    </section>
  {:else if intake?.step === 'location'}
    <form class="stack area-form area-card" onsubmit={setLocation}>
      <label for="service-area">{t('Neighbourhood or postcode area')}</label>
      <input id="service-area" bind:value={locationArea} required minlength="2" maxlength="80" autocomplete="off" placeholder={t('e.g. Amsterdam Oost or 1012 AB')} />
      <p class="lock-note"><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></svg>{t('Your street address stays private until you choose a professional.')}</p>
      <button class="primary" disabled={app.pending || locationArea.trim().length < 2}>{t('Find professionals')}</button>
    </form>
  {/if}
  {#if intake?.step === 'waitlist' && !searching}
    <section class="area-waitlist" aria-live="polite">
      <span class="empty-tile" aria-hidden="true"><svg class="icon" viewBox="0 0 24 24"><path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/></svg></span>
      <p>{t('We haven’t found a published service in this area. You can save the request for the operations team to review. This does not create a booking or promise an automatic match.')}</p>
      {#if intake.waitlisted}
        <p class="success-line" role="status"><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10"/></svg>{t('Saved. Your request stays in your account.')}</p>
      {:else}
        <button class="primary" disabled={app.pending} onclick={() => actions.act('join-waitlist')}>{t('Save to the waitlist')}</button>
      {/if}
      <div class="or-divider" aria-hidden="true"><span>{t('or')}</span></div>
      <form class="stack area-form" onsubmit={setLocation}>
        <label for="change-area">{t('Try a different area')}</label>
        <input id="change-area" bind:value={locationArea} required minlength="2" maxlength="80" autocomplete="off" />
        <button disabled={app.pending}>{t('Search this area')}</button>
      </form>
    </section>
  {/if}
  {#if intake?.step === 'matches' && intake.matches.length}
    {@render jobCard(true)}
    <p class="matches-intro">{t('Published service areas covering')} {intake.location?.label}. {t('Earliest suitable visit is shown first; nothing is booked yet.')}</p>
    <div class="matches" class:show-alt={alternatives}>
      {#each intake.matches.slice(0, 3) as match, index (match.id)}
        <article id={index === 0 ? 'recommended-service' : undefined} class="match-card" class:best={index === 0} class:alt={index > 0} aria-label={t(index === 0 ? 'Recommended service' : 'Alternative service')}>
          <span class="match-label">{t(index === 0 ? 'Best fit' : 'Alternative')}</span>
          <div class="match-person">
            <span class="person-avatar" aria-hidden="true">{initials(match.name)}</span>
            <div><b>{match.name}</b><span>{match.serviceName || match.category}</span></div>
          </div>
          <p class="fit-reason"><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10"/></svg>{match.fitReason}</p>
          <dl class="match-facts">
            <div><dt>{t('Price incl. call-out')}</dt><dd class="service-price">{money(match.totalMinor, match.currency)}</dd></div>
            <div><dt>{t('Takes')}</dt><dd>{match.durationMinutes} {t('minutes')}</dd></div>
            <div><dt>{t('Earliest')}</dt><dd>{match.earliestAppointment ? formatVisit(match.earliestAppointment.startUtc, match.earliestAppointment.endUtc, match.earliestAppointment.timezone) : t('No set times')}</dd></div>
          </dl>
          {#if !match.earliestAppointment}<p class="helper">{t('You’d agree a visit time together in chat.')}</p>{/if}
          <details><summary>{t('What’s included')}</summary>{#if match.serviceDescription}<p>{match.serviceDescription}</p>{/if}<p>{match.materialsPolicy}</p>{#if match.exclusions?.length}<p>{t('Excluded:')} {match.exclusions.join('; ')}</p>{/if}</details>
          <button class={index === 0 ? 'primary' : 'secondary'} disabled={app.pending} onclick={() => actions.act('select-match', match.id)}>{t('Review {name}’s plan', { name: firstName(match.name) })}</button>
          {#if index === 0}<p class="helper">{t('Next you’ll see the full price, pick a time and add your address. No payment yet.')}</p>{:else}<p class="helper helper-spacer" aria-hidden="true">{t('Next you’ll see the full price, pick a time and add your address. No payment yet.')}</p>{/if}
        </article>
      {/each}
    </div>
    <div class="actions matching-actions">
      {#if intake.matches.length > 1}<button class="alt-toggle" disabled={app.pending} aria-expanded={alternatives} onclick={() => { alternatives = !alternatives; }}>{alternatives ? t('Hide alternatives') : t(intake.matches.length === 2 ? 'Compare 1 alternative' : 'Compare {count} alternatives', { count: Math.min(intake.matches.length, 3) - 1 })}</button>{/if}
      <button class="text-button" disabled={app.pending} onclick={actions.focusComposer}>{t('Not my problem? Describe it again')}</button>
    </div>
    {#if intake.budget}<p class="quiet-note">{t('Your budget preference:')} “{intake.budget}” {t('Prices are published by the professional; you can negotiate before approving.')}</p>{/if}
  {/if}
{/if}
