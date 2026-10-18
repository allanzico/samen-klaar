<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Logo from '../../ui/Logo.svelte';
  import Pill from '../../ui/Pill.svelte';
  import type { AppState, Intake, Job } from '../../lib/types';
  import { noteText } from '../../lib/agent-copy';
  import { firstName } from '../../lib/status-ui';
  import { money, visitRange } from '../../lib/format';
  import { initials } from '../../lib/trade';
  import { t } from '../../lib/i18n.svelte';

  // The conversation. Messages carry no timestamps, so there are no day dividers or times.
  // Offer cards describe the open offer; their buttons live in the next step.
  let { app, job, intake, onAnswer, answerHint = true }: { app: AppState; job?: Job; intake?: Intake; onAnswer?: () => void; answerHint?: boolean } = $props();
  const role = $derived(app.role);
  const assigned = $derived(!!job?.id);
  const other = $derived(firstName(role === 'professional' ? job?.customer : job?.professional));
  const all = $derived(intake?.messages ?? []);
  const discussion = $derived(assigned ? all.filter((m) => !!m.threadProfileId && m.threadProfileId === job?.professionalProfileId) : all);
  let showAll = $state(false);
  const LIMIT = 8;
  const visible = $derived(showAll ? discussion : discussion.slice(-LIMIT));
  const hidden = $derived(discussion.length - visible.length);

  function sender(message: { role: string; senderName?: string }) {
    if (message.role === role) return t('You');
    if (message.role === 'professional') return firstName(message.senderName || job?.professional) || t('Professional');
    if (message.role === 'customer') return firstName(message.senderName || job?.customer) || t('Customer');
    return t('{brand} assistant', { brand: app.brandName });
  }
  // My answer to my assistant is the last thing in the thread: it is working on the reply.
  const answered = $derived(visible.length > 0 && visible[visible.length - 1].role === role && visible[visible.length - 1].audience === role);
  // Never promise a reply forever: stop saying the assistant is replying after a while.
  let typingExpired = $state(false);
  $effect(() => {
    typingExpired = false;
    if (!answered) return;
    const timer = window.setTimeout(() => { typingExpired = true; }, 25000);
    return () => window.clearTimeout(timer);
  });
  const typing = $derived(answered && !typingExpired);
  const isPerson = (r: string) => r === 'professional' || r === 'customer';
  // Assistants go by short bot names; an older message without one is just "Assistant".
  const agentName = (m: { agent?: string }) => m.agent || t('Assistant');

  // The assistant's payment message points to the step; only the newest one gets the button.
  const lastPaymentLink = $derived([...visible].reverse().find((m) => m.code === 'link:payment'));
  const firstOfRole = (index: number) => !visible.slice(0, index).some((m) => m.role === visible[index].role);
  const roleName = (r: string) => t(r === 'professional' ? 'Professional' : 'Customer');

  const offerMine = (actor: string) => actor === role;
  const showVisitBooked = $derived(!!job?.appointment && job.appointmentConfirmation?.status !== 'pending' && !app.serverState?.cancelled);
</script>

