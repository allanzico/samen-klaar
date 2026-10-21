<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import { api } from '../../lib/api';
  import { agentUi } from '../../lib/agent.svelte';
  import { activityText, questionText } from '../../lib/agent-copy';
  import type { AgentActivity, AppState, WorkspaceState } from '../../lib/types';
  import { dayTime } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';

  // What your agent did on this request, and the one thing it stopped to ask you. Everything it does
  // goes through the same rules as a button you press yourself.
  let { app, workspace }: { app: AppState; workspace: WorkspaceState } = $props();
  let items = $state<AgentActivity[]>([]);
  let showAll = $state(false);
  const question = $derived(agentUi.escalations.find((item) => item.workspaceId === workspace.workspaceId));
  // "Waiting for your answer" entries repeat the assistant's own question; only real steps belong in the log.
  const steps = $derived(items.filter((item) => !(['awaiting_reply', 'choose_professional'].includes(item.code) && !item.action)));
  const shown = $derived(showAll ? steps : steps.slice(0, 4));
  const settingsCodes = ['address_needed', 'no_match_within_limits', 'no_time_in_window', 'price_above_limit', 'visit_outside_window'];
  // The panel carries my assistant's name once it has said something.
  const myAgent = $derived([...(workspace.intake?.messages ?? [])].reverse().find((m) => m.role === 'agent' && (m.audience === app.role || m.code?.startsWith(`${app.role}:`)))?.agent || t('Your assistant'));
  const refreshKey = $derived(`${workspace.workspaceId}:${workspace.job.id}:${workspace.job.scopeVersion}:${workspace.job.appointment?.slotId ?? ''}:${workspace.job.appointmentConfirmation?.status ?? ''}:${question?.id ?? 0}`);

  $effect(() => {
    void refreshKey;
    let live = true;
    api.agentActivity(workspace.workspaceId).then((result) => { if (live) items = result; }).catch(() => { if (live) items = []; });
    return () => { live = false; };
  });
</script>

{#if steps.length || question}
  <section class="box agent" class:turn={!!question} aria-label={myAgent}>
    <div class="head">
      <span class="mark" aria-hidden="true"><Icon name="bot" size={16}/></span>
      <b>{myAgent}</b>
      {#if question}<Pill variant="your-turn" label={t('Needs you')} small/>{:else}<Pill variant="pending" label={t('Handling this')} small/>{/if}
    </div>
    {#if question && question.code !== 'awaiting_reply'}
      <p class="question">{questionText(question)}</p>
      {#if settingsCodes.includes(question.code)}<button type="button" class="btn btn-outline btn-sm" onclick={() => { agentUi.settingsOpen = true; }}>{t('Change what my agent may do')}</button>{/if}
    {/if}
    {#if steps.length}
      <ol class="log">
        {#each shown as item (item.id)}<li><span>{activityText(item)}</span><time datetime={item.createdAt}>{dayTime(item.createdAt)}</time></li>{/each}
      </ol>
      {#if steps.length > 4}<button type="button" class="btn btn-quiet btn-sm" onclick={() => { showAll = !showAll; }}>{showAll ? t('Show less') : t('Show everything it did')}</button>{/if}
    {/if}
  </section>
{/if}

<style>
  .agent { display: grid; gap: 10px; max-width: 560px; padding: 16px; justify-items: start; }
  .agent.turn { background: var(--sk-turn-soft); border: 0; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; width: 100%; }
  .head b { margin-right: auto; font-size: 13px; }
  .mark { display: grid; place-items: center; width: 28px; height: 28px; border-radius: var(--sk-r-tile); background: var(--sk-brand); color: var(--sk-on-brand); }
  .question { font-size: 14px; line-height: 1.5; font-weight: 500; }
  .log { display: grid; gap: 6px; width: 100%; font-size: 13px; }
  .log li { display: flex; justify-content: space-between; gap: 12px; }
  .log time { color: var(--sk-muted); white-space: nowrap; font-variant-numeric: tabular-nums; }
</style>
