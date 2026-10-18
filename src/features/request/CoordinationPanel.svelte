<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import { api } from '../../lib/api';
  import type { AppState, CoordinationTask, WorkspaceState } from '../../lib/types';
  import { firstName } from '../../lib/status-ui';
  import { t } from '../../lib/i18n.svelte';

  // G01: the assistant's follow-up questions after booking. The assigned professional answers; both
  // sides see the summary for a person to review. Nothing here approves work, a price or a payment.
  let { app, workspace }: { app: AppState; workspace: WorkspaceState } = $props();
  let tasks = $state<CoordinationTask[]>([]);
  let replies = $state<Record<string, string>>({});
  let busy = $state('');
  let error = $state('');
  const pro = $derived(app.role === 'professional');
  const customer = $derived(firstName(workspace.job.customer) || t('the customer'));
  const professional = $derived(firstName(workspace.job.professional) || t('the professional'));
  const shown = $derived(tasks.filter((task) => !['completed', 'obsolete', 'queued'].includes(task.status)));
  const jobKey = $derived(`${workspace.workspaceId}:${workspace.job.id}:${workspace.job.scopeVersion}:${workspace.job.evidence.length}:${workspace.job.detailAdded}`);

  $effect(() => {
    void jobKey;
    if (!workspace.job.id) { tasks = []; return; }
    let live = true;
    api.coordination(workspace.workspaceId).then((result) => { if (live) tasks = result.tasks; }).catch(() => { if (live) tasks = []; });
    return () => { live = false; };
  });

  async function send(task: CoordinationTask) {
    const reply = (replies[task.id] ?? '').trim();
    if (reply.length < 3 || busy) return;
    busy = task.id;
    error = '';
    try {
      tasks = (await api.replyCoordination(workspace.workspaceId, task.id, reply)).tasks;
      replies[task.id] = '';
    } catch (e) { error = e instanceof Error ? e.message.trim() : t('That reply could not be sent.'); }
    finally { busy = ''; }
  }
  const actionLabel = (action: string) => t(action === 'request-work-note' ? 'Next: add the work note' : action === 'review-scope' ? 'Next: review the extra work with {name}' : action === 'review-completion' ? 'Next: review the completed work' : 'Next: our team reviews this', { name: pro ? customer : professional });
</script>

{#each shown as task (task.id)}
  <section class="box task" class:turn={pro && task.status === 'awaiting_reply'} aria-label={t('Follow-up question')}>
    <div class="head">
      <span class="assistant" aria-hidden="true"><Icon name="message" size={16}/></span>
      <b>{t('{brand} assistant', { brand: app.brandName })}</b>
      {#if task.status === 'operator_review'}<Pill variant="pending" label={t('Our team is reviewing')} small/>
      {:else if task.status === 'awaiting_reply'}<Pill variant={pro ? 'your-turn' : 'waiting'} label={pro ? t('Your turn') : t('Waiting on {name}', { name: professional })} small/>{/if}
    </div>
    {#if task.question && task.status !== 'summary_ready'}<p class="question">{task.question}</p>{/if}
    {#if task.status === 'awaiting_reply' && pro}
      <form onsubmit={(event) => { event.preventDefault(); void send(task); }}>
        <div class="field">
          <label for="reply-{task.id}" class="visually-hidden">{t('Your reply')}</label>
          <textarea id="reply-{task.id}" class="textarea" rows="2" maxlength="1500" minlength="3" bind:value={replies[task.id]} placeholder={t('Describe what happened, in your own words.')} disabled={busy === task.id}></textarea>
        </div>
        <button class="btn btn-primary btn-sm" disabled={busy === task.id || (replies[task.id] ?? '').trim().length < 3}>{t('Send reply')}</button>
      </form>
    {:else if task.status === 'awaiting_reply'}
      <p class="secondary-text">{t('{name} has been asked. You’ll see a summary here.', { name: professional })}</p>
    {/if}
    {#if task.summary}
      <div class="summary"><p class="eyebrow small">{t('Summary for review')}</p><p>{task.summary}</p><p class="helper">{t('A summary isn’t approval. Prices, visits and payments only change when you both agree.')}</p></div>
    {/if}
    {#if task.proposedAction && ['summary_ready', 'operator_review'].includes(task.status)}<p class="next">{actionLabel(task.proposedAction)}</p>{/if}
  </section>
{/each}
{#if error}<p class="field-error" role="alert">{error}</p>{/if}

<style>
  .task { display: grid; gap: 10px; max-width: 560px; padding: 16px; }
  .task.turn { background: var(--sk-turn-soft); border: 0; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .head b { margin-right: auto; font-size: 13px; }
  .assistant { display: grid; place-items: center; width: 28px; height: 28px; border-radius: var(--sk-r-tile); background: var(--sk-brand); color: var(--sk-on-brand); }
  .question { font-size: 14px; line-height: 1.5; }
  form { display: grid; gap: 8px; justify-items: start; }
  form .field { width: 100%; }
  .summary { display: grid; gap: 4px; padding: 10px 12px; border-radius: var(--sk-r-card); background: var(--sk-ground); }
  .task.turn .summary { background: var(--sk-surface); }
  .eyebrow.small { font-size: 11px; }
  .next { font-size: 12px; color: var(--sk-muted); }
</style>
