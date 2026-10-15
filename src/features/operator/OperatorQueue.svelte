<script lang="ts">
  import { onMount } from 'svelte';
  import type { AppActions, NotificationDeliveryHealth, OperatorCase, ProfessionalApplication, ProfessionalReviewChecks } from '../../lib/types';
  import { api } from '../../lib/api';
  import { currentLocale, t } from '../../lib/i18n.svelte';

  let { actions }: { actions: AppActions } = $props();
  let cases = $state<OperatorCase[]>([]);
  let applications = $state<ProfessionalApplication[]>([]);
  let notificationDelivery = $state<NotificationDeliveryHealth>({ failed: 0, unconfigured: 0, stalled: 0 });
  let loading = $state(true);
  let pending = $state('');
  let error = $state('');
  let notice = $state('');
  let caseNotes = $state<Record<string, string>>({});
  let tab = $state<'applications' | 'exceptions'>('applications');
  let selectedId = $state('');
  let updatedAt = $state<Date | null>(null);

  onMount(() => { void load(); });

  async function load() {
    loading = true;
    error = '';
    try {
      const result = await api.operatorQueue();
      cases = result.cases;
      applications = result.professionalApplications;
      notificationDelivery = result.notificationDelivery;
      updatedAt = new Date();
    } catch (e) {
      error = e instanceof Error ? e.message : t('The operator queue could not be loaded.');
    } finally {
      loading = false;
    }
  }

  function readyToApprove(checks: ProfessionalReviewChecks) {
    return checks.identityBusiness === 'verified' && checks.paypalOnboarding === 'verified' &&
      (checks.qualification === 'verified' || checks.qualification === 'not_required' && checks.qualificationRationale.trim().length > 0) &&
      (checks.insurance === 'verified' || checks.insurance === 'not_required' && checks.insuranceRationale.trim().length > 0);
  }

  async function review(application: ProfessionalApplication, decision: 'save' | 'approve' | 'reject') {
    pending = application.profile.id;
    error = '';
    notice = '';
    try {
      await api.reviewProfessional(application.profile.id, decision, application.checks);
      notice = decision === 'save' ? t('Professional checks saved.') : decision === 'approve' ? t('Professional approved.') : t('Application declined.');
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : t('The application could not be reviewed.');
    } finally {
      pending = '';
    }
  }

  async function resolveCase(item: OperatorCase, decision: string) {
    pending = item.id;
    error = '';
    notice = '';
    try {
      await api.resolveWorkCase(item.workspaceId, item.kind, decision, caseNotes[item.id] ?? '');
      notice = t('Case decision recorded.');
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : t('The case could not be resolved.');
    } finally {
      pending = '';
    }
  }

  type Pill = { variant: 'error' | 'review' | 'neutral'; label: string };
  function kindPill(kind: string): Pill {
    if (kind === 'job-issue') return { variant: 'error', label: t('Work issue') };
    if (kind === 'late-cancellation') return { variant: 'review', label: t('Late cancellation') };
    if (kind === 'stalled-job') return { variant: 'review', label: t('No reply 24 h') };
    if (kind === 'scope-change') return { variant: 'review', label: t('Scope change') };
    if (kind === 'area-waitlist') return { variant: 'neutral', label: t('No coverage') };
    if (kind === 'intake-review') return { variant: 'neutral', label: t('Intake review') };
    return { variant: 'review', label: t('Missing evidence') };
  }
  function caseTitle(item: OperatorCase) {
    return item.jobTitle || (item.kind === 'intake-review' ? t('Find a safe next step') : item.kind === 'area-waitlist' ? t('Coverage request · {area}', { area: item.location?.label || t('area not set') }) : t('Review this job'));
  }
  function when(value?: string) {
    const date = value ? new Date(value) : null;
    return date && Number.isFinite(date.getTime()) ? new Intl.DateTimeFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }).format(date) : '';
  }
  const money = (minor: number, currency: string) => new Intl.NumberFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', { style: 'currency', currency: /^[A-Za-z]{3}$/.test(currency) ? currency : 'EUR' }).format(minor / 100);
  const checkOptions = {
    identityBusiness: [['pending', 'Not yet'], ['verified', 'Verified']],
    qualification: [['verified', 'Verified'], ['pending', 'Not yet'], ['not_required', 'Not required']],
    insurance: [['verified', 'Verified'], ['pending', 'Not yet'], ['not_required', 'Not required']]
  } as const;

  const selectedApplication = $derived(tab === 'applications' ? applications.find((item) => item.profile.id === selectedId) : undefined);
  const selectedCase = $derived(tab === 'exceptions' ? cases.find((item) => item.id === selectedId) : undefined);
  // Open the first tab that has work and keep a valid selection after every reload.
  $effect(() => {
    if (tab === 'applications' && !applications.length && cases.length) tab = 'exceptions';
    else if (tab === 'exceptions' && !cases.length && applications.length) tab = 'applications';
    const ids = tab === 'applications' ? applications.map((item) => item.profile.id) : cases.map((item) => item.id);
    if (!ids.includes(selectedId)) selectedId = ids[0] ?? '';
  });
