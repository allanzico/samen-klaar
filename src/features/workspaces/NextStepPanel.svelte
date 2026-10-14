<script lang="ts">
  import { tick } from 'svelte';
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import { requestStatus } from '../../lib/request-status';
  import { statusUi, canCancelRequest } from '../../lib/status-ui';
  import { dayAndTime, effectiveSlot, planUi, shortVisit } from '../../lib/plan-ui.svelte';
  import { downloadVisitCalendar } from '../../lib/calendar';
  import { t } from '../../lib/i18n.svelte';

  type Button = { label: string; run: () => unknown; disabled?: boolean; reason?: string };
  type View = {
    kind: 'your-turn' | 'waiting' | 'scheduled' | 'done' | 'info';
    title: string; text?: string; amount?: string;
    banner?: { kind: 'error' | 'info'; title?: string; text: string };
    ledger?: [string, string][]; steps?: { text: string; state: 'done' | 'current' | 'todo' }[];
    primary?: Button; secondary?: Button; quiet?: Button[]; danger?: Button; helper?: string;
  };

  let { app, actions, workspace, money, placement }: { app: AppState; actions: AppActions; workspace: WorkspaceState; money: (minor: number) => string; placement: 'panel' | 'dock' } = $props();

  const job = $derived(workspace.job);
  const status = $derived(requestStatus(workspace, 'customer'));
  const proName = $derived(job.professional || t('Professional'));
  const proFirst = $derived(proName.trim().split(/\s+/)[0]);
  const slots = $derived(workspace.availableSlots);
  const needsAddress = $derived(!!job.location && !job.serviceAddress);
  const slot = $derived(effectiveSlot(slots, !needsAddress));
  const total = $derived(money(job.quote.totalMinor));
  const waitingStatus = $derived(status.label.startsWith('Waiting for '));
  const missingDocs = $derived(job.evidenceSubmitted && !job.detailAdded);
  const sandbox = $derived(app.providerConfig?.paymentProvider === 'paypal_sandbox');

  async function openCancel() {
    planUi.cancelling = true;
    await tick();
    document.querySelector('.cancel-confirmation')?.scrollIntoView({ block: 'center' });
  }
  async function openReport() {
    planUi.reportingIssue = true;
    await tick();
    document.querySelector('.issue-report')?.scrollIntoView({ block: 'center' });
  }
  async function pay() {
    if (job.paymentApprovedVersion !== job.scopeVersion && !(await actions.act('approve-payment'))) return;
    if (app.paypalStatus === 'authorized') await actions.capturePayPal();
    else await actions.beginPayPalCheckout();
  }

  const view = $derived.by((): View | null => {
    if (workspace.cancelled || !job.id) return null;
    const ui = statusUi(status.code, status.label, 'customer', proFirst);
    const code = status.code;
    const pickReason = needsAddress ? t('Add your address to continue') : !slot ? t('Choose a time to continue') : '';
    const steps = (current: number) => [t('{name} comes at the agreed time', { name: proFirst }), t('{name} sends photos and a note when done', { name: proFirst }), t('You approve the work, then pay')].map((text, index) => ({ text, state: (index < current ? 'done' : index === current ? 'current' : 'todo') as 'done' | 'current' | 'todo' }));
    const cancelOption: Button[] = canCancelRequest(workspace) ? [{ label: job.appointment ? t('Cancel visit') : t('Cancel request'), run: openCancel }] : [];

    switch (code) {
      case 'quote': {
        const reason = job.needsClarification ? t('Answer the question above to continue') : needsAddress ? t('Add your address to continue') : slots.length && !slot ? t('Choose a time to continue') : '';
        return {
          kind: 'your-turn', title: ui.title,
          primary: slots.length
            ? { label: slot ? t('Approve {amount} & book {time}', { amount: total, time: shortVisit(slot.startUtc, slot.timezone) }) : t('Approve {amount} & book', { amount: total }), run: () => actions.act('approve-and-book', slot?.slotId), disabled: !!reason, reason }
            : { label: t('Approve {amount} & propose a visit time', { amount: total }), run: async () => { if (await actions.act('approve-quote')) actions.openProposal('date'); }, disabled: !!reason, reason },
          helper: slots.length ? t('Confirms the price and holds the time while {name} confirms. You pay only after approving the finished work.', { name: proFirst }) : t('Nothing is reserved until you both agree on a time.'),
          quiet: [{ label: t('Make a price offer'), run: () => actions.openProposal('price') }, { label: t('Choose another professional'), run: () => actions.act('return-to-matches') }]
        };
      }
      case 'booking': case 'choose_new_visit': case 'choose_visit': {
        const declined = code !== 'booking';
        const earlier = job.appointmentConfirmation?.appointment;
        return {
          kind: 'your-turn', title: t('Choose a visit time'),
          banner: declined ? { kind: 'error', title: earlier ? t('{name} couldn’t confirm {time}', { name: proFirst, time: shortVisit(earlier.startUtc, earlier.timezone) }) : t('The visit wasn’t confirmed'), text: t('Nothing was booked or charged. Pick another time below — your price and address are kept.') } : undefined,
          primary: slots.length
            ? { label: slot ? t('Book {time}', { time: shortVisit(slot.startUtc, slot.timezone) }) : t('Book this visit'), run: () => actions.act('book-appointment', slot?.slotId), disabled: !!pickReason, reason: pickReason }
            : { label: t('Suggest a visit time'), run: () => actions.openProposal('date') },
          text: slots.length ? undefined : t('No published times are available. Suggest a visit time below.'),
          quiet: slots.length ? [{ label: t('Suggest my own time'), run: () => actions.openProposal('date') }] : []
        };
      }
      case 'price': {
        const offer = job.priceProposal;
        if (!offer) return null;
        if (waitingStatus) {
          return {
            kind: 'waiting', title: t('Waiting for {person}', { person: proFirst }), text: t('Waiting for {person}. The current price stays unchanged until accepted.', { person: proFirst }),
            primary: { label: t('Approve {amount} & book', { amount: total }), run: () => undefined, disabled: true, reason: t('Waiting for {person} to answer your {amount} offer', { person: proFirst, amount: money(offer.amountMinor) }) },
            quiet: [{ label: t('Change my offer'), run: () => actions.openProposal('price') }, { label: t('Withdraw offer'), run: () => actions.act('withdraw-price', '', undefined, undefined, { proposalId: offer.id }) }]
          };
        }
        return {
          kind: 'your-turn', title: t('{person} proposed a price', { person: proFirst }), amount: money(offer.amountMinor), text: offer.note,
          primary: { label: t('Accept this price'), run: () => actions.act('accept-price', '', undefined, undefined, { proposalId: offer.id }) },
          secondary: { label: t('Counter with a price'), run: () => actions.openProposal('price') },
          quiet: [{ label: t('Decline offer'), run: () => actions.act('decline-price', '', undefined, undefined, { proposalId: offer.id }) }]
        };
      }
      case 'date': {
        const offer = job.dateProposal;
        if (!offer) return null;
        const when = dayAndTime(offer.appointment.startUtc, offer.appointment.endUtc, offer.appointment.timezone);
        if (waitingStatus) {
          return {
            kind: 'waiting', title: t('Waiting for {person}', { person: proFirst }), amount: when, text: t('Waiting for {person}. This time is not reserved.', { person: proFirst }),
            quiet: [{ label: t('Change my proposed visit'), run: () => actions.openProposal('date') }, { label: t('Withdraw proposal'), run: () => actions.act('withdraw-date', '', undefined, undefined, { proposalId: offer.id }) }]
          };
        }
        return {
          kind: 'your-turn', title: t('{person} proposed a visit', { person: proFirst }), amount: when, text: offer.note || undefined, helper: t('Accepting reserves this visit at the agreed price.'),
          primary: { label: t('Accept and book this visit'), run: () => actions.act('accept-date', '', undefined, undefined, { proposalId: offer.id }) },
          secondary: { label: t('Suggest another time'), run: () => actions.openProposal('date') },
          quiet: [{ label: t('Decline visit'), run: () => actions.act('decline-date', '', undefined, undefined, { proposalId: offer.id }) }]
        };
      }
      case 'appointment_confirmation':
        return { kind: 'waiting', title: t('Time held — {name} confirms next', { name: proFirst }), amount: job.appointment ? dayAndTime(job.appointment.startUtc, job.appointment.endUtc, job.appointment.timezone) : undefined, text: t('The professional has two business hours to confirm this reserved time.'), steps: steps(0), danger: cancelOption[0] };
      case 'booked':
        return {
          kind: 'scheduled', title: ui.title, amount: job.appointment ? dayAndTime(job.appointment.startUtc, job.appointment.endUtc, job.appointment.timezone) : undefined, steps: steps(0),
          secondary: job.appointment ? { label: t('Add to calendar'), run: () => downloadVisitCalendar(job.id, `${job.title} · ${proName}`, job.appointment!.startUtc, job.appointment!.endUtc) } : undefined,
          danger: cancelOption[0]
        };
      case 'working':
        return { kind: 'scheduled', title: t('{name} is working on it', { name: proFirst }), steps: steps(1), banner: { kind: 'info', text: t('If {name} finds more to fix, they’ll send it here first. Nothing extra happens without your approval.', { name: proFirst }) } };
      case 'change': {
        const change = job.change;
        if (!change) return null;
        return {
          kind: 'your-turn', title: t('Review the additional work'), text: change.description,
          ledger: [[t('Agreed so far'), money(job.quote.totalMinor)], [t('Extra work'), `+ ${money(change.amountMinor)}`], [t('New total'), money(job.quote.totalMinor + change.amountMinor)]],
          primary: { label: t('Approve new total · {amount}', { amount: money(job.quote.totalMinor + change.amountMinor) }), run: () => actions.act('approve-change') },
          secondary: { label: t('Keep the original plan'), run: () => actions.act('decline-change') }
        };
      }
      case 'review':
        if (missingDocs) {
          return {
            kind: job.detailsRequested ? 'waiting' : 'your-turn', title: job.detailsRequested ? t('Waiting for {name} to add the work note', { name: proFirst }) : t('Ask {name} for the work note', { name: proFirst }),
            primary: job.detailsRequested ? undefined : { label: t('Ask {name} for the work note', { name: proFirst }), run: () => actions.act('request-details') },
            secondary: { label: t('Approve the work'), run: () => undefined, disabled: true, reason: t('Available once the work note is added') }
          };
        }
        return {
          kind: 'your-turn', title: t('{name} says it’s done. Check their work.', { name: proFirst }),
          primary: { label: t('Approve the work'), run: () => actions.act('approve-completion') },
          secondary: { label: t('Something’s not right'), run: openReport },
          helper: t('Approving doesn’t pay yet — you’ll confirm payment next.')
        };
      case 'payment': {
        const base: View = { kind: 'your-turn', title: t('Pay {name}', { name: proFirst }), amount: total, banner: sandbox ? { kind: 'info', text: t('Test mode. Payments use the PayPal sandbox — no real money moves.') } : undefined };
        if (!sandbox) {
          return job.paymentApprovedVersion !== job.scopeVersion
            ? { ...base, primary: { label: t('Approve amount · {amount}', { amount: total }), run: () => actions.act('approve-payment') } }
            : { ...base, primary: { label: t('Pay {amount} with PayPal', { amount: total }), run: () => undefined, disabled: true, reason: t('Payments are not enabled for this environment.') } };
        }
        if (job.paymentApprovedVersion === job.scopeVersion && ['capture_uncertain', 'authorization_uncertain', 'order_uncertain', 'void_uncertain', 'captured'].includes(app.paypalStatus)) {
          return { ...base, secondary: { label: t('Check payment status'), run: () => actions.reconcilePayPal() }, helper: t('Completion and payment are separate decisions. Payment is handled securely by PayPal.') };
        }
        return {
          ...base, text: app.paypalStatus === 'authorized' ? t('The amount is authorized. Confirm to complete the payment.') : undefined,
          primary: { label: t('Pay {amount} with PayPal', { amount: total }), run: pay },
          helper: t('You’ll confirm the amount on PayPal, then return here.')
        };
      }
      case 'paid':
        return { kind: 'done', title: job.paymentRefunded ? t('Full refund complete') : t('All done'), text: job.paymentRefunded ? `${t('Your full PayPal refund of')} ${total} ${t('is complete.')}` : t('Paid {amount}.', { amount: total }), secondary: { label: t('Start a new request'), run: () => actions.createRequest() } };
      case 'dispute_review':
        return { kind: 'info', title: ui.title, text: t(job.paid ? 'A payment dispute is under operator review.' : 'Payment is paused.') };
      case 'refund_pending':
        return { kind: 'info', title: ui.title, text: t('A full refund was approved and is being processed through PayPal.') };
      case 'refunded':
        return { kind: 'info', title: t('Full refund complete'), text: t('The full PayPal payment has been refunded.') };
      case 'cancellation_review':
        return { kind: 'info', title: ui.title, text: t('Your cancellation request is under review. The visit remains booked until an operator updates the request.') };
      default:
        return null;
    }
  });

  const eyebrow = $derived(view?.kind === 'your-turn' ? t('Your turn') : view?.kind === 'done' ? t('Klaar') : t('Next step'));
