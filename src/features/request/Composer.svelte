<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import type { AppActions, AppState } from '../../lib/types';
  import { draftKey, readFormDraft, saveDraft } from '../../lib/drafts';
  import { defaultVisitEnd, localDateTime, validateVisit } from '../../lib/visit-time';
  import { firstName } from '../../lib/status-ui';
  import { time } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';

  // The composer: a layout row under the thread, never an overlay. After a match it has
  // three modes; a chat message can never change the price or reserve a visit.
  // compact: the one-line composer on plan, check and pay pages (board C12).
  let { app, actions, assigned = false, matching = false, compact = false, replyOnly = false }: { app: AppState; actions: AppActions; assigned?: boolean; matching?: boolean; compact?: boolean; replyOnly?: boolean } = $props();

  const job = $derived(app.serverState?.job);
  const recipient = $derived(assigned ? firstName(app.role === 'professional' ? job?.customer : job?.professional) : '');
  const minimum = $derived(assigned ? 2 : matching || !app.serverState?.intake ? 5 : 2);
  const planning = $derived(!replyOnly && assigned && !app.serverState?.cancelled && !job?.workStarted && !job?.paid);
  const canPrice = $derived(planning && !job?.quoteApproved);
  const canDate = $derived(planning && !!job?.quoteApproved);

  // Proposal drafts, per account, role and request (drafts.ts).
  const key = (() => draftKey(app.session?.account?.id, app.role, app.workspaceId, 'plan'))();
  const saved = readFormDraft(key);
  let price = $state(saved.price || '');
  let priceNote = $state(saved.priceNote || '');
  let start = $state(saved.start || '');
  let end = $state(saved.end || '');
  let dateNote = $state(saved.dateNote || '');
  let adjustingEnd = $state(false);
  let error = $state('');
  let proposalSaved = $state(true);
  let previousMode = '';
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  const duration = $derived(job?.quote.durationMinutes || 60);
  let date = $state(saved.start ? saved.start.slice(0, 10) : '');
  let startTime = $state(saved.start ? saved.start.slice(11, 16) : '');
  let endTime = $state(saved.end ? saved.end.slice(11, 16) : '');
  const today = localDateTime(new Date()).slice(0, 10);

  $effect(() => { proposalSaved = saveDraft(key, [price, priceNote, start, end, dateNote].some(Boolean) ? JSON.stringify({ price: String(price ?? ''), priceNote, start, end, dateNote }) : ''); });
  $effect(() => {
    if (previousMode === app.composeMode) return;
    previousMode = app.composeMode;
    error = '';
    if (app.composeMode === 'price' && !price && job) {
      price = String((job.priceProposal?.status === 'pending' ? job.priceProposal.amountMinor : job.quote.totalMinor) / 100);
      if (!priceNote && job.priceProposal?.actor === app.role) priceNote = job.priceProposal.note;
    }
    if (app.composeMode === 'date' && !start && job?.dateProposal?.status === 'pending') {
      setFrom(localDateTime(new Date(job.dateProposal.appointment.startUtc)), localDateTime(new Date(job.dateProposal.appointment.endUtc)));
      if (!dateNote && job.dateProposal.actor === app.role) dateNote = job.dateProposal.note;
    }
  });
  function setFrom(s: string, e: string) { start = s; end = e; date = s.slice(0, 10); startTime = s.slice(11, 16); endTime = e.slice(11, 16); }
  function updateVisit() {
    error = '';
    start = date && startTime ? `${date}T${startTime}` : '';
    if (!start) { end = ''; return; }
    if (!adjustingEnd || !endTime) { end = defaultVisitEnd(start, duration); endTime = end.slice(11, 16); }
    else end = `${end ? end.slice(0, 10) : date}T${endTime}`;
  }

  function onKey(event: KeyboardEvent) {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      if (app.draft.trim().length >= minimum) void actions.sendMessage();
    }
  }
  async function sendProposal(event: SubmitEvent) {
    event.preventDefault();
    if (app.pending || !job) return;
    if (app.composeMode === 'price') {
      error = Number(price) > 0 && Number(price) <= 1000000 && priceNote.trim().length >= 3 ? '' : t('Enter a positive total and a short note about your offer.');
      if (error) return;
      if (await actions.act('propose-price', priceNote, undefined, undefined, { priceAmountMinor: Math.round(Number(price) * 100) })) { price = ''; priceNote = ''; actions.closeProposal(); }
      else error = app.error;
    } else {
      updateVisit();
      error = validateVisit(start, end, duration);
      if (error) return;
      if (await actions.act('propose-date', dateNote, undefined, undefined, { proposedAppointment: { startUtc: new Date(start).toISOString(), endUtc: new Date(end).toISOString(), timezone } })) {
        setFrom('', ''); dateNote = ''; actions.closeProposal();
      } else error = app.error;
    }
  }
  const replying = $derived(!!(app.role === 'professional' ? app.serverState?.proDialog : app.serverState?.agentDialog)?.awaiting);
  // Once a visit is booked a customer can write to the professional or ask their own assistant (move or cancel the visit).
  const canAsk = $derived(!replyOnly && app.role === 'customer' && assigned && !!job?.appointment && !app.serverState?.cancelled && !replying);
  const toAssistant = $derived(canAsk && app.messageTo === 'assistant');
  const placeholder = $derived(replying ? t('Reply to your assistant…') : toAssistant ? t('Ask your assistant, for example “Can we do Thursday instead?”') : assigned ? t('Write to {name}…', { name: recipient || t('them') }) : matching ? t('Tell us what to change about the request…') : app.serverState?.intake ? t('Your answer, in your own words') : t('What’s wrong? For example, my kitchen tap keeps dripping…'));
  const hint = $derived(toAssistant ? t('Your assistant asks you before it moves or cancels anything.') : assigned ? t('Enter sends · Shift + Enter adds a line') : matching ? t('Timing and budget stay saved.') : t('No need to choose a trade'));
