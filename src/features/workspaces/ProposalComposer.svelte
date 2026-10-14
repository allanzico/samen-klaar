<script lang="ts">
  import type { AppState, AppActions } from '../../lib/types';
  import { defaultVisitEnd, localDateTime, validateVisit } from '../../lib/visit-time';
  import { draftKey, readFormDraft, saveDraft } from '../../lib/drafts';
  import { currentLocale, t } from '../../lib/i18n.svelte';
  let { app, actions }: {app:AppState;actions:AppActions} = $props();
  const key = (() => draftKey(app.session?.account?.id, app.role, app.workspaceId, 'plan'))();
  const saved = readFormDraft(key);
  let price = $state(saved.price || '');
  let priceNote = $state(saved.priceNote || '');
  let start = $state(saved.start || '');
  let end = $state(saved.end || '');
  let dateNote = $state(saved.dateNote || '');
  let error = $state('');
  let draftSaved = $state(true);
  let adjustingEnd = $state(false);
  let previousMode = '';
  let job = $derived(app.serverState!.job);
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  const earliest = localDateTime(new Date(Date.now() + 16 * 60000));
  $effect(() => { draftSaved = saveDraft(key, [price,priceNote,start,end,dateNote].some(Boolean) ? JSON.stringify({price:String(price ?? ''),priceNote,start,end,dateNote}) : ''); });
  $effect(() => {
    if (previousMode !== app.composeMode) {
      previousMode = app.composeMode; error = '';
      if (app.composeMode === 'price' && !price) {
        price = String((job.priceProposal?.status === 'pending' ? job.priceProposal.amountMinor : job.quote.totalMinor) / 100);
        if (!priceNote && job.priceProposal?.actor === app.role) priceNote = job.priceProposal.note;
      }
      if (app.composeMode === 'date' && !start && job.dateProposal?.status === 'pending') {
        start = localDateTime(new Date(job.dateProposal.appointment.startUtc));
        end = localDateTime(new Date(job.dateProposal.appointment.endUtc));
        if (!dateNote && job.dateProposal.actor === app.role) dateNote = job.dateProposal.note;
      }
    }
  });
  function updateStart() { error = ''; if (!adjustingEnd || !end || new Date(end) <= new Date(start)) end = defaultVisitEnd(start,job.quote.durationMinutes); }
  async function send(event: SubmitEvent) {
    event.preventDefault();
    if (app.pending) return;
    if (app.composeMode === 'price') {
      error = Number(price)>0 && Number(price)<=1000000 && priceNote.trim().length>=3 ? '' : t('Enter a positive total and a short note about your offer.');
      if (error) return;
      if (await actions.act('propose-price',priceNote,undefined,undefined,{priceAmountMinor:Math.round(Number(price)*100)})) { price='';priceNote='';actions.closeProposal(); } else error=app.error;
    } else {
      if (!adjustingEnd || !end) end = defaultVisitEnd(start,job.quote.durationMinutes);
      error = validateVisit(start,end,job.quote.durationMinutes);
      if (error) return;
      if (await actions.act('propose-date',dateNote,undefined,undefined,{proposedAppointment:{startUtc:new Date(start).toISOString(),endUtc:new Date(end).toISOString(),timezone}})) { start='';end='';dateNote='';actions.closeProposal(); } else error=app.error;
    }
  }
</script>
{#if app.composeMode !== 'message'}
  <form class="composer proposal-composer" onsubmit={send}>
    <div class="composer-title-row"><h2>{t(app.composeMode === 'price' ? 'Propose a total price' : 'Propose a visit')}</h2><button type="button" class="text-button" disabled={app.pending} onclick={actions.closeProposal}>{t('Back to message')}</button></div>
    {#if app.composeMode === 'price'}
      <label>{t('Proposed total')} ({job.quote.currency})<input id="proposal-price" type="number" bind:value={price} min="0.01" max="1000000" step="0.01" required disabled={app.pending} aria-describedby="proposal-error" aria-invalid={error ? 'true' : undefined}/></label>
      <label for="proposal-note">{t('Note sent with this offer')}<textarea id="proposal-note" bind:value={priceNote} rows="2" required minlength="3" maxlength="500" disabled={app.pending} placeholder={t('Explain your offer or counter-offer.')}></textarea></label>
    {:else}
      <label>{t('Visit start')} ({timezone})<input id="proposal-start" type="datetime-local" bind:value={start} onchange={updateStart} min={earliest} required disabled={app.pending} aria-describedby="proposal-error" aria-invalid={error ? 'true' : undefined}/></label>
      <p class="quiet-note">{job.quote.durationMinutes} {t('minutes')}{end ? ` · ${t('Ends at')} ${new Intl.DateTimeFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB',{hour:'2-digit',minute:'2-digit'}).format(new Date(end))}` : ''}.</p>
      <details bind:open={adjustingEnd}><summary>{t('Adjust the end time')}</summary><label>{t('Visit end')}<input type="datetime-local" bind:value={end} min={defaultVisitEnd(start,job.quote.durationMinutes) || earliest} disabled={app.pending} aria-describedby="proposal-error"/></label></details>
      <label for="proposal-note">{t('Note sent with this visit (optional)')}<textarea id="proposal-note" bind:value={dateNote} rows="2" maxlength="500" disabled={app.pending} placeholder={t('Tell them what works for you.')}></textarea></label>
    {/if}
    <p id="proposal-error" class="field-error" role="alert">{error}</p>
    <div class="composer-bottom"><span>{t(draftSaved ? 'Draft saved. The other person must accept this proposal.' : 'Draft storage is unavailable. Keep this tab open.')}</span><button class="primary" disabled={app.pending}>{t(app.composeMode === 'price' ? 'Send price offer' : 'Propose this visit')}</button></div>
  </form>
{/if}
