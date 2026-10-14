<script lang="ts">
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import type { StepView as View } from '../../lib/step-view';
  import { requestStatus } from '../../lib/request-status';
  import { statusUi } from '../../lib/status-ui';
  import { dayAndTime, shortVisit } from '../../lib/plan-ui.svelte';
  import { shortName } from '../../lib/trade';
  import { t } from '../../lib/i18n.svelte';
  import StepCard from './StepCard.svelte';

  let { app, actions, workspace, money, placement, noteReady, saveNote, openExtra }: {
    app: AppState; actions: AppActions; workspace: WorkspaceState; money: (minor: number) => string; placement: 'panel' | 'dock';
    noteReady: boolean; saveNote: () => unknown; openExtra: () => void;
  } = $props();

  const job = $derived(workspace.job);
  const status = $derived(requestStatus(workspace, 'professional'));
  const customer = $derived(shortName(job.customer || t('Customer')));
  const waitingStatus = $derived(status.label.startsWith('Waiting for '));
  const total = $derived(money(job.quote.totalMinor));
  const visitText = $derived(job.appointment ? dayAndTime(job.appointment.startUtc, job.appointment.endUtc, job.appointment.timezone) : undefined);

  const view = $derived.by((): View | null => {
    if (workspace.cancelled || !job.id) return null;
    const ui = statusUi(status.code, status.label, 'professional', customer);
    const extra = job.quoteApproved && !job.paid && job.change?.status !== 'pending' ? [{ label: t('Propose extra work'), run: openExtra }] : [];
    const message = { label: t('Message {name}', { name: customer }), run: actions.focusComposer };
    const steps = (current: number) => [t('You start the job on the day'), t('You send photos and a note when done'), t('{name} approves the work, then pays', { name: customer })].map((text, index) => ({ text, state: (index < current ? 'done' : index === current ? 'current' : 'todo') as 'done' | 'current' | 'todo' }));

    switch (status.code) {
      case 'quote':
        return { kind: 'waiting', title: t('Waiting for {name} to approve the plan', { name: customer }), text: t('The customer must approve the plan before a visit is booked. Use Price offer to propose a different total.'), quiet: [{ label: t('Make a price offer'), run: () => actions.openProposal('price') }] };
      case 'booking': case 'choose_new_visit': case 'choose_visit':
        return { kind: 'waiting', title: t('Waiting for {name} to choose a time', { name: customer }), quiet: [{ label: t('Suggest a visit time'), run: () => actions.openProposal('date') }] };
      case 'price': {
        const offer = job.priceProposal;
        if (!offer) return null;
        if (waitingStatus) return { kind: 'waiting', title: t('Waiting for {person}', { person: customer }), text: t('Waiting for {person}. The current price stays unchanged until accepted.', { person: customer }), quiet: [{ label: t('Change my offer'), run: () => actions.openProposal('price') }, { label: t('Withdraw offer'), run: () => actions.act('withdraw-price', '', undefined, undefined, { proposalId: offer.id }) }] };
        return {
          kind: 'your-turn', title: t('{person} proposed a price', { person: customer }), amount: money(offer.amountMinor), text: offer.note,
          primary: { label: t('Accept this price'), run: () => actions.act('accept-price', '', undefined, undefined, { proposalId: offer.id }) },
          secondary: { label: t('Counter with a price'), run: () => actions.openProposal('price') },
          quiet: [{ label: t('Decline offer'), run: () => actions.act('decline-price', '', undefined, undefined, { proposalId: offer.id }) }]
        };
      }
      case 'date': {
        const offer = job.dateProposal;
        if (!offer) return null;
        const when = dayAndTime(offer.appointment.startUtc, offer.appointment.endUtc, offer.appointment.timezone);
        if (waitingStatus) return { kind: 'waiting', title: t('Waiting for {person}', { person: customer }), amount: when, text: t('Waiting for {person}. This time is not reserved.', { person: customer }), quiet: [{ label: t('Change my proposed visit'), run: () => actions.openProposal('date') }, { label: t('Withdraw proposal'), run: () => actions.act('withdraw-date', '', undefined, undefined, { proposalId: offer.id }) }] };
        return {
          kind: 'your-turn', title: t('{person} proposed a visit', { person: customer }), amount: when, text: offer.note || undefined, helper: t('Accepting reserves this visit at the agreed price.'),
          primary: { label: t('Accept and book this visit'), run: () => actions.act('accept-date', '', undefined, undefined, { proposalId: offer.id }) },
          secondary: { label: t('Suggest another time'), run: () => actions.openProposal('date') },
          quiet: [{ label: t('Decline visit'), run: () => actions.act('decline-date', '', undefined, undefined, { proposalId: offer.id }) }]
        };
      }
      case 'appointment_confirmation': {
        const due = job.appointmentConfirmation?.dueAt;
        return {
          kind: 'your-turn', title: t('Confirm visit'), amount: visitText,
          text: due && job.appointment ? t('Confirm by {time}', { time: shortVisit(due, job.appointment.timezone) }) : t('This visit is reserved for you. Confirm it within two business hours or decline so the customer can choose another time.'),
          primary: { label: t('Confirm visit'), run: () => actions.act('confirm-appointment') },
          secondary: message,
          danger: { label: t('Decline and release the time'), run: () => actions.act('decline-appointment') }
        };
      }
      case 'booked':
        return {
          kind: 'scheduled', title: t('Visit day'), amount: visitText,
          text: job.serviceAddress || t('The customer must add the full address before approving the plan.'),
          list: { title: t('Proof needed when done'), items: [t('Photos of the finished work'), t('A short work note')] },
          primary: { label: t('Start job'), run: () => actions.act('start-job') },
          secondary: job.serviceAddress ? { label: t('Directions'), href: `https://maps.google.com/?q=${encodeURIComponent(job.serviceAddress)}` } : undefined,
          helper: t('Tells {name} you’ve arrived and started', { name: customer }),
          quiet: [message, ...extra]
        };
      case 'working': {
        const hasPhoto = job.evidence.some((item) => item.assetId);
        const reason = !hasPhoto ? t('Add at least one photo to continue') : !job.detailAdded ? t('Add a work note to continue') : '';
        return {
          kind: 'your-turn', title: t('Finish and send proof'),
          primary: { label: t('Send to {name} for approval', { name: customer }), run: () => actions.act('submit-evidence'), disabled: !!reason, reason },
          secondary: { label: t('Propose extra work'), run: openExtra },
          helper: t('{name} approves the work, then pays {amount}', { name: customer, amount: total })
        };
      }
      case 'change':
        return {
          kind: 'waiting', title: t('Waiting for {person}', { person: customer }), text: job.change?.description,
          primary: { label: t('Send to {name} for approval', { name: customer }), disabled: true, reason: t('Waiting for {person} to answer your extra work offer', { person: customer }) }
        };
      case 'review':
        if (job.detailsRequested && !job.detailAdded) {
          return {
            kind: 'your-turn', title: t('{name} asked for the work note', { name: customer }),
            primary: { label: t('Add work note'), run: saveNote, disabled: !noteReady, reason: noteReady ? '' : t('Write at least 12 characters to continue') }
          };
        }
        return { kind: 'waiting', title: t('Sent for approval'), text: t('Waiting for the customer’s next step.'), steps: steps(2), quiet: extra };
      case 'payment':
        return { kind: 'waiting', title: t('Waiting for {name} to pay', { name: customer }), amount: total };
      case 'paid':
        return { kind: 'done', title: t('Payment complete.'), amount: total };
      case 'dispute_review':
        return { kind: 'info', title: ui.title, text: t(job.paid ? 'A payment dispute is under operator review.' : 'An issue is under operator review. Payment is paused.') };
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
</script>

<StepCard {app} {view} {placement}/>