</script>

<div class="composer-wrap">
  {#if canPrice || canDate}
    <div class="segmented modes" role="tablist" aria-label={t('Message or proposal')}>
      <button type="button" role="tab" aria-selected={app.composeMode === 'message'} disabled={app.pending} onclick={actions.closeProposal}>{t('Message')}</button>
      {#if canPrice}<button type="button" role="tab" aria-selected={app.composeMode === 'price'} disabled={app.pending} onclick={() => actions.openProposal('price')}>{t('Price offer')}</button>{/if}
      {#if canDate}<button type="button" role="tab" aria-selected={app.composeMode === 'date'} disabled={app.pending} onclick={() => actions.openProposal('date')}>{t(job?.appointment ? 'Change visit' : 'Visit time')}</button>{/if}
    </div>
  {/if}

  {#if app.composeMode !== 'message' && (canPrice || canDate) && job}
    <form class="box proposal" onsubmit={sendProposal}>
      {#if app.composeMode === 'price'}
        <div class="field">
          <label for="proposal-price">{t('New total')}</label>
          <div class="money-input"><span aria-hidden="true">€</span><input id="proposal-price" type="number" inputmode="decimal" min="0.01" max="1000000" step="0.01" bind:value={price} required disabled={app.pending} aria-invalid={error ? 'true' : undefined} aria-describedby="proposal-help proposal-error"/></div>
        </div>
        <div class="field">
          <label for="proposal-note">{t('Note sent with this offer')}</label>
          <textarea id="proposal-note" class="textarea" rows="2" required minlength="3" maxlength="500" bind:value={priceNote} disabled={app.pending} placeholder={t('Explain your offer.')}></textarea>
        </div>
        <p id="proposal-help" class="helper">{t('{name} must accept before the price changes.', { name: recipient || t('The other person') })}</p>
        {#if app.role === 'customer'}<p class="helper">{t('Their assistant can negotiate within its limits. A lower offer may need the professional’s personal approval, so you may have to wait for their reply.')}</p>{/if}
      {:else}
        <div class="visit-grid">
          <div class="field"><label for="proposal-start">{t('Date')}</label><input id="proposal-start" class="input" type="date" min={today} bind:value={date} onchange={updateVisit} required disabled={app.pending} aria-invalid={error ? 'true' : undefined} aria-describedby="proposal-help proposal-error"/></div>
          <div class="field"><label for="proposal-time">{t('Start')}</label><input id="proposal-time" class="input" type="time" step="900" bind:value={startTime} onchange={updateVisit} required disabled={app.pending} aria-describedby="proposal-help proposal-error"/></div>
        </div>
        <p class="helper">{#if end}{t('Ends {time}, {minutes} minutes', { time: time(new Date(end).toISOString()), minutes: duration })}{:else}{t('{minutes} minutes, from the service', { minutes: duration })}{/if} · {timezone}</p>
        <details class="disclosure" bind:open={adjustingEnd}>
          <summary>{t('Adjust the end time')}</summary>
          <div class="field"><label for="proposal-end">{t('End')}</label><input id="proposal-end" class="input end-input" type="time" step="900" bind:value={endTime} onchange={updateVisit} disabled={app.pending}/></div>
        </details>
        <div class="field">
          <label for="proposal-note">{t('Note (optional)')}</label>
          <textarea id="proposal-note" class="textarea" rows="2" maxlength="500" bind:value={dateNote} disabled={app.pending} placeholder={t('Tell them what works for you.')}></textarea>
        </div>
        <p id="proposal-help" class="helper">{t('Nothing is reserved until you both agree.')}</p>
      {/if}
      <p id="proposal-error" class="field-error" role="alert">{error}</p>
      <div class="proposal-foot">
        <span class="helper" role="status">{t(proposalSaved ? 'Draft saved.' : 'Draft storage is unavailable. Keep this tab open.')}</span>
        <button class="btn btn-primary" disabled={app.pending} aria-busy={app.pending ? 'true' : undefined}>{t(app.composeMode === 'price' ? 'Send price offer' : 'Propose this visit')}</button>
      </div>
    </form>
  {:else}
    <form class="box message" class:compact onsubmit={actions.sendMessage}>
      {#if canAsk}
        <div class="segmented to" role="group" aria-label={t('Who is this message for?')}>
          <button type="button" aria-pressed={!toAssistant} disabled={app.pending} onclick={() => { app.messageTo = 'professional'; }}>{t('Message {name}', { name: recipient })}</button>
          <button type="button" aria-pressed={toAssistant} disabled={app.pending} onclick={() => { app.messageTo = 'assistant'; }}>{t('Ask your assistant')}</button>
        </div>
      {/if}
      <label class="visually-hidden" for="message">{replyOnly || toAssistant ? t('Message to your assistant') : assigned ? t('Message to {name}', { name: recipient }) : matching ? t('Correct or add to your job description') : t('Your answer')}</label>
      <textarea id="message" bind:value={app.draft} oninput={() => actions.persistDraft()} {placeholder} rows="2" maxlength="500" disabled={app.pending} required minlength={minimum} aria-describedby="message-hint draft-status" onkeydown={onKey}></textarea>
      <div class="message-foot">
        <span id="message-hint" class="hint">{hint}</span>
        <button class="btn btn-primary send" type="submit" disabled={app.pending || app.draft.trim().length < minimum} aria-label={replyOnly || toAssistant ? t('Send to your assistant') : assigned ? t('Send message to {name}', { name: recipient }) : t('Send')}><span class="send-label">{t('Send')}</span><Icon name="send"/></button>
      </div>
    </form>
    <p id="draft-status" class="draft" role="status">{t(app.draftRestored ? 'Unsent draft restored.' : app.draft && app.draftSaved ? 'Draft saved in this browser tab.' : app.draft && !app.draftSaved ? 'Draft storage is unavailable. Keep this tab open.' : '')}</p>
  {/if}
  {#if !assigned}<p class="privacy"><Icon name="lock" size={14}/>{t('Leave out sensitive personal details.')}</p>{/if}
</div>

<style>
  .composer-wrap { display: grid; gap: 10px; width: 100%; max-width: 820px; margin: 0 auto; }
  .modes { justify-self: start; }
  .box.message, .box.proposal { border: 1px solid var(--sk-field); }
  .message { display: grid; gap: 12px; padding: 18px 20px 16px; }
  .message textarea { width: 100%; min-height: 52px; max-height: 40svh; padding: 0; border: 0; background: transparent; resize: none; color: var(--sk-ink); font: 500 16px/1.45 var(--sk-fd); field-sizing: content; }
  .message textarea:focus-visible { outline: none; }
  .message:focus-within { border-color: var(--sk-ink); }
  .message textarea::placeholder { color: var(--sk-muted); }
  .compact.message { grid-template-columns: 1fr auto; grid-template-areas: 'text send'; align-items: center; gap: 10px; padding: 8px 8px 8px 18px; border-width: 1.5px; box-shadow: none; }
  .compact.message textarea { grid-area: text; }
  .compact .message-foot { grid-area: send; }
  /* With a recipient switch the compact composer is two rows: who it is for, then the one-line message. */
  .compact.message:has(.to) { grid-template-areas: 'to to' 'text send'; padding-top: 10px; }
  .compact .to { grid-area: to; padding: 3px; }
  .compact .to > button { min-height: 30px; padding: 0 12px; }
  @media (max-width: 639px) { .compact .to > button { min-height: 40px; } }
  .compact.message textarea { min-height: 24px; font: 400 14px/1.5 var(--sk-ft); }
  .compact .message-foot { padding: 0; border: 0; }
  .compact .hint, .compact .send-label { display: none; }
  .compact .send { width: 44px; min-height: 44px; padding: 0; }
  .to { justify-self: start; }
  .message-foot { display: flex; align-items: center; gap: 12px; padding-top: 12px; border-top: 0.5px solid var(--sk-line); }
  .hint { flex: 1; font-size: 12px; color: var(--sk-muted); }
  .send :global(.icon) { width: 18px; height: 18px; }
  .proposal { display: grid; gap: 14px; padding: 18px 20px 16px; }
  .visit-grid { display: grid; grid-template-columns: 1fr 160px; gap: 12px; }
  .end-input { max-width: 160px; }
  .proposal-foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; padding-top: 12px; border-top: 0.5px solid var(--sk-line); }
  .draft { min-height: 0; font-size: 12px; color: var(--sk-muted); }
  .draft:empty { display: none; }
  .privacy { display: flex; align-items: center; justify-content: center; gap: 6px; font-size: 12px; color: var(--sk-muted); }
  @media (max-width: 639px) {
    .message, .proposal { padding: 14px; border-radius: var(--sk-r-card); }
    .message textarea { font-size: 15px; min-height: 28px; }
    .hint { display: none; }
    .message-foot { justify-content: flex-end; }
    .send-label { display: none; }
    .send { width: 48px; padding: 0; }
    .visit-grid { grid-template-columns: 1fr 1fr; }
  }
</style>