<div class="thread" role="log" aria-label={t('Conversation')} aria-live="polite">
  {#if hidden > 0}
    <button type="button" class="btn btn-light btn-sm earlier" onclick={() => { showAll = true; }}><Icon name="chevron-up"/>{t('Show {count} earlier messages', { count: hidden })}</button>
  {/if}
  {#if assigned && job}
    <p class="system"><Icon name="check" size={14}/>{role === 'professional' ? t('{name} chose you for this job', { name: other || t('The customer') }) : t('You chose {name} for this job', { name: job.professional })}</p>
  {/if}

  {#each visible as message, index (index)}
    {#if message.role === role}
      <div class="mine">
        <p class="bubble">{message.text}</p>
        <span class="meta">{t('You')}</span>
      </div>
    {:else if message.role === 'agent'}
      <div class="theirs">
        <span class="assistant-avatar bot-avatar" aria-hidden="true">{(message.agent || 'A').slice(0, 1)}</span>
        <div class="body">
          <p class="who"><b>{agentName(message)}</b><span class="bot" aria-label={t('Assistant')}><Icon name="bot" size={14}/></span></p>
          <p class="text">{noteText(message)}</p>
          {#if message.code === 'link:payment' && onAnswer && role === 'customer' && job && !job.paid && message === lastPaymentLink}
            <button type="button" class="btn btn-quiet answer" onclick={onAnswer}>{t('Go to the payment step')}</button>
          {/if}
        </div>
      </div>
    {:else}
      <div class="theirs">
        {#if isPerson(message.role)}<span class="avatar" aria-hidden="true">{initials(message.senderName || (message.role === 'professional' ? job?.professional : job?.customer) || '?')}</span>
        {:else}<span class="assistant-avatar" aria-hidden="true"><Logo onNavy wordmark={false} size={22}/></span>{/if}
        <div class="body">
          <p class="who"><b>{sender(message)}</b>{#if isPerson(message.role) && firstOfRole(index)}<span>{roleName(message.role)}</span>{/if}</p>
          <p class="text">{message.text}</p>
        </div>
      </div>
    {/if}
  {/each}


  {#if typing}<p class="typing" role="status"><span class="spinner" aria-hidden="true"></span>{t('Your assistant is replying…')}</p>{/if}

  {#if job && !app.serverState?.cancelled}
    {#if showVisitBooked && job.appointment}
      <div class="agreed">
        <span class="agreed-mark" aria-hidden="true"><Icon name="check" size={16}/></span>
        <div><b>{t('Visit booked · {when}', { when: visitRange(job.appointment.startUtc, job.appointment.endUtc, job.appointment.timezone) })}</b>
          {#if job.appointmentConfirmation?.status === 'confirmed'}<span class="meta">{t('Confirmed by {name}', { name: firstName(job.professional) })}</span>{/if}</div>
      </div>
    {/if}
    {#if !job.workStarted && !job.paid && job.priceProposal?.status === 'pending'}
      {@const mine = offerMine(job.priceProposal.actor)}
      <div class="offer" class:turn={!mine}>
        <div class="offer-head"><Icon name="euro" size={16}/><b>{mine ? t('Your price offer') : t('Price offer from {name}', { name: other })}</b>
          <Pill small variant={mine ? 'waiting' : 'your-turn'} label={mine ? t('Waiting on {name}', { name: other }) : t('Your turn')}/></div>
        <p class="offer-value amount">{money(job.priceProposal.amountMinor, job.quote.currency)}</p>
        {#if job.priceProposal.note}<p class="quote">{job.priceProposal.note}</p>{/if}
        {#if !mine && answerHint}<button type="button" class="btn btn-quiet answer" onclick={onAnswer}>{t('Answer on the right')}</button>{/if}
      </div>
    {/if}
    {#if !job.workStarted && !job.paid && job.dateProposal?.status === 'pending'}
      {@const mine = offerMine(job.dateProposal.actor)}
      <div class="offer" class:turn={!mine}>
        <div class="offer-head"><Icon name="calendar" size={16}/><b>{mine ? t('Your proposed visit') : t('Visit proposed by {name}', { name: other })}</b>
          <Pill small variant={mine ? 'waiting' : 'your-turn'} label={mine ? t('Waiting on {name}', { name: other }) : t('Your turn')}/></div>
        <p class="offer-value amount">{visitRange(job.dateProposal.appointment.startUtc, job.dateProposal.appointment.endUtc, job.dateProposal.appointment.timezone)}</p>
        {#if job.dateProposal.note}<p class="quote">{job.dateProposal.note}</p>{/if}
        {#if !mine && answerHint}<button type="button" class="btn btn-quiet answer" onclick={onAnswer}>{t('Answer on the right')}</button>{/if}
      </div>
    {/if}
    {#if job.change?.status === 'pending'}
      {@const mine = role === 'professional'}
      <div class="offer" class:turn={!mine}>
        <div class="offer-head"><Icon name="wrench" size={16}/><b>{mine ? t('Extra work you proposed') : t('Extra work from {name}', { name: other })}</b>
          <Pill small variant={mine ? 'waiting' : 'your-turn'} label={mine ? t('Waiting on {name}', { name: other }) : t('Your turn')}/></div>
        <p class="offer-value amount">+ {money(job.change.amountMinor, job.quote.currency)}</p>
        <p class="quote">{job.change.description} {t('New total {amount}.', { amount: money(job.quote.totalMinor + job.change.amountMinor, job.quote.currency) })}</p>
        {#if !mine && answerHint}<button type="button" class="btn btn-quiet answer" onclick={onAnswer}>{t('Answer on the right')}</button>{/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .thread { display: flex; flex-direction: column; gap: 18px; width: 100%; max-width: 820px; margin: 0 auto; }
  .earlier { align-self: center; }
  .typing { display: flex; align-items: center; gap: 8px; padding-left: 48px; font-size: 13px; color: var(--sk-muted); }
  @media (max-width: 639px) { .typing { padding-left: 0; } }
  .system { display: flex; align-items: center; justify-content: center; gap: 6px; font-size: 12px; color: var(--sk-muted); text-align: center; }
  .system::before, .system::after { content: ""; flex: 1; max-width: 140px; height: 1px; background: var(--sk-line-2); }
  .theirs { display: grid; grid-template-columns: 36px 1fr; gap: 12px; align-items: start; }
  .assistant-avatar { display: grid; place-items: center; width: 36px; height: 36px; border-radius: var(--sk-r-tile); background: var(--sk-brand); color: var(--sk-on-brand); }
  .who { display: flex; align-items: baseline; gap: 8px; }
  .who b { font-size: 14px; font-weight: 700; }
  .who span { font-size: 12px; color: var(--sk-muted); }
  .bot-avatar { font: 700 15px/1 var(--sk-fd); }
  .who .bot { display: inline-flex; align-items: center; }
  .text { max-width: 62ch; font-size: 14px; line-height: 1.55; white-space: pre-wrap; overflow-wrap: anywhere; }
  .mine { display: grid; justify-items: end; gap: 4px; }
  .bubble { max-width: 78%; padding: 11px 15px; border-radius: 16px 16px 4px 16px; background: var(--sk-soft); font-size: 14px; line-height: 1.55; white-space: pre-wrap; overflow-wrap: anywhere; }
  .meta { font-size: 12px; color: var(--sk-muted); }
  .agreed { display: flex; align-items: center; gap: 12px; max-width: 560px; padding: 14px 16px; border-radius: var(--sk-r-card); background: var(--sk-surface); border: 1px solid var(--sk-line); }
  .agreed b { display: block; font-size: 14px; font-weight: 700; }
  .agreed-mark { display: grid; place-items: center; flex: 0 0 auto; width: 28px; height: 28px; border-radius: 50%; background: var(--sk-brand); color: var(--sk-on-brand); }
  .offer { display: grid; gap: 8px; max-width: 560px; padding: 16px; border-radius: var(--sk-r-card); background: var(--sk-surface); border: 1px solid var(--sk-line); }
  .offer.turn { background: var(--sk-turn-soft); border: 0; }
  .offer-head { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; font-size: 13px; }
  .offer-head b { font-weight: 700; margin-right: auto; }
  .offer-value { font-size: 20px; }
  .answer { justify-self: start; margin-left: -8px; }
  @media (max-width: 1279px) { .answer { display: none; } }
  @media (max-width: 639px) { .thread { gap: 16px; } .bubble { max-width: 88%; } }
</style>
