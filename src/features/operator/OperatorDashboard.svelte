<script lang="ts">
import CaseDetailPanel from './CaseDetailPanel.svelte';
import PolicyAdmin from './PolicyAdmin.svelte';
  import { onMount } from 'svelte';
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import Banner from '../../ui/Banner.svelte';
  import AppBar from '../shell/AppBar.svelte';
  import { showToast } from '../../ui/toast-state.svelte';
  import type { AppActions, AppState, NotificationDeliveryHealth, OperatorCase, ProfessionalApplication, ProfessionalReviewChecks } from '../../lib/types';
  import type { PillVariant } from '../../lib/status-ui';
  import { api } from '../../lib/api';
  import { caseDescription } from '../../lib/operator-copy';
  import { money, time, dayTime } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';

  // G01–G13. Operator decisions stay disabled until the reason has 10 characters.
  let { app, actions }: { app: AppState; actions: AppActions } = $props();
  let cases = $state<OperatorCase[]>([]);
  let applications = $state<ProfessionalApplication[]>([]);
  let delivery = $state<NotificationDeliveryHealth>({ failed: 0, unconfigured: 0, stalled: 0 });
  let loading = $state(true);
  let pending = $state('');
  let error = $state('');
  let notes = $state<Record<string, string>>({});
  let tab = $state<'applications' | 'exceptions'>('applications');
  let selectedId = $state('');
  let view = $state<'queue' | 'policies'>('queue');
  let updatedAt = $state<Date | null>(null);

  onMount(() => { void load(); });
  async function load() {
    loading = true;
    error = '';
    try {
      const result = await api.operatorQueue();
      cases = result.cases;
      applications = result.professionalApplications;
      delivery = result.notificationDelivery;
      updatedAt = new Date();
    } catch (e) { error = e instanceof Error ? e.message : 'The operator queue could not be loaded.'; }
    finally { loading = false; }
  }

  const ready = (checks: ProfessionalReviewChecks) => checks.identityBusiness === 'verified' && checks.paypalOnboarding === 'verified'
    && (checks.qualification === 'verified' || (checks.qualification === 'not_required' && checks.qualificationRationale.trim().length > 0))
    && (checks.insurance === 'verified' || (checks.insurance === 'not_required' && checks.insuranceRationale.trim().length > 0));

  async function review(application: ProfessionalApplication, decision: 'save' | 'approve' | 'reject') {
    pending = application.profile.id;
    error = '';
    try {
      await api.reviewProfessional(application.profile.id, decision, application.checks);
      showToast(decision === 'save' ? t('Checks saved') : decision === 'approve' ? t('Professional approved: {name}', { name: application.profile.displayName }) : t('Application declined: {name}', { name: application.profile.displayName }));
      await load();
    } catch (e) { error = e instanceof Error ? e.message : 'The application could not be reviewed.'; }
    finally { pending = ''; }
  }
  async function resolve(item: OperatorCase, decision: string) {
    pending = item.id;
    error = '';
    try {
      await api.resolveWorkCase(item.workspaceId, item.kind, decision, notes[item.id] ?? '');
      showToast(t('Decision recorded'));
      await load();
    } catch (e) { error = e instanceof Error ? e.message : 'The case could not be resolved.'; }
    finally { pending = ''; }
  }

  function kind(item: OperatorCase): { variant: PillVariant; label: string } {
    switch (item.kind) {
      case 'job-issue':
        if (item.status === 'refund_pending') return { variant: 'pending', label: t('Refund processing') };
        if (item.status === 'partial_refund_review' || item.status === 'payment_reversal_review') return { variant: 'warn', label: t('PayPal reconciliation') };
        return { variant: 'error', label: t('Work issue') };
      case 'late-cancellation': return { variant: 'warn', label: t('Late cancellation') };
      case 'late-reschedule': return { variant: 'warn', label: t('Late reschedule') };
      case 'withdrawal': return { variant: 'warn', label: t('Withdrawal request') };
      case 'stalled-job': return { variant: 'warn', label: t('No reply 24 h') };
      case 'scope-change': return { variant: 'warn', label: t('Scope change') };
      case 'area-waitlist': return { variant: 'neutral', label: t('No coverage') };
      case 'intake-review': return { variant: 'pending', label: t('Pending review') };
      default: return { variant: 'warn', label: t('Missing evidence') };
    }
  }
  const caseTitle = (item: OperatorCase) => item.jobTitle || (item.kind === 'area-waitlist' ? t('Coverage request · {area}', { area: item.location?.label || t('area not set') }) : item.kind === 'intake-review' ? t('Find a safe next step') : t('Review this job'));
  const caseLine = (item: OperatorCase) => [item.category ? t(item.category) : '', item.location?.label ?? '', item.reportedAt ? dayTime(item.reportedAt) : ''].filter(Boolean).join(', ');
  const checkOptions = {
    identityBusiness: [['verified', 'Verified'], ['pending', 'Not yet']],
    qualification: [['verified', 'Verified'], ['pending', 'Not yet'], ['not_required', 'Not required']],
    insurance: [['verified', 'Verified'], ['pending', 'Not yet'], ['not_required', 'Not required']]
  } as const;

  const selectedApplication = $derived(tab === 'applications' ? applications.find((a) => a.profile.id === selectedId) : undefined);
  const selectedCase = $derived(tab === 'exceptions' ? cases.find((c) => c.id === selectedId) : undefined);
  $effect(() => {
    if (tab === 'applications' && !applications.length && cases.length) tab = 'exceptions';
    else if (tab === 'exceptions' && !cases.length && applications.length) tab = 'applications';
    const ids = tab === 'applications' ? applications.map((a) => a.profile.id) : cases.map((c) => c.id);
    if (!ids.includes(selectedId)) selectedId = ids[0] ?? '';
  });
  function blocker(checks: ProfessionalReviewChecks) {
    if (checks.paypalOnboarding !== 'verified') return t('Not ready to approve. PayPal seller onboarding isn’t connected yet.');
    if (checks.identityBusiness !== 'verified') return t('Not ready to approve. Identity and business aren’t verified yet.');
    return t('Not ready to approve. Answer every check, with a reason for anything not required.');
  }