</script>

{#snippet decision(item: OperatorCase, actionsList: { label: string; decision: string; primary?: boolean }[])}
  <label>{t('Your decision')}<textarea rows="3" maxlength="1000" bind:value={caseNotes[item.id]} disabled={pending === item.id} placeholder={t('Record the reason for this decision.')}></textarea></label>
  <div class="actions">
    {#each actionsList as action}<button class:primary={action.primary} disabled={pending === item.id || (caseNotes[item.id] ?? '').trim().length < 10} onclick={() => resolveCase(item, action.decision)}>{action.label}</button>{/each}
  </div>
  {#if (caseNotes[item.id] ?? '').trim().length < 10}<p class="helper">{t('Write at least 10 characters to continue')}</p>{/if}
{/snippet}

<section class="operator-view">
  <div class="operator-heading">
    <div><h1>{t('What needs a person')}</h1><p class="lead">{t('Applications and customer requests that need a person’s review.')}</p></div>
    <div class="operator-refresh">{#if updatedAt}<span class="helper">{t('Updated {time}', { time: new Intl.DateTimeFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', { hour: '2-digit', minute: '2-digit' }).format(updatedAt) })}</span>{/if}<button class="secondary" disabled={loading} onclick={load}>{t('Refresh queue')}</button></div>
  </div>
  {#if error}<div class="error" role="alert">{error}</div>{/if}
  {#if notice}<div class="operator-notice" role="status">{notice}</div>{/if}
  {#if notificationDelivery.failed || notificationDelivery.unconfigured || notificationDelivery.stalled}
    <div class="banner info" role="status">
      <b>{t('Email updates need attention')}</b>
      {#if notificationDelivery.unconfigured}<span>{t('{count} updates are waiting because SMTP is not configured. In-app notifications remain available.', { count: notificationDelivery.unconfigured })}</span>{/if}
      {#if notificationDelivery.failed}<span>{t('{count} emails failed. The worker retries automatically; check the SMTP configuration and delivery logs.', { count: notificationDelivery.failed })}</span>{/if}
      {#if notificationDelivery.stalled}<span>{t('{count} delivery attempts have been sending for over 15 minutes and are awaiting recovery.', { count: notificationDelivery.stalled })}</span>{/if}
    </div>
  {/if}
  {#if loading && !updatedAt}<div class="card loading" role="status">{t('Loading the review queue…')}</div>
  {:else if !applications.length && !cases.length}
    <div class="card empty-state"><span><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10"/></svg></span><h2>{t('Nothing needs review.')}</h2><p>{t('The queue is clear.')}</p></div>
  {:else}
    <div class="operator-layout">
      <div class="operator-list">
        <div class="segmented operator-tabs" role="tablist" aria-label={t('Review queue')}>
          <button role="tab" id="tab-applications" aria-selected={tab === 'applications'} class:current={tab === 'applications'} onclick={() => { tab = 'applications'; }}>{t('Applications · {count}', { count: applications.length })}</button>
          <button role="tab" id="tab-exceptions" aria-selected={tab === 'exceptions'} class:current={tab === 'exceptions'} onclick={() => { tab = 'exceptions'; }}>{t('Exceptions · {count}', { count: cases.length })}</button>
        </div>
        <div role="tabpanel" aria-labelledby={tab === 'applications' ? 'tab-applications' : 'tab-exceptions'} class="queue-items">
          {#if tab === 'applications'}
            {#each applications as application (application.profile.id)}
              <button class="queue-item" class:current={application.profile.id === selectedId} aria-current={application.profile.id === selectedId ? 'true' : undefined} onclick={() => { selectedId = application.profile.id; }}>
                <span class="queue-item-top"><b>{application.profile.displayName}</b><span class="pill pill--review">{t('Pending review')}</span></span>
                <small>{t(application.profile.category)} · {application.profile.serviceArea}</small>
              </button>
            {/each}
          {:else}
            {#each cases as item (item.id)}
              <button class="queue-item" class:current={item.id === selectedId} aria-current={item.id === selectedId ? 'true' : undefined} onclick={() => { selectedId = item.id; }}>
                <span class="queue-item-top"><b>{caseTitle(item)}</b><span class="pill pill--{kindPill(item.kind).variant}">{kindPill(item.kind).label}</span></span>
                <small>{item.category ? t(item.category) : t('Needs classification')}{when(item.reportedAt) ? ` · ${when(item.reportedAt)}` : ''}</small>
              </button>
            {/each}
          {/if}
        </div>
      </div>

      <div class="operator-detail">
        {#if selectedApplication}
          {@const application = selectedApplication}
          <article>
            <div class="detail-head"><div><h2>{application.profile.displayName}</h2>{#if application.email}<small>{application.email}</small>{/if}</div><span class="pill pill--review">{t('Pending review')}</span></div>
            <section class="card detail-card"><p class="eyebrow">{t('Service')}</p>
              <h3>{application.profile.serviceName}</h3>
              <p class="quiet-note">{t(application.profile.category)} · {money(application.profile.priceMinor + application.profile.calloutMinor, application.profile.currency)} {t('including call-out')} · {application.profile.durationMinutes} {t('minutes')}</p>
              {#if application.profile.serviceDescription}<p>{application.profile.serviceDescription}</p>{/if}
            </section>
            <section class="card detail-card"><p class="eyebrow">{t('Coverage & terms')}</p>
              <p>{t('Service areas:')} {application.profile.serviceArea}</p>
              <p class="quiet-note">{t('Materials:')} {application.profile.materialsPolicy}</p>
              {#if application.profile.exclusions?.length}<p class="quiet-note">{t('Excluded:')} {application.profile.exclusions.join(' · ')}</p>{/if}
            </section>
            <section class="card detail-card"><p class="eyebrow">{t('Checks')}</p>
              <p class="quiet-note">{t('Confirm each item using the team’s review policy. Record only the result and rationale here; do not enter document numbers or upload identity or qualification files.')}</p>
              <div class="check-list">
                <div class="check-line"><span id="chk-identity">{t('Identity and business')}</span>
                  <div class="segmented" role="group" aria-labelledby="chk-identity">{#each checkOptions.identityBusiness as [value, label]}<button type="button" class:current={application.checks.identityBusiness === value} aria-pressed={application.checks.identityBusiness === value} onclick={() => { application.checks.identityBusiness = value; }}>{t(label)}</button>{/each}</div>
                </div>
                <div class="check-line"><span>{t('PayPal seller onboarding')}</span><span class="pill pill--{application.checks.paypalOnboarding === 'verified' ? 'done' : 'neutral'}">{application.checks.paypalOnboarding === 'verified' ? t('Connected and eligible') : t('Not connected')}</span></div>
                <div class="check-line"><span id="chk-qualification">{t('Relevant qualification')}</span>
                  <div class="segmented" role="group" aria-labelledby="chk-qualification">{#each checkOptions.qualification as [value, label]}<button type="button" class:current={application.checks.qualification === value} aria-pressed={application.checks.qualification === value} onclick={() => { application.checks.qualification = value; }}>{t(label)}</button>{/each}</div>
                </div>
                {#if application.checks.qualification === 'not_required'}<label>{t('Why is qualification not required for this service?')}<textarea rows="2" maxlength="500" bind:value={application.checks.qualificationRationale}></textarea></label>{/if}
                <div class="check-line"><span id="chk-insurance">{t('Relevant insurance')}</span>
                  <div class="segmented" role="group" aria-labelledby="chk-insurance">{#each checkOptions.insurance as [value, label]}<button type="button" class:current={application.checks.insurance === value} aria-pressed={application.checks.insurance === value} onclick={() => { application.checks.insurance = value; }}>{t(label)}</button>{/each}</div>
                </div>
                {#if application.checks.insurance === 'not_required'}<label>{t('Why is insurance not required for this service?')}<textarea rows="2" maxlength="500" bind:value={application.checks.insuranceRationale}></textarea></label>{/if}
              </div>
              <p class="review-readiness" role="status">{readyToApprove(application.checks) ? t('Required checks are complete.') : t('Complete all checks before approval.')}</p>
            </section>
            <div class="detail-actions">
              <button class="primary" disabled={pending === application.profile.id || !readyToApprove(application.checks)} onclick={() => review(application, 'approve')}>{t('Approve professional')}</button>
              <button class="secondary" disabled={pending === application.profile.id} onclick={() => review(application, 'save')}>{t('Save checks')}</button>
              <button class="danger" disabled={pending === application.profile.id} onclick={() => review(application, 'reject')}>{t('Decline')}</button>
            </div>
            {#if !readyToApprove(application.checks)}<p class="helper">{t('Approval is available once every check is answered.')}</p>{/if}
          </article>
        {:else if selectedCase}
          {@const item = selectedCase}
          <article>
            <div class="detail-head"><div><h2>{caseTitle(item)}</h2></div><span class="pill pill--{kindPill(item.kind).variant}">{kindPill(item.kind).label}</span></div>
            <dl class="summary-strip">
              <div><dt>{t('Category')}</dt><dd>{item.category ? t(item.category) : t('Needs classification')}</dd></div>
              {#if item.kind === 'job-issue'}<div><dt>{t('Payment')}</dt><dd>{item.paid ? t('Captured') : t('Not captured')}</dd></div>{/if}
              {#if item.urgency}<div><dt>{t('Timing')}</dt><dd>{item.urgency}</dd></div>{/if}
              {#if when(item.reportedAt)}<div><dt>{t('Reported')}</dt><dd>{when(item.reportedAt)}</dd></div>{/if}
            </dl>
            <section class="card detail-card"><p class="eyebrow">{t('Customer’s report')}</p>
              <blockquote class="offer-note">{item.description}</blockquote>
              {#if item.detail}<p>{item.detail}</p>{/if}
            </section>
            <section class="card detail-card">
              {#if item.kind === 'area-waitlist'}<p class="quiet-note">{t('No professional with a published service area matched. Contact or resolve this request using the team’s support process; do not create an unapproved match.')}</p>{/if}
              {#if item.kind === 'stalled-job'}<p class="quiet-note">{t('Automatic escalation after 24 hours without a response. Review the request timeline and contact the responsible person; the system will not approve work or move money automatically.')}</p>{/if}
              {#if item.kind === 'late-cancellation'}
                <p class="quiet-note">{t('The customer requested cancellation within 24 hours of the visit. Approving releases the reservation with no automatic cancellation fee; keeping it leaves the visit booked.')}</p>
                {@render decision(item, [{ label: t('Approve cancellation'), decision: 'approve-cancellation' }, { label: t('Keep visit booked'), decision: 'keep-visit' }])}
              {/if}
              {#if item.kind === 'job-issue'}
                <p class="quiet-note">{t('Customer-reported work issue. Review the request and record a human decision.')}</p>
                {#if item.status === 'partial_refund_review' || item.status === 'payment_reversal_review'}
                  <p class="quiet-note">{t(item.status === 'partial_refund_review' ? 'PayPal reported a partial refund. Reconcile the provider transaction and any related case before closing this review. The app will not issue another refund from this case.' : 'PayPal reported a payment reversal. Reconcile the provider transaction and any related case before closing this review. The app will not issue another refund from this case.')}</p>
                  {@render decision(item, [{ label: t('Close after reconciliation'), decision: 'close-review' }])}
                {:else if item.paid}
                  <p class="quiet-note">{t('This payment was captured. Choose a full PayPal refund or close the review without refund. The refund action is safe to retry if PayPal’s response is delayed.')}</p>
                  {@render decision(item, item.status === 'refund_pending' ? [{ label: t('Retry full refund'), decision: 'refund', primary: true }] : [{ label: t('Refund full payment'), decision: 'refund', primary: true }, { label: t('Close without refund'), decision: 'close-review' }])}
                {:else}
                  {@render decision(item, [{ label: t('Request rework'), decision: 'request-rework' }, { label: t('Close review'), decision: 'close-review' }])}
                {/if}
              {/if}
              <p class="quiet-note">{t('This exception is visible to authorized operators. Keep the customer’s agreed scope and payment approvals intact while reviewing it.')}</p>
            </section>
          </article>
        {/if}
      </div>
    </div>
  {/if}
</section>