</script>

{#snippet button(btn: Button, kind: 'primary' | 'secondary' | 'quiet' | 'danger')}
  <button class:primary={kind === 'primary'} class:secondary={kind === 'secondary'} class:text-button={kind === 'quiet'} class:danger={kind === 'danger'} class="next-step-button" disabled={app.pending || btn.disabled} aria-busy={kind === 'primary' && app.pending && !btn.disabled ? 'true' : undefined} onclick={() => { void btn.run(); }}>{btn.label}</button>
  {#if btn.reason}<p class="reason">{btn.reason}</p>{/if}
{/snippet}

{#if view}
  <section class="next-step {view.kind} next-step-{placement}" aria-label={t('Next step')}>
    {#if view.banner}<div class="banner {view.banner.kind}" role={view.banner.kind === 'error' ? 'alert' : undefined}>{#if view.banner.title}<b>{view.banner.title}</b>{/if}<span>{view.banner.text}</span></div>{/if}
    <p class="eyebrow next-step-eyebrow">{eyebrow}</p>
    <h2 class="next-step-title">{view.title}</h2>
    {#if view.amount}<p class="next-step-amount">{view.amount}</p>{/if}
    {#if view.text}<p class="next-step-text">{view.text}</p>{/if}
    {#if view.ledger}<dl class="ledger">{#each view.ledger as [label, value], index}<div class:total-row={index === view.ledger.length - 1}><dt>{label}</dt><dd>{value}</dd></div>{/each}</dl>{/if}
    {#if view.steps}
      <p class="eyebrow next-step-after">{t('After that')}</p>
      <ol class="after-steps">{#each view.steps as step, index}<li class={step.state}><span class="step-dot" aria-hidden="true">{#if step.state === 'done'}<svg class="icon" viewBox="0 0 24 24"><path d="m5 12 5 5 9-10"/></svg>{:else}{index + 1}{/if}</span>{step.text}</li>{/each}</ol>
    {/if}
    <div class="next-step-actions">
      {#if view.primary}{@render button(view.primary, 'primary')}{/if}
      {#if view.secondary}{@render button(view.secondary, 'secondary')}{/if}
      {#if view.helper}<p class="helper">{view.helper}</p>{/if}
      {#if view.quiet?.length}<div class="next-step-quiet">{#each view.quiet as btn}{@render button(btn, 'quiet')}{/each}</div>{/if}
      {#if view.danger}{@render button(view.danger, 'danger')}{/if}
    </div>
  </section>
{/if}