</script>

{#snippet bar()}
  {#if updatedAt}<span class="updated">{t('Updated {time}', { time: time(updatedAt.toISOString()) })}</span>{/if}
  <button type="button" class="btn btn-sm btn-outline-navy" disabled={loading} onclick={load}><Icon name="refresh"/>{t('Refresh queue')}</button>
{/snippet}

{#snippet decision(item: OperatorCase, options: { label: string; decision: string; primary?: boolean; danger?: boolean }[])}
  <section class="box card" aria-labelledby="decision-{item.id}">
    <h3 id="decision-{item.id}" class="small-title">{t('Your decision')}</h3>
    <div class="field">
      <label for="reason-{item.id}">{t('Reason')}</label>
      <textarea id="reason-{item.id}" class="textarea" rows="3" maxlength="1000" bind:value={notes[item.id]} disabled={pending === item.id} aria-describedby="reason-help-{item.id}"></textarea>
      <p id="reason-help-{item.id}" class="field-help">{(notes[item.id] ?? '').trim().length < 10 ? t('Write at least 10 characters to continue') : t('Saved with the decision. Visible to operators only.')}</p>
    </div>
    <div class="row">
      {#each options as option (option.decision)}
        <button type="button" class="btn {option.primary ? 'btn-primary' : option.danger ? 'btn-danger' : 'btn-outline'}" disabled={pending === item.id || (notes[item.id] ?? '').trim().length < 10} aria-busy={pending === item.id ? 'true' : undefined} onclick={() => resolve(item, option.decision)}>{option.label}</button>
      {/each}
    </div>
  </section>
{/snippet}

<AppBar {app} {actions} badge={t('Operator')} extra={bar} onHome={() => { window.location.assign('/operator'); }}/>

<main id="main-content" class="operator">
  {#if delivery.failed || delivery.unconfigured || delivery.stalled}
    <div class="delivery">
      <Banner kind="warn" title={t('Email updates need attention')}>
        {#if delivery.failed}<p>{t('{count} emails failed. Retrying automatically.', { count: delivery.failed })}</p>{/if}
        {#if delivery.unconfigured}<p>{t('{count} updates are waiting because email isn’t set up. In-app notifications still work.', { count: delivery.unconfigured })}</p>{/if}
        {#if delivery.stalled}<p>{t('{count} sends have been stuck for over 15 minutes and are being recovered.', { count: delivery.stalled })}</p>{/if}
      </Banner>
    </div>
  {/if}
  <div class="layout">
    <aside class="list" aria-label={t('Review queue')}>
      <h1 class="list-title">{t('Operator dashboard')}</h1>
      <div class="segmented tabs" role="tablist" aria-label={t('Operator sections')}>
        <button type="button" role="tab" aria-selected={view === 'queue'} onclick={() => { view = 'queue'; }}>{t('Review queue')}</button>
        <button type="button" role="tab" aria-selected={view === 'policies'} onclick={() => { view = 'policies'; }}>{t('Policies')}</button>
      </div>
      {#if view === 'queue'}
      <div class="segmented tabs" role="tablist" aria-label={t('Review queue')}>
        <button type="button" role="tab" id="tab-applications" aria-selected={tab === 'applications'} aria-controls="queue" onclick={() => { tab = 'applications'; }}>{t('Applications ({count})', { count: applications.length })}</button>
        <button type="button" role="tab" id="tab-exceptions" aria-selected={tab === 'exceptions'} aria-controls="queue" onclick={() => { tab = 'exceptions'; }}>{t('Exceptions ({count})', { count: cases.length })}</button>
      </div>
      <div id="queue" class="queue" role="tabpanel" aria-labelledby={tab === 'applications' ? 'tab-applications' : 'tab-exceptions'}>
        {#if loading && !updatedAt}<p class="status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Loading the review queue…')}</p>
        {:else if tab === 'applications'}
          {#each applications as application (application.profile.id)}
            <button type="button" class="queue-row" aria-current={application.profile.id === selectedId ? 'true' : undefined} onclick={() => { selectedId = application.profile.id; }}>
              <span class="queue-top"><b>{application.profile.displayName}</b><Pill variant="pending" label={t('Pending review')}/></span>
              <small>{t(application.profile.category)}, {application.profile.serviceArea}</small>
            </button>
          {/each}
        {:else}
          {#each cases as item (item.id)}
            <button type="button" class="queue-row" aria-current={item.id === selectedId ? 'true' : undefined} onclick={() => { selectedId = item.id; }}>
              <span class="queue-top"><b>{caseTitle(item)}</b><Pill variant={kind(item).variant} label={kind(item).label}/></span>
              <small>{caseLine(item) || t('Needs classification')}</small>
            </button>
          {/each}
        {/if}
      </div>
      {/if}
    </aside>

    <section class="detail" aria-live="polite">
      {#if error}<Banner kind="error" text={t(error)} onDismiss={() => { error = ''; }}/>{/if}
      {#if view === 'policies'}
        <PolicyAdmin/>
      {:else if !loading && !applications.length && !cases.length}
        <div class="empty">
          <span class="icon-tile icon-tile-48" aria-hidden="true"><Icon name="check"/></span>
          <h2>{t('Nothing needs review')}</h2>
          <p>{t('The queue is clear. New applications and exceptions appear here.')}</p>
          <button type="button" class="btn btn-outline" onclick={load}>{t('Refresh queue')}</button>
        </div>
      {:else if selectedApplication}
        {@const application = selectedApplication}
        <header class="detail-head">
          <div><h2 class="detail-title">{application.profile.displayName}</h2>{#if application.email}<p class="secondary-text">{application.email}</p>{/if}</div>
          <Pill variant="pending" label={t('Pending review')}/>
        </header>
        <section class="box card">
          <p class="eyebrow small">{t('Service')}</p>
          <h3 class="small-title">{application.profile.serviceName}</h3>
          <p class="secondary-text">{t('{category}, {amount} including call-out, {minutes} minutes', { category: t(application.profile.category), amount: money(application.profile.priceMinor + application.profile.calloutMinor, application.profile.currency), minutes: application.profile.durationMinutes })}</p>
          {#if application.profile.serviceDescription}<p>{application.profile.serviceDescription}</p>{/if}
        </section>
        <section class="box card">
          <p class="eyebrow small">{t('Coverage and terms')}</p>
          <p><b>{t('Service areas:')}</b> {application.profile.serviceArea}</p>
          <p><b>{t('Materials:')}</b> {application.profile.materialsPolicy}</p>
          {#if application.profile.exclusions?.length}<p><b>{t('Not included:')}</b> {application.profile.exclusions.join(', ')}</p>{/if}
        </section>
        <section class="box card">
          <p class="eyebrow small">{t('Checks')}</p>
          <p class="secondary-text">{t('Record the result and a short reason. No document numbers or files.')}</p>
          <ul class="rows checks">
            <li><span id="chk-identity">{t('Identity and business')}</span>
              <div class="segmented" role="radiogroup" aria-labelledby="chk-identity">{#each checkOptions.identityBusiness as [value, label]}<button type="button" role="radio" aria-checked={application.checks.identityBusiness === value} onclick={() => { application.checks.identityBusiness = value; }}>{t(label)}</button>{/each}</div></li>
            <li><span>{t('PayPal seller onboarding')}</span><Pill variant={application.checks.paypalOnboarding === 'verified' ? 'done' : 'neutral'} label={application.checks.paypalOnboarding === 'verified' ? t('Connected and eligible') : t('Not connected')}/></li>
            <li><span id="chk-qualification">{t('Relevant qualification')}</span>
              <div class="segmented" role="radiogroup" aria-labelledby="chk-qualification">{#each checkOptions.qualification as [value, label]}<button type="button" role="radio" aria-checked={application.checks.qualification === value} onclick={() => { application.checks.qualification = value; }}>{t(label)}</button>{/each}</div></li>
            {#if application.checks.qualification === 'not_required'}<li class="why"><div class="field"><label for="why-qualification">{t('Why isn’t a qualification needed for this service?')}</label><textarea id="why-qualification" class="textarea" rows="2" maxlength="500" bind:value={application.checks.qualificationRationale}></textarea></div></li>{/if}
            <li><span id="chk-insurance">{t('Relevant insurance')}</span>
              <div class="segmented" role="radiogroup" aria-labelledby="chk-insurance">{#each checkOptions.insurance as [value, label]}<button type="button" role="radio" aria-checked={application.checks.insurance === value} onclick={() => { application.checks.insurance = value; }}>{t(label)}</button>{/each}</div></li>
            {#if application.checks.insurance === 'not_required'}<li class="why"><div class="field"><label for="why-insurance">{t('Why isn’t insurance needed for this service?')}</label><textarea id="why-insurance" class="textarea" rows="2" maxlength="500" bind:value={application.checks.insuranceRationale}></textarea></div></li>{/if}
          </ul>
        </section>
        {#if !ready(application.checks)}<Banner kind="warn" text={blocker(application.checks)}/>{/if}
        <div class="row">
          <div class="stack">
            <button type="button" class="btn btn-primary" disabled={pending === application.profile.id || !ready(application.checks)} aria-describedby="approve-reason" onclick={() => review(application, 'approve')}>{t('Approve professional')}</button>
            {#if !ready(application.checks)}<p id="approve-reason" class="reason">{t('Answer every check and connect PayPal to approve.')}</p>{/if}
          </div>
          <button type="button" class="btn btn-outline" disabled={pending === application.profile.id} onclick={() => review(application, 'save')}>{t('Save checks')}</button>
          <button type="button" class="btn btn-danger-link" disabled={pending === application.profile.id} onclick={() => review(application, 'reject')}>{t('Decline')}</button>
        </div>
      {:else if selectedCase}
        {@const item = selectedCase}
        <header class="detail-head"><div><h2 class="detail-title">{caseTitle(item)}</h2>{#if caseLine(item)}<p class="secondary-text">{caseLine(item)}</p>{/if}</div><Pill variant={kind(item).variant} label={kind(item).label}/></header>
        <dl class="strip">
          <div><dt>{t('Category')}</dt><dd>{item.category ? t(item.category) : t('Needs classification')}</dd></div>
          {#if item.kind === 'job-issue'}<div><dt>{t('Payment')}</dt><dd>{item.paid ? t('Captured') : t('Not captured')}</dd></div>{/if}
          {#if item.urgency}<div><dt>{t('Timing')}</dt><dd>{t(item.urgency)}</dd></div>{/if}
          {#if item.reportedAt}<div><dt>{t('Reported')}</dt><dd>{dayTime(item.reportedAt)}</dd></div>{/if}
        </dl>
        <section class="box card">
          <p class="eyebrow small">{item.kind === 'job-issue' ? t('Customer’s report') : t('What happened')}</p>
          <p class="quote">{caseDescription(item)}</p>
          {#if item.detail}<p>{item.detail}</p>{/if}
        </section>
        {#key item.id}<CaseDetailPanel {item}/>{/key}
        {#if item.kind === 'late-cancellation'}
          <Banner kind="info" text={t('Within 24 hours of the visit. Approving releases the visit. No fee.')}/>
          {@render decision(item, [{ label: t('Approve cancellation'), decision: 'approve-cancellation', primary: true }, { label: t('Keep visit booked'), decision: 'keep-visit' }])}
        {:else if item.kind === 'late-reschedule'}
          <Banner kind="info" text={t('The visit is within 24 hours. Approving moves the booking to the proposed time. The old time is released. No fee.')}/>
          {@render decision(item, [{ label: t('Approve reschedule'), decision: 'approve-reschedule', primary: true }, { label: t('Keep visit booked'), decision: 'keep-visit' }])}
        {:else if item.kind === 'withdrawal'}
          <Banner kind="info" text={item.paid ? t('This job is paid. A withdrawal can’t cancel it automatically. Refer it to work review, where a person decides on a refund, or keep the request.') : t('Approve to cancel the request and release the visit and any PayPal hold, or refer it to work review if work has started.')}/>
          {@render decision(item, item.paid ? [{ label: t('Refer to work review'), decision: 'review-work', primary: true }, { label: t('Keep the request'), decision: 'keep-request' }] : [{ label: t('Approve withdrawal'), decision: 'approve-withdrawal', primary: true }, { label: t('Refer to work review'), decision: 'review-work' }, { label: t('Keep the request'), decision: 'keep-request' }])}
        {:else if item.kind === 'job-issue'}
          {#if item.status === 'partial_refund_review' || item.status === 'payment_reversal_review'}
            <Banner kind="warn" text={t(item.status === 'partial_refund_review' ? 'PayPal reported a partial refund. Check the payment in PayPal before closing. SamenKlaar won’t refund again from this case.' : 'PayPal reported a payment reversal. Check the payment in PayPal before closing. SamenKlaar won’t refund again from this case.')}/>
            {@render decision(item, [{ label: t('Reconcile and close'), decision: 'close-review', primary: true }])}
          {:else if item.paid}
            <Banner kind="info" text={item.status === 'refund_pending' ? t('The refund isn’t confirmed yet. Retrying is safe; it never refunds twice.') : t('This payment was captured. Refund in full, or close without a refund.')}/>
            {@render decision(item, item.status === 'refund_pending' ? [{ label: t('Retry full refund'), decision: 'refund', primary: true }] : [{ label: t('Refund full payment'), decision: 'refund', danger: true }, { label: t('Close without refund'), decision: 'close-review' }])}
          {:else}
            <Banner kind="info" text={t('Payment isn’t captured. Ask for rework, or close the review.')}/>
            {@render decision(item, [{ label: t('Request rework'), decision: 'request-rework', primary: true }, { label: t('Close review'), decision: 'close-review' }])}
          {/if}
        {:else if item.kind === 'stalled-job'}
          <Banner kind="info" text={t('Escalated automatically after 24 hours without a reply. Nothing to decide here; the system never approves work or moves money on its own.')}/>
        {:else if item.kind === 'area-waitlist'}
          <Banner kind="info" text={t('No reviewed professional covers this area. Follow up through support; don’t create a match by hand.')}/>
        {:else if item.kind === 'scope-change'}
          <Banner kind="info" text={t('Waiting on the customer to answer the extra work. Information only.')}/>
        {:else}
          <Banner kind="info" text={t('The professional hasn’t sent everything needed. Information only.')}/>
        {/if}
      {/if}
    </section>
  </div>
</main>

<style>
  .updated { font-size: 13px; color: var(--sk-on-brand-2); white-space: nowrap; }
  .operator { min-height: calc(100svh - var(--sk-topbar-h)); }
  .delivery { padding: 16px 24px 0; }
  .layout { display: grid; grid-template-columns: 340px minmax(0, 1fr); min-height: calc(100svh - var(--sk-topbar-h)); }
  .list { display: grid; align-content: start; gap: 16px; padding: 24px 20px; background: var(--sk-surface); border-right: 0.5px solid var(--sk-line); }
  .list-title { font: 800 26px/1.1 var(--sk-fd); font-stretch: 90%; }
  .tabs { width: 100%; }
  .tabs > button { flex: 1; }
  .queue { display: grid; gap: 8px; }
  .queue-row { display: grid; gap: 6px; padding: 14px; border: 1px solid var(--sk-line); border-radius: var(--sk-r-card); background: var(--sk-surface); color: var(--sk-ink); text-align: left; }
  .queue-row[aria-current="true"] { border: 2px solid var(--sk-ink); padding: 13px; }
  .queue-top { display: flex; justify-content: space-between; align-items: flex-start; gap: 8px; }
  .queue-top b { font-weight: 600; overflow-wrap: anywhere; }
  .queue-row small { font-size: 12px; color: var(--sk-muted); }
  .status { display: flex; align-items: center; gap: 10px; }
  .detail { display: grid; align-content: start; gap: 18px; max-width: 1020px; padding: 28px 40px 56px; }
  .detail-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
  .detail-title { font: 800 26px/1.1 var(--sk-fd); }
  .card { display: grid; gap: 10px; padding: 18px 22px; }
  .eyebrow.small { font-size: 12px; }
  .checks li { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 12px; padding: 12px 0; }
  .checks li.why { display: block; }
  .row { display: flex; flex-wrap: wrap; gap: 12px; align-items: flex-start; }
  .stack { display: grid; gap: 2px; }
  .strip { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 0; padding: 12px 0; border-radius: var(--sk-r-card); background: var(--sk-surface); border: 1px solid var(--sk-line); }
  .strip > div { padding: 0 16px; }
  .strip > div + div { border-left: 1px solid var(--sk-line); }
  .strip dt { font-size: 12px; color: var(--sk-muted); }
  .strip dd { font-weight: 600; }
  @media (max-width: 1023px) {
    .layout { grid-template-columns: 1fr; }
    .list { border-right: 0; border-bottom: 0.5px solid var(--sk-line); }
    .detail { padding: 24px 20px 48px; }
  }
  @media (max-width: 639px) { .updated { display: none; } .detail-title, .list-title { font-size: 20px; } .delivery { padding: 12px 16px 0; } }
</style>
