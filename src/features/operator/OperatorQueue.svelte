<script lang="ts">
  import { onMount } from 'svelte';
  import type { AppActions, NotificationDeliveryHealth, OperatorCase, ProfessionalApplication, ProfessionalReviewChecks } from '../../lib/types';
  import { api } from '../../lib/api';

  let { actions }: { actions: AppActions } = $props();
  let cases = $state<OperatorCase[]>([]);
  let applications = $state<ProfessionalApplication[]>([]);
  let notificationDelivery = $state<NotificationDeliveryHealth>({ failed: 0, unconfigured: 0, stalled: 0 });
  let loading = $state(true);
  let pending = $state('');
  let error = $state('');
  let notice = $state('');
  let caseNotes = $state<Record<string, string>>({});

  onMount(() => { void load(); });

  async function load() {
    loading = true;
    error = '';
    try {
      const result = await api.operatorQueue();
      cases = result.cases;
      applications = result.professionalApplications;
      notificationDelivery = result.notificationDelivery;
    } catch (e) {
      error = e instanceof Error ? e.message : 'The operator queue could not be loaded.';
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
      notice = decision === 'save' ? 'Professional checks saved.' : decision === 'approve' ? 'Professional approved.' : 'Application declined.';
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'The application could not be reviewed.';
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
      notice = 'Case decision recorded.';
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'The case could not be resolved.';
    } finally {
      pending = '';
    }
  }

  function kindLabel(kind: string) {
    if (kind === 'intake-review') return 'INTAKE REVIEW';
    if (kind === 'area-waitlist') return 'AREA WAITLIST';
    if (kind === 'late-cancellation') return 'LATE CANCELLATION';
    if (kind === 'job-issue') return 'JOB ISSUE';
    if (kind === 'scope-change') return 'SCOPE CHANGE';
    if (kind === 'stalled-job') return 'STALLED JOB';
    return 'MISSING EVIDENCE';
  }
</script>

