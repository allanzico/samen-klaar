<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import { api } from '../../lib/api';
  import type { CaseNote, WorkspaceState } from '../../lib/types';
  import { stamp } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';

  // G05: messages our team sent about this request. Participants only see notes addressed to them.
  let { workspace }: { workspace: WorkspaceState } = $props();
  let notes = $state<CaseNote[]>([]);
  const key = $derived(`${workspace.workspaceId}:${workspace.job.timeline.length}`);

  $effect(() => {
    void key;
    if (!workspace.job.id) { notes = []; return; }
    let live = true;
    api.supportNotes(workspace.workspaceId).then((r) => { if (live) notes = r.notes; }).catch(() => { if (live) notes = []; });
    return () => { live = false; };
  });
</script>

{#if notes.length}
  <section class="box support" aria-labelledby="support-title">
    <h2 id="support-title" class="small-title"><Icon name="users" size={16}/>{t('Messages from our team')}</h2>
    <ul class="rows">
      {#each notes as note (note.id)}<li><p>{note.note}</p><time datetime={note.createdAt}>{stamp(note.createdAt)}</time></li>{/each}
    </ul>
  </section>
{/if}

<style>
  .support { display: grid; gap: 6px; padding: 16px 18px; }
  h2 { display: flex; align-items: center; gap: 8px; font-size: 14px; }
  li { display: grid; gap: 2px; padding: 10px 0; font-size: 14px; }
  time { font-size: 12px; color: var(--sk-muted); }
</style>
