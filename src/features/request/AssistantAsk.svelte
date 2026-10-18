<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import { noteText } from '../../lib/agent-copy';
  import { t } from '../../lib/i18n.svelte';

  // The assistant's open question, first on the page: the person answers here, with a button or in
  // the composer below, instead of hunting for it in the conversation under the plan.
  let { app, actions, workspace }: { app: AppState; actions: AppActions; workspace: WorkspaceState } = $props();
  const role = $derived(app.role);
  const dialog = $derived(role === 'professional' ? workspace.proDialog : role === 'customer' ? workspace.agentDialog : undefined);
  const question = $derived([...(workspace.intake?.messages ?? [])].reverse().find((m) => m.role === 'agent' && m.audience === role));
  const choices = $derived(dialog?.choices ?? []);
</script>

{#if dialog?.awaiting && question}
  <section class="ask" aria-label={question.agent || t('Your assistant')} aria-live="polite">
    <div class="head">
      <span class="avatar" aria-hidden="true"><Icon name="bot" size={18}/></span>
      <b>{question.agent || t('Your assistant')}</b>
      <Pill variant="your-turn" label={t('Needs you')} small/>
    </div>
    <p class="text">{noteText(question)}</p>
    {#if choices.length}
      <div class="choices" role="group" aria-label={t('Quick replies')}>
        {#each choices as choice (choice.id + choice.label)}
          <button type="button" class="chip" disabled={app.pending} onclick={() => actions.act('agent-reply', choice.label, undefined, undefined, { choiceId: choice.id })}>{choice.label}</button>
        {/each}
      </div>
    {/if}
    <p class="helper">{t('Or type your answer in the box below.')}</p>
  </section>
{/if}

<style>
  .ask { display: grid; gap: 12px; max-width: 820px; margin: 0 auto; padding: 16px 18px; border-radius: var(--sk-r-box); background: var(--sk-turn-soft); }
  .head { display: flex; align-items: center; gap: 8px; }
  .head b { margin-right: auto; font-size: 14px; }
  .avatar { display: grid; place-items: center; width: 32px; height: 32px; border-radius: var(--sk-r-tile); background: var(--sk-brand); color: var(--sk-on-brand); }
  .text { font-size: 15px; line-height: 1.55; white-space: pre-wrap; overflow-wrap: anywhere; }
  .choices { display: flex; flex-wrap: wrap; gap: 8px; }
  .helper { margin: 0; }
</style>