<section class="operator-view">
  <div class="operator-heading"><div><span class="eyebrow">OPERATOR WORKSPACE</span><h1>What needs<br/>a decision?</h1><p class="lead">Applications and customer requests that need a person’s review.</p></div><button class="secondary" onclick={actions.logout}>Sign out</button></div>
  {#if error}<div class="error" role="alert">{error}</div>{/if}
  {#if notice}<div class="operator-notice" role="status">{notice}</div>{/if}
  {#if notificationDelivery.failed || notificationDelivery.unconfigured || notificationDelivery.stalled}
    <div class="card notification-delivery-card" role="status">
      <span class="eyebrow">EMAIL DELIVERY · NEEDS ATTENTION</span>
      <h2>Email updates need attention</h2>
      {#if notificationDelivery.unconfigured}<p>{notificationDelivery.unconfigured} {notificationDelivery.unconfigured === 1 ? 'update is' : 'updates are'} waiting because SMTP is not configured. In-app notifications remain available.</p>{/if}
      {#if notificationDelivery.failed}<p>{notificationDelivery.failed} {notificationDelivery.failed === 1 ? 'email failed' : 'emails failed'}. The worker retries automatically; check the SMTP configuration and delivery logs.</p>{/if}
      {#if notificationDelivery.stalled}<p>{notificationDelivery.stalled} delivery {notificationDelivery.stalled === 1 ? 'attempt has' : 'attempts have'} been sending for over 15 minutes and are awaiting recovery.</p>{/if}
    </div>
  {/if}
  {#if loading}<div class="card loading" role="status">Loading the review queue…</div>
  {:else if !applications.length && !cases.length && !notificationDelivery.failed && !notificationDelivery.unconfigured && !notificationDelivery.stalled}
    <div class="card empty-state"><span>✓</span><h2>Nothing needs review.</h2><p>The queue is clear.</p></div>
  {:else}
    {#if applications.length}
      <div class="queue-section"><span class="eyebrow">PROFESSIONAL APPLICATIONS · {applications.length}</span>
        {#each applications as application (application.profile.id)}
          <article class="card operator-card">
            <span class="eyebrow">NEW PROFESSIONAL</span>
            <h2>{application.profile.displayName}</h2>
            <p>{application.profile.serviceName} · {application.profile.category}</p>
            <div class="facts"><span>{application.profile.currency} {(application.profile.priceMinor + application.profile.calloutMinor) / 100}</span><span>{application.profile.durationMinutes} min</span></div>
            {#if application.profile.serviceDescription}<p>{application.profile.serviceDescription}</p>{/if}
            <p>Service areas: {application.profile.serviceArea}</p>
            <p class="quiet-note">Materials: {application.profile.materialsPolicy}</p>
            {#if application.email}<small>{application.email}</small>{/if}
            <div class="review-checklist">
              <h3>Professional checks</h3>
              <p class="quiet-note">Confirm each item using the team’s review policy. Record only the result and rationale here; do not enter document numbers or upload identity or qualification files.</p>
              <div class="review-check-grid">
                <label>Identity and business
                  <select bind:value={application.checks.identityBusiness}>
                    <option value="pending">Not verified</option><option value="verified">Verified</option>
                  </select>
                </label>
                <div class="review-check-status">
                  <span>PayPal seller onboarding</span>
                  <span class="review-check-value">{application.checks.paypalOnboarding === 'verified' ? 'Connected and eligible' : 'Not connected'}</span>
                </div>
                <label>Relevant qualification
                  <select bind:value={application.checks.qualification}>
                    <option value="pending">Review needed</option><option value="verified">Verified</option><option value="not_required">Not required</option>
                  </select>
                </label>
                <label>Relevant insurance
                  <select bind:value={application.checks.insurance}>
                    <option value="pending">Review needed</option><option value="verified">Verified</option><option value="not_required">Not required</option>
                  </select>
                </label>
              </div>
              {#if application.checks.qualification === 'not_required'}
                <label>Why is qualification not required for this service?
                  <textarea rows="2" maxlength="500" bind:value={application.checks.qualificationRationale}></textarea>
                </label>
              {/if}
              {#if application.checks.insurance === 'not_required'}
                <label>Why is insurance not required for this service?
                  <textarea rows="2" maxlength="500" bind:value={application.checks.insuranceRationale}></textarea>
                </label>
              {/if}
              <p class="review-readiness" role="status">{readyToApprove(application.checks) ? 'Required checks are complete.' : 'Complete all checks before approval.'}</p>
            </div>
            <div class="actions">
              <button disabled={pending === application.profile.id} onclick={() => review(application, 'save')}>Save checks</button>
              <button class="primary" disabled={pending === application.profile.id || !readyToApprove(application.checks)} onclick={() => review(application, 'approve')}>Approve professional</button>
              <button disabled={pending === application.profile.id} onclick={() => review(application, 'reject')}>Decline</button>
            </div>
          </article>
        {/each}
      </div>
    {/if}
    {#if cases.length}
      <div class="queue-section"><span class="eyebrow">CUSTOMER EXCEPTIONS · {cases.length}</span>
        {#each cases as item (item.id)}
          <article class="card operator-card">
            <span class="eyebrow">{kindLabel(item.kind)}</span>
            <h2>{item.jobTitle || (item.kind === 'intake-review' ? 'Find a safe next step' : item.kind === 'area-waitlist' ? `Coverage request · ${item.location?.label || 'area not set'}` : 'Review this job')}</h2>
            <p>{item.description}</p>
            {#if item.detail}<p>{item.detail}</p>{/if}
            <div class="facts"><span>{item.category ?? 'Needs classification'}</span>{#if item.urgency}<span>{item.urgency}</span>{/if}</div>
            {#if item.kind === 'area-waitlist'}<p class="quiet-note">No professional with a published service area matched. Contact or resolve this request using the team’s support process; do not create an unapproved match.</p>{/if}
            {#if item.kind === 'stalled-job'}<p class="quiet-note">Automatic escalation after 24 hours without a response. Review the request timeline and contact the responsible person; the system will not approve work or move money automatically.</p>{/if}
            {#if item.kind === 'late-cancellation'}
              <p class="quiet-note">The customer requested cancellation within 24 hours of the visit. Approving releases the reservation with no automatic cancellation fee; keeping it leaves the visit booked.</p>
              <label>Decision rationale<textarea rows="2" maxlength="1000" bind:value={caseNotes[item.id]} disabled={pending === item.id}></textarea></label>
              <div class="actions"><button disabled={pending === item.id || (caseNotes[item.id] ?? '').trim().length < 10} onclick={() => resolveCase(item, 'approve-cancellation')}>Approve cancellation</button><button disabled={pending === item.id || (caseNotes[item.id] ?? '').trim().length < 10} onclick={() => resolveCase(item, 'keep-visit')}>Keep visit booked</button></div>
            {/if}
            {#if item.kind === 'job-issue'}
              <p class="quiet-note">Customer-reported work issue. Review the request and record a human decision.</p>
              {#if item.status === 'partial_refund_review' || item.status === 'payment_reversal_review'}
                <p class="quiet-note">PayPal reported a {item.status === 'partial_refund_review' ? 'partial refund' : 'payment reversal'}. Reconcile the provider transaction and any related case before closing this review. The app will not issue another refund from this case.</p>
                <label>Reconciliation notes<textarea rows="2" maxlength="1000" bind:value={caseNotes[item.id]} disabled={pending === item.id}></textarea></label>
                <div class="actions"><button disabled={pending === item.id || (caseNotes[item.id] ?? '').trim().length < 10} onclick={() => resolveCase(item, 'close-review')}>Close after reconciliation</button></div>
              {:else if item.paid}
                <p class="quiet-note">This payment was captured. Choose a full PayPal refund or close the review without refund. The refund action is safe to retry if PayPal’s response is delayed.</p>
                <label>Decision rationale<textarea rows="2" maxlength="1000" bind:value={caseNotes[item.id]} disabled={pending === item.id}></textarea></label>
                <div class="actions"><button class="primary" disabled={pending === item.id || (caseNotes[item.id] ?? '').trim().length < 10} onclick={() => resolveCase(item, 'refund')}>{item.status === 'refund_pending' ? 'Retry full refund' : 'Refund full payment'}</button>{#if item.status !== 'refund_pending'}<button disabled={pending === item.id || (caseNotes[item.id] ?? '').trim().length < 10} onclick={() => resolveCase(item, 'close-review')}>Close without refund</button>{/if}</div>
              {:else}
                <label>Decision rationale<textarea rows="2" maxlength="1000" bind:value={caseNotes[item.id]} disabled={pending === item.id}></textarea></label>
                <div class="actions"><button disabled={pending === item.id || (caseNotes[item.id] ?? '').trim().length < 10} onclick={() => resolveCase(item, 'request-rework')}>Request rework</button><button disabled={pending === item.id || (caseNotes[item.id] ?? '').trim().length < 10} onclick={() => resolveCase(item, 'close-review')}>Close review</button></div>
              {/if}
            {/if}
            <p class="quiet-note">This exception is visible to authorized operators. Keep the customer’s agreed scope and payment approvals intact while reviewing it.</p>
          </article>
        {/each}
      </div>
    {/if}
  {/if}
  <button class="text-button" onclick={load}>Refresh queue</button>
</section>
